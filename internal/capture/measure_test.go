package capture

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/noonat/eyeball/internal/store"
)

func Test_measureAsksTheBaseAboutEveryPathAtOnce(t *testing.T) {
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
	req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(req.Changes).To(HaveLen(6))

	// On a first round every path resolves from the base, so asking per path
	// is one git process per captured file. One call answers for all of them.
	// Counted through measure itself, so dropping its pre-pass shows up here
	// rather than only in how long a capture takes.
	before := NewReader(r.root, base, blobs, nil)
	now := NewReader(r.root, base, blobs, req.Files)
	changes, err := measure(t.Context(), before, now)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(changes).To(HaveLen(6))
	g.Expect(before.lookups).To(Equal(1))
	g.Expect(now.lookups).To(Equal(1))
}

func Test_movedSkipsAnUnchangedDigest(t *testing.T) {
	g := NewWithT(t)
	// Most of a later round's capture is files that still differ from the base
	// and have not moved since the last round. Skipping them on the digest is
	// what keeps a round from reading every file it holds.
	previous := []store.File{
		{Path: "same.txt", Digest: "aaa"},
		{Path: "edited.txt", Digest: "bbb"},
	}
	current := []store.File{
		{Path: "same.txt", Digest: "aaa"},
		{Path: "edited.txt", Digest: "ccc"},
	}

	g.Expect(moved(previous, current)).To(Equal([]string{"edited.txt"}))
}

func Test_movedSortsAndDoesNotRepeat(t *testing.T) {
	g := NewWithT(t)
	previous := []store.File{{Path: "b.txt", Digest: "1"}, {Path: "c.txt", Digest: "1"}}
	current := []store.File{{Path: "a.txt", Digest: "2"}, {Path: "b.txt", Digest: "2"}}

	// c.txt is only on the old side and a.txt only on the new, and b.txt is on
	// both with different content. Each is named once, in path order.
	g.Expect(moved(previous, current)).To(Equal([]string{"a.txt", "b.txt", "c.txt"}))
}

func Test_movedTellsAbsentFromDeleted(t *testing.T) {
	g := NewWithT(t)
	// A capture holds what differs from the base, so a path it does not name
	// matches the base. That is not the same as a path it names as deleted,
	// and folding the two together would miss a file removed and put back.
	previous := []store.File{{Path: "gone.txt"}}
	current := []store.File{}

	g.Expect(moved(previous, current)).To(Equal([]string{"gone.txt"}))
}

func TestComparePaths(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	names := []string{"a.txt", "b.txt", "c.txt", "d.txt"}
	for _, name := range names {
		r.write(g, name, "before\n")
	}
	base := r.commit(g, "base")
	for _, name := range names {
		r.write(g, name, "after\nand more\n")
	}
	blobs, _ := newBlobs(g, t)
	req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	st, err := Inspect(t.Context(), r.root, base, false)
	g.Expect(err).NotTo(HaveOccurred())

	// Only the paths asked about, in the order asked, with their object ids
	// loaded in one call per reader rather than one per path.
	before := st.Reader(blobs, nil)
	now := st.Reader(blobs, req.Files)
	got, err := ComparePaths(t.Context(), before, now, []string{"c.txt", "a.txt"})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(HaveLen(2))
	g.Expect(got[0].Path).To(Equal("c.txt"))
	g.Expect(got[1].Path).To(Equal("a.txt"))
	g.Expect(before.lookups).To(Equal(1))
	g.Expect(now.lookups).To(Equal(1))

	// The counts are Compare's for the same paths.
	all, err := Compare(t.Context(), st.Reader(blobs, nil), st.Reader(blobs, req.Files))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(all).To(HaveLen(4))
	g.Expect(got[0]).To(Equal(all[2]))
	g.Expect(got[1]).To(Equal(all[0]))
	g.Expect(got[0].Result.Added).To(Equal(2))
	g.Expect(got[0].Result.Removed).To(Equal(1))
}
