package capture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/diff"
	. "github.com/onsi/gomega"
)

func TestReadWorkingCopy(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	r := newRepo(g, filepath.Join(t.TempDir(), "project"))
	r.write(g, ".gitignore", "private.txt\n")
	r.write(g, "tracked.txt", "one\ntwo\n")
	r.write(g, "gone.txt", "removed\nfrom disk\n")
	r.write(g, "vendor/x/a.txt", "committed\n")
	r.write(g, "olddir/f.txt", "in a directory since removed\n")
	g.Expect(os.Symlink("old-target", filepath.Join(r.root, "link"))).To(Succeed())
	head := r.commit(g, "base")
	r.write(g, "tracked.txt", "one\n2\nthree\n")
	g.Expect(os.Remove(filepath.Join(r.root, "gone.txt"))).To(Succeed())
	// A deletion keeps its row though its directory is gone, since nothing
	// reads its disk side.
	g.Expect(os.RemoveAll(filepath.Join(r.root, "olddir"))).To(Succeed())
	g.Expect(os.Remove(filepath.Join(r.root, "link"))).To(Succeed())
	g.Expect(os.Symlink("new-target", filepath.Join(r.root, "link"))).To(Succeed())
	r.write(g, "new.txt", "a\nb\n")
	r.write(g, "private.txt", "a secret\n")
	// A tracked file below a directory that has since become a repository of
	// its own. git diff still lists it, and the live read refuses it.
	r.write(g, "vendor/x/a.txt", "edited\n")
	newRepo(g, filepath.Join(r.root, "vendor", "x"))

	wc, err := ReadWorkingCopy(t.Context(), r.root, false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(wc.Listed).To(BeTrue())
	g.Expect(wc.Head).To(Equal(head))
	g.Expect(wc.State.Live).To(Equal(LiveIgnoreRules))
	g.Expect(wc.Entries).To(Equal([]Entry{
		{Path: "gone.txt", Deleted: true},
		{Path: "link", Symlink: true},
		{Path: "new.txt"},
		{Path: "olddir/f.txt", Deleted: true},
		{Path: "tracked.txt"},
	}))
	listed, err := Changed(t.Context(), r.root, head)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(listed).To(ContainElement(Entry{Path: "vendor/x/a.txt"}), "git lists the nested file, so the drop is the working copy's own")
	_, held := wc.Holds("vendor/x/a.txt")
	g.Expect(held).To(BeFalse())
	e, held := wc.Holds("gone.txt")
	g.Expect(held).To(BeTrue())
	g.Expect(e.Deleted).To(BeTrue())
}

func TestReadWorkingCopy_states(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	cases := []struct {
		name   string
		setup  func(g *WithT, dir string) string
		inside bool
		live   Live
	}{
		{
			name: "a missing root",
			setup: func(g *WithT, dir string) string {
				return filepath.Join(dir, "gone")
			},
			live: LiveRootMissing,
		},
		{
			name: "a lost repository",
			setup: func(g *WithT, dir string) string {
				r := newRepo(g, filepath.Join(dir, "project"))
				r.write(g, "a.txt", "a\n")
				r.commit(g, "base")
				g.Expect(os.Rename(filepath.Join(r.root, gitDir), filepath.Join(dir, "moved.git"))).To(Succeed())
				return r.root
			},
			live: LiveLost,
		},
		{
			name: "a .git file naming a directory that is gone",
			setup: func(g *WithT, dir string) string {
				writeFile(g, filepath.Join(dir, "worktree"), gitDir, "gitdir: "+filepath.Join(dir, "gone")+"\n")
				writeFile(g, filepath.Join(dir, "worktree"), "a.txt", "a\n")
				return filepath.Join(dir, "worktree")
			},
			live: LiveLost,
		},
		{
			name: "a project outside git",
			setup: func(g *WithT, dir string) string {
				writeFile(g, filepath.Join(dir, "scratch"), Marker, "")
				writeFile(g, filepath.Join(dir, "scratch"), "a.txt", "a\n")
				return filepath.Join(dir, "scratch")
			},
			live: LiveOpen,
		},
		{
			name: "a marker project inside a checkout that does not ignore it",
			setup: func(g *WithT, dir string) string {
				_, notes := markerInCheckout(g, dir)
				return notes
			},
			live: LiveIgnoreRules,
		},
		{
			name: "a marker project the checkout ignores",
			setup: func(g *WithT, dir string) string {
				outer, _ := markerInCheckout(g, dir)
				return filepath.Join(outer.root, "ign")
			},
			live: LiveIgnored,
		},
		{
			name: "a marker project whose checkout lost its .git",
			setup: func(g *WithT, dir string) string {
				outer, notes := markerInCheckout(g, dir)
				g.Expect(os.Rename(filepath.Join(outer.root, gitDir), filepath.Join(dir, "moved.git"))).To(Succeed())
				return notes
			},
			inside: true,
			live:   LiveCheckoutGone,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			root := c.setup(g, t.TempDir())
			wc, err := ReadWorkingCopy(t.Context(), root, c.inside)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(wc.State.Live).To(Equal(c.live))
			g.Expect(wc.Listed).To(BeFalse())
			g.Expect(wc.Entries).To(BeEmpty())
		})
	}
}

func TestReadWorkingCopy_unborn(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "one\ntwo\n")
	r.git(g, "add", "a.txt")
	r.write(g, "b.txt", "untracked\n")

	// A branch with no commit compares against nothing, so every file is
	// wholly added. That is a git project, not one outside git.
	wc, err := ReadWorkingCopy(t.Context(), r.root, false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(wc.Listed).To(BeTrue())
	g.Expect(wc.Head).To(BeEmpty())
	g.Expect(wc.Entries).To(Equal([]Entry{{Path: "a.txt"}, {Path: "b.txt"}}))
	moved, err := wc.Compare(t.Context(), []string{"a.txt", "b.txt"})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(counts(moved)).To(Equal([]string{"a.txt +2 -0", "b.txt +1 -0"}))
}

func TestReadWorkingCopy_vanished(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	head := r.commit(g, "base")
	r.write(g, "a.txt", "changed\n")
	afterListing = func() {
		g.Expect(os.Remove(filepath.Join(r.root, "editing.txt"))).To(Succeed())
	}
	t.Cleanup(func() { afterListing = nil })

	// git listed the file and it went before it was examined. A live page
	// drops it rather than failing.
	r.write(g, "editing.txt", "an agent's scratch file\n")
	wc, err := ReadWorkingCopy(t.Context(), r.root, false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(wc.Entries).To(Equal([]Entry{{Path: "a.txt"}}))

	// A frozen round still refuses it.
	r.write(g, "editing.txt", "back\n")
	_, err = Changed(t.Context(), r.root, head)
	g.Expect(errors.Is(err, ErrVanished)).To(BeTrue())
}

func TestWorkingCopy_Compare(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	r := newRepo(g, t.TempDir())
	r.write(g, "tracked.txt", "one\ntwo\n")
	r.write(g, "gone.txt", "removed\nfrom disk\n")
	r.write(g, "shrunk.txt", strings.Repeat("x", diff.MaxBytes+1))
	g.Expect(os.Symlink("old-target", filepath.Join(r.root, "link"))).To(Succeed())
	r.commit(g, "base")
	r.write(g, "shrunk.txt", "small now\n")
	r.write(g, "tracked.txt", "one\n2\nthree\n")
	g.Expect(os.Remove(filepath.Join(r.root, "gone.txt"))).To(Succeed())
	g.Expect(os.Remove(filepath.Join(r.root, "link"))).To(Succeed())
	g.Expect(os.Symlink("new-target", filepath.Join(r.root, "link"))).To(Succeed())
	r.write(g, "new.txt", "a\nb\n")
	r.write(g, "big.txt", strings.Repeat("x", diff.MaxBytes+1))
	wc, err := ReadWorkingCopy(t.Context(), r.root, false)
	g.Expect(err).NotTo(HaveOccurred())

	// HEAD's side reads from the commit, where Open would have returned the
	// same, and the disk's side reads the file now. A changed link is its
	// target text on both sides, so it moves one line, not every line.
	paths := []string{"big.txt", "gone.txt", "link", "new.txt", "shrunk.txt", "tracked.txt"}
	moved, err := wc.Compare(t.Context(), paths)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(counts(moved)).To(Equal([]string{
		"big.txt too large",
		"gone.txt +0 -2",
		"link +1 -1",
		"new.txt +2 -0",
		"shrunk.txt too large",
		"tracked.txt +2 -1",
	}))
	// Every path's object id came from one ls-tree.
	g.Expect(wc.head.lookups).To(Equal(1))
}

func TestWorkingCopy_Sides(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "committed\n")
	r.write(g, "b.txt", "committed\n")
	r.write(g, "d.txt", "deleted\n")
	r.commit(g, "base")
	g.Expect(os.Remove(filepath.Join(r.root, "d.txt"))).To(Succeed())
	r.write(g, "a.txt", "on disk\n")
	r.write(g, "b.txt", "on disk\n")
	r.write(g, "c.txt", "untracked\n")
	wc, err := ReadWorkingCopy(t.Context(), r.root, false)
	g.Expect(err).NotTo(HaveOccurred())

	head, disk, tooLarge, err := wc.Sides(t.Context(), "a.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(tooLarge).To(BeFalse())
	g.Expect(string(head)).To(Equal("committed\n"))
	g.Expect(string(disk)).To(Equal("on disk\n"))

	// A path the listing does not hold is not read at all.
	_, _, _, err = wc.Sides(t.Context(), "nothing.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())

	// Removed since the listing, the file is a deletion as of now.
	g.Expect(os.Remove(filepath.Join(r.root, "b.txt"))).To(Succeed())
	head, disk, _, err = wc.Sides(t.Context(), "b.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(head)).To(Equal("committed\n"))
	g.Expect(disk).To(BeNil())

	// git failing to say what it ignores is not an absence. Read as one, the
	// row would show every line removed.
	writeFile(g, r.root, ".git/index", "garbage\n")
	_, _, _, err = wc.Sides(t.Context(), "c.txt")
	g.Expect(errors.Is(err, ErrRulesUnreadable)).To(BeTrue())
	_, err = wc.Compare(t.Context(), []string{"c.txt"})
	g.Expect(errors.Is(err, ErrRulesUnreadable)).To(BeTrue())
	// A deletion asks nothing of the disk, so it still reads.
	head, disk, _, err = wc.Sides(t.Context(), "d.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(head)).To(Equal("deleted\n"))
	g.Expect(disk).To(BeNil())
}

// counts is each comparison as its path and its counts, or as too large.
func counts(moved []Moved) []string {
	out := make([]string, 0, len(moved))
	for _, m := range moved {
		if m.Result.TooLarge {
			out = append(out, m.Path+" too large")
			continue
		}
		out = append(out, m.Path+" +"+itoa(m.Result.Added)+" -"+itoa(m.Result.Removed))
	}
	return out
}
