package capture

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
)

// Listing says where a directory listing's entries come from, which is what
// the tree's notice tells a reader.
type Listing string

const (
	// ListingFrozen is the base and the round's capture, both exact as of the
	// round.
	ListingFrozen Listing = "frozen"
	// ListingDisk is the working copy as it is now, and the round's capture.
	ListingDisk Listing = "disk"
	// ListingCapture is the round's capture alone, which holds only what the
	// agent changed.
	ListingCapture Listing = "capture"
)

// DirEntry is one name in a directory listing.
type DirEntry struct {
	// Name is the entry's name within its directory.
	Name string
	// Path is the entry's path from the project root, slash separated.
	Path string
	// Dir reports a directory.
	Dir bool
	// Deleted reports a file the round records as deleted. The round's scope
	// decides whether the tree shows it, which the reader does not know.
	Deleted bool
}

// Listing says which sources List reads.
//
// A round with a base lists the base, and the disk is never asked, since the
// base already holds every file the capture does not. A git project with no
// base lists the capture alone. It is an unborn branch, whose capture is
// complete, or a base that is gone, where the disk would list every file git
// ignores. Every state that refuses a live read lists the capture alone as
// well, since a listing from disk would name files no address serves.
func (r *Reader) Listing() Listing {
	switch {
	case r.base != "":
		return ListingFrozen
	case r.git || r.live.Refused():
		return ListingCapture
	}
	return ListingDisk
}

// List names what one directory holds as of the round, one level deep, in
// byte order of name. The empty path is the project root.
//
// The capture is merged over the other source. A captured file is listed
// whatever git ignores, because it opens from the capture, and a deletion the
// round records is listed as deleted even where the disk has the file again,
// since opening it says the same.
//
// A directory the disk cannot list, because it is gone or a check refuses
// it, still lists what the capture holds under it, and is ErrNotFound where
// the capture holds nothing. That keeps the listing in step with what an
// address serves.
func (r *Reader) List(ctx context.Context, dir string) ([]DirEntry, error) {
	clean := ""
	if strings.Trim(dir, "/") != "" {
		var err error
		if clean, err = CleanPath(dir); err != nil {
			return nil, err
		}
	}
	entries := map[string]DirEntry{}
	var diskErr error
	switch r.Listing() {
	case ListingFrozen:
		listed, err := r.listBase(ctx, clean)
		if err != nil {
			return nil, err
		}
		add(entries, listed)
	case ListingDisk:
		listed, err := r.listDisk(ctx, clean)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		diskErr = err
		add(entries, listed)
	}
	r.listCapture(entries, clean)
	if len(entries) == 0 && diskErr != nil {
		return nil, diskErr
	}
	out := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// add puts listed entries into a listing by name.
func add(entries map[string]DirEntry, listed []DirEntry) {
	for _, e := range listed {
		entries[e.Name] = e
	}
}

// listBase names what the base holds in a directory, one level deep.
//
// ls-tree given a directory with a trailing slash lists its entries, and with
// no path at all lists the root. It runs at the root, so the path it is given
// and the paths it prints are project paths. A directory the base lacks lists nothing.
// --literal-pathspecs keeps a leading colon from being read as pathspec magic,
// as it does in loadOIDs: without it a directory named :cdir lists nothing.
//
// A blob is a file, a symlink among them, since the reader serves a link's
// target text. A gitlink is left out, because its contents belong to another
// repository.
func (r *Reader) listBase(ctx context.Context, clean string) ([]DirEntry, error) {
	args := []string{"--literal-pathspecs", "ls-tree", "-z", r.base}
	if clean != "" {
		args = append(args, "--", clean+"/")
	}
	out, err := run(ctx, r.root, args...)
	if err != nil {
		return nil, err
	}
	var listed []DirEntry
	for _, record := range splitNUL(string(out)) {
		meta, p, found := strings.Cut(record, "\t")
		if !found {
			return nil, errors.Newf("ls-tree entry %q has no path", record)
		}
		fields := strings.Fields(meta)
		if len(fields) != 3 {
			return nil, errors.Newf("ls-tree entry %q has %d fields, not 3", meta, len(fields))
		}
		switch fields[1] {
		case "blob":
			listed = append(listed, DirEntry{Name: path.Base(p), Path: p})
		case "tree":
			listed = append(listed, DirEntry{Name: path.Base(p), Path: p, Dir: true})
		}
	}
	return listed, nil
}

// listDisk names what the working copy holds in a directory, one level deep,
// through an os.Root on the project root.
//
// The directory and every one above it pass the same check a live read's
// parents do, so a listing never runs through a symlink or into another
// repository. os.Root keeps a directory swapped for a link after that check
// from leading outside the root.
//
// It skips .git, a directory holding a .git entry, and anything that is not a
// regular file, a directory or a symlink, which are what a live read refuses.
// A symlink is listed as a file and opens on its target text. Where the
// project follows ignore rules, what git ignores is dropped.
func (r *Reader) listDisk(ctx context.Context, clean string) ([]DirEntry, error) {
	root, err := os.OpenRoot(r.root)
	if err != nil {
		return nil, notFoundOr(err, "open the project root "+r.root)
	}
	defer func() { _ = root.Close() }()

	name := "."
	if clean != "" {
		if err := checkDir(root, clean, clean); err != nil {
			return nil, err
		}
		name = clean
	}
	r.step("checked")
	dirents, err := fs.ReadDir(root.FS(), name)
	if err != nil {
		return nil, notFoundOr(err, "list "+name)
	}
	var listed []DirEntry
	for _, d := range dirents {
		if d.Name() == gitDir {
			continue
		}
		p := path.Join(clean, d.Name())
		switch t := d.Type(); {
		case t&fs.ModeSymlink != 0, t.IsRegular():
			listed = append(listed, DirEntry{Name: d.Name(), Path: p})
		case t.IsDir():
			if _, err := root.Lstat(filepath.FromSlash(p + "/" + gitDir)); !missing(err) {
				// Another repository, or a directory that cannot be searched,
				// which nothing under can be read from either.
				continue
			}
			listed = append(listed, DirEntry{Name: d.Name(), Path: p, Dir: true})
		}
	}
	if r.live == LiveIgnoreRules {
		return dropIgnored(ctx, r.root, listed)
	}
	return listed, nil
}

// listCapture merges what the round's capture holds under a directory into a
// listing, over whatever the other source listed.
//
// Each captured path contributes its next segment below the directory, as a
// file or as a directory. A deletion counts, so a file the round deleted is
// listed as deleted and the directory holding one still lists.
func (r *Reader) listCapture(entries map[string]DirEntry, clean string) {
	prefix := ""
	if clean != "" {
		prefix = clean + "/"
	}
	held := make([]string, 0, len(r.files))
	for p := range r.files {
		held = append(held, p)
	}
	// In path order, so a file deleted at a name and a directory created at
	// the same name later always resolve the same way.
	sort.Strings(held)
	for _, p := range held {
		rest, under := strings.CutPrefix(p, prefix)
		if !under {
			continue
		}
		name, _, deeper := strings.Cut(rest, "/")
		if deeper {
			entries[name] = DirEntry{Name: name, Path: prefix + name, Dir: true}
			continue
		}
		entries[name] = DirEntry{Name: name, Path: p, Deleted: r.files[p].Digest == ""}
	}
}

// dropIgnored removes from a listing what git ignores, in one check-ignore
// call.
//
// Each path goes as ./<path>, NUL separated, for the reason the live read
// gives: a bare leading colon is pathspec magic, and an ignored :other.txt
// would read as not ignored. check-ignore names the ignored paths as it was
// given them and exits 0, or names none and exits 1. Anything else is a
// failure to say, ErrRulesUnreadable, and never a reason to list the entries
// unchecked. The batch stops at the first path inside a submodule with a
// 128, which is an error here as well.
func dropIgnored(ctx context.Context, root string, listed []DirEntry) ([]DirEntry, error) {
	if len(listed) == 0 {
		return nil, nil
	}
	spelled := func(e DirEntry) string { return "./" + e.Path }
	var in bytes.Buffer
	for _, e := range listed {
		in.WriteString(spelled(e))
		in.WriteByte(0)
	}
	cmd := ignoreCommand(ctx, root, "--stdin", "-z")
	cmd.Stdin = &in
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && exitCode(err) != 1 {
		unreadable := errors.Wrapf(ErrRulesUnreadable, "git check-ignore --stdin: %s", message(&stderr))
		return nil, errors.WithSecondaryError(unreadable, err)
	}
	ignored := map[string]struct{}{}
	for _, p := range splitNUL(string(out)) {
		ignored[p] = struct{}{}
	}
	kept := make([]DirEntry, 0, len(listed))
	for _, e := range listed {
		if _, gone := ignored[spelled(e)]; gone {
			continue
		}
		kept = append(kept, e)
	}
	return kept, nil
}
