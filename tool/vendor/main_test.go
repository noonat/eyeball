package main

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/noonat/eyeball"
	"github.com/noonat/eyeball/internal/appbuild"
)

func Test_packageJSON(t *testing.T) {
	g := NewWithT(t)
	g.Expect(packageJSON("web/node_modules/react/cjs/react.production.js")).To(Equal("web/node_modules/react/package.json"))
	g.Expect(packageJSON("web/node_modules/@tanstack/query-core/build/x.js")).To(Equal("web/node_modules/@tanstack/query-core/package.json"))
	g.Expect(packageJSON("web/node_modules/a/node_modules/b/index.js")).To(Equal("web/node_modules/a/node_modules/b/package.json"))
}

func TestCompare(t *testing.T) {
	g := NewWithT(t)
	committed := t.TempDir()
	fresh := t.TempDir()
	write := func(dir, name, body string) {
		path := filepath.Join(dir, name)
		g.Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
		g.Expect(os.WriteFile(path, []byte(body), 0o644)).To(Succeed())
	}
	write(committed, "a/index.js", "one")
	write(fresh, "a/index.js", "one")
	diffs, err := Compare(committed, fresh)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(diffs).To(BeEmpty())

	write(committed, "a/gone.js", "x")
	write(fresh, "a/index.js", "two")
	write(fresh, "b/new.js", "y")
	diffs, err = Compare(committed, fresh)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(diffs).To(ConsistOf(
		"  differs: a/index.js",
		"  missing from the commit: b/new.js",
		"  only committed: a/gone.js",
	))
}

// A copy staged the way the binary stages it bundles to the same bytes as
// node_modules does. That catches a missing package.json and the module-type
// flag, which change the bytes without failing the build. A file the copy lacks
// altogether fails the build instead. The committed tree is held by
// TestVendor_committedTree and make vendor-check.
func TestVendor(t *testing.T) {
	g := NewWithT(t)
	root, err := repoRoot()
	g.Expect(err).NotTo(HaveOccurred())
	_, err = os.Stat(filepath.Join(root, modulesDir))
	g.Expect(err).NotTo(HaveOccurred(), "web/node_modules is missing. Run: make tools")

	fresh := t.TempDir()
	g.Expect(Vendor(root, fresh)).To(Succeed())
	want, err := appbuild.Build(root, nil)
	g.Expect(err).NotTo(HaveOccurred())
	got, err := eyeball.BuildApp(os.DirFS(fresh))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Script).To(Equal(want.Script))
}

// The committed tree is what a fresh run gives. This is what make check runs,
// and a vendored file deleted or an import that needs a new one makes it differ.
func TestVendor_committedTree(t *testing.T) {
	g := NewWithT(t)
	root, err := repoRoot()
	g.Expect(err).NotTo(HaveOccurred())
	fresh := t.TempDir()
	g.Expect(Vendor(root, fresh)).To(Succeed())
	diffs, err := Compare(filepath.Join(root, depsDir), fresh)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(diffs).To(BeEmpty())
}
