package eyeball

import (
	"io"
	"io/fs"
	"testing"
	"testing/fstest"

	. "github.com/onsi/gomega"
	modzip "golang.org/x/mod/zip"
)

// zipFile is one file of a tree, offered to the module zip as the Go command
// offers a file on disk.
type zipFile struct {
	fsys fs.FS
	path string
}

// Path names the file from the module root.
func (f zipFile) Path() string {
	return f.path
}

// Lstat describes the file.
func (f zipFile) Lstat() (fs.FileInfo, error) {
	return fs.Stat(f.fsys, f.path)
}

// Open reads the file.
func (f zipFile) Open() (io.ReadCloser, error) {
	return f.fsys.Open(f.path)
}

// refusedByZip lists the paths a module zip would leave out or refuse, each
// with the reason, by running the check the Go command runs when it builds one.
// The tree needs a go.mod, which the check reads for the module's Go version.
func refusedByZip(fsys fs.FS, paths []string) []string {
	files := []modzip.File{zipFile{fsys: fsys, path: "go.mod"}}
	for _, path := range paths {
		files = append(files, zipFile{fsys: fsys, path: path})
	}
	checked, _ := modzip.CheckFiles(files)
	var out []string
	for _, f := range checked.Omitted {
		out = append(out, f.Path+": "+f.Err.Error())
	}
	for _, f := range checked.Invalid {
		out = append(out, f.Path+": "+f.Err.Error())
	}
	return out
}

// pathsIn lists the files an embedded tree holds, as the embed directive named
// them.
func pathsIn(g *WithT, trees ...fs.FS) []string {
	var out []string
	for _, tree := range trees {
		err := fs.WalkDir(tree, ".", func(path string, entry fs.DirEntry, err error) error {
			if err == nil && !entry.IsDir() {
				out = append(out, path)
			}
			return err
		})
		g.Expect(err).NotTo(HaveOccurred())
	}
	return out
}

// A file the binary embeds has to reach a go install through the module proxy,
// and the proxy serves a module zip. That zip leaves out files more than one
// level below a vendor directory, so an embed there would match nothing and the
// build would fail. This asks the zip code itself, over every file the binary
// embeds.
func Test_embedsReachModuleZip(t *testing.T) {
	g := NewWithT(t)
	paths := pathsIn(g, Fonts, Templates, Scripts, Sources, Deps, Icons)
	paths = append(paths, "docs/design/app.css", "web/static/shell.css")
	g.Expect(paths).To(ContainElement(ContainSubstring("node_modules/react/")))
	g.Expect(paths).To(ContainElement(HavePrefix("web/src/")))
	g.Expect(paths).To(ContainElement(HavePrefix("web/ts/")), "the app stages the shared scripts, so the binary has to hold them")

	disk := fstest.MapFS{}
	repo := repoFS(g)
	for _, path := range append(paths, "go.mod") {
		body, err := fs.ReadFile(repo, path)
		g.Expect(err).NotTo(HaveOccurred())
		disk[path] = &fstest.MapFile{Data: body}
	}
	g.Expect(refusedByZip(disk, paths)).To(BeEmpty())
}

// The check has to be able to fail. A package under web/vendor is the first
// design, and the zip drops it, while a file directly in web/vendor survives.
func Test_moduleZipDropsVendor(t *testing.T) {
	g := NewWithT(t)
	tree := fstest.MapFS{
		"go.mod":                                 {Data: []byte("module example.com/m\n\ngo 1.26\n")},
		"web/vendor/node_modules/react/index.js": {Data: []byte("x")},
		"web/vendor/lib.min.js":                  {Data: []byte("x")},
	}
	paths := []string{"web/vendor/node_modules/react/index.js", "web/vendor/lib.min.js"}
	refused := refusedByZip(tree, paths)
	g.Expect(refused).To(HaveLen(1))
	g.Expect(refused[0]).To(HavePrefix("web/vendor/node_modules/react/index.js"))
}
