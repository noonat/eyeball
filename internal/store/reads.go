package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/cockroachdb/errors"
)

// rowSelect is the shared shape of every listing: a review joined to its latest
// round, with the decision on that round if there is one.
//
// The join picks the latest round rather than aggregating over all of them,
// because a review with an earlier round decided and its newest one waiting is
// waiting, not working.
const rowSelect = `SELECT r.id, r.project_id, p.name, r.agent, r.title, r.state,
		o.number, o.created,
		(SELECT count(*) FROM round_files f WHERE f.round_id = o.id AND f.in_scope = 1)
	FROM reviews r
	JOIN projects p ON p.id = r.project_id
	JOIN rounds o
		ON o.review_id = r.id
		AND o.number = (SELECT max(number) FROM rounds WHERE review_id = r.id)
	LEFT JOIN decisions d ON d.round_id = o.id`

// ReviewRow is one review as a listing shows it: the review, plus the headline
// numbers from its latest round.
type ReviewRow struct {
	// ReviewID identifies the review.
	ReviewID int64
	// ProjectID is the project the review belongs to.
	ProjectID int64
	// ProjectName is that project's display name. It is on every row rather
	// than behind a filter, because one surface carries every project at once.
	ProjectName string
	// Agent is the agent that opened the review, on every row for the same
	// reason.
	Agent string
	// Title is what the row shows.
	Title string
	// State is one of StateOpen, StateApproved or StateAbandoned.
	State State
	// Round is the latest round's number.
	Round int
	// Files counts the paths that round captured inside the review's scope.
	// Files it captured for context are not the size of the change.
	Files int
	// RequestedAt is when the latest round was asked for, which is what the
	// queue turns into how long something has been waiting.
	RequestedAt time.Time
}

// Queue is what needs attention, in the three groups the surface shows.
type Queue struct {
	// Waiting is every review with a request outstanding, longest wait first.
	// Nothing competes with it.
	Waiting []ReviewRow
	// Working is the reviews an agent holds, most recently answered first.
	Working []ReviewRow
	// Settled is what was approved recently, newest first and cut short.
	Settled []ReviewRow
}

// RoundFile is one path a round captured, as it was stored.
type RoundFile struct {
	// File is the path, the digest and the size.
	File
	// InScope says whether the review's paths cover this one. It was decided
	// when the round was written and does not move afterward.
	InScope bool
}

// Queue reads the three groups the surface opens on, keeping at most
// settledLimit of what was approved.
//
// An abandoned review is in none of them. The agent withdrew the question, so
// there is nothing waiting, nothing being acted on, and nothing that was
// decided.
func (s *Store) Queue(ctx context.Context, settledLimit int) (Queue, error) {
	waiting := rowSelect + " WHERE r.state = ? AND d.round_id IS NULL ORDER BY o.created, r.id"
	working := rowSelect + " WHERE r.state = ? AND d.round_id IS NOT NULL ORDER BY d.decided DESC, r.id"
	settled := rowSelect + " WHERE r.state = ? ORDER BY d.decided DESC, r.id LIMIT ?"

	// One snapshot for all three groups. Read separately, a verdict landing
	// between the waiting and working queries lists the same review in both,
	// and a new round landing between them drops it from the queue entirely
	// while it is waiting for an answer.
	var out Queue
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		if out.Waiting, err = rows(ctx, tx, waiting, StateOpen); err != nil {
			return err
		}
		if out.Working, err = rows(ctx, tx, working, StateOpen); err != nil {
			return err
		}
		out.Settled, err = rows(ctx, tx, settled, StateApproved, settledLimit)
		return err
	})
	if err != nil {
		return Queue{}, err
	}
	return out, nil
}

// ReviewsFor lists one agent's reviews in one project, newest request first.
//
// This is the slice an agent sees. The reviewer's surface combines every
// project and every agent, and an agent reading that would have to skip past a
// spec review in another project, or act on it.
func (s *Store) ReviewsFor(ctx context.Context, projectID int64, agent string) ([]ReviewRow, error) {
	query := rowSelect + " WHERE r.project_id = ? AND r.agent = ? ORDER BY o.created DESC, r.id DESC"
	var out []ReviewRow
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = rows(ctx, tx, query, projectID, agent)
		return err
	})
	return out, err
}

// Review reads one review, its claimed paths, and the number of its latest
// round, which is what a caller needs before it can ask for a round.
func (s *Store) Review(ctx context.Context, reviewID int64) (Review, error) {
	var out Review
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		query := `SELECT r.id, r.project_id, r.agent, r.title, r.state,
				coalesce(max(o.number), 0)
			FROM reviews r
			LEFT JOIN rounds o ON o.review_id = r.id
			WHERE r.id = ?
			GROUP BY r.id`
		row := tx.QueryRowContext(ctx, query, reviewID)
		err := row.Scan(&out.ID, &out.ProjectID, &out.Agent, &out.Title, &out.State, &out.LatestRound)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.Wrapf(ErrNotFound, "review %d", reviewID)
			}
			return errors.Wrap(err, "read review")
		}
		out.Paths, err = reviewPaths(ctx, tx, reviewID)
		return err
	})
	if err != nil {
		return Review{}, err
	}
	return out, nil
}

// Round reads one round of a review by its number, which is how it is addressed
// everywhere outside the store.
func (s *Store) Round(ctx context.Context, reviewID int64, number int) (Round, error) {
	query := `SELECT id, review_id, number, note, base_commit
		FROM rounds WHERE review_id = ? AND number = ?`
	var out Round
	row := s.db.QueryRowContext(ctx, query, reviewID, number)
	err := row.Scan(&out.ID, &out.ReviewID, &out.Number, &out.Note, &out.BaseCommit)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Round{}, errors.Wrapf(ErrNotFound, "review %d round %d", reviewID, number)
		}
		return Round{}, errors.Wrap(err, "read round")
	}
	return out, nil
}

// RoundFiles reads everything a round captured, in path order.
func (s *Store) RoundFiles(ctx context.Context, roundID int64) ([]RoundFile, error) {
	query := `SELECT path, digest, size, in_scope
		FROM round_files WHERE round_id = ? ORDER BY path`
	rows, err := s.db.QueryContext(ctx, query, roundID)
	if err != nil {
		return nil, errors.Wrap(err, "read round files")
	}
	defer func() { _ = rows.Close() }()

	var out []RoundFile
	for rows.Next() {
		var file RoundFile
		if err := rows.Scan(&file.Path, &file.Digest, &file.Size, &file.InScope); err != nil {
			return nil, errors.Wrap(err, "scan round file")
		}
		out = append(out, file)
	}
	return out, errors.Wrap(rows.Err(), "read round files")
}

// Comments reads a round's comments in the order the review was read in: by
// path, then by where in the file the comment sits.
//
// A review-level comment has no path and comes last. It is the point about the
// whole approach rather than about a place, and it is written once the reading
// is done.
func (s *Store) Comments(ctx context.Context, roundID int64) ([]Comment, error) {
	query := `SELECT id, round_id, level, coalesce(path, ''), coalesce(start_line, 0),
			coalesce(end_line, 0), coalesce(quoted, ''), body
		FROM comments WHERE round_id = ?
		ORDER BY path IS NULL, path, start_line, id`
	rows, err := s.db.QueryContext(ctx, query, roundID)
	if err != nil {
		return nil, errors.Wrap(err, "read comments")
	}
	defer func() { _ = rows.Close() }()

	var out []Comment
	for rows.Next() {
		var c Comment
		err := rows.Scan(&c.ID, &c.RoundID, &c.Level, &c.Path, &c.StartLine, &c.EndLine, &c.Quoted, &c.Body)
		if err != nil {
			return nil, errors.Wrap(err, "scan comment")
		}
		out = append(out, c)
	}
	return out, errors.Wrap(rows.Err(), "read comments")
}

// readTx runs fn inside a read transaction, which is one snapshot and does not
// take the write lock.
//
// The DSN sets _txlock=immediate for every transaction on this handle, so a
// plain BeginTx would make a read wait on a writer and, past busy_timeout,
// fail. Measured against this driver: with another process holding the write
// lock, a default transaction blocked for the whole timeout and returned
// SQLITE_BUSY, and a read-only one opened at once.
//
// ReadOnly is a lock-mode hint here rather than a guarantee. This driver still
// executes a write inside such a transaction, so only the read methods use it.
func (s *Store) readTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return errors.Wrap(err, "begin read transaction")
	}
	defer func() { _ = tx.Rollback() }()
	return fn(tx)
}

// rows runs one of the listing queries inside the caller's transaction, so
// several listings can share one snapshot.
func rows(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]ReviewRow, error) {
	found, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.Wrap(err, "read reviews")
	}
	defer func() { _ = found.Close() }()

	var out []ReviewRow
	for found.Next() {
		var (
			row       ReviewRow
			requested string
		)
		err := found.Scan(
			&row.ReviewID,
			&row.ProjectID,
			&row.ProjectName,
			&row.Agent,
			&row.Title,
			&row.State,
			&row.Round,
			&requested,
			&row.Files,
		)
		if err != nil {
			return nil, errors.Wrap(err, "scan review")
		}
		if row.RequestedAt, err = ParseTime(requested); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, errors.Wrap(found.Err(), "read reviews")
}
