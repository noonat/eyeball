package viewdoc

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// importers lists the files, as paths from the module root, whose imports name
// this package, given each file's text.
func importers(g *WithT, sources map[string]string) []string {
	g.THelper()
	var out []string
	for name, text := range sources {
		file, err := parser.ParseFile(token.NewFileSet(), name, text, parser.ImportsOnly)
		g.Expect(err).NotTo(HaveOccurred(), name)
		for _, imp := range file.Imports {
			if imp.Path.Value == `"github.com/noonat/eyeball/internal/viewdoc"` {
				out = append(out, name)
			}
		}
	}
	return out
}

func Test_importersFindsAnImport(t *testing.T) {
	g := NewWithT(t)
	sources := map[string]string{
		"a.go": "package a\n\nimport \"github.com/noonat/eyeball/internal/viewdoc\"\n",
		"b.go": "package b\n\nimport \"fmt\"\n",
	}
	g.Expect(importers(g, sources)).To(Equal([]string{"a.go"}))
}

func Test_onlyTestsImportIt(t *testing.T) {
	g := NewWithT(t)
	sources := map[string]string{}
	err := filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "testdata" || d.Name() == ".git") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		text, err := os.ReadFile(path)
		sources[path] = string(text)
		return err
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(sources).To(HaveKey(filepath.Join("..", "..", "internal", "server", "surface.go")))
	g.Expect(importers(g, sources)).To(BeEmpty(), "the daemon draws no screen as HTML")
}
