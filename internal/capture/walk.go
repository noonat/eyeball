package capture

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
)

// ErrTooManyFiles reports a walk that found more files than a review is likely
// to have meant. Nothing outside git says what is ignored, so a review naming a
// directory with a dependency tree under it would otherwise capture all of it.
var ErrTooManyFiles = errors.New("too many files under the review's paths")

// ErrNoSuchPath reports a review path the project does not have.
var ErrNoSuchPath = errors.New("path is not in the project")

// walkLimit is how many files a walk will take before refusing.
//
// A guess written down so it can be corrected, not a measured number. It is
// well above any review a person reads and well below a dependency directory.
const walkLimit = 1000

// Walk lists the files under the review's paths, for a project with no git.
//
// There is no way to ask what changed, so the capture is every file under the
// paths the review named and nothing outside them. Files the agent touched
// elsewhere are not captured, which is the cost of running outside git.
//
// The result is sorted by path and holds one entry per path, the same shape
// Changed returns, because Freeze does not care which of them produced it.
func Walk(ctx context.Context, root string, paths []string) ([]Entry, error) {
	found := make(map[string]Entry)
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(err, "walk the review's paths")
		}
		if err := walkOne(root, p, found); err != nil {
			return nil, err
		}
	}
	return keep(root, found)
}

// walkOne adds everything under one of the review's paths.
//
// A path the project does not have is a refusal rather than a silent miss. A
// review naming two paths where one is a typo would otherwise capture the other
// and report nothing about it.
func walkOne(root, p string, found map[string]Entry) error {
	// The review's paths have not been through the store yet, so nothing has
	// checked them. Without this a path of ../secrets reads files outside the
	// project into the blob store, and the store's own refusal comes after the
	// content is already written and blobs are never pruned.
	inside, err := safePath(p)
	if err != nil {
		return errors.Wrapf(ErrNoSuchPath, "%q is not a path inside the project", p)
	}
	p = inside
	full := filepath.Join(root, filepath.FromSlash(p))
	info, err := os.Lstat(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.Wrapf(ErrNoSuchPath, "%s, so name a path the project has", p)
		}
		return errors.Wrapf(err, "stat %s", p)
	}
	if !info.IsDir() {
		found[clean(p)] = Entry{Path: clean(p)}
		return nil
	}
	return walkDir(root, p, full, found)
}

// walkDir adds every file under one of the review's directories.
//
// WalkDir reads directory entries without following what they point at, so a
// symlink to a directory arrives as a plain entry and is never descended
// through. Whether a path is worth capturing is decided later by describe, in
// one place for both enumerations.
func walkDir(root, p, full string, found map[string]Entry) error {
	return filepath.WalkDir(full, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return errors.Wrapf(err, "walk %s", p)
		}
		if d.IsDir() {
			if d.Name() == gitDir {
				// A project with no marker of its own can still contain a
				// repository, and its object store is not review material.
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return errors.Wrapf(err, "locate %s under the project root", name)
		}
		if len(found) >= walkLimit {
			return errors.Wrapf(ErrTooManyFiles, "%s holds more than %d, so name narrower paths", p, walkLimit)
		}
		slash := filepath.ToSlash(rel)
		found[slash] = Entry{Path: slash}
		return nil
	})
}
