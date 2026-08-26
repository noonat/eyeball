package convention

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_packagesListedFlagsAnUnlistedPackage(t *testing.T) {
	g := NewWithT(t)
	root := t.TempDir()

	doc := filepath.Join(root, "docs")
	g.Expect(os.MkdirAll(doc, 0o755)).To(Succeed())
	layout := "```\ninternal/store/       the database\n```\n"
	docPath := filepath.Join(doc, "architecture.md")
	g.Expect(os.WriteFile(docPath, []byte(layout), 0o644)).To(Succeed())

	pkgs := []string{"store", "capture"}
	for _, pkg := range pkgs {
		dir := filepath.Join(root, "internal", pkg)
		g.Expect(os.MkdirAll(dir, 0o755)).To(Succeed())
		src := filepath.Join(dir, "doc.go")
		g.Expect(os.WriteFile(src, []byte("package x\n"), 0o644)).To(Succeed())
	}

	found, err := packagesListedIn(root)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(HaveLen(1))
	g.Expect(found[0].What).To(ContainSubstring("internal/capture"))
}
