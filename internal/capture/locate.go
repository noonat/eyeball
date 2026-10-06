package capture

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/store"
)

// PathType says what a path names as of a round.
type PathType string

const (
	// PathFile is a file, read from the source Locate names.
	PathFile PathType = "file"
	// PathDir is a directory.
	PathDir PathType = "dir"
	// PathDeleted is a file the round records as deleted.
	PathDeleted PathType = "deleted"
)

// Locate says what p names as of the round, and which source a file is read
// from. A path that names nothing is ErrNotFound.
//
// The question is settled before anything reads, and without listing
// anything recursively, in a fixed order. The round's capture comes first, by
// the rule List applies to it, so an address and the tree always agree. Then
// the base, asked for the one entry at the path. Last, where the state allows
// a live read, the disk. The order matters because the sources disagree: a
// file the agent deleted may be back on disk, and reading the base's directory
// as a file list costs a line per file under it.
//
// A file the disk holds is reported as found without asking git whether it is
// ignored. The live read that follows asks, and refuses one that is, so the
// question costs one git call rather than two. A directory is asked here,
// since nothing reads it afterward, and so is a path the disk lacks, since a
// failure to read the rules is a reason the page names.
func (r *Reader) Locate(ctx context.Context, p string) (PathType, Source, error) {
	clean, err := CleanPath(p)
	if err != nil {
		return "", "", err
	}
	best := claimNone
	for _, f := range r.files {
		best = max(best, claimOn(clean, f))
	}
	switch best {
	case claimFile:
		return PathFile, SourceRound, nil
	case claimDeleted:
		return PathDeleted, SourceRound, nil
	case claimDirBelow, claimDeletedBelow:
		return PathDir, "", nil
	}
	if r.base != "" {
		found, err := r.baseEntry(ctx, clean)
		if err != nil {
			return "", "", err
		}
		switch found {
		case "blob":
			return PathFile, SourceBase, nil
		case "tree":
			return PathDir, "", nil
		case "commit":
			// A gitlink. Its contents belong to another repository, which is
			// why the capture and the tree leave it out.
			return "", "", errors.Wrapf(ErrNotFound, "%s is a submodule", clean)
		}
	}
	return r.locateWorking(ctx, clean)
}

// baseEntry is the type of the one entry the base holds at a path, blob, tree
// or commit, or empty where it holds none.
//
// ls-tree with no -r names the entry at the path itself, so a directory costs
// one line rather than one per file under it. --literal-pathspecs keeps a
// leading colon from being read as pathspec magic, as in loadOIDs: without it
// a file named :weird.txt matches nothing and would be read live instead. A
// blob's object id is remembered, so reading the file costs no second lookup.
func (r *Reader) baseEntry(ctx context.Context, clean string) (string, error) {
	r.lookups++
	out, err := run(ctx, r.root, "--literal-pathspecs", "ls-tree", "-z", r.base, "--", clean)
	if err != nil {
		return "", err
	}
	// A path names one entry at most, so the first is the answer.
	entries := splitNUL(string(out))
	if len(entries) == 0 {
		return "", nil
	}
	meta, _, found := strings.Cut(entries[0], "\t")
	if !found {
		return "", errors.Newf("ls-tree entry %q has no path", entries[0])
	}
	fields := strings.Fields(meta)
	if len(fields) != 3 {
		return "", errors.Newf("ls-tree entry %q has %d fields, not 3", meta, len(fields))
	}
	if fields[1] == "blob" {
		r.oids[clean] = fields[2]
	}
	return fields[1], nil
}

// locateWorking says what the disk holds at a path no frozen source has, under
// the checks a live read makes before it reads.
func (r *Reader) locateWorking(ctx context.Context, clean string) (PathType, Source, error) {
	if r.live.Refused() {
		return "", "", r.refusal(clean)
	}
	root, err := os.OpenRoot(r.root)
	if err != nil {
		return "", "", notFoundOr(err, "open the project root "+r.root)
	}
	defer func() { _ = root.Close() }()

	if err := checkParents(root, clean); err != nil {
		return "", "", err
	}
	info, err := root.Lstat(filepath.FromSlash(clean))
	if missing(err) && r.live == LiveIgnoreRules {
		// The answer order asks git before it looks at the disk. Asking once
		// the disk has no such path keeps that answer: a missing path whose
		// rules git cannot read is refused for that reason, which the page
		// names, rather than as merely missing.
		if err := refuseIgnored(ctx, r.root, clean); err != nil {
			return "", "", err
		}
	}
	if err != nil {
		return "", "", notFoundOr(err, "stat "+clean)
	}
	switch mode := info.Mode(); {
	case mode.IsDir():
		if r.live == LiveIgnoreRules {
			if err := refuseIgnored(ctx, r.root, clean); err != nil {
				return "", "", err
			}
		}
		if _, err := root.Lstat(filepath.FromSlash(clean + "/" + gitDir)); !missing(err) {
			// Another repository, or a directory that cannot be searched,
			// which the listing leaves out as well.
			return "", "", errors.Wrapf(ErrNotFound, "%s holds a repository of its own", clean)
		}
		return PathDir, "", nil
	case mode&fs.ModeSymlink != 0, mode.IsRegular():
		return PathFile, SourceWorking, nil
	}
	return "", "", errors.Wrapf(ErrNotFound, "%s is not a regular file", clean)
}

// claim is what one captured path says about a name at or above it. The
// values are ranked, and the highest claim on a name decides what it is.
//
// What exists as of the round outranks what the round deleted. The agent can
// delete a file a and create a/x, or delete a/x and create a file a, and the
// capture then records both. The name is a directory in the first case and a
// file in the second, as of the round, and a deleted file that was there
// before is shown on the review page.
type claim int

const (
	// claimNone is a captured path that says nothing about the name.
	claimNone claim = iota
	// claimDeletedBelow is a deletion under the name, which makes it a
	// directory holding only deletions.
	claimDeletedBelow
	// claimDeleted is a deletion of the name itself.
	claimDeleted
	// claimDirBelow is a file under the name, which makes it a directory.
	claimDirBelow
	// claimFile is a file at the name.
	claimFile
)

// claimOn is what the captured file f says about the name p.
func claimOn(p string, f store.File) claim {
	live := f.Digest != ""
	switch {
	case f.Path == p && live:
		return claimFile
	case f.Path == p:
		return claimDeleted
	case !strings.HasPrefix(f.Path, p+"/"):
		return claimNone
	case live:
		return claimDirBelow
	}
	return claimDeletedBelow
}
