package store

import (
	"context"
	"database/sql"
	"path"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

// State is where a review is in its lifecycle. Waiting and working are not
// states: they follow from whether the latest round has a decision.
type State string

const (
	// StateOpen is a review the agent is still asking about.
	StateOpen State = "open"
	// StateApproved is a review that ended in an approval.
	StateApproved State = "approved"
	// StateAbandoned is a review the agent withdrew.
	StateAbandoned State = "abandoned"
)

// ErrInvalidPath reports a path that is not relative, cleaned and slash
// separated.
var ErrInvalidPath = errors.New("invalid path")

// ErrPathClaimed reports a path an open review in the same project already
// covers. The message names that review, because the fix is to look at it.
var ErrPathClaimed = errors.New("path already claimed")

// ErrNotFound reports a row that is not there.
var ErrNotFound = errors.New("not found")

// ErrNotOpen reports a review that has already been approved or abandoned.
var ErrNotOpen = errors.New("review is not open")

// ErrAwaitingVerdict reports a review whose latest round has no decision, so
// there is nothing for a new round to answer.
var ErrAwaitingVerdict = errors.New("review is awaiting a verdict")

// Project is one repository or directory eyeball has seen.
type Project struct {
	// ID is the project's identity in the store.
	ID int64
	// Root is the absolute path of the project's root, and its identity outside
	// the store. A project that moves is a new project.
	Root string
	// Name is what the surface displays, and repeats freely across projects.
	Name string
}

// Review is one question an agent asked about one set of paths.
type Review struct {
	// ID is the review's identity, and what an agent passes to eyeball wait.
	ID int64
	// ProjectID is the project the review belongs to.
	ProjectID int64
	// Agent is the name of the agent that opened it.
	Agent string
	// Title is what the queue shows.
	Title string
	// State is one of StateOpen, StateApproved or StateAbandoned.
	State State
	// Paths are the files and directories the review covers, normalized.
	Paths []string
	// LatestRound is the number of the newest round, which is the one a
	// reviewer is looking at and the one a capture diffs against.
	LatestRound int
}

// Round is one request for review within a review, frozen at the moment it was
// asked.
type Round struct {
	// ID is the round's identity in the store.
	ID int64
	// ReviewID is the review the round belongs to.
	ReviewID int64
	// Number counts from one, and is how a round is addressed in a URL.
	Number int
	// Note is the agent's brief, required on every request.
	Note string
	// BaseCommit is what the capture was taken against, empty outside git.
	BaseCommit string
}

// File is one path a round captured.
//
// Whether the path is inside the review's scope is not here: the store works it
// out from the review's own paths when the round is written. The match is Go
// code, and letting a caller assert the answer would let a capture disagree
// with the review it belongs to.
type File struct {
	// Path is relative to the project root, cleaned and slash separated.
	Path string
	// Digest is the sha256 of the content in the blob store, and empty where
	// the agent deleted the file.
	Digest string
	// Size is the content's length in bytes, and zero for a deleted file.
	Size int64
}

// Request is what an agent asks for review, on a first round and on every one
// after it.
type Request struct {
	// Note says what to look at, and on a later round what was done about the
	// last one.
	Note string
	// BaseCommit is the commit the capture differs from, empty outside git.
	BaseCommit string
	// Files is everything the capture holds, in or out of the review's scope.
	Files []File
}

// NewReview is a review to open, together with its first request.
type NewReview struct {
	// ProjectID is the project the review belongs to.
	ProjectID int64
	// Agent is the name of the agent opening it.
	Agent string
	// Title is what the queue will show.
	Title string
	// Paths are the files and directories the review claims.
	Paths []string
	// Request is the first round.
	Request Request
}

// EnsureProject returns the project rooted at root, inserting it if this is the
// first time it has been seen.
//
// The root is the identity, so a project that moves is a new one and
// re-registering it costs nothing. The name is read only on insert: a root that
// is already known keeps whatever it was registered with, and an empty name
// defaults to the root's last element.
func (s *Store) EnsureProject(ctx context.Context, root, name string) (Project, error) {
	if !filepath.IsAbs(root) {
		return Project{}, errors.Wrapf(ErrInvalidPath, "project root %q is relative", root)
	}
	root = filepath.Clean(root)
	if name == "" {
		name = filepath.Base(root)
	}

	var out Project
	err := s.tx(ctx, func(tx *sql.Tx) error {
		insert := "INSERT INTO projects (root, name, created) VALUES (?, ?, ?) ON CONFLICT (root) DO NOTHING"
		if _, err := tx.ExecContext(ctx, insert, root, name, FormatTime(s.now())); err != nil {
			return errors.Wrap(err, "insert project")
		}
		query := "SELECT id, root, name FROM projects WHERE root = ?"
		row := tx.QueryRowContext(ctx, query, root)
		if err := row.Scan(&out.ID, &out.Root, &out.Name); err != nil {
			return errors.Wrap(err, "read project")
		}
		return nil
	})
	return out, err
}

// OpenReview creates a review, its claimed paths, and its first round with
// everything that round captured, in one transaction.
//
// A path an open review in the same project already covers is refused, because
// the same change would otherwise appear in two places under two verdicts.
func (s *Store) OpenReview(ctx context.Context, nr NewReview) (Review, Round, error) {
	review := Review{
		ProjectID: nr.ProjectID,
		Agent:     strings.TrimSpace(nr.Agent),
		Title:     strings.TrimSpace(nr.Title),
		State:     StateOpen,
	}
	if review.Agent == "" {
		return Review{}, Round{}, errors.New("a review needs an agent")
	}
	if review.Title == "" {
		return Review{}, Round{}, errors.New("a review needs a title")
	}
	if len(nr.Paths) == 0 {
		return Review{}, Round{}, errors.New("a review needs at least one path")
	}
	paths, err := normalizePaths(nr.Paths)
	if err != nil {
		return Review{}, Round{}, err
	}
	review.Paths = paths

	round, err := s.checkRequest(nr.Request)
	if err != nil {
		return Review{}, Round{}, err
	}
	round.Number = 1
	review.LatestRound = 1

	err = s.tx(ctx, func(tx *sql.Tx) error {
		if err := refuseClaimed(ctx, tx, nr.ProjectID, paths); err != nil {
			return err
		}
		at := FormatTime(s.now())
		insert := "INSERT INTO reviews (project_id, agent, title, state, created, updated) VALUES (?, ?, ?, ?, ?, ?)"
		res, err := tx.ExecContext(ctx, insert, review.ProjectID, review.Agent, review.Title, review.State, at, at)
		if err != nil {
			return errors.Wrap(err, "insert review")
		}
		if review.ID, err = res.LastInsertId(); err != nil {
			return errors.Wrap(err, "read review id")
		}
		for _, p := range paths {
			claim := "INSERT INTO review_paths (review_id, path) VALUES (?, ?)"
			if _, err := tx.ExecContext(ctx, claim, review.ID, p); err != nil {
				return errors.Wrapf(err, "claim path %s", p)
			}
		}
		round.ReviewID = review.ID
		return s.insertRound(ctx, tx, &round, paths, nr.Request.Files)
	})
	if err != nil {
		return Review{}, Round{}, err
	}
	return review, round, nil
}

// OpenRound creates the next round of a review, with everything it captured.
//
// It is refused unless the review is open and its latest round has a decision.
// A review whose latest round is still waiting has nothing for a new round to
// answer, and an approved review has ended.
func (s *Store) OpenRound(ctx context.Context, reviewID int64, req Request) (Round, error) {
	round, err := s.checkRequest(req)
	if err != nil {
		return Round{}, err
	}
	round.ReviewID = reviewID

	err = s.tx(ctx, func(tx *sql.Tx) error {
		state, latest, decided, err := reviewProgress(ctx, tx, reviewID)
		if err != nil {
			return err
		}
		if state != StateOpen {
			return errors.Wrapf(ErrNotOpen, "review %d is %s", reviewID, state)
		}
		if !decided {
			return errors.Wrapf(ErrAwaitingVerdict, "review %d round %d has no decision", reviewID, latest)
		}
		round.Number = latest + 1
		paths, err := reviewPaths(ctx, tx, reviewID)
		if err != nil {
			return err
		}
		return s.insertRound(ctx, tx, &round, paths, req.Files)
	})
	if err != nil {
		return Round{}, err
	}
	return round, nil
}

// Abandon withdraws a review, which is the agent saying the question no longer
// needs an answer. It works whether or not the latest round has a decision, and
// is refused on a review that has already ended.
func (s *Store) Abandon(ctx context.Context, reviewID int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		state, _, _, err := reviewProgress(ctx, tx, reviewID)
		if err != nil {
			return err
		}
		if state != StateOpen {
			return errors.Wrapf(ErrNotOpen, "review %d is %s", reviewID, state)
		}
		update := "UPDATE reviews SET state = ?, updated = ? WHERE id = ?"
		if _, err := tx.ExecContext(ctx, update, StateAbandoned, FormatTime(s.now()), reviewID); err != nil {
			return errors.Wrap(err, "abandon review")
		}
		return nil
	})
}

// checkRequest validates the parts of a request that do not depend on the
// database, and returns the round it describes.
func (s *Store) checkRequest(req Request) (Round, error) {
	note := strings.TrimSpace(req.Note)
	if note == "" {
		return Round{}, errors.New("a request needs a note saying what to look at")
	}
	seen := make(map[string]struct{}, len(req.Files))
	for _, f := range req.Files {
		path, err := normalizePath(f.Path)
		if err != nil {
			return Round{}, err
		}
		if _, again := seen[path]; again {
			return Round{}, errors.Wrapf(ErrInvalidPath, "%q is captured twice in one round", path)
		}
		seen[path] = struct{}{}
		if f.Size < 0 {
			return Round{}, errors.Newf("%q cannot have a size of %d", path, f.Size)
		}
		if f.Digest == "" && f.Size != 0 {
			return Round{}, errors.Newf("%q has no content to store, so its size cannot be %d", path, f.Size)
		}
	}
	return Round{Note: note, BaseCommit: req.BaseCommit}, nil
}

// insertRound writes a round and its captured files, marking each file with
// whether the review's paths cover it.
func (s *Store) insertRound(ctx context.Context, tx *sql.Tx, round *Round, paths []string, files []File) error {
	insert := "INSERT INTO rounds (review_id, number, note, base_commit, created) VALUES (?, ?, ?, ?, ?)"
	args := []any{round.ReviewID, round.Number, round.Note, round.BaseCommit, FormatTime(s.now())}
	res, err := tx.ExecContext(ctx, insert, args...)
	if err != nil {
		return errors.Wrap(err, "insert round")
	}
	if round.ID, err = res.LastInsertId(); err != nil {
		return errors.Wrap(err, "read round id")
	}
	for _, f := range files {
		p, err := normalizePath(f.Path)
		if err != nil {
			return err
		}
		add := "INSERT INTO round_files (round_id, path, digest, size, in_scope) VALUES (?, ?, ?, ?, ?)"
		inScope := pathsCover(paths, p)
		if _, err := tx.ExecContext(ctx, add, round.ID, p, f.Digest, f.Size, inScope); err != nil {
			return errors.Wrapf(err, "capture file %s", p)
		}
	}
	return nil
}

// tx runs fn inside a write transaction, rolling back unless it returns nil.
//
// The DSN opens every transaction as BEGIN IMMEDIATE, so a read followed by a
// write cannot interleave with another writer's. That is what makes the overlap
// refusal hold rather than letting two callers both pass a check only one
// should.
func (s *Store) tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	return errors.Wrap(tx.Commit(), "commit transaction")
}

// pathsCover reports whether any of a review's claimed paths covers p.
func pathsCover(paths []string, p string) bool {
	for _, claimed := range paths {
		if pathsOverlap(claimed, p) {
			return true
		}
	}
	return false
}

// normalizePath cleans one path and refuses anything that is not relative to a
// project root.
//
// The separator is a forward slash and is not translated, because a backslash
// is a legal character in a filename on the platforms the daemon runs on, and
// git reports paths with slashes already. A path that escapes its root, is
// absolute, or is empty is refused rather than clamped: a caller that meant
// something else should hear about it.
func normalizePath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", errors.Wrap(ErrInvalidPath, "a path cannot be empty")
	}
	if strings.HasPrefix(p, "/") {
		return "", errors.Wrapf(ErrInvalidPath, "%q is absolute; pass a path relative to the project root", p)
	}
	out := path.Clean(p)
	if out == "." {
		return "", errors.Wrapf(ErrInvalidPath, "%q names the project root rather than a path in it", p)
	}
	if out == ".." || strings.HasPrefix(out, "../") {
		return "", errors.Wrapf(ErrInvalidPath, "%q leaves the project root", p)
	}
	return out, nil
}

// normalizePaths cleans a review's claimed paths and refuses a set where one
// covers another, which would claim the same file twice.
func normalizePaths(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, p := range in {
		cleaned, err := normalizePath(p)
		if err != nil {
			return nil, err
		}
		for _, kept := range out {
			if pathsOverlap(kept, cleaned) {
				return nil, errors.Wrapf(ErrInvalidPath, "%q and %q cover the same files", kept, cleaned)
			}
		}
		out = append(out, cleaned)
	}
	return out, nil
}

// pathsOverlap reports whether two normalized paths cover any file in common.
//
// A directory covers everything under it, so the comparison is at a segment
// boundary: docs claims docs/product.md and does not claim docsite.
func pathsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// refuseClaimed reports a path an open review in the same project already
// covers, naming that review so the fix is to go and look at it.
func refuseClaimed(ctx context.Context, tx *sql.Tx, projectID int64, paths []string) error {
	query := `SELECT p.path, r.id, r.title
		FROM review_paths p JOIN reviews r ON r.id = p.review_id
		WHERE r.project_id = ? AND r.state = ?`
	rows, err := tx.QueryContext(ctx, query, projectID, StateOpen)
	if err != nil {
		return errors.Wrap(err, "read claimed paths")
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			claimed, title string
			id             int64
		)
		if err := rows.Scan(&claimed, &id, &title); err != nil {
			return errors.Wrap(err, "scan claimed path")
		}
		for _, want := range paths {
			if pathsOverlap(claimed, want) {
				return errors.Wrapf(ErrPathClaimed, "%q is covered by %q in review %d (%s)", want, claimed, id, title)
			}
		}
	}
	return errors.Wrap(rows.Err(), "read claimed paths")
}

// reviewPaths reads a review's claimed paths.
func reviewPaths(ctx context.Context, tx *sql.Tx, reviewID int64) ([]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT path FROM review_paths WHERE review_id = ?", reviewID)
	if err != nil {
		return nil, errors.Wrap(err, "read review paths")
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, errors.Wrap(err, "scan review path")
		}
		out = append(out, p)
	}
	return out, errors.Wrap(rows.Err(), "read review paths")
}

// reviewProgress reports a review's state, its latest round number, and whether
// that round has a decision.
func reviewProgress(ctx context.Context, tx *sql.Tx, reviewID int64) (State, int, bool, error) {
	// The join picks the latest round specifically. Aggregating a decision
	// across every round would report a review as decided while its newest
	// round was still waiting.
	query := `SELECT r.state,
			coalesce(o.number, 0),
			CASE WHEN d.round_id IS NOT NULL THEN 1 ELSE 0 END
		FROM reviews r
		LEFT JOIN rounds o
			ON o.review_id = r.id
			AND o.number = (SELECT max(number) FROM rounds WHERE review_id = r.id)
		LEFT JOIN decisions d ON d.round_id = o.id
		WHERE r.id = ?`
	var (
		state           State
		latest, decided int
	)
	row := tx.QueryRowContext(ctx, query, reviewID)
	if err := row.Scan(&state, &latest, &decided); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", 0, false, errors.Wrapf(ErrNotFound, "review %d", reviewID)
		}
		return "", 0, false, errors.Wrap(err, "read review")
	}
	return state, latest, decided == 1, nil
}
