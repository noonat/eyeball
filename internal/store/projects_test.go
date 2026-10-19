package store

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestStore_MergeProjects(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	main, err := s.EnsureProject(t.Context(), "/tmp/harbor", "harbor")
	g.Expect(err).NotTo(HaveOccurred())
	worktree, err := s.EnsureProject(t.Context(), "/tmp/harbor-retry", "harbor")
	g.Expect(err).NotTo(HaveOccurred())
	both := []Project{main, worktree}
	for _, p := range both {
		_, round := openReview(g, t, s, p.ID, "docs/a.md")
		addComment(g, t, s, round.ID, "a point")
	}
	counts := func() []int {
		var reviews, rounds, comments int
		row := s.db.QueryRowContext(t.Context(), `SELECT
			(SELECT count(*) FROM reviews),
			(SELECT count(*) FROM rounds),
			(SELECT count(*) FROM comments)`)
		g.Expect(row.Scan(&reviews, &rounds, &comments)).To(Succeed())
		return []int{reviews, rounds, comments}
	}
	before := counts()

	g.Expect(s.MergeProjects(t.Context(), worktree.ID, main.ID)).To(Succeed())

	// Deleting a project cascades to its reviews, so a merge that deleted
	// before it moved would lose them here.
	g.Expect(counts()).To(Equal(before))
	rows, err := s.ReviewsIn(t.Context(), main.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(rows).To(HaveLen(2))
	copies, err := s.WorkingCopies(t.Context(), main.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(copies).To(HaveLen(2))
	_, err = s.Project(t.Context(), worktree.ID)
	g.Expect(err).To(MatchError(ErrNotFound))
}

func TestStore_MergeProjects_mainCheckoutBecomesTheRoot(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	worktree, err := s.EnsureProject(t.Context(), "/tmp/harbor-retry", "harbor")
	g.Expect(err).NotTo(HaveOccurred())
	main, err := s.EnsureProject(t.Context(), "/tmp/harbor", "harbor")
	g.Expect(err).NotTo(HaveOccurred())
	mainCopy, err := s.EnsureWorkingCopy(t.Context(), main.ID, main.Root)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(s.MarkMain(t.Context(), mainCopy.ID)).To(Succeed())

	g.Expect(s.MergeProjects(t.Context(), main.ID, worktree.ID)).To(Succeed())

	merged, err := s.Project(t.Context(), worktree.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(merged.Root).To(Equal(main.Root))
}

func TestStore_SetProjectRoot(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	p := ensureProject(g, t, s)
	other, err := s.EnsureProject(t.Context(), "/tmp/other", "other")
	g.Expect(err).NotTo(HaveOccurred())
	worktree, err := s.EnsureWorkingCopy(t.Context(), p.ID, "/tmp/project-retry")
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(s.SetProjectRoot(t.Context(), p.ID, worktree.Root)).To(Succeed())
	moved, err := s.Project(t.Context(), p.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(moved.Root).To(Equal(worktree.Root))

	g.Expect(s.SetProjectRoot(t.Context(), p.ID, other.Root)).To(MatchError(ErrWrongProject))
	g.Expect(s.SetProjectRoot(t.Context(), p.ID, "/tmp/nowhere")).To(MatchError(ErrNotFound))
}

func TestStore_SetRepository(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	p := ensureProject(g, t, s)

	g.Expect(s.SetRepository(t.Context(), p.ID, "/tmp/project/.git")).To(Succeed())
	g.Expect(s.SetRepository(t.Context(), p.ID, "/tmp/elsewhere/.git")).To(Succeed(), "a project keeps the repository it was first given")
	found, err := s.ProjectByRepository(t.Context(), "/tmp/project/.git")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found.ID).To(Equal(p.ID))
	g.Expect(found.Repository).To(Equal("/tmp/project/.git"))

	_, err = s.ProjectByRepository(t.Context(), "/tmp/elsewhere/.git")
	g.Expect(err).To(MatchError(ErrNotFound))
	pending, err := s.ProjectsWithoutRepository(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(pending).To(BeEmpty())

	// A second project naming the same repository is refused, so the server
	// can merge it into the one that holds it.
	other, err := s.EnsureProject(t.Context(), "/tmp/other", "other")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(s.SetRepository(t.Context(), other.ID, "/tmp/project/.git")).To(MatchError(ErrRepositoryTaken))
}
