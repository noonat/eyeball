package capture

import (
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"
)

func TestResolveBase(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	first := r.commit(g, "first")
	r.write(g, "a.txt", "aa\n")
	second := r.commit(g, "second")

	// An empty ref is HEAD, which is what a first round asks for when the
	// agent named nothing.
	head, err := ResolveBase(t.Context(), r.root, "")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(head).To(Equal(second))

	// A named ref resolves to the commit it points at, not to its own
	// spelling, because a branch moves and a base must not.
	named, err := ResolveBase(t.Context(), r.root, "main")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(named).To(Equal(second))

	older, err := ResolveBase(t.Context(), r.root, first)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(older).To(Equal(first))
}

func TestResolveBase_endOfOptions(t *testing.T) {
	g := NewWithT(t)

	// The behavior cannot be tested through ResolveBase: with the peel
	// appended, every flag-shaped ref is refused whether or not the separator
	// is there, so a test through the front door would pass with it removed.
	// What is worth guarding is that it is still in the argv, ahead of the ref.
	args := baseArgs("main")
	g.Expect(args).To(ContainElement("--end-of-options"))
	g.Expect(args[len(args)-1]).To(Equal("main^{commit}"))
	g.Expect(args[len(args)-2]).To(Equal("--end-of-options"))
}

func TestResolveBase_noGit(t *testing.T) {
	g := NewWithT(t)

	// No repository means no base, which is a fact rather than a failure. The
	// capture walks the review's paths instead.
	base, err := ResolveBase(t.Context(), t.TempDir(), "")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(base).To(BeEmpty())
}

func TestResolveBase_refusedRefs(t *testing.T) {
	refs := []struct {
		name string
		// ref is what the agent named. A percent sign stands in for the
		// repository's tree id, which is only known once it is built.
		ref string
	}{
		{
			name: "a path in the working tree",
			ref:  "a.txt",
		},
		{
			name: "a tree rather than a commit",
			ref:  "%",
		},
		{
			name: "a ref that does not exist",
			ref:  "no-such-branch",
		},
		{
			name: "a ref spelled like a flag",
			ref:  "--git-dir=/tmp/elsewhere",
		},
		{
			name: "an empty tree id",
			ref:  "4b825dc642cb6eb9a060e54bf8d69288fbee4904",
		},
	}
	for _, c := range refs {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			r := newRepo(g, t.TempDir())
			r.write(g, "a.txt", "a\n")
			r.commit(g, "base")
			ref := c.ref
			if ref == "%" {
				ref = r.git(g, "rev-parse", "HEAD^{tree}")
			}

			_, err := ResolveBase(t.Context(), r.root, ref)
			g.Expect(errors.Is(err, ErrBadBase)).To(BeTrue())
			g.Expect(err.Error()).To(ContainSubstring(ref))
		})
	}
}

func TestResolveBase_unbornBranch(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	r.git(g, "add", "-A")

	// A repository with no commit has nothing to measure against, so the
	// answer is the same empty base a project without git gets.
	base, err := ResolveBase(t.Context(), r.root, "")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(base).To(BeEmpty())
}

func TestResolveBase_unbornBranchInARepositoryWithCommits(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	r.commit(g, "base")
	r.git(g, "checkout", "--quiet", "--orphan", "fresh")

	// The repository has a branch and a commit, so it is not empty, and HEAD
	// still names a branch that has none. An emptiness check would answer the
	// other way here, which is why the question is about HEAD.
	base, err := ResolveBase(t.Context(), r.root, "")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(base).To(BeEmpty())
}

func TestResolveBase_unresolvableHead(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	r.commit(g, "base")
	// A raw object id in HEAD is a detached head, and this one names nothing.
	r.write(g, ".git/HEAD", "0000000000000000000000000000000000000000\n")

	// A HEAD that names nothing is not an unborn branch, and answering with an
	// empty base would capture every tracked file against the empty tree.
	_, err := ResolveBase(t.Context(), r.root, "")
	g.Expect(errors.Is(err, ErrBadBase)).To(BeTrue())
}
