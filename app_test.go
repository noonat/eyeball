package eyeball

import (
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	. "github.com/onsi/gomega"
)

// repoFS is the repository's files on disk, from its root.
func repoFS(g *WithT) fs.FS {
	_, err := os.Stat("go.mod")
	g.Expect(err).NotTo(HaveOccurred(), "tests of this package run in the repository root")
	return os.DirFS(".")
}

func TestApp(t *testing.T) {
	g := NewWithT(t)
	out, err := App()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(out.Script).NotTo(BeEmpty())
	g.Expect(string(out.Script)).NotTo(ContainSubstring("process.env"), "the production define was not applied")

	// The shared scripts are staged beside the sources, so the app's calls into
	// them are bundled and not left to resolve at run time.
	g.Expect(string(out.Script)).To(ContainSubstring("eyeball:blink"))
	g.Expect(string(out.Script)).To(ContainSubstring("prefers-color-scheme"))
	g.Expect(string(out.Script)).To(ContainSubstring("eyeball-build"))
}

func TestBuildApp_missingPackage(t *testing.T) {
	g := NewWithT(t)
	deps, err := fs.Sub(Deps, "web/deps/node_modules")
	g.Expect(err).NotTo(HaveOccurred())

	// The vendored React is what the bundle imports. With it gone the build
	// fails, so a bundle that built did not borrow React from anywhere else.
	without := fstest.MapFS{}
	err = fs.WalkDir(deps, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || strings.HasPrefix(path, "react/") {
			return err
		}
		body, err := fs.ReadFile(deps, path)
		without[path] = &fstest.MapFile{Data: body}
		return err
	})
	g.Expect(err).NotTo(HaveOccurred())
	_, err = BuildApp(without)
	g.Expect(err).To(MatchError(ContainSubstring("react")))
}
