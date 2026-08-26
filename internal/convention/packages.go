package convention

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
)

// layoutPath is the document holding the list of packages, relative to the root.
const layoutPath = "docs/architecture.md"

// layoutLine matches one entry in the layout block: a path ending in a slash,
// then the one-line description.
var layoutLine = regexp.MustCompile(`^((?:cmd|internal|tool|web)/[a-z0-9/_-]*)/?\s{2,}\S`)

// PackagesListed reports a package on disk that the layout block in
// docs/architecture.md does not name.
//
// A list missing entries stops being read as a list of what exists, and the
// document is where somebody looks to find out what a package is for.
//
// The reverse is allowed. The document describes the target, so it may name a
// package that has not been written yet.
func PackagesListed() ([]Finding, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	return packagesListedIn(root)
}

// packagesListedIn is PackagesListed against a named root, so a test can build
// a tree that violates the rule instead of the repository having to.
func packagesListedIn(root string) ([]Finding, error) {
	listed, err := listedPackages(root)
	if err != nil {
		return nil, err
	}
	onDisk, err := packageDirs(root)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, dir := range onDisk {
		if _, ok := listed[dir]; ok {
			continue
		}
		out = append(out, Finding{
			At:    layoutPath,
			Check: "packages-listed",
			What:  "package " + dir + " is not in the layout block",
		})
	}
	return out, nil
}

// listedPackages reads the paths out of the layout block in the architecture
// document.
func listedPackages(root string) (map[string]struct{}, error) {
	body, err := os.ReadFile(filepath.Join(root, layoutPath))
	if err != nil {
		return nil, errors.Wrapf(err, "read %s", layoutPath)
	}
	out := map[string]struct{}{}
	for _, line := range strings.Split(string(body), "\n") {
		m := layoutLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		out[strings.TrimSuffix(m[1], "/")] = struct{}{}
	}
	if len(out) == 0 {
		return nil, errors.Newf("no layout block found in %s", layoutPath)
	}
	return out, nil
}

// packageDirs lists every directory under cmd, internal, tool and web that holds
// Go source.
func packageDirs(root string) ([]string, error) {
	var out []string
	tops := []string{"cmd", "internal", "tool", "web"}
	for _, top := range tops {
		base := filepath.Join(root, top)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return errors.WithStack(err)
			}
			if !d.IsDir() {
				return nil
			}
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			has, err := hasGo(path)
			if err != nil {
				return err
			}
			if has {
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return errors.WithStack(err)
				}
				out = append(out, filepath.ToSlash(rel))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

// hasGo reports whether a directory holds at least one Go file.
func hasGo(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, errors.WithStack(err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return true, nil
		}
	}
	return false, nil
}
