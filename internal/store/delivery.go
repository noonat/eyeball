package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/cockroachdb/errors"
)

// releasedWhere is the condition a comment's round and review meet once the
// agent may receive it: the round carries a decision, or the review has ended.
// An abandoned review's undecided round has nothing left to wait for, and a
// comment the reviewer writes on it goes out on its own, as one written on a
// decided round does.
const releasedWhere = `(d.round_id IS NOT NULL OR r.state <> 'open')`

// lateExpr is whether a released comment arrived after the batch it belongs to:
// written after its round's decision, or on a round that never got one.
//
// A comment written in the same millisecond as the verdict counts as on time.
// The timestamps cannot order the two. Calling it late would label a comment
// written before the verdict as written after it, and calling it on time costs
// only that it waits for the next verdict's batch, where it still arrives.
const lateExpr = `CASE WHEN d.round_id IS NULL OR c.created > d.decided THEN 1 ELSE 0 END`

// commentColumns are the columns every comment read scans, in scanComment's
// order, for a query that aliases comments as c.
const commentColumns = `c.id, c.round_id, c.level, coalesce(c.path, ''), coalesce(c.start_line, 0),
	coalesce(c.end_line, 0), coalesce(c.quoted, ''), c.body, c.created, c.reply, c.delivered`

// scanner is the one method of sql.Row and sql.Rows a scan needs.
type scanner interface {
	Scan(dest ...any) error
}

// scanComment reads commentColumns, followed by any extra destinations the
// query selects after them.
func scanComment(row scanner, extra ...any) (Comment, error) {
	var (
		c                  Comment
		created, delivered string
	)
	dest := []any{
		&c.ID,
		&c.RoundID,
		&c.Level,
		&c.Path,
		&c.StartLine,
		&c.EndLine,
		&c.Quoted,
		&c.Body,
		&created,
		&c.Reply,
		&delivered,
	}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return Comment{}, errors.Wrap(err, "scan comment")
	}
	var err error
	if c.Created, err = ParseTime(created); err != nil {
		return Comment{}, err
	}
	if delivered != "" {
		if c.Delivered, err = ParseTime(delivered); err != nil {
			return Comment{}, err
		}
	}
	return c, nil
}

// Decision reads the verdict on a round, and ErrNotFound for a round that has
// none yet.
func (s *Store) Decision(ctx context.Context, roundID int64) (Decision, error) {
	query := "SELECT round_id, verdict, note, decided FROM decisions WHERE round_id = ?"
	var (
		out     Decision
		decided string
	)
	err := s.db.QueryRowContext(ctx, query, roundID).Scan(&out.RoundID, &out.Verdict, &out.Note, &decided)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Decision{}, errors.Wrapf(ErrNotFound, "round %d has no decision", roundID)
		}
		return Decision{}, errors.Wrap(err, "read decision")
	}
	if out.Decided, err = ParseTime(decided); err != nil {
		return Decision{}, err
	}
	return out, nil
}

// Released reads every comment on a review that the agent may receive, in
// round order and then in the order Comments reads a round.
//
// A comment on an open review's undecided round is never here. Nothing reaches
// the agent before the verdict, and this is the read that enforces it.
func (s *Store) Released(ctx context.Context, reviewID int64) ([]ReleasedComment, error) {
	query := `SELECT ` + commentColumns + `, o.number, ` + lateExpr + `
		FROM comments c
		JOIN rounds o ON o.id = c.round_id
		JOIN reviews r ON r.id = o.review_id
		LEFT JOIN decisions d ON d.round_id = o.id
		WHERE o.review_id = ? AND ` + releasedWhere + `
		ORDER BY o.number, c.path IS NULL, c.path, c.start_line, c.id`
	rows, err := s.db.QueryContext(ctx, query, reviewID)
	if err != nil {
		return nil, errors.Wrap(err, "read released comments")
	}
	defer func() { _ = rows.Close() }()

	var out []ReleasedComment
	for rows.Next() {
		var (
			rc   ReleasedComment
			late int
		)
		if rc.Comment, err = scanComment(rows, &rc.Round, &late); err != nil {
			return nil, err
		}
		rc.Late = late == 1
		out = append(out, rc)
	}
	return out, errors.Wrap(rows.Err(), "read released comments")
}

// MarkDelivered stamps the comments an agent's client says it printed.
//
// Only released comments of this review are stamped. An id belonging to another
// review, or to a comment not yet released, is ignored rather than refused: the
// client is reporting what it showed, and a stray id in that report is not a
// reason to lose the stamps on the rest. A comment already stamped keeps its
// first time.
func (s *Store) MarkDelivered(ctx context.Context, reviewID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ")
	update := `UPDATE comments SET delivered = ?
		WHERE delivered = '' AND id IN (` + placeholders + `) AND id IN (
			SELECT c.id FROM comments c
			JOIN rounds o ON o.id = c.round_id
			JOIN reviews r ON r.id = o.review_id
			LEFT JOIN decisions d ON d.round_id = o.id
			WHERE o.review_id = ? AND ` + releasedWhere + `)`
	args := make([]any, 0, len(ids)+2)
	args = append(args, FormatTime(s.now()))
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, reviewID)
	return s.tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, update, args...)
		return errors.Wrap(err, "mark comments delivered")
	})
}

// MarkVerdictDelivered stamps the verdict on one round of a review as received
// by the agent. A round with no verdict, or one already stamped, is left alone.
func (s *Store) MarkVerdictDelivered(ctx context.Context, reviewID int64, round int) error {
	update := `UPDATE decisions SET delivered = ?
		WHERE delivered = '' AND round_id = (SELECT id FROM rounds WHERE review_id = ? AND number = ?)`
	return s.tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, update, FormatTime(s.now()), reviewID, round)
		return errors.Wrap(err, "mark the verdict delivered")
	})
}

// Reply stores the agent's answer to one released comment, replacing any
// earlier one. A comment that is not released yet is refused, since the agent
// cannot have seen it.
func (s *Store) Reply(ctx context.Context, commentID int64, body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return errors.Wrap(ErrMissing, "a reply needs a body")
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		query := `SELECT ` + releasedWhere + `
			FROM comments c
			JOIN rounds o ON o.id = c.round_id
			JOIN reviews r ON r.id = o.review_id
			LEFT JOIN decisions d ON d.round_id = o.id
			WHERE c.id = ?`
		var released bool
		if err := tx.QueryRowContext(ctx, query, commentID).Scan(&released); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.Wrapf(ErrNotFound, "comment %d", commentID)
			}
			return errors.Wrap(err, "read comment")
		}
		if !released {
			return errors.Wrapf(ErrNotReleased, "comment %d is on a round still waiting for a verdict", commentID)
		}
		update := "UPDATE comments SET reply = ?, replied = ? WHERE id = ?"
		if _, err := tx.ExecContext(ctx, update, body, FormatTime(s.now()), commentID); err != nil {
			return errors.Wrap(err, "store reply")
		}
		return nil
	})
}

// ErrNotReleased reports a comment the agent cannot have received yet, because
// its round has no verdict and its review is still open.
var ErrNotReleased = errors.New("comment is not released")
