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
	"github.com/noonat/eyeball/internal/diff"
	"github.com/noonat/eyeball/internal/store"
)

func Test_freezeFileRefusesAVanishedPath(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	blobs, _ := newBlobs(g, t)

	// The enumeration lstats every path, so this is the narrower gap between
	// that stat and the read. Recording it as deleted would put a deletion the
	// agent did not make into a frozen round.
	_, err := freezeFile(dir, blobs, Entry{Path: "gone.txt"})
	g.Expect(errors.Is(err, ErrVanished)).To(BeTrue())
	g.Expect(err.Error()).To(ContainSubstring("gone.txt"))
}

func Test_storeContentDoesNotReadTwiceOnAHit(t *testing.T) {
	g := NewWithT(t)
	blobs, _ := newBlobs(g, t)
	_, _, err := blobs.Put(strings.NewReader("already held"))
	g.Expect(err).NotTo(HaveOccurred())
	reads := 0
	next := func() (io.ReadCloser, error) {
		reads++
		return io.NopCloser(strings.NewReader("already held")), nil
	}

	// Put refuses to duplicate content on its own, so no new blob appears
	// either way. What asking Has first saves is the second read of the file
	// and the temporary Put writes and then removes, which is the whole cost
	// of recapturing a file that has not changed since the last round.
	got, err := storeContent(blobs, "a.txt", next)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(reads).To(Equal(1))
	g.Expect(readBlob(g, blobs, got.Digest)).To(Equal("already held"))
}

func Test_storeContentRecordsTheDigestOfWhatWasStored(t *testing.T) {
	g := NewWithT(t)
	blobs, _ := newBlobs(g, t)
	reads := 0
	// A file rewritten between the hashing pass and the write. Only the second
	// read reaches the store, so only its digest describes what the store now
	// holds, and recording the first would name content that is not there.
	next := func() (io.ReadCloser, error) {
		reads++
		if reads == 1 {
			return io.NopCloser(strings.NewReader("before")), nil
		}
		return io.NopCloser(strings.NewReader("after the rewrite")), nil
	}

	got, err := storeContent(blobs, "a.txt", next)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(reads).To(Equal(2))
	g.Expect(got.Size).To(Equal(int64(len("after the rewrite"))))
	g.Expect(readBlob(g, blobs, got.Digest)).To(Equal("after the rewrite"))
}

func TestFreeze(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "keep.txt", "keep\n")
	r.write(g, "edit.txt", "before\n")
	r.write(g, "gone.txt", "gone\n")
	base := r.commit(g, "base")
	r.write(g, "edit.txt", "after\n")
	g.Expect(os.Remove(filepath.Join(r.root, "gone.txt"))).To(Succeed())
	r.write(g, "added.txt", "added\n")
	blobs, _ := newBlobs(g, t)

	got, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.BaseCommit).To(Equal(base))
	// The note is the caller's to fill. The store refuses a request without
	// one, so forgetting fails loudly rather than recording a round with no
	// brief.
	g.Expect(got.Note).To(BeEmpty())
	g.Expect(paths(got.Files)).To(Equal([]string{"added.txt", "edit.txt", "gone.txt"}))
	g.Expect(content(g, blobs, got.Files, "added.txt")).To(Equal("added\n"))
	g.Expect(content(g, blobs, got.Files, "edit.txt")).To(Equal("after\n"))

	// Every digest a round names has to be readable, or the round cannot be
	// displayed at all.
	for _, f := range got.Files {
		if f.Digest == "" {
			continue
		}
		held, hasErr := blobs.Has(f.Digest)
		g.Expect(hasErr).NotTo(HaveOccurred())
		g.Expect(held).To(BeTrue())
	}
}

func TestFreeze_baseGainedACommit(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	r.git(g, "add", "-A")
	blobs, _ := newBlobs(g, t)

	first, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(first.BaseCommit).To(BeEmpty())

	r.commit(g, "first commit")
	r.write(g, "b.txt", "b\n")

	// An empty base is the empty tree, and what that is compared against grows
	// as the repository does. A second round here would capture every tracked
	// file rather than the agent's work.
	previous := []store.File{{Path: "a.txt", Digest: first.Files[0].Digest}}
	_, err = Freeze(t.Context(), Options{Round: 2, Root: r.root, Previous: previous, Blobs: blobs})
	g.Expect(errors.Is(err, ErrBaseGained)).To(BeTrue())

	// And a round whose own previous round captured nothing, which the store
	// hands back as nil. Keyed on that being nil, the guard would read this as
	// a first round and let it capture every tracked file in the repository.
	_, err = Freeze(t.Context(), Options{Round: 3, Root: r.root, Previous: nil, Blobs: blobs})
	g.Expect(errors.Is(err, ErrBaseGained)).To(BeTrue())
}

func TestFreeze_deletedFile(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	r.write(g, "gone.txt", "gone\n")
	base := r.commit(g, "base")
	g.Expect(os.Remove(filepath.Join(r.root, "gone.txt"))).To(Succeed())
	blobs, _ := newBlobs(g, t)

	// An empty digest is what keeps a deletion distinguishable from a file
	// that never changed.
	got, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Files).To(Equal([]store.File{{Path: "gone.txt"}}))
}

func TestFreeze_emptyFirstRound(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	base := r.commit(g, "base")
	blobs, _ := newBlobs(g, t)

	// Nothing differs from the base, so the agent named the wrong paths,
	// forgot to write a file, or based the review on its own work.
	_, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(errors.Is(err, ErrNothingCaptured)).To(BeTrue())
	g.Expect(err.Error()).To(ContainSubstring(base))
}

func TestFreeze_emptyLaterRound(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	base := r.commit(g, "base")
	r.write(g, "a.txt", "edited\n")
	blobs, _ := newBlobs(g, t)
	first, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	r.write(g, "a.txt", "a\n")

	// The working tree matches the base again, which is a revert rather than a
	// quiet round. It is allowed, because the round shows the reverts and the
	// note says why, and refusing would discard the note.
	got, err := Freeze(t.Context(), Options{Round: 2,
		Root:     r.root,
		Base:     base,
		Previous: first.Files,
		Blobs:    blobs,
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Files).To(BeEmpty())
	g.Expect(got.BaseCommit).To(Equal(base))
}

func TestFreeze_laterRoundAfterAnEmptyOne(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "original\n")
	base := r.commit(g, "base")
	r.write(g, "a.txt", "edited\n")
	blobs, _ := newBlobs(g, t)
	first, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())

	// Round two reverts the work, so it captures nothing, which is allowed.
	r.write(g, "a.txt", "original\n")
	second, err := Freeze(t.Context(), Options{
		Round:    2,
		Root:     r.root,
		Base:     base,
		Previous: first.Files,
		Blobs:    blobs,
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(second.Files).To(BeEmpty())

	// Round three reads that empty capture back as its previous round. The
	// store returns nil for a round that captured nothing, so nil is what a
	// caller hands over here. Keyed on the file list rather than on the round
	// number, this is taken for a first round and refused for capturing
	// nothing, even though round two was allowed to.
	g.Expect(second.Files).To(BeEmpty())
	third, err := Freeze(t.Context(), Options{
		Round:    3,
		Root:     r.root,
		Base:     base,
		Previous: nil,
		Blobs:    blobs,
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(third.Files).To(BeEmpty())
	g.Expect(third.Changes).To(BeEmpty())
}

func TestFreeze_measuresAFileTooLargeToDiff(t *testing.T) {
	sizes := []struct {
		name    string
		content string
	}{
		{
			name: "past the byte limit",
			// Never read: one byte over is enough to know the diff refuses it.
			content: strings.Repeat("x", diff.MaxBytes+1),
		},
		{
			name: "past the line limit",
			// Read, and refused by the diff on lines rather than bytes. Its own
			// answer there is the length of each side, which as a round's size
			// would read as this file moving every line it has.
			content: strings.Repeat("line\n", 25000),
		},
	}
	for _, c := range sizes {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			r := newRepo(g, t.TempDir())
			r.write(g, "small.txt", "small\n")
			base := r.commit(g, "base")
			r.write(g, "big.txt", c.content)
			blobs, _ := newBlobs(g, t)

			// The file moved, and how much is not known. Both limits say the
			// same thing, which is what keeps two refusals from reporting
			// opposite numbers.
			got, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got.Changes).To(Equal([]store.Change{{Path: "big.txt"}}))
		})
	}
}

func TestFreeze_measuresAFirstRoundAgainstAnEmptyBase(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	writeFile(g, dir, "docs/a.md", "one\ntwo\nthree\n")
	blobs, _ := newBlobs(g, t)

	// Nothing to measure against, so every line of every file is an addition.
	got, err := Freeze(t.Context(), Options{Round: 1, Root: dir, Paths: []string{"docs"}, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Changes).To(Equal([]store.Change{{Path: "docs/a.md", Added: 3}}))
}

func TestFreeze_measuresAModeOnlyChange(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "run.sh", "echo hi\n")
	base := r.commit(g, "base")
	g.Expect(os.Chmod(filepath.Join(r.root, "run.sh"), 0o700)).To(Succeed())
	blobs, _ := newBlobs(g, t)

	// git says the file differs, and its content does not. It moved no lines
	// and it is still a file that moved, which is what keeps a first round's
	// file count equal to the number of files it captured.
	got, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(paths(got.Files)).To(Equal([]string{"run.sh"}))
	g.Expect(got.Changes).To(Equal([]store.Change{{Path: "run.sh"}}))
}

func TestFreeze_measuresARevertedFile(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "original\n")
	base := r.commit(g, "base")
	r.write(g, "a.txt", "edited\n")
	blobs, _ := newBlobs(g, t)
	first, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())

	r.write(g, "a.txt", "original\n")
	second, err := Freeze(t.Context(), Options{Round: 2,
		Root:     r.root,
		Base:     base,
		Previous: first.Files,
		Blobs:    blobs,
	})
	g.Expect(err).NotTo(HaveOccurred())

	// The file matches the base again, so the capture holds nothing for it. It
	// still moved since the last round, which is why the size is not a count
	// of what the round captured.
	g.Expect(second.Files).To(BeEmpty())
	g.Expect(second.Changes).To(Equal([]store.Change{{Path: "a.txt", Added: 1, Removed: 1}}))
}

func TestFreeze_measuresAnUnchangedRound(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "original\n")
	base := r.commit(g, "base")
	r.write(g, "a.txt", "edited\n")
	blobs, _ := newBlobs(g, t)
	first, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())

	// The agent answered the verdict with a note and no edit. The capture is
	// the same as last time, so nothing moved, and no file is read to find out.
	second, err := Freeze(t.Context(), Options{Round: 2,
		Root:     r.root,
		Base:     base,
		Previous: first.Files,
		Blobs:    blobs,
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(second.Files).To(Equal(first.Files))
	g.Expect(second.Changes).To(BeEmpty())
}

func TestFreeze_measuresOneFileOfSix(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	names := []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt", "f.txt"}
	for _, name := range names {
		r.write(g, name, "before\n")
	}
	base := r.commit(g, "base")
	for _, name := range names {
		r.write(g, name, "after\n")
	}
	blobs, _ := newBlobs(g, t)
	first, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(first.Changes).To(HaveLen(6))

	r.write(g, "c.txt", "after, and again\n")
	second, err := Freeze(t.Context(), Options{Round: 2,
		Root:     r.root,
		Base:     base,
		Previous: first.Files,
		Blobs:    blobs,
	})
	g.Expect(err).NotTo(HaveOccurred())

	// All six still differ from the base, so the capture holds all six. One
	// moved since the last round, and that is the change the round opens on.
	g.Expect(second.Files).To(HaveLen(6))
	g.Expect(second.Changes).To(Equal([]store.Change{{Path: "c.txt", Added: 1, Removed: 1}}))
}

func TestFreeze_measuresWhatAnEmptyLaterRoundLost(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "original\n")
	base := r.commit(g, "base")
	r.write(g, "a.txt", "edited\n")
	r.write(g, "added.txt", "brand new\n")
	blobs, _ := newBlobs(g, t)
	first, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())

	r.write(g, "a.txt", "original\n")
	g.Expect(os.Remove(filepath.Join(r.root, "added.txt"))).To(Succeed())
	second, err := Freeze(t.Context(), Options{Round: 2,
		Root:     r.root,
		Base:     base,
		Previous: first.Files,
		Blobs:    blobs,
	})
	g.Expect(err).NotTo(HaveOccurred())

	// The working tree matches the base again, so the capture is empty. Every
	// path the previous round held still moved, which is what the round shows.
	g.Expect(second.Files).To(BeEmpty())
	g.Expect(second.Changes).To(Equal([]store.Change{
		{Path: "a.txt", Added: 1, Removed: 1},
		{Path: "added.txt", Removed: 1},
	}))
}

func TestFreeze_needsItsRoundNumber(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	base := r.commit(g, "base")
	blobs, _ := newBlobs(g, t)

	// A later round may capture nothing, so an empty file list cannot say
	// which round this is. Leaving it out fails here rather than treating a
	// third round as a first one.
	_, err := Freeze(t.Context(), Options{Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("round number"))
}

func TestFreeze_noGit(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	writeFile(g, dir, "docs/a.md", "a\n")
	writeFile(g, dir, "outside.txt", "no\n")
	blobs, _ := newBlobs(g, t)

	// Without git there is nothing to ask what changed, so the review's paths
	// are the whole of the scope and the base is empty.
	got, err := Freeze(t.Context(), Options{Round: 1, Root: dir, Paths: []string{"docs"}, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.BaseCommit).To(BeEmpty())
	g.Expect(paths(got.Files)).To(Equal([]string{"docs/a.md"}))
}

func TestFreeze_readsNothingOutsideTheProject(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	writeFile(g, dir, "project/docs/a.md", "mine\n")
	writeFile(g, dir, "secrets/key.txt", "not mine\n")
	blobs, dir2 := newBlobs(g, t)
	root := filepath.Join(dir, "project")

	// The review's paths have not been through the store yet. Without a check
	// here the walk reads files outside the project into the blob store, and
	// the store's refusal comes after the content is already written.
	_, err := Freeze(t.Context(), Options{
		Round: 1,
		Root:  root,
		Paths: []string{"../secrets"},
		Blobs: blobs,
	})
	g.Expect(errors.Is(err, ErrNoSuchPath)).To(BeTrue())
	g.Expect(stored(g, dir2)).To(BeZero())
}

func TestFreeze_recaptureWritesNoNewBlob(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	base := r.commit(g, "base")
	r.write(g, "a.txt", "changed\n")
	r.write(g, "b.txt", "new\n")
	blobs, dir := newBlobs(g, t)

	first, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	after := stored(g, dir)

	// Round two recaptures every file that still differs from the base, and
	// most are byte for byte what round one captured. Has is what makes that
	// cost a read rather than a copy and two fsyncs.
	second, err := Freeze(t.Context(), Options{Round: 2, Root: r.root, Base: base, Previous: first.Files, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(second.Files).To(Equal(first.Files))
	g.Expect(stored(g, dir)).To(Equal(after))
}

func TestFreeze_symlink(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "target.txt", "the target's bytes\n")
	base := r.commit(g, "base")
	g.Expect(os.Symlink("target.txt", filepath.Join(r.root, "a.link"))).To(Succeed())
	blobs, _ := newBlobs(g, t)

	// The content of a link is the path it names. Following it would capture
	// the target's bytes, which is not what the agent added.
	got, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(paths(got.Files)).To(Equal([]string{"a.link"}))
	g.Expect(content(g, blobs, got.Files, "a.link")).To(Equal("target.txt"))
	g.Expect(got.Files[0].Size).To(Equal(int64(len("target.txt"))))
}

// newBlobs opens a blob store for one test, returning the directory as well,
// because the store does not say where it keeps its files.
func newBlobs(g *WithT, t *testing.T) (*blob.Store, string) {
	g.THelper()
	dir := t.TempDir()
	blobs, err := blob.Open(dir)
	g.Expect(err).NotTo(HaveOccurred())
	return blobs, dir
}

// paths lists a round's paths, which is what most assertions here are about.
func paths(files []store.File) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

// readBlob reads back what the store holds at a digest.
func readBlob(g *WithT, blobs *blob.Store, digest string) string {
	g.THelper()
	r, err := blobs.Open(digest)
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { _ = r.Close() }()
	b, err := io.ReadAll(r)
	g.Expect(err).NotTo(HaveOccurred())
	return string(b)
}

// content reads back what a round stored for one path.
func content(g *WithT, blobs *blob.Store, files []store.File, path string) string {
	g.THelper()
	for _, f := range files {
		if f.Path != path {
			continue
		}
		r, err := blobs.Open(f.Digest)
		g.Expect(err).NotTo(HaveOccurred())
		defer func() { _ = r.Close() }()
		b, err := io.ReadAll(r)
		g.Expect(err).NotTo(HaveOccurred())
		return string(b)
	}
	g.Expect(path).To(BeEmpty(), "the round holds no such path")
	return ""
}

// stored counts the files under a blob store's directory, excluding the
// temporaries, so a test can tell a write from a hit.
func stored(g *WithT, dir string) int {
	g.THelper()
	count := 0
	err := filepath.WalkDir(dir, func(name string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "tmp" {
				return filepath.SkipDir
			}
			return nil
		}
		count++
		return nil
	})
	g.Expect(err).NotTo(HaveOccurred())
	return count
}
