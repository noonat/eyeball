package store

import (
	"context"
	"database/sql"
	"path/filepath"

	"github.com/cockroachdb/errors"
)

// WorkingCopy is one directory a round was captured from: a main checkout, a
// linked worktree, or a directory named by a marker file.
type WorkingCopy struct {
	// ID is the working copy's identity in the store.
	ID int64
	// ProjectID is the project the working copy belongs to.
	ProjectID int64
	// Root is the working copy's absolute path, and its identity outside the
	// store.
	Root string
	// Main records that git named this the repository's main checkout. It is
	// false until a request from it is seen.
	Main bool
}

// EnsureWorkingCopy returns the working copy at root, inserting it under
// projectID if this is the first time it has been seen. A root already known
// keeps the project it has, which may not be projectID: a directory recreated
// at a gone root is found here, and the caller decides what to do about it.
func (s *Store) EnsureWorkingCopy(ctx context.Context, projectID int64, root string) (WorkingCopy, error) {
	if !filepath.IsAbs(root) {
		return WorkingCopy{}, errors.Wrapf(ErrInvalidPath, "working copy root %q is relative", root)
	}
	root = filepath.Clean(root)
	var out WorkingCopy
	err := s.tx(ctx, func(tx *sql.Tx) error {
		insert := "INSERT INTO working_copies (project_id, root, created) VALUES (?, ?, ?) ON CONFLICT (root) DO NOTHING"
		if _, err := tx.ExecContext(ctx, insert, projectID, root, FormatTime(s.now())); err != nil {
			return errors.Wrap(err, "insert working copy")
		}
		var err error
		out, err = scanCopy(tx.QueryRowContext(ctx, copyQuery+" WHERE root = ?", root))
		return err
	})
	return out, err
}

// MarkMain records that git named a working copy its repository's main
// checkout. It only ever sets the flag, since a main checkout stays one.
func (s *Store) MarkMain(ctx context.Context, copyID int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE working_copies SET main = 1 WHERE id = ? AND main = 0", copyID)
		return errors.Wrap(err, "mark the main checkout")
	})
}

// WorkingCopyByID reads one working copy.
func (s *Store) WorkingCopyByID(ctx context.Context, copyID int64) (WorkingCopy, error) {
	out, err := scanCopy(s.db.QueryRowContext(ctx, copyQuery+" WHERE id = ?", copyID))
	if errors.Is(err, sql.ErrNoRows) {
		return WorkingCopy{}, errors.Wrapf(ErrNotFound, "working copy %d", copyID)
	}
	return out, err
}

// WorkingCopies reads a project's working copies, the main checkout first and
// the rest in the order they were first seen.
func (s *Store) WorkingCopies(ctx context.Context, projectID int64) ([]WorkingCopy, error) {
	rows, err := s.db.QueryContext(ctx, copyQuery+" WHERE project_id = ? ORDER BY main DESC, id", projectID)
	if err != nil {
		return nil, errors.Wrap(err, "read working copies")
	}
	defer func() { _ = rows.Close() }()
	var out []WorkingCopy
	for rows.Next() {
		c, err := scanCopy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, errors.Wrap(rows.Err(), "read working copies")
}

// copyQuery is the select every working copy read starts from, so scanCopy
// has one column order to follow.
const copyQuery = "SELECT id, project_id, root, main FROM working_copies"

// scanCopy reads one row of copyQuery.
func scanCopy(row interface{ Scan(...any) error }) (WorkingCopy, error) {
	var out WorkingCopy
	err := row.Scan(&out.ID, &out.ProjectID, &out.Root, &out.Main)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		err = errors.Wrap(err, "read working copy")
	}
	return out, err
}

// rootCopy finds the working copy at a project's root, which is the one a first
// round is captured from when the request names none.
func rootCopy(ctx context.Context, tx *sql.Tx, projectID int64) (int64, error) {
	var id int64
	query := `SELECT c.id FROM working_copies c JOIN projects p ON p.root = c.root WHERE p.id = ?`
	if err := tx.QueryRowContext(ctx, query, projectID).Scan(&id); err != nil {
		return 0, errors.Wrapf(err, "find the working copy at project %d's root", projectID)
	}
	return id, nil
}

// requireCopyIn refuses a working copy that belongs to another project, since a
// round captured there would read another repository's files.
func requireCopyIn(ctx context.Context, tx *sql.Tx, copyID, projectID int64) error {
	var owner int64
	if err := tx.QueryRowContext(ctx, "SELECT project_id FROM working_copies WHERE id = ?", copyID).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Wrapf(ErrNotFound, "working copy %d", copyID)
		}
		return errors.Wrap(err, "read working copy")
	}
	if owner != projectID {
		return errors.Wrapf(ErrWrongProject, "working copy %d belongs to project %d, not %d", copyID, owner, projectID)
	}
	return nil
}

// moveCopy sets the working copy a later round is captured from. Zero keeps the
// previous round's. Another working copy of the project is checked for claims
// there, as a new review would be.
func (s *Store) moveCopy(ctx context.Context, tx *sql.Tx, round *Round, want int64, paths []string) error {
	var previous, projectID int64
	query := `SELECT rd.working_copy_id, r.project_id FROM rounds rd JOIN reviews r ON r.id = rd.review_id
		WHERE rd.review_id = ? AND rd.number = ?`
	if err := tx.QueryRowContext(ctx, query, round.ReviewID, round.Number-1).Scan(&previous, &projectID); err != nil {
		return errors.Wrap(err, "read the previous round's working copy")
	}
	round.WorkingCopyID = previous
	if want == 0 || want == previous {
		return nil
	}
	if err := requireCopyIn(ctx, tx, want, projectID); err != nil {
		return err
	}
	if err := refuseClaimed(ctx, tx, want, round.ReviewID, paths); err != nil {
		return err
	}
	round.WorkingCopyID = want
	return nil
}
