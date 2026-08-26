package store

import (
	"strconv"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"
)

func TestStore_Comments(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)
	_, round := openReview(g, t, s, project.ID, "docs")

	// Written out of order on purpose, so the order that comes back is the
	// query's and not the order they were added in.
	written := []NewComment{
		{
			Level: LevelReview,
			Body:  "the approach is wrong",
		},
		{
			Level:     LevelLine,
			Path:      "docs/z.md",
			StartLine: 3,
			EndLine:   3,
			Quoted:    "q",
			Body:      "on z",
		},
		{
			Level:     LevelLine,
			Path:      "docs/a.md",
			StartLine: 40,
			EndLine:   41,
			Quoted:    "q",
			Body:      "late in a",
		},
		{
			Level: LevelFile,
			Path:  "docs/a.md",
			Body:  "about a",
		},
		{
			Level:     LevelLine,
			Path:      "docs/a.md",
			StartLine: 7,
			EndLine:   7,
			Quoted:    "q",
			Body:      "early in a",
		},
	}
	for _, nc := range written {
		nc.RoundID = round.ID
		_, err := s.AddComment(t.Context(), nc)
		g.Expect(err).NotTo(HaveOccurred())
	}

	got, err := s.Comments(t.Context(), round.ID)
	g.Expect(err).NotTo(HaveOccurred())
	bodies := make([]string, 0, len(got))
	for _, c := range got {
		bodies = append(bodies, c.Body)
	}
	// By path, then by where in the file it sits. A file-level comment has no
	// line, so it comes before the lines of that file. The review-level comment
	// has no path at all and comes last.
	g.Expect(bodies).To(Equal([]string{
		"about a",
		"early in a",
		"late in a",
		"on z",
		"the approach is wrong",
	}))
	g.Expect(got[0].Level).To(Equal(LevelFile))
	g.Expect(got[4].Path).To(BeEmpty())
}

func TestStore_Queue(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)

	waiting, waitingRound, err := s.OpenReview(t.Context(), NewReview{
		ProjectID: project.ID,
		Agent:     "agent-1",
		Title:     "still waiting",
		Paths:     []string{"docs"},
		Request: Request{
			Note: "look at this",
			Files: []File{
				{
					Path:   "docs/a.md",
					Digest: digest('a'),
					Size:   1,
				},
				{
					Path:   "docs/b.md",
					Digest: digest('b'),
					Size:   2,
				},
				{
					Path:   "Makefile",
					Digest: digest('c'),
					Size:   3,
				},
			},
		},
	})
	g.Expect(err).NotTo(HaveOccurred())

	working, workingRound := openReview(g, t, s, project.ID, "internal")
	g.Expect(s.Decide(t.Context(), workingRound.ID, VerdictChanges, "again")).To(Succeed())

	settled, settledRound := openReview(g, t, s, project.ID, "web")
	g.Expect(s.Decide(t.Context(), settledRound.ID, VerdictApprove, "")).To(Succeed())

	gone, _ := openReview(g, t, s, project.ID, "tool")
	g.Expect(s.Abandon(t.Context(), gone.ID)).To(Succeed())

	queue, err := s.Queue(t.Context(), 10)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ids(queue.Waiting)).To(Equal([]int64{waiting.ID}))
	g.Expect(ids(queue.Working)).To(Equal([]int64{working.ID}))
	g.Expect(ids(queue.Settled)).To(Equal([]int64{settled.ID}))

	row := queue.Waiting[0]
	g.Expect(row.ProjectID).To(Equal(project.ID))
	g.Expect(row.ProjectName).To(Equal("project"))
	g.Expect(row.Agent).To(Equal("agent-1"))
	g.Expect(row.Title).To(Equal("still waiting"))
	g.Expect(row.State).To(Equal(StateOpen))
	g.Expect(row.Round).To(Equal(waitingRound.Number))
	// Makefile was captured for context, so it is not the size of the change.
	g.Expect(row.Files).To(Equal(2))
	g.Expect(row.RequestedAt).To(BeTemporally("~", time.Now().UTC(), time.Minute))

	// The abandoned review is in none of the three: the question was withdrawn,
	// so there is nothing waiting, nothing being acted on and nothing decided.
	for _, id := range append(append(ids(queue.Waiting), ids(queue.Working)...), ids(queue.Settled)...) {
		g.Expect(id).NotTo(Equal(gone.ID))
	}
}

func TestStore_Queue_afterASecondRound(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)
	review, first := openReview(g, t, s, project.ID, "docs")
	g.Expect(s.Decide(t.Context(), first.ID, VerdictChanges, "again")).To(Succeed())

	second, err := s.OpenRound(t.Context(), review.ID, Request{
		Note: "did what the last round asked",
		Files: []File{
			{
				Path:   "docs/a.md",
				Digest: digest('a'),
				Size:   1,
			},
			{
				Path:   "docs/b.md",
				Digest: digest('b'),
				Size:   2,
			},
		},
	})
	g.Expect(err).NotTo(HaveOccurred())

	queue, err := s.Queue(t.Context(), 10)
	g.Expect(err).NotTo(HaveOccurred())

	// Round 1 is decided and round 2 is waiting, so the review is waiting. A
	// listing that joined every round rather than the latest one would put it
	// in both groups at once, and report round 1's numbers in the row.
	g.Expect(ids(queue.Waiting)).To(Equal([]int64{review.ID}))
	g.Expect(queue.Working).To(BeEmpty())
	g.Expect(queue.Waiting[0].Round).To(Equal(second.Number))
	g.Expect(queue.Waiting[0].Files).To(Equal(2))

	// The agent's own slice reads through the same join, so it lists the review
	// once rather than once per round.
	mine, err := s.ReviewsFor(t.Context(), project.ID, "agent-1")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ids(mine)).To(Equal([]int64{review.ID}))
	g.Expect(mine[0].Round).To(Equal(2))
}

func TestStore_Queue_ordering(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	// A clock the test moves by hand, so the order is the query's rather than
	// whatever the machine managed between two inserts.
	at := time.Date(2026, 8, 26, 4, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return at }
	project := ensureProject(g, t, s)

	var (
		waiting []int64
		working []int64
		settled []int64
	)
	for i := range 3 {
		at = at.Add(time.Hour)
		review, _ := openReview(g, t, s, project.ID, "waiting"+strconv.Itoa(i))
		waiting = append(waiting, review.ID)
	}
	for i := range 2 {
		at = at.Add(time.Hour)
		review, round := openReview(g, t, s, project.ID, "working"+strconv.Itoa(i))
		at = at.Add(time.Minute)
		g.Expect(s.Decide(t.Context(), round.ID, VerdictChanges, "again")).To(Succeed())
		working = append(working, review.ID)
	}
	for i := range 4 {
		at = at.Add(time.Hour)
		review, round := openReview(g, t, s, project.ID, "settled"+strconv.Itoa(i))
		at = at.Add(time.Minute)
		g.Expect(s.Decide(t.Context(), round.ID, VerdictApprove, "")).To(Succeed())
		settled = append(settled, review.ID)
	}

	queue, err := s.Queue(t.Context(), 2)
	g.Expect(err).NotTo(HaveOccurred())

	// Longest wait first, because that is the one holding an agent up.
	g.Expect(ids(queue.Waiting)).To(Equal(waiting))

	// Most recently answered first, which is the one the agent is acting on.
	g.Expect(ids(queue.Working)).To(Equal([]int64{working[1], working[0]}))

	// Newest first and cut to the limit, because the point of the tail is
	// finding a verdict given an hour ago without a search.
	g.Expect(ids(queue.Settled)).To(Equal([]int64{settled[3], settled[2]}))
}

func TestStore_Review(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)
	opened, first := openReview(g, t, s, project.ID, "docs")

	got, err := s.Review(t.Context(), opened.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.ID).To(Equal(opened.ID))
	g.Expect(got.ProjectID).To(Equal(project.ID))
	g.Expect(got.Agent).To(Equal("agent-1"))
	g.Expect(got.State).To(Equal(StateOpen))
	g.Expect(got.Paths).To(Equal([]string{"docs"}))
	g.Expect(got.LatestRound).To(Equal(1))

	// The latest round is what a capture diffs against, so it has to move.
	g.Expect(s.Decide(t.Context(), first.ID, VerdictChanges, "again")).To(Succeed())
	_, err = s.OpenRound(t.Context(), opened.ID, Request{Note: "did it"})
	g.Expect(err).NotTo(HaveOccurred())

	got, err = s.Review(t.Context(), opened.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.LatestRound).To(Equal(2))

	_, err = s.Review(t.Context(), 9999)
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestStore_ReviewsFor(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	mine := ensureProject(g, t, s)
	other, err := s.EnsureProject(t.Context(), "/tmp/other", "other")
	g.Expect(err).NotTo(HaveOccurred())

	first, _ := openReview(g, t, s, mine.ID, "docs")
	second, _ := openReview(g, t, s, mine.ID, "internal")
	g.Expect(s.Abandon(t.Context(), second.ID)).To(Succeed())

	// Another agent in the same project, and this agent in another project.
	elsewhere, _, err := s.OpenReview(t.Context(), NewReview{
		ProjectID: mine.ID,
		Agent:     "agent-2",
		Title:     "not mine",
		Paths:     []string{"web"},
		Request:   Request{Note: "n"},
	})
	g.Expect(err).NotTo(HaveOccurred())
	elsewhereProject, _, err := s.OpenReview(t.Context(), NewReview{
		ProjectID: other.ID,
		Agent:     "agent-1",
		Title:     "another project",
		Paths:     []string{"docs"},
		Request:   Request{Note: "n"},
	})
	g.Expect(err).NotTo(HaveOccurred())

	got, err := s.ReviewsFor(t.Context(), mine.ID, "agent-1")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ids(got)).To(ConsistOf(first.ID, second.ID))
	g.Expect(ids(got)).NotTo(ContainElement(elsewhere.ID))
	g.Expect(ids(got)).NotTo(ContainElement(elsewhereProject.ID))

	// An agent's own slice keeps abandoned reviews, unlike the queue: the agent
	// is the one that withdrew the question and eyeball status says so.
	states := map[int64]State{}
	for _, row := range got {
		states[row.ReviewID] = row.State
	}
	g.Expect(states[second.ID]).To(Equal(StateAbandoned))
}

func TestStore_Round(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)

	review, first, err := s.OpenReview(t.Context(), NewReview{
		ProjectID: project.ID,
		Agent:     "agent-1",
		Title:     "a title",
		Paths:     []string{"docs"},
		Request:   Request{Note: "look at this", BaseCommit: "abc123"},
	})
	g.Expect(err).NotTo(HaveOccurred())

	got, err := s.Round(t.Context(), review.ID, 1)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal(first))

	_, err = s.Round(t.Context(), review.ID, 2)
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestStore_RoundFiles(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)

	_, round, err := s.OpenReview(t.Context(), NewReview{
		ProjectID: project.ID,
		Agent:     "agent-1",
		Title:     "a title",
		Paths:     []string{"docs"},
		Request: Request{
			Note: "look at this",
			Files: []File{
				{
					Path:   "Makefile",
					Digest: digest('c'),
					Size:   3,
				},
				{
					Path:   "docs/gone.md",
					Digest: "",
					Size:   0,
				},
				{
					Path:   "docs/a.md",
					Digest: digest('a'),
					Size:   1,
				},
			},
		},
	})
	g.Expect(err).NotTo(HaveOccurred())

	got, err := s.RoundFiles(t.Context(), round.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]RoundFile{
		{
			File: File{
				Path:   "Makefile",
				Digest: digest('c'),
				Size:   3,
			},
			InScope: false,
		},
		{
			File: File{
				Path:   "docs/a.md",
				Digest: digest('a'),
				Size:   1,
			},
			InScope: true,
		},
		{
			File: File{
				Path:   "docs/gone.md",
				Digest: "",
				Size:   0,
			},
			InScope: true,
		},
	}))
}

// ids lists the review ids of a listing, which is what an ordering assertion is
// about.
func ids(rows []ReviewRow) []int64 {
	out := make([]int64, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ReviewID)
	}
	return out
}
