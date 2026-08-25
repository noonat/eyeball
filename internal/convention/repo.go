package convention

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

// ErrNoRoot is returned when no go.mod is found above the working directory.
var ErrNoRoot = errors.New("no go.mod above the working directory")

// repoRoot walks up from the working directory for the module root, so a check
// reports paths relative to the repository wherever the test was run from.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", errors.WithStack(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNoRoot
		}
		dir = parent
	}
}

// Package is one directory of Go files, parsed together.
type Package struct {
	// Dir is the directory, relative to the repository root.
	Dir string
	// Files are every Go file in it, parsed with comments kept.
	Files []*ast.File
}

// ParseRepo parses every Go package in the repository that the rules apply to.
//
// testdata is skipped. Its files break conventions on purpose, and a check that
// flagged them would fail the build for doing its job. The go tool ignores those
// directories for the same reason.
func ParseRepo(fset *token.FileSet) ([]Package, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	var out []Package
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if path != root && (name == "testdata" || strings.HasPrefix(name, ".")) {
			return filepath.SkipDir
		}
		pkg, err := parseDir(fset, root, path)
		if err != nil {
			return err
		}
		if len(pkg.Files) > 0 {
			out = append(out, pkg)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ParseFixture parses one check's fixture directory under testdata.
//
// A fixture is parsed on its own, never as part of the repository, because it is
// meant to be broken and nothing else should see it.
func ParseFixture(fset *token.FileSet, check string) (Package, error) {
	root, err := repoRoot()
	if err != nil {
		return Package{}, err
	}
	dir := filepath.Join(root, "internal", "convention", "testdata", check)
	if _, err := os.Stat(dir); err != nil {
		return Package{}, errors.Wrapf(err, "no fixture for check %q", check)
	}
	return parseDir(fset, root, dir)
}

// parseDir parses the Go files directly in one directory.
func parseDir(fset *token.FileSet, root, dir string) (Package, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Package{}, errors.WithStack(err)
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return Package{}, errors.WithStack(err)
	}
	pkg := Package{Dir: rel}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return Package{}, errors.Wrapf(err, "parse %s", path)
		}
		pkg.Files = append(pkg.Files, file)
	}
	return pkg, nil
}
