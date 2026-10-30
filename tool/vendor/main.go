// Command vendor copies the files the app's bundle reads out of web/node_modules
// into web/deps/node_modules, which the binary embeds.
//
// The set is what esbuild's metafile lists for the app's imports, plus each
// package's package.json, which esbuild reads to resolve the package's exports.
// It changes when an import does, so a check against the lockfile would miss an
// import that needs a file that is not vendored yet. With --check the command
// vendors into a temporary directory instead and fails when that differs from
// the committed tree.
package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball/internal/appbuild"
)

// depsDir is the committed tree, relative to the repository root.
const depsDir = "web/deps/node_modules"

// modulesDir is the installed packages, relative to the repository root.
const modulesDir = "web/node_modules"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "vendor: %v\n", err)
		os.Exit(1)
	}
}

// run vendors, or checks, as its arguments say.
func run(args []string) error {
	check := len(args) == 1 && args[0] == "--check"
	if len(args) > 0 && !check {
		return errors.Newf("usage: vendor [--check], got %q", strings.Join(args, " "))
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, modulesDir)); err != nil {
		return errors.New("web/node_modules is missing. Run: make tools")
	}
	committed := filepath.Join(root, depsDir)
	if !check {
		if err := os.RemoveAll(committed); err != nil {
			return errors.Wrap(err, "clear web/deps")
		}
		return Vendor(root, committed)
	}
	fresh, err := os.MkdirTemp("", "vendor-check-")
	if err != nil {
		return errors.WithStack(err)
	}
	defer os.RemoveAll(fresh)
	if err := Vendor(root, fresh); err != nil {
		return err
	}
	diffs, err := Compare(committed, fresh)
	if err != nil {
		return err
	}
	if len(diffs) > 0 {
		return errors.Newf("web/deps/node_modules is stale. Run: make vendor\n%s", strings.Join(diffs, "\n"))
	}
	return nil
}

// Vendor bundles the app from a repository root's web/node_modules and copies
// what the bundle read, with each package's package.json, into dest.
func Vendor(root, dest string) error {
	res, err := appbuild.Build(root, nil)
	if err != nil {
		return err
	}
	files := map[string]struct{}{}
	for _, input := range res.Inputs {
		if !strings.Contains(input, "node_modules/") {
			continue
		}
		files[input] = struct{}{}
		files[packageJSON(input)] = struct{}{}
	}
	if len(files) == 0 {
		return errors.New("the bundle read nothing from node_modules")
	}
	for input := range files {
		rel := input[strings.Index(input, "node_modules/")+len("node_modules/"):]
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(input)))
		if err != nil {
			return errors.Wrapf(err, "read %s", input)
		}
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return errors.WithStack(err)
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// packageJSON is the package.json of the package a file belongs to. A scoped
// package, such as @tanstack/react-query, is two path segments.
func packageJSON(file string) string {
	at := strings.LastIndex(file, "node_modules/") + len("node_modules/")
	segments := strings.Split(file[at:], "/")
	depth := 1
	if strings.HasPrefix(segments[0], "@") {
		depth = 2
	}
	return file[:at] + strings.Join(segments[:depth], "/") + "/package.json"
}

// Compare lists how two trees differ: a file in one and not the other, or in
// both with different bytes.
func Compare(want, got string) ([]string, error) {
	wantFiles, err := readTree(want)
	if err != nil {
		return nil, err
	}
	gotFiles, err := readTree(got)
	if err != nil {
		return nil, err
	}
	var diffs []string
	for name, body := range wantFiles {
		other, ok := gotFiles[name]
		switch {
		case !ok:
			diffs = append(diffs, "  only committed: "+name)
		case !bytes.Equal(body, other):
			diffs = append(diffs, "  differs: "+name)
		}
	}
	for name := range gotFiles {
		if _, ok := wantFiles[name]; !ok {
			diffs = append(diffs, "  missing from the commit: "+name)
		}
	}
	sort.Strings(diffs)
	return diffs, nil
}

// readTree reads every file under a directory, keyed by its slash path. A
// directory that does not exist holds nothing.
func readTree(dir string) (map[string][]byte, error) {
	out := map[string][]byte{}
	if _, err := os.Stat(dir); err != nil {
		return out, nil
	}
	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return errors.WithStack(err)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return errors.WithStack(err)
		}
		out[filepath.ToSlash(rel)] = body
		return nil
	}
	if err := filepath.WalkDir(dir, walk); err != nil {
		return nil, errors.Wrapf(err, "read %s", dir)
	}
	return out, nil
}

// repoRoot walks up from the working directory for the module root.
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
			return "", errors.New("no go.mod above the working directory")
		}
		dir = parent
	}
}
