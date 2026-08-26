package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"
)

func Test_checkComment(t *testing.T) {
	bad := []struct {
		name string
		nc   NewComment
	}{
		{
			name: "a level that does not exist",
			nc:   NewComment{Level: "hunk", Path: "a.md", Body: "a point"},
		},
		{
			name: "no body",
			nc:   NewComment{Level: LevelFile, Path: "a.md", Body: "   "},
		},
		{
			name: "a review-level comment naming a path",
			nc:   NewComment{Level: LevelReview, Path: "a.md", Body: "a point"},
		},
		{
			name: "a file-level comment naming no path",
			nc:   NewComment{Level: LevelFile, Body: "a point"},
		},
		{
			name: "a path that climbs out",
			nc:   NewComment{Level: LevelFile, Path: "../etc/passwd", Body: "a point"},
		},
		{
			name: "a file-level comment carrying a range",
			nc: NewComment{
				Level:     LevelFile,
				Path:      "a.md",
				StartLine: 1,
				EndLine:   1,
				Quoted:    "x",
				Body:      "a point",
			},
		},
		{
			name: "a file-level comment carrying quoted text",
			nc: NewComment{
				Level:  LevelFile,
				Path:   "a.md",
				Quoted: "x",
				Body:   "a point",
			},
		},
		{
			name: "a line comment with no range",
			nc: NewComment{
				Level: LevelLine,
				Path:  "a.md",
				Body:  "a point",
			},
		},
		{
			name: "a range starting below line one",
			nc: NewComment{
				Level:     LevelLine,
				Path:      "a.md",
				StartLine: 0,
				EndLine:   4,
				Quoted:    "x",
				Body:      "a point",
			},
		},
		{
			name: "a range ending before it starts",
			nc: NewComment{
				Level:     LevelLine,
				Path:      "a.md",
				StartLine: 9,
				EndLine:   4,
				Quoted:    "x",
				Body:      "a point",
			},
		},
		{
			name: "a range with no quoted text",
			nc: NewComment{
				Level:     LevelLine,
				Path:      "a.md",
				StartLine: 1,
				EndLine:   1,
				Body:      "a point",
			},
		},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			_, err := checkComment(c.nc)
			g.Expect(err).To(HaveOccurred())
		})
	}
}

func TestStore_AddComment(t *testing.T) {
	levels := []struct {
		name string
		nc   NewComment
		want Comment
	}{
		{
			name: "a line",
			nc: NewComment{
				Level:     LevelLine,
				Path:      "./docs/product.md",
				StartLine: 12,
				EndLine:   12,
				Quoted:    "one line",
				Body:      "  a point  ",
			},
			want: Comment{
				Level:     LevelLine,
				Path:      "docs/product.md",
				StartLine: 12,
				EndLine:   12,
				Quoted:    "one line",
				Body:      "a point",
			},
		},
		{
			name: "a passage",
			nc: NewComment{
				Level:     LevelPassage,
				Path:      "docs/product.md",
				StartLine: 20,
				EndLine:   24,
				Quoted:    "a passage",
				Body:      "a point",
			},
			want: Comment{
				Level:     LevelPassage,
				Path:      "docs/product.md",
				StartLine: 20,
				EndLine:   24,
				Quoted:    "a passage",
				Body:      "a point",
			},
		},
		{
			name: "a file",
			nc: NewComment{
				Level: LevelFile,
				Path:  "docs/product.md",
				Body:  "a point",
			},
			want: Comment{
				Level: LevelFile,
				Path:  "docs/product.md",
				Body:  "a point",
			},
		},
		{
			name: "the review",
			nc: NewComment{
				Level: LevelReview,
				Body:  "the approach is wrong",
			},
			want: Comment{
				Level: LevelReview,
				Body:  "the approach is wrong",
			},
		},
	}
	for _, c := range levels {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			s := openStore(g, t)
			project := ensureProject(g, t, s)
			_, round := openReview(g, t, s, project.ID, "docs")

			c.nc.RoundID = round.ID
			got, err := s.AddComment(t.Context(), c.nc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got.ID).NotTo(BeZero())

			c.want.ID = got.ID
			c.want.RoundID = round.ID
			g.Expect(got).To(Equal(c.want))
			g.Expect(readComment(g, t, s, got.ID)).To(Equal(c.want))
		})
	}
}

func TestStore_AddComment_onAClosedRound(t *testing.T) {
	g := NewWithT(t)
	// Refusing these would discard what somebody wrote to protect nothing. A
	// comment on a closed round goes out on its own instead of waiting for a
	// verdict that has already been given.
	s := openStore(g, t)
	project := ensureProject(g, t, s)

	decidedReview, decidedRound := openReview(g, t, s, project.ID, "docs")
	g.Expect(s.Decide(t.Context(), decidedRound.ID, VerdictApprove, "")).To(Succeed())
	g.Expect(stateOf(g, t, s, decidedReview.ID)).To(Equal(StateApproved))
	_, err := s.AddComment(t.Context(), fileComment(decidedRound.ID))
	g.Expect(err).NotTo(HaveOccurred())

	abandonedReview, abandonedRound := openReview(g, t, s, project.ID, "internal")
	g.Expect(s.Abandon(t.Context(), abandonedReview.ID)).To(Succeed())
	_, err = s.AddComment(t.Context(), fileComment(abandonedRound.ID))
	g.Expect(err).NotTo(HaveOccurred())

	_, err = s.AddComment(t.Context(), fileComment(9999))
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestStore_Decide(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)
	review, round := openReview(g, t, s, project.ID, "docs")

	// Asking for changes leaves the review open, so the agent can ask again.
	g.Expect(s.Decide(t.Context(), round.ID, VerdictChanges, "make it shorter")).To(Succeed())
	g.Expect(stateOf(g, t, s, review.ID)).To(Equal(StateOpen))

	var (
		verdict Verdict
		note    string
	)
	row := s.db.QueryRowContext(t.Context(), "SELECT verdict, note FROM decisions WHERE round_id = ?", round.ID)
	g.Expect(row.Scan(&verdict, &note)).To(Succeed())
	g.Expect(verdict).To(Equal(VerdictChanges))
	g.Expect(note).To(Equal("make it shorter"))

	// Approving the next round ends the review, in the same transaction as the
	// decision that caused it.
	second, err := s.OpenRound(t.Context(), review.ID, Request{Note: "did it"})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(s.Decide(t.Context(), second.ID, VerdictApprove, "")).To(Succeed())
	g.Expect(stateOf(g, t, s, review.ID)).To(Equal(StateApproved))
}

func TestStore_Decide_refusals(t *testing.T) {
	t.Run("a change request with no comment and no note", func(t *testing.T) {
		g := NewWithT(t)
		s := openStore(g, t)
		project := ensureProject(g, t, s)
		_, round := openReview(g, t, s, project.ID, "docs")

		err := s.Decide(t.Context(), round.ID, VerdictChanges, "  ")
		g.Expect(errors.Is(err, ErrEmptyChangeRequest)).To(BeTrue())
		g.Expect(err.Error()).To(ContainSubstring("what to change"))

		// One comment is enough: the agent has something to act on.
		_, err = s.AddComment(t.Context(), fileComment(round.ID))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(s.Decide(t.Context(), round.ID, VerdictChanges, "")).To(Succeed())
	})

	t.Run("an approval needs neither", func(t *testing.T) {
		g := NewWithT(t)
		s := openStore(g, t)
		project := ensureProject(g, t, s)
		_, round := openReview(g, t, s, project.ID, "docs")
		g.Expect(s.Decide(t.Context(), round.ID, VerdictApprove, "")).To(Succeed())
	})

	t.Run("a round that already carries a verdict", func(t *testing.T) {
		g := NewWithT(t)
		s := openStore(g, t)
		project := ensureProject(g, t, s)
		_, round := openReview(g, t, s, project.ID, "docs")
		g.Expect(s.Decide(t.Context(), round.ID, VerdictChanges, "again")).To(Succeed())
		err := s.Decide(t.Context(), round.ID, VerdictApprove, "")
		g.Expect(errors.Is(err, ErrAlreadyDecided)).To(BeTrue())
	})

	t.Run("a round of an abandoned review", func(t *testing.T) {
		g := NewWithT(t)
		s := openStore(g, t)
		project := ensureProject(g, t, s)
		review, round := openReview(g, t, s, project.ID, "docs")
		g.Expect(s.Abandon(t.Context(), review.ID)).To(Succeed())
		err := s.Decide(t.Context(), round.ID, VerdictApprove, "")
		g.Expect(errors.Is(err, ErrNotOpen)).To(BeTrue())
	})

	t.Run("a verdict that is neither", func(t *testing.T) {
		g := NewWithT(t)
		s := openStore(g, t)
		project := ensureProject(g, t, s)
		_, round := openReview(g, t, s, project.ID, "docs")
		g.Expect(s.Decide(t.Context(), round.ID, "maybe", "")).NotTo(Succeed())
	})

	t.Run("a round that does not exist", func(t *testing.T) {
		g := NewWithT(t)
		s := openStore(g, t)
		err := s.Decide(t.Context(), 9999, VerdictApprove, "")
		g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
	})
}

func TestStore_DeleteComment(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)
	_, round := openReview(g, t, s, project.ID, "docs")

	comment, err := s.AddComment(t.Context(), fileComment(round.ID))
	g.Expect(err).NotTo(HaveOccurred())

	// The row goes rather than being marked deleted: nothing downstream has
	// seen it, so there is nothing to reconcile.
	g.Expect(s.DeleteComment(t.Context(), comment.ID)).To(Succeed())
	var count int
	row := s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM comments WHERE id = ?", comment.ID)
	g.Expect(row.Scan(&count)).To(Succeed())
	g.Expect(count).To(BeZero())

	g.Expect(errors.Is(s.DeleteComment(t.Context(), comment.ID), ErrNotFound)).To(BeTrue())
}

func TestStore_EditComment(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)
	_, round := openReview(g, t, s, project.ID, "docs")

	comment, err := s.AddComment(t.Context(), fileComment(round.ID))
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(s.EditComment(t.Context(), comment.ID, "  a better point  ")).To(Succeed())
	g.Expect(readComment(g, t, s, comment.ID).Body).To(Equal("a better point"))

	g.Expect(s.EditComment(t.Context(), comment.ID, "   ")).NotTo(Succeed())
	g.Expect(errors.Is(s.EditComment(t.Context(), 9999, "x"), ErrNotFound)).To(BeTrue())
}

func TestStore_EditComment_afterTheRoundCloses(t *testing.T) {
	// Two separate things close a round, and each needs its own case. A change
	// request leaves the review open, so only the decision closes the round.
	// An approval and an abandonment both move the review's state as well, and
	// would mask a missing check on the decision.
	closings := []struct {
		name string
		how  string
	}{
		{
			name: "a change request releases it, review still open",
			how:  "changes",
		},
		{
			name: "an approval releases it and ends the review",
			how:  "approve",
		},
		{
			name: "abandoning the review closes the round too",
			how:  "abandon",
		},
	}
	for _, c := range closings {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			s := openStore(g, t)
			project := ensureProject(g, t, s)
			review, round := openReview(g, t, s, project.ID, "docs")
			comment, err := s.AddComment(t.Context(), fileComment(round.ID))
			g.Expect(err).NotTo(HaveOccurred())

			switch c.how {
			case "abandon":
				g.Expect(s.Abandon(t.Context(), review.ID)).To(Succeed())
			case "changes":
				g.Expect(s.Decide(t.Context(), round.ID, VerdictChanges, "do it again")).To(Succeed())
				g.Expect(stateOf(g, t, s, review.ID)).To(Equal(StateOpen))
			default:
				g.Expect(s.Decide(t.Context(), round.ID, VerdictApprove, "")).To(Succeed())
			}

			err = s.EditComment(t.Context(), comment.ID, "too late")
			g.Expect(errors.Is(err, ErrRoundClosed)).To(BeTrue())
			err = s.DeleteComment(t.Context(), comment.ID)
			g.Expect(errors.Is(err, ErrRoundClosed)).To(BeTrue())

			// The body is untouched, which is what makes the refusal worth
			// having rather than just tidy.
			g.Expect(readComment(g, t, s, comment.ID).Body).To(Equal("a point"))
		})
	}
}

// BenchmarkStore_AddComment prices the setting the store actually runs with.
func BenchmarkStore_AddComment(b *testing.B) {
	benchmarkAddComment(b, "FULL")
}

// BenchmarkStore_AddComment_synchronousNormal prices the setting FULL was
// chosen over, so the difference between them is a number rather than a claim.
func BenchmarkStore_AddComment_synchronousNormal(b *testing.B) {
	benchmarkAddComment(b, "NORMAL")
}

// benchmarkAddComment writes comments one at a time, which is the shape of the
// real workload: one fsync per write transaction, a few writes a minute.
func benchmarkAddComment(b *testing.B, mode string) {
	g := NewWithT(b)
	ctx := b.Context()
	s := openSynchronous(g, b, ctx, mode)
	project, err := s.EnsureProject(ctx, "/tmp/project", "project")
	g.Expect(err).NotTo(HaveOccurred())
	_, round, err := s.OpenReview(ctx, newReview(project.ID, "docs"))
	g.Expect(err).NotTo(HaveOccurred())

	for i := 0; b.Loop(); i++ {
		nc := fileComment(round.ID)
		nc.Body = "a point, number " + strconv.Itoa(i)
		if _, err := s.AddComment(ctx, nc); err != nil {
			b.Fatal(err)
		}
	}
}

// fileComment is the smallest comment that stores, for a test whose subject is
// something other than the comment's shape.
func fileComment(roundID int64) NewComment {
	return NewComment{
		RoundID: roundID,
		Level:   LevelFile,
		Path:    "docs/product.md",
		Body:    "a point",
	}
}

// openSynchronous opens a migrated store with synchronous set to mode, so the
// cost of FULL can be compared against NORMAL rather than asserted.
func openSynchronous(g *WithT, tb testing.TB, ctx context.Context, mode string) *Store {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "eyeball.db")
	migrated, err := Open(ctx, path)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(migrated.Close()).To(Succeed())

	tuned := strings.Replace(dsn(path), "synchronous%28FULL%29", "synchronous%28"+mode+"%29", 1)
	db, err := sql.Open("sqlite", tuned)
	g.Expect(err).NotTo(HaveOccurred())
	db.SetMaxOpenConns(1)
	tb.Cleanup(func() { _ = db.Close() })

	var got string
	g.Expect(db.QueryRow("PRAGMA synchronous").Scan(&got)).To(Succeed())
	g.Expect(got).To(Equal(map[string]string{"FULL": "2", "NORMAL": "1"}[mode]))
	return &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// readComment reads a comment back out of the row, so a test checks what was
// stored rather than what was returned.
func readComment(g *WithT, t *testing.T, s *Store, commentID int64) Comment {
	g.THelper()
	query := `SELECT id, round_id, level, coalesce(path, ''), coalesce(start_line, 0),
			coalesce(end_line, 0), coalesce(quoted, ''), body
		FROM comments WHERE id = ?`
	var out Comment
	row := s.db.QueryRowContext(t.Context(), query, commentID)
	err := row.Scan(&out.ID, &out.RoundID, &out.Level, &out.Path, &out.StartLine, &out.EndLine, &out.Quoted, &out.Body)
	g.Expect(err).NotTo(HaveOccurred())
	return out
}
