package store

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"
)

func Test_normalizePath(t *testing.T) {
	paths := []struct {
		name string
		in   string
		want string
		bad  bool
	}{
		{name: "already clean", in: "docs/product.md", want: "docs/product.md"},
		{name: "a directory", in: "docs", want: "docs"},
		{name: "leading dot slash", in: "./docs", want: "docs"},
		{name: "trailing slash", in: "docs/", want: "docs"},
		{name: "doubled separator", in: "docs//product.md", want: "docs/product.md"},
		{name: "an interior dot dot that stays inside", in: "docs/x/../product.md", want: "docs/product.md"},
		{name: "empty", in: "", bad: true},
		{name: "only spaces", in: "   ", bad: true},
		{name: "absolute", in: "/etc/passwd", bad: true},
		{name: "the root itself", in: ".", bad: true},
		{name: "climbing out", in: "../secrets", bad: true},
		{name: "climbing out the long way", in: "docs/../../secrets", bad: true},
	}
	for _, c := range paths {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := normalizePath(c.in)
			if c.bad {
				g.Expect(errors.Is(err, ErrInvalidPath)).To(BeTrue())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(c.want))
		})
	}
}

func Test_pathsOverlap(t *testing.T) {
	pairs := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{
			name: "the same path",
			a:    "docs",
			b:    "docs",
			want: true,
		},
		{
			name: "a directory and a file under it",
			a:    "docs",
			b:    "docs/product.md",
			want: true,
		},
		{
			name: "a file under a directory, the other way round",
			a:    "docs/product.md",
			b:    "docs",
			want: true,
		},
		{
			name: "two levels down",
			a:    "internal",
			b:    "internal/store/store.go",
			want: true,
		},
		// The segment boundary is the whole point: a prefix match on the string
		// alone would make docs claim docsite.
		{
			name: "a name that only starts the same",
			a:    "docs",
			b:    "docsite",
			want: false,
		},
		{
			name: "a longer name that only starts the same",
			a:    "docs/a",
			b:    "docs/ab",
			want: false,
		},
		{
			name: "unrelated",
			a:    "docs",
			b:    "internal",
			want: false,
		},
	}
	for _, c := range pairs {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(pathsOverlap(c.a, c.b)).To(Equal(c.want))
		})
	}
}

func TestStore_Abandon(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)
	review, _ := openReview(g, t, s, project.ID, "docs")

	// Abandoning works with the round still waiting, which is the case an agent
	// hits when it decides the question no longer needs an answer.
	g.Expect(s.Abandon(t.Context(), review.ID)).To(Succeed())
	g.Expect(stateOf(g, t, s, review.ID)).To(Equal(StateAbandoned))

	err := s.Abandon(t.Context(), review.ID)
	g.Expect(errors.Is(err, ErrNotOpen)).To(BeTrue())

	err = s.Abandon(t.Context(), 9999)
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestStore_EnsureProject(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)

	first, err := s.EnsureProject(t.Context(), "/tmp/project", "web")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(first.ID).NotTo(BeZero())
	g.Expect(first.Root).To(Equal("/tmp/project"))
	g.Expect(first.Name).To(Equal("web"))

	// The root is the identity, so seeing it again returns the same row rather
	// than a second project.
	again, err := s.EnsureProject(t.Context(), "/tmp/project/", "renamed")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(again.ID).To(Equal(first.ID))
	g.Expect(again.Name).To(Equal("web"))

	// An empty name falls back to the root's last element.
	defaulted, err := s.EnsureProject(t.Context(), "/tmp/other", "")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(defaulted.Name).To(Equal("other"))

	_, err = s.EnsureProject(t.Context(), "relative/path", "x")
	g.Expect(errors.Is(err, ErrInvalidPath)).To(BeTrue())
}

func TestStore_OpenReview(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)

	review, round, err := s.OpenReview(t.Context(), NewReview{
		ProjectID: project.ID,
		Agent:     "  agent-1  ",
		Title:     "the store",
		Paths:     []string{"./docs/", "internal/store"},
		Request: Request{
			Note:       "look at the schema",
			BaseCommit: "abc123",
			Files: []File{
				{Path: "docs/product.md", Digest: digest('a'), Size: 12},
				{Path: "./internal/store/store.go", Digest: digest('b'), Size: 40},
				{Path: "Makefile", Digest: digest('c'), Size: 9},
				{Path: "docs/gone.md", Digest: "", Size: 0},
			},
		},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(review.Agent).To(Equal("agent-1"))
	g.Expect(review.State).To(Equal(StateOpen))
	g.Expect(review.Paths).To(Equal([]string{"docs", "internal/store"}))
	g.Expect(round.Number).To(Equal(1))
	g.Expect(round.ReviewID).To(Equal(review.ID))
	g.Expect(round.BaseCommit).To(Equal("abc123"))

	// The store decides what is in scope from the review's own paths, and
	// normalizes each captured path on the way in.
	scope := map[string]bool{}
	rows, err := s.db.QueryContext(t.Context(), "SELECT path, in_scope FROM round_files WHERE round_id = ?", round.ID)
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			p  string
			in bool
		)
		g.Expect(rows.Scan(&p, &in)).To(Succeed())
		scope[p] = in
	}
	g.Expect(rows.Err()).NotTo(HaveOccurred())
	g.Expect(scope).To(Equal(map[string]bool{
		"docs/product.md":         true,
		"internal/store/store.go": true,
		"Makefile":                false,
		"docs/gone.md":            true,
	}))
}

func TestStore_OpenReview_concurrentClaims(t *testing.T) {
	g := NewWithT(t)
	// The pool is deliberately wider than one connection, so what serializes
	// these callers is _txlock=immediate rather than database/sql handing out
	// the only connection it has. Under a deferred transaction two of them read
	// no claim and both insert, and the refusal never fires.
	s := openWideStore(g, t)
	project := ensureProject(g, t, s)

	const callers = 8
	var wg sync.WaitGroup
	reviews := make([]Review, callers)
	errs := make([]error, callers)
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reviews[i], _, errs[i] = s.OpenReview(t.Context(), NewReview{
				ProjectID: project.ID,
				Agent:     "agent-1",
				Title:     "a claim on docs",
				Paths:     []string{"docs"},
				Request:   Request{Note: "look at this"},
			})
		}()
	}
	wg.Wait()

	var (
		won    int
		winner Review
	)
	for i := range callers {
		if errs[i] == nil {
			won++
			winner = reviews[i]
		}
	}
	g.Expect(won).To(Equal(1))
	for i := range callers {
		if errs[i] == nil {
			continue
		}
		g.Expect(errors.Is(errs[i], ErrPathClaimed)).To(BeTrue())
		g.Expect(errs[i].Error()).To(ContainSubstring("a claim on docs"))
	}

	// One review, one claim, one round: the losers rolled back whole.
	var reviewCount, pathCount, roundCount int
	row := s.db.QueryRowContext(t.Context(), "SELECT (SELECT count(*) FROM reviews), (SELECT count(*) FROM review_paths), (SELECT count(*) FROM rounds)")
	g.Expect(row.Scan(&reviewCount, &pathCount, &roundCount)).To(Succeed())
	g.Expect(reviewCount).To(Equal(1))
	g.Expect(pathCount).To(Equal(1))
	g.Expect(roundCount).To(Equal(1))
	g.Expect(winner.ID).NotTo(BeZero())
}

func TestStore_OpenReview_refusals(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)
	held, _ := openReview(g, t, s, project.ID, "docs")

	// A path the open review covers, at a segment boundary in both directions.
	claimed := []string{"docs", "docs/product.md", "docs/a/b/c.md"}
	for _, p := range claimed {
		t.Run("refuses "+p, func(t *testing.T) {
			g := NewWithT(t)
			_, _, err := s.OpenReview(t.Context(), newReview(project.ID, p))
			g.Expect(errors.Is(err, ErrPathClaimed)).To(BeTrue())
			g.Expect(err.Error()).To(ContainSubstring("docs"))
		})
	}

	// A name that merely starts the same is a different path.
	free := []string{"docsite", "internal/store", "docs.md"}
	for _, p := range free {
		t.Run("allows "+p, func(t *testing.T) {
			g := NewWithT(t)
			s := openStore(g, t)
			project := ensureProject(g, t, s)
			openReview(g, t, s, project.ID, "docs")
			_, _, err := s.OpenReview(t.Context(), newReview(project.ID, p))
			g.Expect(err).NotTo(HaveOccurred())
		})
	}

	// A review that has ended releases its paths.
	t.Run("a path is free once the review holding it is abandoned", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(s.Abandon(t.Context(), held.ID)).To(Succeed())
		_, _, err := s.OpenReview(t.Context(), newReview(project.ID, "docs/product.md"))
		g.Expect(err).NotTo(HaveOccurred())
	})

	// Each row names the refusal it must trip. Every ProjectID comes from a
	// project that exists, because a made-up one fails the foreign key and
	// would let any of these pass for the wrong reason.
	bad := []struct {
		name string
		nr   NewReview
		want string
	}{
		{
			name: "no agent",
			nr: NewReview{
				Title:   "t",
				Paths:   []string{"docs"},
				Request: Request{Note: "n"},
			},
			want: "needs an agent",
		},
		{
			name: "no title",
			nr: NewReview{
				Agent:   "a",
				Paths:   []string{"docs"},
				Request: Request{Note: "n"},
			},
			want: "needs a title",
		},
		{
			name: "no paths",
			nr: NewReview{
				Agent:   "a",
				Title:   "t",
				Request: Request{Note: "n"},
			},
			want: "at least one path",
		},
		{
			name: "no note",
			nr: NewReview{
				Agent: "a",
				Title: "t",
				Paths: []string{"docs"},
			},
			want: "needs a note",
		},
		{
			name: "a path that climbs out",
			nr: NewReview{
				Agent:   "a",
				Title:   "t",
				Paths:   []string{"../etc"},
				Request: Request{Note: "n"},
			},
			want: "leaves the project root",
		},
		{
			name: "two paths covering each other",
			nr: NewReview{
				Agent:   "a",
				Title:   "t",
				Paths:   []string{"docs", "docs/product.md"},
				Request: Request{Note: "n"},
			},
			want: "cover the same files",
		},
		{
			name: "one file captured twice under different spellings",
			nr: NewReview{
				Agent: "a",
				Title: "t",
				Paths: []string{"docs"},
				Request: Request{
					Note: "n",
					Files: []File{
						{
							Path:   "docs/a.md",
							Digest: digest('a'),
							Size:   1,
						},
						{
							Path:   "./docs/a.md",
							Digest: digest('a'),
							Size:   1,
						},
					},
				},
			},
			want: "captured twice",
		},
		{
			name: "a file of negative size",
			nr: NewReview{
				Agent: "a",
				Title: "t",
				Paths: []string{"docs"},
				Request: Request{
					Note: "n",
					Files: []File{
						{
							Path:   "docs/a.md",
							Digest: digest('a'),
							Size:   -1,
						},
					},
				},
			},
			want: "cannot have a size of",
		},
		{
			name: "a deleted file carrying a size",
			nr: NewReview{
				Agent: "a",
				Title: "t",
				Paths: []string{"docs"},
				Request: Request{
					Note: "n",
					Files: []File{
						{
							Path:   "docs/a.md",
							Digest: "",
							Size:   12,
						},
					},
				},
			},
			want: "no content to store",
		},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			fresh := openStore(g, t)
			project := ensureProject(g, t, fresh)
			c.nr.ProjectID = project.ID

			_, _, err := fresh.OpenReview(t.Context(), c.nr)
			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring(c.want))
		})
	}
}

func TestStore_OpenRound(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)
	review, first := openReview(g, t, s, project.ID, "docs")
	g.Expect(s.Decide(t.Context(), first.ID, VerdictChanges, "again")).To(Succeed())

	second, err := s.OpenRound(t.Context(), review.ID, Request{
		Note:       "did what the last round asked",
		BaseCommit: first.BaseCommit,
		Files:      []File{{Path: "docs/product.md", Digest: digest('a'), Size: 20}},
		Changes:    []Change{{Path: "docs/product.md", Added: 4, Removed: 1}},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(second.Number).To(Equal(2))
	g.Expect(second.ID).NotTo(Equal(first.ID))
	g.Expect(second.Files).To(Equal(1))
	g.Expect(second.Added).To(Equal(4))
	g.Expect(second.Removed).To(Equal(1))

	// The second round's files are marked against the review's paths, which the
	// store reads back rather than being told again.
	var inScope bool
	row := s.db.QueryRowContext(t.Context(), "SELECT in_scope FROM round_files WHERE round_id = ?", second.ID)
	g.Expect(row.Scan(&inScope)).To(Succeed())
	g.Expect(inScope).To(BeTrue())
}

func TestStore_OpenRound_baseIsFixedAtTheFirstRound(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)
	review, first := openReview(g, t, s, project.ID, "docs")
	g.Expect(s.Decide(t.Context(), first.ID, VerdictChanges, "again")).To(Succeed())

	// A base that followed HEAD would make the review's own total mean a
	// different thing on every round, and an agent that had committed its work
	// would measure from it and capture nothing.
	_, err := s.OpenRound(t.Context(), review.ID, Request{
		Note:       "measured from somewhere else",
		BaseCommit: "1111111111111111111111111111111111111111",
	})
	g.Expect(errors.Is(err, ErrBaseMoved)).To(BeTrue())
	g.Expect(err.Error()).To(ContainSubstring("1111111111111111111111111111111111111111"))
}

func TestStore_OpenRound_changeRefusals(t *testing.T) {
	requests := []struct {
		name    string
		changes []Change
		want    string
	}{
		{
			name:    "one path moving twice",
			changes: []Change{{Path: "docs/a.md"}, {Path: "docs/a.md"}},
			want:    "moved twice",
		},
		{
			name:    "a path that climbs out",
			changes: []Change{{Path: "../elsewhere.md"}},
			want:    "invalid path",
		},
		{
			name:    "lines counted below zero",
			changes: []Change{{Path: "docs/a.md", Added: -1}},
			want:    "cannot have moved",
		},
	}
	for _, c := range requests {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			s := openStore(g, t)
			project := ensureProject(g, t, s)
			review, first := openReview(g, t, s, project.ID, "docs")
			g.Expect(s.Decide(t.Context(), first.ID, VerdictChanges, "again")).To(Succeed())

			_, err := s.OpenRound(t.Context(), review.ID, Request{
				Note:       "n",
				BaseCommit: first.BaseCommit,
				Changes:    c.changes,
			})
			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring(c.want))
		})
	}
}

func TestStore_OpenRound_countsAPathOnlyOnceNormalized(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)
	review, first := openReview(g, t, s, project.ID, "docs")
	g.Expect(s.Decide(t.Context(), first.ID, VerdictChanges, "again")).To(Succeed())

	// The scope test compares paths as text, so an unnormalized one would score
	// outside a scope that covers it and the stored size would come out short.
	second, err := s.OpenRound(t.Context(), review.ID, Request{
		Note:       "n",
		BaseCommit: first.BaseCommit,
		Changes:    []Change{{Path: "./docs/a.md", Added: 7, Removed: 2}},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(second.Files).To(Equal(1))
	g.Expect(second.Added).To(Equal(7))
	g.Expect(second.Removed).To(Equal(2))
}

func TestStore_OpenRound_countsARevertedFile(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	project := ensureProject(g, t, s)
	review, first := openReview(g, t, s, project.ID, "docs")
	g.Expect(s.Decide(t.Context(), first.ID, VerdictChanges, "again")).To(Succeed())

	// The file the agent put back matches the base again, so the capture holds
	// nothing for it. It still moved since the last round, which is why the
	// size is not a count of what the round captured.
	second, err := s.OpenRound(t.Context(), review.ID, Request{
		Note:       "put it back",
		BaseCommit: first.BaseCommit,
		Changes:    []Change{{Path: "docs/a.md", Added: 0, Removed: 4}},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(second.Files).To(Equal(1))
	g.Expect(second.Removed).To(Equal(4))

	files, err := s.RoundFiles(t.Context(), second.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(files).To(BeEmpty())
}

func TestStore_OpenRound_refusals(t *testing.T) {
	t.Run("a round that has no decision yet", func(t *testing.T) {
		g := NewWithT(t)
		s := openStore(g, t)
		project := ensureProject(g, t, s)
		review, _ := openReview(g, t, s, project.ID, "docs")
		_, err := s.OpenRound(t.Context(), review.ID, Request{Note: "n"})
		g.Expect(errors.Is(err, ErrAwaitingVerdict)).To(BeTrue())
	})

	// The case that catches asking whether ANY round is decided rather than the
	// latest one: round 1 has a verdict and round 2 does not.
	t.Run("an earlier round decided and the latest one waiting", func(t *testing.T) {
		g := NewWithT(t)
		s := openStore(g, t)
		project := ensureProject(g, t, s)
		review, first := openReview(g, t, s, project.ID, "docs")
		g.Expect(s.Decide(t.Context(), first.ID, VerdictChanges, "again")).To(Succeed())
		second, err := s.OpenRound(t.Context(), review.ID, Request{Note: "round two"})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(second.Number).To(Equal(2))

		_, err = s.OpenRound(t.Context(), review.ID, Request{Note: "round three"})
		g.Expect(errors.Is(err, ErrAwaitingVerdict)).To(BeTrue())
	})

	t.Run("an abandoned review", func(t *testing.T) {
		g := NewWithT(t)
		s := openStore(g, t)
		project := ensureProject(g, t, s)
		review, first := openReview(g, t, s, project.ID, "docs")
		g.Expect(s.Decide(t.Context(), first.ID, VerdictChanges, "again")).To(Succeed())
		g.Expect(s.Abandon(t.Context(), review.ID)).To(Succeed())
		_, err := s.OpenRound(t.Context(), review.ID, Request{Note: "n"})
		g.Expect(errors.Is(err, ErrNotOpen)).To(BeTrue())
	})

	t.Run("a review that does not exist", func(t *testing.T) {
		g := NewWithT(t)
		s := openStore(g, t)
		_, err := s.OpenRound(t.Context(), 9999, Request{Note: "n"})
		g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
	})
}

// ensureProject registers one project for a test to hang reviews off.
func ensureProject(g *WithT, t *testing.T, s *Store) Project {
	g.THelper()
	p, err := s.EnsureProject(t.Context(), "/tmp/project", "project")
	g.Expect(err).NotTo(HaveOccurred())
	return p
}

// newReview is the smallest review that claims one path.
func newReview(projectID int64, claim string) NewReview {
	return NewReview{
		ProjectID: projectID,
		Agent:     "agent-1",
		Title:     "a title",
		Paths:     []string{claim},
		Request:   Request{Note: "look at this"},
	}
}

// openReview opens a review claiming one path and fails the test if it cannot.
func openReview(g *WithT, t *testing.T, s *Store, projectID int64, claim string) (Review, Round) {
	g.THelper()
	review, round, err := s.OpenReview(t.Context(), newReview(projectID, claim))
	g.Expect(err).NotTo(HaveOccurred())
	return review, round
}

// openWideStore opens a migrated store whose pool holds several connections, so
// a concurrency test exercises SQLite's locking rather than database/sql's
// queue for a single connection.
func openWideStore(g *WithT, t *testing.T) *Store {
	g.THelper()
	path := filepath.Join(t.TempDir(), "eyeball.db")
	migrated, err := Open(t.Context(), path)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(migrated.Close()).To(Succeed())

	db, err := sql.Open("sqlite", dsn(path))
	g.Expect(err).NotTo(HaveOccurred())
	db.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = db.Close() })
	return &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// stateOf reads a review's state straight out of the row.
func stateOf(g *WithT, t *testing.T, s *Store, reviewID int64) State {
	g.THelper()
	var state State
	row := s.db.QueryRowContext(t.Context(), "SELECT state FROM reviews WHERE id = ?", reviewID)
	g.Expect(row.Scan(&state)).To(Succeed())
	return state
}
