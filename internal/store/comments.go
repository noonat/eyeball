package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/cockroachdb/errors"
)

// Level is where a comment attaches. A line and a passage both carry a range,
// because a passage in a rendered document maps to lines in the frozen source.
type Level string

const (
	// LevelLine is a comment on a line or a range of lines.
	LevelLine Level = "line"
	// LevelPassage is a comment on a passage of a rendered document.
	LevelPassage Level = "passage"
	// LevelFile is a comment about a file rather than a place in it.
	LevelFile Level = "file"
	// LevelReview is a comment about the whole review, which is where the
	// point that the approach is wrong has to go.
	LevelReview Level = "review"
)

// Verdict is what a reviewer decides. Both verdicts release the round: asking
// for changes finishes a reading as completely as approving does.
type Verdict string

const (
	// VerdictApprove ends the review.
	VerdictApprove Verdict = "approve"
	// VerdictChanges closes the round and lets the agent ask again.
	VerdictChanges Verdict = "changes"
)

// ErrRoundClosed reports a round whose comments have been released, so nothing
// in it can be edited or deleted any more.
var ErrRoundClosed = errors.New("round is closed")

// ErrAlreadyDecided reports a round that already carries a verdict.
var ErrAlreadyDecided = errors.New("round is already decided")

// ErrEmptyChangeRequest reports a change request with no comment and no note.
// The agent would be told to change something without being told what.
var ErrEmptyChangeRequest = errors.New("change request says nothing")

// Comment is one point the reviewer made on a round.
type Comment struct {
	// ID is the comment's identity, and what an agent replies to.
	ID int64
	// RoundID is the round the comment was written on.
	RoundID int64
	// Level is one of LevelLine, LevelPassage, LevelFile or LevelReview.
	Level Level
	// Path is the file the comment is about, empty at LevelReview.
	Path string
	// StartLine and EndLine bound the comment, and are zero where the level
	// carries no range.
	StartLine, EndLine int
	// Quoted is the text the range covers, so an agent never has to work out
	// what was pointed at.
	Quoted string
	// Body is what the reviewer wrote.
	Body string
}

// NewComment is a comment to store.
type NewComment struct {
	// RoundID is the round being commented on.
	RoundID int64
	// Level is one of LevelLine, LevelPassage, LevelFile or LevelReview.
	Level Level
	// Path is the file the comment is about, and is left empty at LevelReview.
	Path string
	// StartLine and EndLine bound the comment, and are left zero at LevelFile
	// and LevelReview.
	StartLine, EndLine int
	// Quoted is the text the range covers, and travels with a range.
	Quoted string
	// Body is what the reviewer wrote.
	Body string
}

// AddComment stores one comment.
//
// It is allowed on a decided round and on an abandoned review. Refusing it
// would discard what somebody wrote in order to protect nothing, and a comment
// on a closed round goes out on its own rather than waiting for a verdict that
// has already been given.
func (s *Store) AddComment(ctx context.Context, nc NewComment) (Comment, error) {
	comment, err := checkComment(nc)
	if err != nil {
		return Comment{}, err
	}
	err = s.tx(ctx, func(tx *sql.Tx) error {
		if _, _, err := roundState(ctx, tx, nc.RoundID); err != nil {
			return err
		}
		at := FormatTime(s.now())
		insert := `INSERT INTO comments
			(round_id, level, path, start_line, end_line, quoted, body, created, updated)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
		args := []any{
			comment.RoundID,
			comment.Level,
			nullText(comment.Path),
			nullLine(comment.StartLine),
			nullLine(comment.EndLine),
			nullText(comment.Quoted),
			comment.Body,
			at,
			at,
		}
		res, err := tx.ExecContext(ctx, insert, args...)
		if err != nil {
			return errors.Wrap(err, "insert comment")
		}
		if comment.ID, err = res.LastInsertId(); err != nil {
			return errors.Wrap(err, "read comment id")
		}
		return nil
	})
	if err != nil {
		return Comment{}, err
	}
	return comment, nil
}

// EditComment replaces a comment's body while its round is still open.
//
// Once the round is decided the comment has been released, so what the agent
// received and what the store holds would otherwise stop matching.
func (s *Store) EditComment(ctx context.Context, commentID int64, body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return errors.New("a comment needs a body")
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		if err := requireOpenRound(ctx, tx, commentID); err != nil {
			return err
		}
		update := "UPDATE comments SET body = ?, updated = ? WHERE id = ?"
		if _, err := tx.ExecContext(ctx, update, body, FormatTime(s.now()), commentID); err != nil {
			return errors.Wrap(err, "edit comment")
		}
		return nil
	})
}

// DeleteComment removes a comment while its round is still open.
//
// The row goes rather than being marked deleted. Nothing downstream has seen
// it, and a tombstone would have to be filtered out of every read and every
// release to protect a comment the reviewer removed on purpose.
func (s *Store) DeleteComment(ctx context.Context, commentID int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if err := requireOpenRound(ctx, tx, commentID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM comments WHERE id = ?", commentID); err != nil {
			return errors.Wrap(err, "delete comment")
		}
		return nil
	})
}

// Decide records a verdict on a round, releasing everything written on it.
//
// An approval ends the review, so the review's state moves in the same
// transaction as the decision that caused it. A change request with no comment
// and no note is refused: the agent would be told to change something without
// being told what, and would either guess or come back to ask.
func (s *Store) Decide(ctx context.Context, roundID int64, verdict Verdict, note string) error {
	if verdict != VerdictApprove && verdict != VerdictChanges {
		return errors.Newf("verdict %q is neither %s nor %s", verdict, VerdictApprove, VerdictChanges)
	}
	note = strings.TrimSpace(note)

	return s.tx(ctx, func(tx *sql.Tx) error {
		state, decided, err := roundState(ctx, tx, roundID)
		if err != nil {
			return err
		}
		if decided {
			return errors.Wrapf(ErrAlreadyDecided, "round %d", roundID)
		}
		if state != StateOpen {
			return errors.Wrapf(ErrNotOpen, "the review of round %d is %s", roundID, state)
		}
		if verdict == VerdictChanges && note == "" {
			comments, err := countComments(ctx, tx, roundID)
			if err != nil {
				return err
			}
			if comments == 0 {
				return errors.Wrap(ErrEmptyChangeRequest, "write a comment or add a note saying what to change")
			}
		}
		at := FormatTime(s.now())
		insert := "INSERT INTO decisions (round_id, verdict, note, decided) VALUES (?, ?, ?, ?)"
		if _, err := tx.ExecContext(ctx, insert, roundID, verdict, note, at); err != nil {
			return errors.Wrap(err, "insert decision")
		}
		if verdict != VerdictApprove {
			return nil
		}
		update := `UPDATE reviews SET state = ?, updated = ?
			WHERE id = (SELECT review_id FROM rounds WHERE id = ?)`
		if _, err := tx.ExecContext(ctx, update, StateApproved, at, roundID); err != nil {
			return errors.Wrap(err, "approve review")
		}
		return nil
	})
}

// checkComment validates a comment and returns what will be stored.
//
// The schema carries the same rules as CHECK constraints. They are repeated
// here so a caller gets a sentence naming what is wrong rather than a
// constraint failure naming a table.
func checkComment(nc NewComment) (Comment, error) {
	out := Comment{
		RoundID:   nc.RoundID,
		Level:     nc.Level,
		StartLine: nc.StartLine,
		EndLine:   nc.EndLine,
		Quoted:    nc.Quoted,
		Body:      strings.TrimSpace(nc.Body),
	}
	switch nc.Level {
	case LevelLine, LevelPassage, LevelFile, LevelReview:
	default:
		return Comment{}, errors.Newf("comment level %q is not one of line, passage, file or review", nc.Level)
	}
	if out.Body == "" {
		return Comment{}, errors.New("a comment needs a body")
	}

	ranged := nc.Level == LevelLine || nc.Level == LevelPassage
	if nc.Level == LevelReview {
		if nc.Path != "" {
			return Comment{}, errors.Newf("a review-level comment has nowhere to point, so it cannot name %q", nc.Path)
		}
	} else {
		path, err := normalizePath(nc.Path)
		if err != nil {
			return Comment{}, err
		}
		out.Path = path
	}

	if !ranged {
		if nc.StartLine != 0 || nc.EndLine != 0 {
			return Comment{}, errors.Newf("a %s comment carries no line range", nc.Level)
		}
		if strings.TrimSpace(nc.Quoted) != "" {
			return Comment{}, errors.Newf("a %s comment carries no quoted text", nc.Level)
		}
		out.Quoted = ""
		return out, nil
	}
	if nc.StartLine < 1 {
		return Comment{}, errors.Newf("a %s comment needs a first line, counting from one", nc.Level)
	}
	if nc.EndLine < nc.StartLine {
		return Comment{}, errors.Newf("a range ending at %d cannot start at %d", nc.EndLine, nc.StartLine)
	}
	if nc.Quoted == "" {
		return Comment{}, errors.New("a comment on a range carries the text it covers, so the agent does not have to work it out")
	}
	return out, nil
}

// countComments counts what a round is holding.
func countComments(ctx context.Context, tx *sql.Tx, roundID int64) (int, error) {
	var out int
	row := tx.QueryRowContext(ctx, "SELECT count(*) FROM comments WHERE round_id = ?", roundID)
	if err := row.Scan(&out); err != nil {
		return 0, errors.Wrap(err, "count comments")
	}
	return out, nil
}

// nullLine is what a line-number column holds where a level carries no range.
func nullLine(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

// nullText is what an optional text column holds where there is nothing to
// store. The schema uses NULL for a path that does not exist rather than ”,
// because a review-level comment has no path at all.
func nullText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// requireOpenRound reports a comment whose round has closed, which is what
// makes it read-only.
func requireOpenRound(ctx context.Context, tx *sql.Tx, commentID int64) error {
	query := `SELECT r.state, CASE WHEN d.round_id IS NOT NULL THEN 1 ELSE 0 END
		FROM comments c
		JOIN rounds o ON o.id = c.round_id
		JOIN reviews r ON r.id = o.review_id
		LEFT JOIN decisions d ON d.round_id = o.id
		WHERE c.id = ?`
	var (
		state   State
		decided int
	)
	row := tx.QueryRowContext(ctx, query, commentID)
	if err := row.Scan(&state, &decided); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Wrapf(ErrNotFound, "comment %d", commentID)
		}
		return errors.Wrap(err, "read comment")
	}
	if decided == 1 {
		return errors.Wrapf(ErrRoundClosed, "comment %d was released by a verdict", commentID)
	}
	if state != StateOpen {
		return errors.Wrapf(ErrRoundClosed, "comment %d belongs to a review that is %s", commentID, state)
	}
	return nil
}

// roundState reports the state of a round's review and whether the round
// carries a decision.
func roundState(ctx context.Context, tx *sql.Tx, roundID int64) (State, bool, error) {
	query := `SELECT r.state, CASE WHEN d.round_id IS NOT NULL THEN 1 ELSE 0 END
		FROM rounds o
		JOIN reviews r ON r.id = o.review_id
		LEFT JOIN decisions d ON d.round_id = o.id
		WHERE o.id = ?`
	var (
		state   State
		decided int
	)
	row := tx.QueryRowContext(ctx, query, roundID)
	if err := row.Scan(&state, &decided); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, errors.Wrapf(ErrNotFound, "round %d", roundID)
		}
		return "", false, errors.Wrap(err, "read round")
	}
	return state, decided == 1, nil
}
