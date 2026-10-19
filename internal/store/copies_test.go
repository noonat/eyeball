package store

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestStore_EnsureWorkingCopy(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	p := ensureProject(g, t, s)

	first, err := s.EnsureWorkingCopy(t.Context(), p.ID, "/tmp/project-retry")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(first.ProjectID).To(Equal(p.ID))
	g.Expect(first.Main).To(BeFalse())

	again, err := s.EnsureWorkingCopy(t.Context(), p.ID, "/tmp/project-retry/")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(again).To(Equal(first), "a root is cleaned before it is looked up")

	// A root already known keeps its project, so a directory recreated where a
	// gone project's working copy was is found under that project.
	other, err := s.EnsureProject(t.Context(), "/tmp/other", "other")
	g.Expect(err).NotTo(HaveOccurred())
	kept, err := s.EnsureWorkingCopy(t.Context(), other.ID, "/tmp/project-retry")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(kept.ProjectID).To(Equal(p.ID))

	_, err = s.EnsureWorkingCopy(t.Context(), p.ID, "relative/path")
	g.Expect(err).To(MatchError(ErrInvalidPath))
}

func TestStore_EnsureWorkingCopy_projectRoot(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	p := ensureProject(g, t, s)

	// EnsureProject registers the working copy at its root, so a first round
	// that names no copy has one to record.
	copies, err := s.WorkingCopies(t.Context(), p.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(copies).To(HaveLen(1))
	g.Expect(copies[0].Root).To(Equal(p.Root))
}

func TestStore_MarkMain(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	p := ensureProject(g, t, s)
	c, err := s.EnsureWorkingCopy(t.Context(), p.ID, p.Root)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(s.MarkMain(t.Context(), c.ID)).To(Succeed())
	g.Expect(s.MarkMain(t.Context(), c.ID)).To(Succeed(), "marking twice changes nothing")
	read, err := s.WorkingCopyByID(t.Context(), c.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(read.Main).To(BeTrue())
}

func TestStore_WorkingCopies(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	p := ensureProject(g, t, s)
	worktree, err := s.EnsureWorkingCopy(t.Context(), p.ID, "/tmp/a-worktree")
	g.Expect(err).NotTo(HaveOccurred())
	main, err := s.EnsureWorkingCopy(t.Context(), p.ID, "/tmp/z-main")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(s.MarkMain(t.Context(), main.ID)).To(Succeed())

	copies, err := s.WorkingCopies(t.Context(), p.ID)
	g.Expect(err).NotTo(HaveOccurred())
	roots := make([]string, 0, len(copies))
	for _, c := range copies {
		roots = append(roots, c.Root)
	}
	g.Expect(roots).To(Equal([]string{main.Root, p.Root, worktree.Root}), "the main checkout first, then the order they were seen")
}

func TestStore_WorkingCopyByID(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)

	_, err := s.WorkingCopyByID(t.Context(), 99)
	g.Expect(err).To(MatchError(ErrNotFound))
}
