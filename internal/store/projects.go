package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/cockroachdb/errors"
)

// projectQuery is the select every project read starts from. A project not yet
// asked about has no repository, which reads as empty.
const projectQuery = "SELECT id, root, name, COALESCE(repository, ''), inside_checkout FROM projects"

// ProjectByRepository reads the project whose working copies share the git
// common directory repo.
func (s *Store) ProjectByRepository(ctx context.Context, repo string) (Project, error) {
	var out Project
	row := s.db.QueryRowContext(ctx, projectQuery+" WHERE repository = ?", repo)
	if err := row.Scan(&out.ID, &out.Root, &out.Name, &out.Repository, &out.InsideCheckout); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Project{}, errors.Wrapf(ErrNotFound, "project of repository %s", repo)
		}
		return Project{}, errors.Wrap(err, "read project")
	}
	return out, nil
}

// ErrRepositoryTaken reports a repository another project already holds. Two
// working copies of one repository seen for the first time at the same moment
// each register a project, and the second to record the repository finds it
// taken.
var ErrRepositoryTaken = errors.New("another project holds the repository")

// SetRepository records the git common directory a project's working copies
// share. It is set once, when git is first asked, so a project that already has
// one keeps it. It is refused with ErrRepositoryTaken when another project
// holds the repository, checked in the same write transaction so two callers
// cannot both pass.
func (s *Store) SetRepository(ctx context.Context, projectID int64, repo string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var holder int64
		err := tx.QueryRowContext(ctx, "SELECT id FROM projects WHERE repository = ? AND id <> ?", repo, projectID).Scan(&holder)
		if err == nil {
			return errors.Wrapf(ErrRepositoryTaken, "project %d holds %s", holder, repo)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return errors.Wrap(err, "read the repository's project")
		}
		_, err = tx.ExecContext(ctx, "UPDATE projects SET repository = ? WHERE id = ? AND repository IS NULL", repo, projectID)
		return errors.Wrap(err, "record the project's repository")
	})
}

// SetProjectRoot moves a project's root to one of its working copies, which is
// what the queue and the project view show. It is moved to the main checkout
// once that has been seen.
func (s *Store) SetProjectRoot(ctx context.Context, projectID int64, root string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if err := requireRootIn(ctx, tx, projectID, root); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "UPDATE projects SET root = ? WHERE id = ?", root, projectID)
		return errors.Wrap(err, "move the project's root")
	})
}

// ProjectsWithoutRepository reads the projects git has not been asked about,
// which are the ones written before working copies existed.
func (s *Store) ProjectsWithoutRepository(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, projectQuery+" WHERE repository IS NULL ORDER BY id")
	if err != nil {
		return nil, errors.Wrap(err, "read projects")
	}
	defer func() { _ = rows.Close() }()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Root, &p.Name, &p.Repository, &p.InsideCheckout); err != nil {
			return nil, errors.Wrap(err, "scan project")
		}
		out = append(out, p)
	}
	return out, errors.Wrap(rows.Err(), "read projects")
}

// MergeProjects moves every working copy and review of from into into, and
// deletes from. Two projects found to share a repository are one project.
//
// Everything moves before the row is deleted, because deleting a project
// cascades to its reviews and their rounds, comments and decisions. If from's
// root is a main checkout, into takes it as its root.
func (s *Store) MergeProjects(ctx context.Context, from, into int64) error {
	if from == into {
		return nil
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		var mainRoot sql.NullString
		query := `SELECT c.root FROM working_copies c JOIN projects p ON p.root = c.root
			WHERE p.id = ? AND c.main = 1`
		if err := tx.QueryRowContext(ctx, query, from).Scan(&mainRoot); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.Wrap(err, "read the merged project's root")
		}
		moves := []string{
			"UPDATE working_copies SET project_id = ? WHERE project_id = ?",
			"UPDATE reviews SET project_id = ? WHERE project_id = ?",
		}
		for _, move := range moves {
			if _, err := tx.ExecContext(ctx, move, into, from); err != nil {
				return errors.Wrap(err, "move into the merged project")
			}
		}
		var left int
		count := `SELECT (SELECT count(*) FROM working_copies WHERE project_id = ?)
			+ (SELECT count(*) FROM reviews WHERE project_id = ?)`
		if err := tx.QueryRowContext(ctx, count, from, from).Scan(&left); err != nil {
			return errors.Wrap(err, "count what the merged project still holds")
		}
		if left != 0 {
			return errors.Newf("project %d still holds %d rows after the merge", from, left)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", from); err != nil {
			return errors.Wrap(err, "delete the merged project")
		}
		if mainRoot.Valid {
			if _, err := tx.ExecContext(ctx, "UPDATE projects SET root = ? WHERE id = ?", mainRoot.String, into); err != nil {
				return errors.Wrap(err, "move the project's root to its main checkout")
			}
		}
		return nil
	})
}

// requireRootIn refuses a root that is not a working copy of the project.
func requireRootIn(ctx context.Context, tx *sql.Tx, projectID int64, root string) error {
	var owner int64
	if err := tx.QueryRowContext(ctx, "SELECT project_id FROM working_copies WHERE root = ?", root).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Wrapf(ErrNotFound, "working copy at %s", root)
		}
		return errors.Wrap(err, "read working copy")
	}
	if owner != projectID {
		return errors.Wrapf(ErrWrongProject, "%s belongs to project %d, not %d", root, owner, projectID)
	}
	return nil
}

// RetireRoot moves the working copy at root, and a project rooted there, to a
// name that is not on disk, so the directory now at root can register as a
// working copy of something else. It is for a directory removed and made again
// as another repository, or as a marker directory, or as git where a marker
// directory was. The old project keeps its reviews, which read their captures,
// and the queue leaves it out once nothing of it is on disk.
//
// Merging the old project into the new one was rejected: a project git was
// never asked about cannot be told from a marker project, so a merge would file
// one repository's reviews under another's.
func (s *Store) RetireRoot(ctx context.Context, root string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, "SELECT id FROM working_copies WHERE root = ?", root).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			id = 0
		} else if err != nil {
			return errors.Wrap(err, "read the working copy to retire")
		}
		retired := fmt.Sprintf("%s (retired %d)", root, id)
		if id != 0 {
			if _, err := tx.ExecContext(ctx, "UPDATE working_copies SET root = ? WHERE id = ?", retired, id); err != nil {
				return errors.Wrap(err, "retire the working copy")
			}
		}
		_, err = tx.ExecContext(ctx, "UPDATE projects SET root = ? WHERE root = ?", retired, root)
		return errors.Wrap(err, "retire the project's root")
	})
}
