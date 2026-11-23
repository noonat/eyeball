package eyeball

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball/internal/appbuild"
)

// AppBundle is the React app, built.
type AppBundle struct {
	// Script is the app's bundle, to load as a classic script.
	Script []byte
}

var (
	appOnce sync.Once
	app     AppBundle
	appErr  error
)

// App is the React app bundled from the files the binary embeds, built the
// first time it is asked for and kept. It is the same bundle tool/vendor builds
// from web/node_modules, which a test holds the two to. A failed first build is
// kept for the life of the process, so a transient failure such as a full disk
// stays until a restart.
func App() (AppBundle, error) {
	appOnce.Do(func() {
		deps, err := fs.Sub(Deps, "web/deps/node_modules")
		if err != nil {
			appErr = errors.WithStack(err)
			return
		}
		app, appErr = BuildApp(deps)
	})
	return app, appErr
}

// BuildApp bundles the app over a tree of packages, as the layout of
// node_modules. It writes the embedded sources, the shared scripts and the
// packages to a temporary directory and bundles from there, since esbuild resolves a
// package through its package.json exports on a real directory. The write is
// about 0.8 MB today, with React alone, and grows toward the 1.4 MB measured
// once the router and the query library are imported. The directory is removed
// once the bundle is built.
func BuildApp(deps fs.FS) (AppBundle, error) {
	dir, err := os.MkdirTemp("", "eyeball-app-")
	if err != nil {
		return AppBundle{}, errors.WithStack(err)
	}
	defer os.RemoveAll(dir)
	if err := writeTree(Sources, "web/src", filepath.Join(dir, "web", "src")); err != nil {
		return AppBundle{}, err
	}
	// web/package.json is what makes esbuild read web/src as ES modules, and
	// that changes how it wraps each CommonJS package it imports. The real one
	// sets this and has nothing else the build reads, so a copy that says only
	// that keeps this bundle the same as the one built from web/node_modules.
	manifest := filepath.Join(dir, "web", "package.json")
	if err := os.WriteFile(manifest, []byte(`{"type":"module"}`), 0o644); err != nil {
		return AppBundle{}, errors.WithStack(err)
	}
	// The app imports the framework-free modules in web/ts, which the service
	// worker shares, so a storage key or a header name has one copy. They sit
	// beside web/src, where its imports reach them.
	if err := writeTree(Scripts, "web/ts", filepath.Join(dir, "web", "ts")); err != nil {
		return AppBundle{}, err
	}
	modules := filepath.Join(dir, "node_modules")
	if err := writeTree(deps, ".", modules); err != nil {
		return AppBundle{}, err
	}
	res, err := appbuild.Build(dir, []string{modules})
	if err != nil {
		return AppBundle{}, err
	}
	return AppBundle{Script: res.Script}, nil
}

// writeTree writes the files under a directory of a file system into dest.
func writeTree(src fs.FS, from string, dest string) error {
	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		body, err := fs.ReadFile(src, path)
		if err != nil {
			return errors.WithStack(err)
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return errors.WithStack(err)
		}
		target := filepath.Join(dest, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return errors.WithStack(err)
		}
		return errors.WithStack(os.WriteFile(target, body, 0o644))
	}
	return fs.WalkDir(src, from, walk)
}
