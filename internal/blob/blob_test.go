package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"
)

func Test_tmpNameCannotBeAFanOutName(t *testing.T) {
	g := NewWithT(t)
	// A fan-out directory is named by two hex characters, so a temporary
	// directory holding a non-hex character can never be mistaken for one.
	g.Expect(tmpName).NotTo(BeEmpty())
	g.Expect(strings.ContainsAny(tmpName, "ghijklmnopqrstuvwxyz")).To(BeTrue())
}

func TestOpen(t *testing.T) {
	g := NewWithT(t)
	dir := filepath.Join(t.TempDir(), "does", "not", "exist")

	store, err := Open(dir)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(store).NotTo(BeNil())
	g.Expect(filepath.Join(dir, tmpName)).To(BeADirectory())
}

func TestStore_invalidDigest(t *testing.T) {
	digests := []struct {
		name   string
		digest string
	}{
		{
			name:   "path separator",
			digest: "../" + strings.Repeat("a", digestLen-3),
		},
		{
			name:   "uppercase digit",
			digest: strings.Repeat("A", digestLen),
		},
		{
			name:   "too short",
			digest: strings.Repeat("a", digestLen-1),
		},
		{
			name:   "too long",
			digest: strings.Repeat("a", digestLen+1),
		},
		{
			name:   "empty",
			digest: "",
		},
		{
			name:   "non-hex letter",
			digest: strings.Repeat("g", digestLen),
		},
	}
	for _, c := range digests {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			store := newStore(g, t.TempDir())

			_, openErr := store.Open(c.digest)
			g.Expect(errors.Is(openErr, ErrInvalidDigest)).To(BeTrue())

			_, statErr := store.Stat(c.digest)
			g.Expect(errors.Is(statErr, ErrInvalidDigest)).To(BeTrue())

			_, hasErr := store.Has(c.digest)
			g.Expect(errors.Is(hasErr, ErrInvalidDigest)).To(BeTrue())
		})
	}
}

func TestStore_Has(t *testing.T) {
	g := NewWithT(t)
	store := newStore(g, t.TempDir())

	absent, err := store.Has(sum("nothing stored under this"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(absent).To(BeFalse())

	digest, _, err := store.Put(strings.NewReader("stored"))
	g.Expect(err).NotTo(HaveOccurred())

	present, err := store.Has(digest)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(present).To(BeTrue())
}

func TestStore_Open(t *testing.T) {
	g := NewWithT(t)
	store := newStore(g, t.TempDir())
	content := "a file the agent changed\n"

	digest, _, err := store.Put(strings.NewReader(content))
	g.Expect(err).NotTo(HaveOccurred())

	f, err := store.Open(digest)
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { _ = f.Close() }()

	got, err := io.ReadAll(f)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(got)).To(Equal(content))
}

func TestStore_Open_absent(t *testing.T) {
	g := NewWithT(t)
	store := newStore(g, t.TempDir())

	_, err := store.Open(sum("never stored"))
	g.Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
	g.Expect(errors.Is(err, ErrInvalidDigest)).To(BeFalse())
}

func TestStore_Put(t *testing.T) {
	g := NewWithT(t)
	root := t.TempDir()
	store := newStore(g, root)
	content := "the content of a captured file"

	digest, size, err := store.Put(strings.NewReader(content))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(digest).To(Equal(sum(content)))
	g.Expect(size).To(Equal(int64(len(content))))

	// The fan-out is the first two pairs of the digest, and the file is named
	// by the whole of it.
	path := filepath.Join(root, digest[0:2], digest[2:4], digest)
	g.Expect(path).To(BeARegularFile())
	g.Expect(os.ReadFile(path)).To(Equal([]byte(content)))
	g.Expect(tempFiles(g, root)).To(BeEmpty())
}

func TestStore_Put_contentAlreadyHeld(t *testing.T) {
	g := NewWithT(t)
	root := t.TempDir()
	store := newStore(g, root)
	content := "a file that did not change between rounds"

	digest, _, err := store.Put(strings.NewReader(content))
	g.Expect(err).NotTo(HaveOccurred())
	path := filepath.Join(root, digest[0:2], digest[2:4], digest)
	first, err := os.Stat(path)
	g.Expect(err).NotTo(HaveOccurred())

	// Storing the same bytes again leaves the file that is already there, so
	// re-capturing an unchanged file does not rewrite it.
	again, size, err := store.Put(strings.NewReader(content))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(again).To(Equal(digest))
	g.Expect(size).To(Equal(int64(len(content))))

	second, err := os.Stat(path)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(os.SameFile(first, second)).To(BeTrue())
	g.Expect(second.ModTime()).To(Equal(first.ModTime()))
	g.Expect(tempFiles(g, root)).To(BeEmpty())
}

func TestStore_Put_readerFails(t *testing.T) {
	g := NewWithT(t)
	root := t.TempDir()
	store := newStore(g, root)
	failure := errors.New("the file went away mid-read")

	_, _, err := store.Put(io.MultiReader(strings.NewReader("half a file"), failedReader{err: failure}))
	g.Expect(errors.Is(err, failure)).To(BeTrue())

	// Nothing partial is left where a digest could name it, and the temporary
	// is gone rather than accumulating.
	g.Expect(tempFiles(g, root)).To(BeEmpty())
	g.Expect(storedFiles(g, root)).To(BeEmpty())
}

func TestStore_Stat(t *testing.T) {
	g := NewWithT(t)
	store := newStore(g, t.TempDir())
	content := "measured rather than assumed"

	digest, _, err := store.Put(strings.NewReader(content))
	g.Expect(err).NotTo(HaveOccurred())

	size, err := store.Stat(digest)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(size).To(Equal(int64(len(content))))
}

// failedReader returns err instead of bytes, standing in for a file that is
// unreadable partway through a capture.
type failedReader struct {
	err error
}

// Read reports the failure this reader was made to produce.
func (r failedReader) Read([]byte) (int, error) {
	return 0, r.err
}

// newStore opens a store under dir and fails the test if it cannot.
func newStore(g *WithT, dir string) *Store {
	g.THelper()
	store, err := Open(dir)
	g.Expect(err).NotTo(HaveOccurred())
	return store
}

// sum is the digest Put would return for content, derived here rather than
// taken from the code under test.
func sum(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// storedFiles lists the blobs under root, which is everything outside the
// temporary directory.
func storedFiles(g *WithT, dir string) []string {
	g.THelper()
	return walk(g, dir, false)
}

// tempFiles lists what is left in the store's temporary directory.
func tempFiles(g *WithT, dir string) []string {
	g.THelper()
	return walk(g, filepath.Join(dir, tmpName), true)
}

// walk lists the regular files under dir. Outside the temporary directory it is
// skipped, so a caller asking for blobs does not count partial writes.
func walk(g *WithT, dir string, temp bool) []string {
	g.THelper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if !temp && d.Name() == tmpName {
				return filepath.SkipDir
			}
			return nil
		}
		out = append(out, path)
		return nil
	})
	g.Expect(err).NotTo(HaveOccurred())
	return out
}
