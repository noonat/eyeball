package capture

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"
)

func TestWalk(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	writeFile(g, dir, "docs/a.md", "a\n")
	writeFile(g, dir, "docs/deep/b.md", "b\n")
	writeFile(g, dir, "src/c.go", "c\n")
	writeFile(g, dir, "outside.txt", "no\n")

	// Every file under the paths the review named, and nothing outside them.
	// A project with no git cannot be asked what changed, so naming the paths
	// is the whole of the scope.
	got, err := Walk(t.Context(), dir, []string{"docs", "src/c.go"})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{
		{Path: "docs/a.md"},
		{Path: "docs/deep/b.md"},
		{Path: "src/c.go"},
	}))
}

func TestWalk_missingPath(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	writeFile(g, dir, "docs/a.md", "a\n")

	// A review naming two paths where one is a typo would otherwise capture
	// the other and report nothing about it.
	_, err := Walk(t.Context(), dir, []string{"docs", "spec"})
	g.Expect(errors.Is(err, ErrNoSuchPath)).To(BeTrue())
	g.Expect(err.Error()).To(ContainSubstring("spec"))
}

func TestWalk_nestedRepository(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	writeFile(g, dir, "docs/a.md", "a\n")
	writeFile(g, dir, "docs/.git/objects/pack/x.idx", "binary\n")
	writeFile(g, dir, "docs/.git/HEAD", "ref: refs/heads/main\n")

	// A project named by a marker file can still contain a repository, and its
	// object store is not review material.
	got, err := Walk(t.Context(), dir, []string{"docs"})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{{Path: "docs/a.md"}}))
}

func TestWalk_skipsWhatCannotBeRead(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	writeFile(g, dir, "docs/a.md", "a\n")
	listener, err := net.Listen("unix", filepath.Join(dir, "docs", "dev.sock"))
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = listener.Close() })

	// A development server leaves one of these in a working tree, and opening
	// it is not a read that returns bytes.
	got, walkErr := Walk(t.Context(), dir, []string{"docs"})
	g.Expect(walkErr).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{{Path: "docs/a.md"}}))
}

func TestWalk_symlinks(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	writeFile(g, dir, "docs/a.md", "a\n")
	writeFile(g, dir, "elsewhere/big.md", "big\n")
	g.Expect(os.Symlink("a.md", filepath.Join(dir, "docs", "toFile.link"))).To(Succeed())
	g.Expect(os.Symlink(filepath.Join(dir, "elsewhere"), filepath.Join(dir, "docs", "toDir.link"))).To(Succeed())

	// A link is captured as the target it names. Descending through the one
	// pointing at a directory would pull in a tree the review never named, and
	// a link pointing back up its own path would not terminate.
	got, err := Walk(t.Context(), dir, []string{"docs"})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{
		{Path: "docs/a.md"},
		{Path: "docs/toDir.link", Symlink: true},
		{Path: "docs/toFile.link", Symlink: true},
	}))
}

func TestWalk_tooManyFiles(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	for i := 0; i <= walkLimit; i++ {
		writeFile(g, dir, fmt.Sprintf("vendor/f%04d.txt", i), "x\n")
	}

	// Nothing outside git says what is ignored, so a review naming a directory
	// with a dependency tree under it would otherwise capture all of it.
	_, err := Walk(t.Context(), dir, []string{"vendor"})
	g.Expect(errors.Is(err, ErrTooManyFiles)).To(BeTrue())
	g.Expect(err.Error()).To(ContainSubstring("vendor"))
}

// writeFile puts content at a slash-separated path under dir, creating the
// directories above it.
func writeFile(g *WithT, dir, path, content string) {
	g.THelper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	g.Expect(os.MkdirAll(filepath.Dir(full), 0o700)).To(Succeed())
	g.Expect(os.WriteFile(full, []byte(content), 0o600)).To(Succeed())
}
