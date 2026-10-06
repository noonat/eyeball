package capture

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"

	"github.com/noonat/eyeball/internal/blob"
)

func Test_chunksStayUnderTheBudget(t *testing.T) {
	g := NewWithT(t)
	paths := []string{"aaaa", "bbbb", "cccc", "dddd"}

	// Five bytes each with the separator, so two fit in a budget of ten.
	tight := chunks(paths, 10)
	g.Expect(tight).To(HaveLen(2))
	g.Expect(tight[0]).To(Equal([]string{"aaaa", "bbbb"}))
	g.Expect(tight[1]).To(Equal([]string{"cccc", "dddd"}))

	roomy := chunks(paths, 1<<20)
	g.Expect(roomy).To(HaveLen(1))
	g.Expect(roomy[0]).To(Equal(paths))

	// A path longer than the whole budget still has to be asked about.
	single := chunks([]string{"aaaa"}, 1)
	g.Expect(single).To(HaveLen(1))
	g.Expect(single[0]).To(Equal([]string{"aaaa"}))
}

func TestCleanPath(t *testing.T) {
	paths := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "ordinary",
			in:   "docs/a.md",
			want: "docs/a.md",
		},
		{
			name: "a leading slash is dropped",
			in:   "/docs/a.md",
			want: "docs/a.md",
		},
		{
			name: "climbing out is refused",
			in:   "../../etc/passwd",
			want: "",
		},
		{
			name: "climbing out from inside is refused",
			in:   "docs/../../etc/passwd",
			want: "",
		},
		{
			name: "the root itself names no file",
			in:   "",
			want: "",
		},
		{
			name: "the repository's own directory is refused",
			in:   ".git/config",
			want: "",
		},
		{
			name: "and so is the directory itself",
			in:   ".git",
			want: "",
		},
		{
			name: "a nested repository's directory is refused",
			in:   "vendor/x/.git/config",
			want: "",
		},
		{
			name: "a .git segment last is refused",
			in:   "vendor/x/.git",
			want: "",
		},
		{
			name: "a name that only starts with .git is a file",
			in:   ".github/workflows/ci.yml",
			want: ".github/workflows/ci.yml",
		},
		{
			name: "a name that only ends with .git is a file",
			in:   "vendor/x.git/config",
			want: "vendor/x.git/config",
		},
		{
			name: "a%00b as the handler receives it",
			in:   "a\x00b",
			want: "",
		},
	}
	for _, c := range paths {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := CleanPath(c.in)
			if c.want == "" {
				g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(c.want))
		})
	}
}

func TestNewReader(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	r, blobs := readerRepo(g, t)
	r.write(g, ".gitignore", "private.txt\n")
	r.write(g, "private.txt", "a secret\n")
	r.commit(g, "base")
	plain := t.TempDir()
	writeFile(g, plain, "private.txt", "no repository says to hide it\n")

	// A root holding a .git entry reads live only what git does not ignore,
	// even through a reader no Inspect built.
	_, _, err := NewReader(r.root, "", blobs, nil).Open(t.Context(), "private.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())

	// A root with none has no rules to apply.
	rc, source, err := NewReader(plain, "", blobs, nil).Open(t.Context(), "private.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceWorking))
	g.Expect(readAll(g, rc)).To(Equal("no repository says to hide it\n"))
	g.Expect(NewReader(r.root, "", blobs, nil).Listing()).To(Equal(ListingCapture))
	g.Expect(NewReader(plain, "", blobs, nil).Listing()).To(Equal(ListingDisk))

	// A root that cannot be looked at counts as a git project, so its listing
	// is the capture alone rather than a disk nothing said how to filter.
	loop := filepath.Join(t.TempDir(), "loop")
	g.Expect(os.Symlink(loop, loop)).To(Succeed())
	g.Expect(NewReader(loop, "", blobs, nil).Listing()).To(Equal(ListingCapture))
}

func TestReader_Deleted(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	r, blobs := readerRepo(g, t)
	r.write(g, "kept.txt", "kept\n")
	base := r.commit(g, "base")
	g.Expect(os.Remove(filepath.Join(r.root, "untouched.txt"))).To(Succeed())
	r.write(g, "kept.txt", "changed\n")
	req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	r.write(g, "untouched.txt", "somebody put it back\n")
	st, err := Inspect(t.Context(), r.root, base, false)
	g.Expect(err).NotTo(HaveOccurred())
	reader := st.Reader(blobs, req.Files)

	// The working copy has the file again, and the round still says it is
	// gone, which is what the page shows.
	g.Expect(reader.Deleted("untouched.txt")).To(BeTrue())
	g.Expect(reader.Deleted("/untouched.txt")).To(BeTrue())
	_, _, err = reader.Open(t.Context(), "untouched.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())

	// A changed file, a file the round does not hold, and a path that names
	// nothing are not deleted.
	notDeleted := []string{"kept.txt", "nowhere.txt", "../untouched.txt", ""}
	for _, p := range notDeleted {
		g.Expect(reader.Deleted(p)).To(BeFalse(), p)
	}
}

func TestReader_Frozen(t *testing.T) {
	g := NewWithT(t)
	r, blobs := readerRepo(g, t)
	base := r.commit(g, "base")
	r.write(g, "changed.txt", "as the round had it\n")
	req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	r.write(g, "changed.txt", "moved on since\n")
	reader := NewReader(r.root, base, blobs, req.Files)

	// The capture first, which is the only source a file under review is read
	// from, and it holds what the round had rather than what is on disk now.
	rc, source, err := reader.Frozen(t.Context(), "changed.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceRound))
	g.Expect(readAll(g, rc)).To(Equal("as the round had it\n"))

	// Then the base, for a file the capture does not hold because it matches.
	rc, source, err = reader.Frozen(t.Context(), "untouched.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceBase))
	g.Expect(readAll(g, rc)).To(Equal("untouched\n"))

	// The working copy is not a third source here. A path in neither does not
	// exist as of the round, and reading the live file would put content the
	// agent never had into a frozen comparison.
	r.write(g, "appeared-later.txt", "not in the round\n")
	_, _, err = reader.Frozen(t.Context(), "appeared-later.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())

	// A path out of the root is cleaned before git sees it, which would
	// otherwise fail on a path outside the repository.
	_, _, err = reader.Frozen(t.Context(), "../outside.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestReader_Frozen_deletedPath(t *testing.T) {
	g := NewWithT(t)
	r, blobs := readerRepo(g, t)
	base := r.commit(g, "base")
	g.Expect(os.Remove(filepath.Join(r.root, "untouched.txt"))).To(Succeed())
	req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	reader := NewReader(r.root, base, blobs, req.Files)

	// The round says it is gone, so it is gone. Falling through to the base
	// would show the file the agent deleted as though it were still there.
	_, _, err = reader.Frozen(t.Context(), "untouched.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestReader_Frozen_emptyBase(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	writeFile(g, dir, "docs/a.md", "written from scratch\n")
	blobs, _ := newBlobs(g, t)
	req, err := Freeze(t.Context(), Options{Round: 1, Root: dir, Paths: []string{"docs"}, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	reader := NewReader(dir, "", blobs, req.Files)

	rc, source, err := reader.Frozen(t.Context(), "docs/a.md")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceRound))
	g.Expect(readAll(g, rc)).To(Equal("written from scratch\n"))

	// There is no commit to ask, so the second source is skipped rather than
	// asked with an empty revision.
	_, _, err = reader.Frozen(t.Context(), "docs/other.md")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestReader_Frozen_gitlink(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	inner := newRepo(g, filepath.Join(dir, "inner"))
	inner.write(g, "f.txt", "one\n")
	inner.commit(g, "inner base")
	r := newRepo(g, filepath.Join(dir, "outer"))
	r.write(g, "a.txt", "a\n")
	addSubmodule(g, r, inner.root, "sub")
	base := r.commit(g, "base")
	blobs, _ := newBlobs(g, t)
	reader := NewReader(r.root, base, blobs, nil)

	// The base holds a gitlink at that path, and cat-file blob on a commit id
	// fails. Treating a non-blob entry as absent is what keeps a submodule
	// from turning a read into an error.
	_, _, err := reader.Frozen(t.Context(), "sub")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestReader_Frozen_pathWithANewline(t *testing.T) {
	g := NewWithT(t)
	r, blobs := readerRepo(g, t)
	r.write(g, "weird\nname.txt", "awkward\n")
	base := r.commit(g, "base")
	reader := NewReader(r.root, base, blobs, nil)

	// The path goes on a command line and comes back through a NUL separated
	// listing, so nothing along the way may split it.
	rc, source, err := reader.Frozen(t.Context(), "weird\nname.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceBase))
	g.Expect(readAll(g, rc)).To(Equal("awkward\n"))
}

func TestReader_Frozen_pathWithPathspecMagic(t *testing.T) {
	g := NewWithT(t)
	r, blobs := readerRepo(g, t)
	r.write(g, ":weird.txt", "leading colon\n")
	base := r.commit(g, "base")
	reader := NewReader(r.root, base, blobs, nil)

	// A leading colon is pathspec magic. Without --literal-pathspecs git parses
	// it as such, matches nothing, and the file reads as absent from the base
	// rather than as an error, which is the silent kind of wrong.
	rc, source, err := reader.Frozen(t.Context(), ":weird.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceBase))
	g.Expect(readAll(g, rc)).To(Equal("leading colon\n"))
}

func TestReader_Frozen_readsInBatches(t *testing.T) {
	g := NewWithT(t)
	r, blobs := readerRepo(g, t)
	for i := 0; i < 20; i++ {
		r.write(g, fileName(i), "content\n")
	}
	base := r.commit(g, "base")
	reader := NewReader(r.root, base, blobs, nil)
	reader.batch = 12

	// Enough paths to need several calls, since they travel on the command
	// line and the operating system bounds that.
	names := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		names = append(names, fileName(i))
	}
	g.Expect(len(chunks(names, reader.batch))).To(BeNumerically(">", 1))
	g.Expect(reader.loadOIDs(t.Context(), names)).To(Succeed())
	for _, name := range names {
		g.Expect(reader.oids[name]).NotTo(BeEmpty())
	}
}

func TestReader_Frozen_remembersWhatTheBaseLacks(t *testing.T) {
	g := NewWithT(t)
	r, blobs := readerRepo(g, t)
	base := r.commit(g, "base")
	reader := NewReader(r.root, base, blobs, nil)

	_, _, err := reader.Frozen(t.Context(), "nope.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())

	// An absent path is remembered as absent. Without that, every read of one
	// asks git again, which over a round's file list is a process per file.
	oid, known := reader.oids["nope.txt"]
	g.Expect(known).To(BeTrue())
	g.Expect(oid).To(BeEmpty())
}

func TestReader_Open(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	r, blobs := readerRepo(g, t)
	base := r.commit(g, "base")
	r.write(g, "appeared-later.txt", "only on disk\n")
	reader := NewReader(r.root, base, blobs, nil)

	// The third source, for a file the round never saw. Showing nothing
	// instead sends the reviewer to a computer, which ends the review.
	rc, source, err := reader.Open(t.Context(), "appeared-later.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceWorking))
	g.Expect(readAll(g, rc)).To(Equal("only on disk\n"))

	// A path in none of the three.
	_, _, err = reader.Open(t.Context(), "nowhere.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())

	// A root removed after the reader was built has nothing left to read.
	gone := filepath.Join(t.TempDir(), "gone")
	writeFile(g, gone, "a.txt", "a\n")
	removed := NewReader(gone, "", blobs, nil)
	g.Expect(os.RemoveAll(gone)).To(Succeed())
	_, _, err = removed.Open(t.Context(), "a.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestReader_Open_deletedPathStaysDeleted(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	r, blobs := readerRepo(g, t)
	base := r.commit(g, "base")
	g.Expect(os.Remove(filepath.Join(r.root, "untouched.txt"))).To(Succeed())
	req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	reader := NewReader(r.root, base, blobs, req.Files)
	r.write(g, "untouched.txt", "somebody put it back\n")

	// Browsing still shows it as gone. The round is what is being read, and
	// the working copy having it again is a later fact.
	_, _, err = reader.Open(t.Context(), "untouched.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestReader_Open_ignored(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	r, blobs := readerRepo(g, t)
	r.write(g, ".gitignore", "private.txt\nbuild/\n:other.txt\n")
	base := r.commit(g, "base")
	r.write(g, "private.txt", "a secret\n")
	r.write(g, "build/out", "built\n")
	r.write(g, ":other.txt", "ignored, with a leading colon\n")
	r.write(g, ":visible.txt", "not ignored, with a leading colon\n")
	r.write(g, "visible.txt", "not ignored\n")
	st, err := Inspect(t.Context(), r.root, base, false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(st.Live).To(Equal(LiveIgnoreRules))
	reader := st.Reader(blobs, nil)

	// Nothing links to an ignored file, but every device on the tailnet can
	// type its address. Given as :other.txt rather than ./:other.txt, git
	// reads the colon as pathspec magic and calls the file not ignored.
	refused := []string{"private.txt", "build/out", ":other.txt"}
	for _, p := range refused {
		_, _, err := reader.Open(t.Context(), p)
		g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue(), p)
		g.Expect(errors.Is(err, ErrRulesUnreadable)).To(BeFalse(), p)
	}
	served := map[string]string{
		"visible.txt":  "not ignored\n",
		":visible.txt": "not ignored, with a leading colon\n",
	}
	for p, want := range served {
		rc, source, err := reader.Open(t.Context(), p)
		g.Expect(err).NotTo(HaveOccurred(), p)
		g.Expect(source).To(Equal(SourceWorking))
		g.Expect(readAll(g, rc)).To(Equal(want))
	}
}

func TestReader_Open_linkedParent(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	root := filepath.Join(dir, "project")
	outside := filepath.Join(dir, "outside")
	g.Expect(os.MkdirAll(root, 0o700)).To(Succeed())
	g.Expect(os.MkdirAll(outside, 0o700)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("not the project's\n"), 0o600)).To(Succeed())
	g.Expect(os.Symlink(outside, filepath.Join(root, "link"))).To(Succeed())
	blobs, err := blob.Open(t.TempDir())
	g.Expect(err).NotTo(HaveOccurred())

	// No base and no capture, so the working copy is the only source, and the
	// path runs through a link to outside the project.
	_, _, err = NewReader(root, "", blobs, nil).Open(t.Context(), "link/secret.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
	g.Expect(err.Error()).To(ContainSubstring("runs through link, a symlink"))
}

func TestReader_Open_nestedRepository(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	r, blobs := readerRepo(g, t)
	base := r.commit(g, "base")
	newRepo(g, filepath.Join(r.root, "vendor", "x")).write(g, "x.go", "x\n")
	writeFile(g, r.root, "vendor/x/.git/config", "[remote \"origin\"]\n\turl = https://user:token@example.invalid/x\n")
	newRepo(g, filepath.Join(r.root, "vendor", "y")).write(g, "bar.go", "another repository's file\n")
	writeFile(g, r.root, "vendor/z/ok.go", "the project's own\n")
	st, err := Inspect(t.Context(), r.root, base, false)
	g.Expect(err).NotTo(HaveOccurred())
	reader := st.Reader(blobs, nil)

	// git does not ignore a file in an untracked repository inside the
	// project, so the parent check is what refuses it. The capture and the
	// tree skip that repository as another one's content.
	refused := []string{"vendor/x/.git/config", "vendor/y/bar.go"}
	for _, p := range refused {
		_, _, err := reader.Open(t.Context(), p)
		g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue(), p)
		g.Expect(errors.Is(err, ErrRulesUnreadable)).To(BeFalse(), p)
	}
	rc, source, err := reader.Open(t.Context(), "vendor/z/ok.go")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceWorking))
	g.Expect(readAll(g, rc)).To(Equal("the project's own\n"))
}

func TestReader_Open_notARegularFile(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	r, blobs := readerRepo(g, t)
	base := r.commit(g, "base")
	reader := NewReader(r.root, base, blobs, nil)
	g.Expect(os.MkdirAll(filepath.Join(r.root, "adir"), 0o700)).To(Succeed())

	_, _, err := reader.Open(t.Context(), "adir")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestReader_Open_notFound(t *testing.T) {
	setup := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	dir := t.TempDir()
	r, blobs := readerRepo(setup, t)
	r.write(setup, "go.mod", "module x\n")
	base := r.commit(setup, "base")
	r.write(setup, "a.txt", "inside\n")
	writeFile(setup, dir, "etc/hostname", "not the project's\n")
	long := strings.Repeat("a", 300)
	links := map[string]string{
		"hostlink": "/etc/hostname",
		"etclink":  "/etc",
		"outlink":  filepath.Join(dir, "etc"),
		"inlink":   "a.txt",
		"loop":     "loop",
		"dirlink":  "adir",
	}
	for name, target := range links {
		setup.Expect(os.Symlink(target, filepath.Join(r.root, name))).To(Succeed())
	}
	setup.Expect(syscall.Mkfifo(filepath.Join(r.root, "fifo"), 0o600)).To(Succeed())
	r.write(setup, "adir/f.txt", "reached through a link\n")
	st, err := Inspect(t.Context(), r.root, base, false)
	setup.Expect(err).NotTo(HaveOccurred())
	reader := st.Reader(blobs, nil)

	// A link reads as the text it points at, wherever that is, and is never
	// followed. Everything else here would be a server error or a read from
	// outside the project if the read met it unchecked.
	reads := []struct {
		name   string
		path   string
		want   string
		reason string
	}{
		{
			name: "a link to /etc/hostname reads as its target text",
			path: "hostlink",
			want: "/etc/hostname",
		},
		{
			name: "a link inside the root reads as its target text",
			path: "inlink",
			want: "a.txt",
		},
		{
			name: "a link to itself reads as its target text",
			path: "loop",
			want: "loop",
		},
		{
			name: "an untracked file reads live",
			path: "a.txt",
			want: "inside\n",
		},
		{
			name:   "a path through a link to /etc",
			path:   "etclink/hostname",
			want:   "",
			reason: "a symlink",
		},
		{
			name:   "a path through a link to a directory outside",
			path:   "outlink/hostname",
			want:   "",
			reason: "a symlink",
		},
		{
			name:   "a path through a link to a directory inside the root",
			path:   "dirlink/f.txt",
			want:   "",
			reason: "a symlink",
		},
		{
			name:   "a path through a link loop",
			path:   "loop/x",
			want:   "",
			reason: "a symlink",
		},
		{
			name:   "a FIFO",
			path:   "fifo",
			want:   "",
			reason: "not a regular file",
		},
		{
			name:   "a directory",
			path:   "adir",
			want:   "",
			reason: "not a regular file",
		},
		{
			name:   "a path below a file",
			path:   "go.mod/x",
			want:   "",
			reason: "which is not a directory",
		},
		{
			name: "a 300-byte segment",
			path: long,
			want: "",
		},
		{
			name: "a path below a 300-byte segment",
			path: long + "/x",
			want: "",
		},
	}
	for _, c := range reads {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			rc, source, err := reader.Open(t.Context(), c.path)
			if c.want == "" {
				g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
				g.Expect(errors.Is(err, ErrRulesUnreadable)).To(BeFalse())
				g.Expect(err.Error()).To(ContainSubstring(c.reason))
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(source).To(Equal(SourceWorking))
			g.Expect(readAll(g, rc)).To(Equal(c.want))
		})
	}
}

func TestReader_Open_nul(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	writeFile(g, dir, Marker, "")
	writeFile(g, dir, "a", "a\n")
	blobs, _ := newBlobs(g, t)
	st, err := Inspect(t.Context(), dir, "", false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(st.Live).To(Equal(LiveOpen))

	// %00 in an address arrives as a NUL. Lstat refuses it with "invalid
	// argument", which is no answer a page can show.
	_, _, err = st.Reader(blobs, nil).Open(t.Context(), "a\x00b")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestReader_Open_rulesUnreadable(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	r, blobs := readerRepo(g, t)
	base := r.commit(g, "base")
	r.write(g, "changed.txt", "captured\n")
	req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	r.write(g, "appeared-later.txt", "only on disk\n")
	// A truncated index makes check-ignore exit 128, with a message that has
	// nothing to do with a submodule.
	g.Expect(os.WriteFile(filepath.Join(r.root, gitDir, "index"), []byte("garbage\n"), 0o600)).To(Succeed())
	st, err := Inspect(t.Context(), r.root, base, false)
	g.Expect(err).NotTo(HaveOccurred())
	reader := st.Reader(blobs, req.Files)

	// The live read is refused and says why. The captured file does not
	// depend on the rules and still opens.
	_, _, err = reader.Open(t.Context(), "appeared-later.txt")
	g.Expect(errors.Is(err, ErrRulesUnreadable)).To(BeTrue())
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
	rc, source, err := reader.Open(t.Context(), "changed.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceRound))
	g.Expect(readAll(g, rc)).To(Equal("captured\n"))
}

func TestReader_Open_submodule(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	dir := t.TempDir()
	inner := newRepo(g, filepath.Join(dir, "inner"))
	inner.write(g, "f.txt", "one\n")
	inner.commit(g, "inner base")
	r := newRepo(g, filepath.Join(dir, "outer"))
	r.write(g, "a.txt", "a\n")
	addSubmodule(g, r, inner.root, "mod")
	base := r.commit(g, "base")
	// Deinit empties the submodule's directory and takes its .git with it, so
	// the parent check finds nothing to refuse and the path reaches git.
	r.git(g, "submodule", "deinit", "--quiet", "--force", "mod")
	r.write(g, "mod/m.go", "written into the empty submodule\n")
	r.write(g, "visible.txt", "not ignored\n")
	blobs, _ := newBlobs(g, t)
	st, err := Inspect(t.Context(), r.root, base, false)
	g.Expect(err).NotTo(HaveOccurred())
	reader := st.Reader(blobs, nil)

	// check-ignore exits 128 for a path inside a submodule. That one 128 is
	// an answer: the file belongs to another repository.
	_, _, err = reader.Open(t.Context(), "mod/m.go")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
	g.Expect(errors.Is(err, ErrRulesUnreadable)).To(BeFalse())
	rc, source, err := reader.Open(t.Context(), "visible.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceWorking))
	g.Expect(readAll(g, rc)).To(Equal("not ignored\n"))

	// The answer is read from git's message, which git translates. The read
	// pins the locale, so the daemon's own makes no difference.
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LANGUAGE", "de")
	german := exec.Command("git", "-C", r.root, "check-ignore", "-q", "--", "./mod/m.go")
	out, _ := german.CombinedOutput()
	if !strings.Contains(string(out), "Submodul") {
		t.Skipf("git does not speak German here, so the locale pin cannot be tested: %s", out)
	}
	_, _, err = reader.Open(t.Context(), "mod/m.go")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
	g.Expect(errors.Is(err, ErrRulesUnreadable)).To(BeFalse())

	// A German locale named by LC_ALL rather than LANGUAGE needs LC_ALL=C in
	// the pin, since LANGUAGE= alone leaves git speaking the locale's
	// language. It needs that locale installed to be observed.
	t.Run("German named by LC_ALL", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("LC_ALL", "de_DE.UTF-8")
		t.Setenv("LANGUAGE", "")
		german := exec.Command("git", "-C", r.root, "check-ignore", "-q", "--", "./mod/m.go")
		out, _ := german.CombinedOutput()
		if !strings.Contains(string(out), "Submodul") {
			t.Skipf("the de_DE.UTF-8 locale is not installed, so LC_ALL in the pin cannot be tested: %s", out)
		}
		_, _, err := reader.Open(t.Context(), "mod/m.go")
		g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
		g.Expect(errors.Is(err, ErrRulesUnreadable)).To(BeFalse())
	})
}

func TestReader_Open_swapped(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	root := filepath.Join(dir, "project")
	writeFile(g, root, Marker, "")
	writeFile(g, root, "sub/x.txt", "the project's\n")
	writeFile(g, root, "other.txt", "a different file\n")
	writeFile(g, dir, "outside/x.txt", "not the project's\n")
	blobs, _ := newBlobs(g, t)
	st, err := Inspect(t.Context(), root, "", false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(st.Live).To(Equal(LiveOpen))

	// read opens path with fn run at the named step of the read, which is
	// where a local process racing the read would change the disk.
	read := func(path, at string, fn func()) (io.ReadCloser, error) {
		reader := st.Reader(blobs, nil)
		reader.pause = func(step string) {
			if step == at {
				fn()
			}
		}
		rc, _, err := reader.Open(t.Context(), path)
		return rc, err
	}
	reset := func() {
		g.Expect(os.RemoveAll(filepath.Join(root, "sub"))).To(Succeed())
		writeFile(g, root, "sub/x.txt", "the project's\n")
	}

	// Left alone, the read succeeds, so each refusal below is the swap's.
	rc, err := read("sub/x.txt", "stat", func() {})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(readAll(g, rc)).To(Equal("the project's\n"))

	// A parent swapped for a link after it was checked. os.Root refuses the
	// path that now leaves the root, and the refusal is not found.
	_, err = read("sub/x.txt", "checked", func() {
		g.Expect(os.Rename(filepath.Join(root, "sub"), filepath.Join(dir, "sub.old"))).To(Succeed())
		g.Expect(os.Symlink(filepath.Join(dir, "outside"), filepath.Join(root, "sub"))).To(Succeed())
	})
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
	reset()

	// A different regular file moved over the one the Lstat saw.
	_, err = read("sub/x.txt", "stat", func() {
		writeFile(g, root, "other.txt", "a different file\n")
		g.Expect(os.Rename(filepath.Join(root, "other.txt"), filepath.Join(root, "sub", "x.txt"))).To(Succeed())
	})
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
	reset()

	// The file removed after the Lstat saw it, so the open finds nothing.
	_, err = read("sub/x.txt", "stat", func() {
		g.Expect(os.Remove(filepath.Join(root, "sub", "x.txt"))).To(Succeed())
	})
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
	reset()

	// A link removed after the Lstat saw it, so there is no target to read.
	g.Expect(os.Symlink("inside-target", filepath.Join(root, "sub", "l"))).To(Succeed())
	_, err = read("sub/l", "stat", func() {
		g.Expect(os.Remove(filepath.Join(root, "sub", "l"))).To(Succeed())
	})
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
	reset()

	// A link's parent swapped for a link to outside after the Lstat. The
	// target text of the link outside is never read.
	g.Expect(os.Symlink("inside-target", filepath.Join(root, "sub", "l"))).To(Succeed())
	g.Expect(os.Symlink("outside-target", filepath.Join(dir, "outside", "l"))).To(Succeed())
	rc, err = read("sub/l", "stat", func() {
		g.Expect(os.RemoveAll(filepath.Join(root, "sub"))).To(Succeed())
		g.Expect(os.Symlink(filepath.Join(dir, "outside"), filepath.Join(root, "sub"))).To(Succeed())
	})
	if err == nil {
		g.Expect(readAll(g, rc)).NotTo(Equal("outside-target"))
	}
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
	g.Expect(os.Remove(filepath.Join(root, "sub"))).To(Succeed())
	reset()

	// A FIFO swapped in. Opened without O_NONBLOCK it would block until a
	// writer appeared, so the read runs with a deadline and a writer is
	// opened to release it if the deadline passes.
	fifo := filepath.Join(root, "sub", "x.txt")
	done := make(chan error, 1)
	go func() {
		_, err := read("sub/x.txt", "stat", func() {
			g.Expect(os.Remove(fifo)).To(Succeed())
			g.Expect(syscall.Mkfifo(fifo, 0o600)).To(Succeed())
		})
		done <- err
	}()
	select {
	case err = <-done:
		g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
	case <-time.After(5 * time.Second):
		w, _ := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if w != nil {
			_ = w.Close()
		}
		<-done
		t.Fatal("the read blocked opening a FIFO")
	}
}

// readerRepo is a repository with one file the rounds below leave alone, plus a
// blob store to capture into.
func readerRepo(g *WithT, t *testing.T) (*repo, *blob.Store) {
	g.THelper()
	r := newRepo(g, t.TempDir())
	r.write(g, "untouched.txt", "untouched\n")
	blobs, _ := newBlobs(g, t)
	return r, blobs
}

// fileName is the nth path in the batching fixture.
func fileName(n int) string {
	return "batched/file" + strings.Repeat("0", 2-len(itoa(n))) + itoa(n) + ".txt"
}

// itoa is strconv.Itoa without the import, for the fixture above.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

// readAll drains a reader and closes it, which is where a failure that only
// shows up at the end of a stream is reported.
func readAll(g *WithT, rc io.ReadCloser) string {
	g.THelper()
	b, err := io.ReadAll(rc)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(rc.Close()).To(Succeed())
	return string(b)
}
