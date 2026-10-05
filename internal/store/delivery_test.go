package store

import (
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"
)

func TestReviewRow_verdictAndUndelivered(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	stepClock(s)
	project := ensureProject(g, t, s)
	review, round := openReview(g, t, s, project.ID, "docs")
	a := addComment(g, t, s, round.ID, "one")
	addComment(g, t, s, round.ID, "two")

	rows, err := s.ReviewsFor(t.Context(), project.ID, "agent-1")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(rows[0].Verdict).To(BeEmpty())
	g.Expect(rows[0].Undelivered).To(BeZero(), "unreleased comments are not owed")

	g.Expect(s.Decide(t.Context(), round.ID, VerdictChanges, "")).To(Succeed())
	g.Expect(s.MarkDelivered(t.Context(), review.ID, []int64{a.ID})).To(Succeed())
	rows, err = s.ReviewsFor(t.Context(), project.ID, "agent-1")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(rows[0].Verdict).To(Equal(VerdictChanges))
	g.Expect(rows[0].Undelivered).To(Equal(1))

	// With a second round waiting, the outer query's decision is the empty one
	// on round two, and the count still has to see round one's.
	_, err = s.OpenRound(t.Context(), review.ID, Request{Note: "again", Files: []File{{Path: "docs"}}})
	g.Expect(err).NotTo(HaveOccurred())
	rows, err = s.ReviewsFor(t.Context(), project.ID, "agent-1")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(rows[0].Verdict).To(BeEmpty())
	g.Expect(rows[0].Undelivered).To(Equal(1))
}

func TestStore_Decision(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)
	_, round := openReview(g, t, s, project.ID, "docs")

	_, err := s.Decision(t.Context(), round.ID)
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())

	g.Expect(s.Decide(t.Context(), round.ID, VerdictChanges, "tighten it")).To(Succeed())
	got, err := s.Decision(t.Context(), round.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.RoundID).To(Equal(round.ID))
	g.Expect(got.Verdict).To(Equal(VerdictChanges))
	g.Expect(got.Note).To(Equal("tighten it"))
	g.Expect(got.Decided).NotTo(BeZero())
}

func TestStore_MarkDelivered(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	clock := stepClock(s)
	project := ensureProject(g, t, s)
	review, first := openReview(g, t, s, project.ID, "docs")
	other, otherRound := openReview(g, t, s, project.ID, "web")

	mine := addComment(g, t, s, first.ID, "mine")
	theirs := addComment(g, t, s, otherRound.ID, "theirs")
	g.Expect(s.Decide(t.Context(), first.ID, VerdictChanges, "")).To(Succeed())
	g.Expect(s.Decide(t.Context(), otherRound.ID, VerdictChanges, "")).To(Succeed())
	second, err := s.OpenRound(t.Context(), review.ID, Request{Note: "again", Files: []File{{Path: "docs"}}})
	g.Expect(err).NotTo(HaveOccurred())
	unreleased := addComment(g, t, s, second.ID, "still waiting")

	ids := []int64{mine.ID, theirs.ID, unreleased.ID}
	g.Expect(s.MarkDelivered(t.Context(), review.ID, ids)).To(Succeed())
	g.Expect(readComment(g, t, s, mine.ID).Delivered).NotTo(BeZero())
	g.Expect(readComment(g, t, s, theirs.ID).Delivered).To(BeZero(), "another review's comment")
	g.Expect(readComment(g, t, s, unreleased.ID).Delivered).To(BeZero(), "an unreleased comment")

	stamped := readComment(g, t, s, mine.ID).Delivered
	clock()
	g.Expect(s.MarkDelivered(t.Context(), review.ID, []int64{mine.ID})).To(Succeed())
	g.Expect(readComment(g, t, s, mine.ID).Delivered).To(Equal(stamped), "a stamped comment keeps its first time")

	g.Expect(s.MarkDelivered(t.Context(), other.ID, nil)).To(Succeed())
}

func TestStore_MarkVerdictDelivered(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	clock := stepClock(s)
	project := ensureProject(g, t, s)
	review, round := openReview(g, t, s, project.ID, "docs")
	other, otherRound := openReview(g, t, s, project.ID, "web")

	row := func(id int64) ReviewRow {
		rows, err := s.ReviewsIn(t.Context(), project.ID)
		g.Expect(err).NotTo(HaveOccurred())
		for _, r := range rows {
			if r.ReviewID == id {
				return r
			}
		}
		g.Expect(false).To(BeTrue(), "no row for review")
		return ReviewRow{}
	}
	g.Expect(s.MarkVerdictDelivered(t.Context(), review.ID, 1)).To(Succeed(), "no verdict yet is not an error")
	g.Expect(row(review.ID).VerdictUndelivered).To(BeFalse(), "an undecided round has no verdict to receive")

	g.Expect(s.Decide(t.Context(), round.ID, VerdictApprove, "")).To(Succeed())
	g.Expect(s.Decide(t.Context(), otherRound.ID, VerdictApprove, "")).To(Succeed())
	g.Expect(row(review.ID).VerdictUndelivered).To(BeTrue())

	g.Expect(s.MarkVerdictDelivered(t.Context(), review.ID, 1)).To(Succeed())
	g.Expect(row(review.ID).VerdictUndelivered).To(BeFalse())
	g.Expect(row(other.ID).VerdictUndelivered).To(BeTrue(), "another review's verdict was stamped")

	var first string
	read := func() string {
		var stamped string
		q := "SELECT delivered FROM decisions WHERE round_id = ?"
		g.Expect(s.db.QueryRowContext(t.Context(), q, round.ID).Scan(&stamped)).To(Succeed())
		return stamped
	}
	first = read()
	clock()
	g.Expect(s.MarkVerdictDelivered(t.Context(), review.ID, 1)).To(Succeed())
	g.Expect(read()).To(Equal(first), "a stamped verdict keeps its first time")
}

func TestStore_Released(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	stepClock(s)
	project := ensureProject(g, t, s)
	review, first := openReview(g, t, s, project.ID, "docs")

	before := addComment(g, t, s, first.ID, "before the verdict")
	got, err := s.Released(t.Context(), review.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(BeEmpty(), "nothing is released before the verdict")

	g.Expect(s.Decide(t.Context(), first.ID, VerdictChanges, "")).To(Succeed())
	after := addComment(g, t, s, first.ID, "after the verdict")
	second, err := s.OpenRound(t.Context(), review.ID, Request{Note: "again", Files: []File{{Path: "docs"}}})
	g.Expect(err).NotTo(HaveOccurred())
	waiting := addComment(g, t, s, second.ID, "on the waiting round")

	got, err = s.Released(t.Context(), review.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(commentIDs(got)).To(Equal([]int64{before.ID, after.ID}))
	g.Expect(got[0].Late).To(BeFalse())
	g.Expect(got[1].Late).To(BeTrue())
	g.Expect(got[0].Round).To(Equal(1))

	// Abandoning the review releases its undecided round, and everything on
	// it arrived after any batch it could have been part of.
	g.Expect(s.Abandon(t.Context(), review.ID)).To(Succeed())
	got, err = s.Released(t.Context(), review.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(commentIDs(got)).To(Equal([]int64{before.ID, after.ID, waiting.ID}))
	g.Expect(got[2].Late).To(BeTrue())
	g.Expect(got[2].Round).To(Equal(2))
}

func TestStore_Released_sameInstantIsNotLate(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return at }
	project := ensureProject(g, t, s)
	review, round := openReview(g, t, s, project.ID, "docs")
	addComment(g, t, s, round.ID, "written in the same millisecond")
	g.Expect(s.Decide(t.Context(), round.ID, VerdictApprove, "")).To(Succeed())

	got, err := s.Released(t.Context(), review.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(HaveLen(1))
	g.Expect(got[0].Late).To(BeFalse(), "a comment written before the verdict is not labeled as written after it")
}

func TestStore_Reply(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)
	_, round := openReview(g, t, s, project.ID, "docs")
	c := addComment(g, t, s, round.ID, "a point")

	err := s.Reply(t.Context(), c.ID, "done")
	g.Expect(errors.Is(err, ErrNotReleased)).To(BeTrue(), "the agent cannot have seen it")

	g.Expect(s.Decide(t.Context(), round.ID, VerdictChanges, "")).To(Succeed())
	err = s.Reply(t.Context(), c.ID, "  ")
	g.Expect(errors.Is(err, ErrMissing)).To(BeTrue())

	g.Expect(s.Reply(t.Context(), c.ID, "first")).To(Succeed())
	g.Expect(s.Reply(t.Context(), c.ID, "  second  ")).To(Succeed())
	g.Expect(readComment(g, t, s, c.ID).Reply).To(Equal("second"))
	var replied string
	row := s.db.QueryRowContext(t.Context(), "SELECT replied FROM comments WHERE id = ?", c.ID)
	g.Expect(row.Scan(&replied)).To(Succeed())
	g.Expect(replied).NotTo(BeEmpty())

	err = s.Reply(t.Context(), 9999, "x")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestStore_Reply_onAnAbandonedReview(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)
	review, round := openReview(g, t, s, project.ID, "docs")
	c := addComment(g, t, s, round.ID, "a point")
	g.Expect(s.Abandon(t.Context(), review.ID)).To(Succeed())
	g.Expect(s.Reply(t.Context(), c.ID, "noted")).To(Succeed())
}

func TestStore_ReviewsIn(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	stepClock(s)
	project := ensureProject(g, t, s)
	other, err := s.EnsureProject(t.Context(), "/tmp/elsewhere", "")
	g.Expect(err).NotTo(HaveOccurred())

	mine, _ := openReview(g, t, s, project.ID, "docs")
	nr := newReview(project.ID, "web")
	nr.Agent = "agent-2"
	theirs, _, err := s.OpenReview(t.Context(), nr)
	g.Expect(err).NotTo(HaveOccurred())
	openReview(g, t, s, other.ID, "docs")

	got, err := s.ReviewsIn(t.Context(), project.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ids(got)).To(Equal([]int64{theirs.ID, mine.ID}), "newest request first")
	g.Expect(got[0].Agent).To(Equal("agent-2"))
}

// stepClock replaces the store's clock with one that moves a second every time
// it is read, so the order of writes in a test is the order of their times. It
// returns a function that advances it once more.
func stepClock(s *Store) func() time.Time {
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time {
		at = at.Add(time.Second)
		return at
	}
	return s.now
}

// addComment writes a file-level comment on a round and fails the test if it
// cannot.
func addComment(g *WithT, t *testing.T, s *Store, roundID int64, body string) Comment {
	g.THelper()
	c, err := s.AddComment(t.Context(), NewComment{RoundID: roundID, Level: LevelFile, Path: "docs/a.md", Body: body})
	g.Expect(err).NotTo(HaveOccurred())
	return c
}

// commentIDs lists the ids of released comments in order.
func commentIDs(in []ReleasedComment) []int64 {
	out := make([]int64, 0, len(in))
	for _, c := range in {
		out = append(out, c.ID)
	}
	return out
}
