package capture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"
)

func Test_isGitProjectAcceptsALinkedWorktree(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	r := newRepo(g, filepath.Join(dir, "main"))
	r.write(g, "a.txt", "a\n")
	r.commit(g, "base")
	linked := filepath.Join(dir, "linked")
	r.git(g, "worktree", "add", "--quiet", linked)

	// A linked worktree's .git is a file rather than a directory, and the top
	// level it reports is the worktree itself.
	got, err := isGitProject(t.Context(), linked)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(BeTrue())
}

func Test_isGitProjectAcceptsARootReachedThroughASymlink(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	r := newRepo(g, filepath.Join(dir, "real"))
	r.write(g, "a.txt", "a\n")
	r.commit(g, "base")
	link := filepath.Join(dir, "link")
	g.Expect(os.Symlink(r.root, link)).To(Succeed())

	// git prints the top level with symlinks resolved. Comparing the two
	// spellings would refuse every project whose path runs through one, which
	// on some systems is every project under a temporary directory.
	got, err := isGitProject(t.Context(), link)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(BeTrue())
}

func Test_isGitProjectFindsARepository(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	r.commit(g, "base")

	got, err := isGitProject(t.Context(), r.root)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(BeTrue())
}

func Test_isGitProjectRefusesADisplacedWorktree(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	r := newRepo(g, filepath.Join(dir, "repo"))
	elsewhere := filepath.Join(dir, "elsewhere")
	g.Expect(os.MkdirAll(elsewhere, 0o700)).To(Succeed())
	// core.worktree is the one way a root can hold a .git entry and still not
	// be the top level. Enumerating here would return paths relative to
	// elsewhere while capture read files relative to the root.
	r.git(g, "config", "core.worktree", elsewhere)

	_, err := isGitProject(t.Context(), r.root)
	g.Expect(errors.Is(err, ErrNotTopLevel)).To(BeTrue())
	g.Expect(err.Error()).To(ContainSubstring(elsewhere))
}

func Test_isGitProjectRefusesWithNoGitOnPath(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	t.Setenv("PATH", "")

	_, err := isGitProject(t.Context(), r.root)
	g.Expect(errors.Is(err, ErrNoGit)).To(BeTrue())
}

func Test_isGitProjectSkipsADirectoryWithNoGitEntry(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "sub/a.txt", "a\n")
	r.commit(g, "base")
	sub := filepath.Join(r.root, "sub")

	// A directory inside a repository is not a git project of its own, and
	// says so without an error, because the walk handles it.
	got, err := isGitProject(t.Context(), sub)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(BeFalse())
}

func Test_isGitProjectSkipsADirectoryWithNoGitOnPath(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	t.Setenv("PATH", "")

	// Without a .git entry there is nothing for git to answer, so a machine
	// with no git still gets the walk rather than a refusal.
	got, err := isGitProject(t.Context(), dir)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(BeFalse())
}

func Test_runArgsCarriesTheFlagsEveryCallNeeds(t *testing.T) {
	g := NewWithT(t)

	// Neither flag changes anything a test can watch, so the list is what is
	// asserted. --no-optional-locks matters only when another git holds
	// index.lock, which is the agent's own git during a capture.
	args := runArgs("/root", []string{"rev-parse", "HEAD"})
	g.Expect(args).To(Equal([]string{"-C", "/root", "--no-optional-locks", "rev-parse", "HEAD"}))
}

func Test_runReportsWhatGitSaid(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())

	_, err := run(t.Context(), r.root, "cat-file", "-p", "nope")
	g.Expect(err).To(HaveOccurred())
	// The command, so a reader knows what was asked, and git's own words,
	// which is the part an exit status alone throws away.
	g.Expect(err.Error()).To(ContainSubstring("cat-file -p nope"))
	g.Expect(err.Error()).To(ContainSubstring("Not a valid object name"))
}
