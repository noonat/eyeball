package server

import (
	"path/filepath"
	"testing"

	"github.com/noonat/eyeball/internal/store"
	. "github.com/onsi/gomega"
)

// Projects written before projects had repositories: two repositories' main
// checkouts and worktrees, and a root that is gone. Each main checkout and its
// worktree merge into the main checkout's project. The gone one cannot be
// asked, and stays as it was.
func TestServer_FillRepositories(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	g.Expect(err).NotTo(HaveOccurred())
	main := gitRepo(g, filepath.Join(dir, "harbor"))
	worktree := filepath.Join(dir, "harbor-retry")
	git(g, main, "worktree", "add", "--quiet", "-b", "retry", worktree)
	dock := gitRepo(g, filepath.Join(dir, "dock"))
	dockWorktree := filepath.Join(dir, "dock-retry")
	git(g, dock, "worktree", "add", "--quiet", "-b", "retry", dockWorktree)

	// Projects are filled in the order they were written. harbor's worktree
	// comes first, so its main checkout merges the worktree in. dock's main
	// checkout comes first, so its worktree merges into it. Either way the
	// main checkout's project is the one kept.
	var ids []int64
	roots := []string{worktree, main, filepath.Join(dir, "gone"), dock, dockWorktree}
	for _, root := range roots {
		p, err := f.store.EnsureProject(t.Context(), root, "harbor")
		g.Expect(err).NotTo(HaveOccurred())
		_, _, err = f.store.OpenReview(t.Context(), store.NewReview{
			ProjectID: p.ID,
			Agent:     "a",
			Title:     "work in " + filepath.Base(root),
			Paths:     []string{"a.txt"},
			Request:   store.Request{Note: "n", Files: []store.File{{Path: "a.txt"}}},
		})
		g.Expect(err).NotTo(HaveOccurred())
		ids = append(ids, p.ID)
	}
	fromWorktree, fromMain, gone, fromDock, fromDockWorktree := ids[0], ids[1], ids[2], ids[3], ids[4]

	f.s.FillRepositories(t.Context())

	project, err := f.store.Project(t.Context(), fromMain)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(project.Root).To(Equal(main))
	g.Expect(project.Repository).To(Equal(filepath.Join(main, ".git")))
	rows, err := f.store.ReviewsIn(t.Context(), fromMain)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(rows).To(HaveLen(2), "the worktree's review moved into the main checkout's project")
	_, err = f.store.Project(t.Context(), fromWorktree)
	g.Expect(err).To(MatchError(store.ErrNotFound))
	copies, err := f.store.WorkingCopies(t.Context(), fromMain)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(copies).To(HaveLen(2))
	g.Expect(copies[0].Root).To(Equal(main))
	g.Expect(copies[0].Main).To(BeTrue())

	dockRows, err := f.store.ReviewsIn(t.Context(), fromDock)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(dockRows).To(HaveLen(2))
	_, err = f.store.Project(t.Context(), fromDockWorktree)
	g.Expect(err).To(MatchError(store.ErrNotFound))

	left, err := f.store.Project(t.Context(), gone)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(left.Repository).To(BeEmpty())
}
