package capture

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func Test_safePathRefusesWhatWouldReadOutside(t *testing.T) {
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
	}
	for _, c := range paths {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := safePath(c.in)
			if c.want == "" {
				g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(c.want))
		})
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
}

func TestReader_Open_deletedPathStaysDeleted(t *testing.T) {
	g := NewWithT(t)
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

func TestReader_Open_notARegularFile(t *testing.T) {
	g := NewWithT(t)
	r, blobs := readerRepo(g, t)
	base := r.commit(g, "base")
	reader := NewReader(r.root, base, blobs, nil)
	g.Expect(os.MkdirAll(filepath.Join(r.root, "adir"), 0o700)).To(Succeed())

	_, _, err := reader.Open(t.Context(), "adir")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
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
