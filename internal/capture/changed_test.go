package capture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"
)

func Test_changedArgsAsksGitNotToDetectRenames(t *testing.T) {
	g := NewWithT(t)

	// Removing the flag changes nothing this package returns, because parseRaw
	// reads the two-path record as a delete and an add either way. It is
	// asserted on the list because what it saves is git's work, which no
	// assertion on the output can see.
	g.Expect(changedArgs("abc123")).To(Equal([]string{"diff", "--raw", "-z", "--no-renames", "abc123"}))
}

func Test_parseRawReadsARenameAsADeleteAndAnAdd(t *testing.T) {
	g := NewWithT(t)

	// --no-renames beats the configuration, so no repository produces this
	// record. The guard is against a git that behaves differently, and the
	// failure it prevents is silent: the second path would be read as the next
	// record's status and the rest of the round would come out as garbage.
	raw := ":100644 100644 aaaaaaa bbbbbbb R100\x00old.txt\x00new.txt\x00:100644 100644 ccccccc 0000000 M\x00after.txt\x00"
	got, err := parseRaw(raw)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{
		{Path: "old.txt", Deleted: true},
		{Path: "new.txt"},
		{Path: "after.txt"},
	}))
}

func Test_parseRawRefusesAMalformedRecord(t *testing.T) {
	records := []struct {
		name string
		raw  string
	}{
		{
			name: "no leading colon",
			raw:  "100644 100644 aaaaaaa bbbbbbb M\x00a.txt\x00",
		},
		{
			name: "too few fields",
			raw:  ":100644 100644 M\x00a.txt\x00",
		},
		{
			name: "a record with no path after it",
			raw:  ":100644 100644 aaaaaaa bbbbbbb M\x00",
		},
		{
			name: "a rename with only one path",
			raw:  ":100644 100644 aaaaaaa bbbbbbb R100\x00old.txt\x00",
		},
	}
	for _, c := range records {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)

			_, err := parseRaw(c.raw)
			g.Expect(err).To(HaveOccurred())
		})
	}
}

func Test_parseUntrackedSkipsAnEmbeddedRepository(t *testing.T) {
	g := NewWithT(t)

	// git reports an untracked directory holding its own .git as one entry
	// with a trailing slash, rather than as the files inside it. Dropping it
	// here rather than at the later lstat means a repository deleted in the
	// meantime cannot refuse the round as a vanished path.
	got := parseUntracked("a.txt\x00vendored/\x00b.txt\x00")
	g.Expect(got).To(Equal([]Entry{
		{Path: "a.txt"},
		{Path: "b.txt"},
	}))
}

func TestChanged(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "keep.txt", "keep\n")
	r.write(g, "edit.txt", "before\n")
	r.write(g, "gone.txt", "gone\n")
	base := r.commit(g, "base")
	r.write(g, "edit.txt", "after\n")
	g.Expect(os.Remove(filepath.Join(r.root, "gone.txt"))).To(Succeed())
	r.write(g, "added.txt", "added\n")

	got, err := Changed(t.Context(), r.root, base)
	g.Expect(err).NotTo(HaveOccurred())
	// Sorted by path, one entry each, and the file nobody touched is absent
	// because it resolves out of git at the base for nothing.
	g.Expect(got).To(Equal([]Entry{
		{Path: "added.txt"},
		{Path: "edit.txt"},
		{Path: "gone.txt", Deleted: true},
	}))
}

func TestChanged_awkwardPaths(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "tracked\nnewline.txt", "one\n")
	base := r.commit(g, "base")
	r.write(g, "tracked\nnewline.txt", "two\n")
	r.write(g, "untracked\nnewline.txt", "three\n")

	// A line-based parse of -z output turns each of these into two paths that
	// are both wrong.
	got, err := Changed(t.Context(), r.root, base)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{
		{Path: "tracked\nnewline.txt"},
		{Path: "untracked\nnewline.txt"},
	}))
}

func TestChanged_embeddedRepository(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	base := r.commit(g, "base")
	inner := newRepo(g, filepath.Join(r.root, "vendored"))
	inner.write(g, "b.txt", "b\n")

	// ls-files reports an untracked directory holding its own .git as one
	// entry with a trailing slash, so nothing under it is captured and the
	// directory itself is never opened as a file.
	got, err := Changed(t.Context(), r.root, base)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(BeEmpty())
}

func TestChanged_ignoredFiles(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, ".gitignore", "build/\n")
	base := r.commit(g, "base")
	r.write(g, "build/out.bin", "junk\n")
	r.write(g, "src.txt", "src\n")

	got, err := Changed(t.Context(), r.root, base)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{{Path: "src.txt"}}))
}

func TestChanged_noCommitsYet(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "staged.txt", "staged\n")
	r.git(g, "add", "-A")
	r.write(g, "loose.txt", "loose\n")

	// An empty base is the empty tree, which reports every staged file as
	// added, and untracked files arrive as they always do.
	got, err := Changed(t.Context(), r.root, "")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{
		{Path: "loose.txt"},
		{Path: "staged.txt"},
	}))
}

func TestChanged_pathReportedByBothCalls(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	base := r.commit(g, "base")
	r.git(g, "rm", "--cached", "--quiet", "a.txt")

	// The ordinary way a tracked file becomes untracked. The raw diff calls it
	// deleted and ls-files calls it present, and two rows for one path is a
	// duplicate the store refuses outright. The file is on disk, so the
	// untracked answer is the true one.
	got, err := Changed(t.Context(), r.root, base)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{{Path: "a.txt"}}))
}

func TestChanged_rename(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "old.txt", "content\n")
	base := r.commit(g, "base")
	r.git(g, "mv", "old.txt", "new.txt")

	// A capture is a path-to-content list, so a rename is two entries whether
	// or not git was asked to detect one.
	got, err := Changed(t.Context(), r.root, base)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{
		{Path: "new.txt"},
		{Path: "old.txt", Deleted: true},
	}))
}

func TestChanged_sha256Repository(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir(), "--object-format=sha256")
	r.write(g, "a.txt", "a\n")
	r.git(g, "add", "-A")

	// The empty tree has a different id under SHA-256, and such a repository
	// rejects the SHA-1 constant outright, so the id has to be asked for.
	got, err := Changed(t.Context(), r.root, "")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{{Path: "a.txt"}}))
}

func TestChanged_stagedAndEditedAgain(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "one\n")
	base := r.commit(g, "base")
	r.write(g, "a.txt", "two\n")
	r.git(g, "add", "a.txt")
	r.write(g, "a.txt", "three\n")

	// The record carries a real destination object id once a path is staged,
	// naming the staged copy rather than the file on disk. Nothing in this
	// result may depend on that id.
	got, err := Changed(t.Context(), r.root, base)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{{Path: "a.txt"}}))
}

func TestChanged_submoduleAdded(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	inner := newRepo(g, filepath.Join(dir, "inner"))
	inner.write(g, "f.txt", "one\n")
	inner.commit(g, "inner base")
	r := newRepo(g, filepath.Join(dir, "outer"))
	r.write(g, "a.txt", "a\n")
	base := r.commit(g, "base")
	addSubmodule(g, r, inner.root, "sub")

	// The gitlink has mode 160000 on the destination side. Its worktree entry
	// is a directory, so reading it as a file fails, and its contents are
	// another repository's.
	got, err := Changed(t.Context(), r.root, base)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{{Path: ".gitmodules"}}))
}

func TestChanged_submoduleRemoved(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	inner := newRepo(g, filepath.Join(dir, "inner"))
	inner.write(g, "f.txt", "one\n")
	inner.commit(g, "inner base")
	r := newRepo(g, filepath.Join(dir, "outer"))
	r.write(g, "a.txt", "a\n")
	addSubmodule(g, r, inner.root, "sub")
	base := r.commit(g, "base")
	r.git(g, "rm", "--quiet", "sub")

	// A removed submodule reports :160000 000000 <oid> 0000000 D, so mode
	// 160000 is on the source side only and a check on the destination alone
	// would record it as an ordinary deleted file and then try to read it.
	// .gitmodules is emptied rather than removed, so it reports as modified.
	got, err := Changed(t.Context(), r.root, base)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{{Path: ".gitmodules"}}))
}

func TestChanged_symlinks(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "target.txt", "target\n")
	r.write(g, "dir/inner.txt", "inner\n")
	g.Expect(os.Symlink("target.txt", filepath.Join(r.root, "tracked.link"))).To(Succeed())
	base := r.commit(g, "base")
	r.write(g, "later.txt", "later\n")
	g.Expect(os.Symlink("target.txt", filepath.Join(r.root, "toFile.link"))).To(Succeed())
	g.Expect(os.Symlink("dir", filepath.Join(r.root, "toDir.link"))).To(Succeed())
	g.Expect(os.Remove(filepath.Join(r.root, "tracked.link"))).To(Succeed())
	g.Expect(os.Symlink("later.txt", filepath.Join(r.root, "tracked.link"))).To(Succeed())

	// Untracked links arrive from ls-files with no mode at all, and the one
	// pointing at a directory is indistinguishable there from the one pointing
	// at a file. Reading either with os.ReadFile is wrong, and reading the
	// second fails with EISDIR and takes the capture with it.
	got, err := Changed(t.Context(), r.root, base)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{
		{Path: "later.txt"},
		{Path: "toDir.link", Symlink: true},
		{Path: "toFile.link", Symlink: true},
		{Path: "tracked.link", Symlink: true},
	}))
}

func TestChanged_typeChange(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	r.write(g, "was_file", "content\n")
	base := r.commit(g, "base")
	g.Expect(os.Remove(filepath.Join(r.root, "was_file"))).To(Succeed())
	g.Expect(os.Symlink("a.txt", filepath.Join(r.root, "was_file"))).To(Succeed())

	got, err := Changed(t.Context(), r.root, base)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{{Path: "was_file", Symlink: true}}))
}

func TestChanged_vanishedPath(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())

	// An enumeration reported it and then it went. Recording it as deleted
	// would put a deletion the agent did not make into a frozen round.
	_, err := keep(r.root, map[string]Entry{"gone.txt": {Path: "gone.txt"}})
	g.Expect(errors.Is(err, ErrVanished)).To(BeTrue())
	g.Expect(err.Error()).To(ContainSubstring("gone.txt"))
}

// addSubmodule registers other as a submodule of r at name.
//
// protocol.file.allow is off by default since the 2022 advisory, and a fixture
// cloning from a path on disk needs it turned back on for the one call.
func addSubmodule(g *WithT, r *repo, other, name string) {
	g.THelper()
	r.git(g, "-c", "protocol.file.allow=always", "submodule", "add", "--quiet", other, name)
}
