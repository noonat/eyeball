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
// the capture holds nothing. The base and the capture list nothing for a
// directory they lack, so whether dir is a directory at all is Locate's
// question, asked first.
func (r *Reader) List(ctx context.Context, dir string) ([]DirEntry, error) {
	clean, err := cleanDir(dir)
	if err != nil {
		return nil, err
	}
	byDir, diskErrs, err := r.gather(ctx, []string{clean})
	if err != nil {
		return nil, err
	}
	if len(byDir[clean]) == 0 && diskErrs[clean] != nil {
		return nil, diskErrs[clean]
	}
	return sorted(byDir[clean]), nil
}

// ListSubdirs lists several subdirectories of dir, one level each, keyed by
// name, as List would list each one.
//
// The base lists them in one ls-tree call. Sibling pathspecs each list one
// level, where a parent given beside its own child would lose the parent's
// entries, which is why the names are taken one segment each under a shared
// dir. A subdirectory the disk cannot list lists what the capture holds under
// it, or nothing.
func (r *Reader) ListSubdirs(ctx context.Context, dir string, names []string) (map[string][]DirEntry, error) {
	clean, err := cleanDir(dir)
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(names))
	for _, name := range names {
		sub, err := CleanPath(path.Join(clean, name))
		if err != nil {
			return nil, err
		}
		want := clean
		if want == "" {
			want = "."
		}
		if path.Dir(sub) != want {
			return nil, errors.Newf("%q is not one segment below %q", name, clean)
		}
		dirs = append(dirs, sub)
	}
	byDir, _, err := r.gather(ctx, dirs)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]DirEntry, len(names))
	for _, sub := range dirs {
		out[path.Base(sub)] = sorted(byDir[sub])
	}
	return out, nil
}

// cleanDir is dir as a project path, with the root as the empty path.
func cleanDir(dir string) (string, error) {
	if strings.Trim(dir, "/") == "" {
		return "", nil
	}
	return CleanPath(dir)
}

// gather lists each of dirs from the reader's sources, by name within each
// directory, along with why the disk could not list one. The directories are
// siblings, or one directory alone.
func (r *Reader) gather(ctx context.Context, dirs []string) (map[string]map[string]DirEntry, map[string]error, error) {
	byDir := make(map[string]map[string]DirEntry, len(dirs))
	for _, d := range dirs {
		byDir[d] = map[string]DirEntry{}
	}
	diskErrs := map[string]error{}
	var listed []DirEntry
	switch r.Listing() {
	case ListingFrozen:
		var err error
		if listed, err = r.listBase(ctx, dirs); err != nil {
			return nil, nil, err
		}
	case ListingDisk:
		for _, d := range dirs {
			found, err := r.listDisk(d)
			if err != nil && !errors.Is(err, ErrNotFound) {
				return nil, nil, err
			}
			if err != nil {
				diskErrs[d] = err
			}
			listed = append(listed, found...)
		}
		if r.live == LiveIgnoreRules {
			var err error
			if listed, err = dropIgnored(ctx, r.root, listed); err != nil {
				return nil, nil, err
			}
		}
	}
	for _, e := range listed {
		parent := path.Dir(e.Path)
		if parent == "." {
			parent = ""
		}
		if entries, asked := byDir[parent]; asked {
			entries[e.Name] = e
		}
	}
	for _, d := range dirs {
		r.listCapture(byDir[d], d)
	}
	return byDir, diskErrs, nil
}

// sorted is a directory's entries in byte order of name.
func sorted(entries map[string]DirEntry) []DirEntry {
	out := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// listBase names what the base holds in each of dirs, one level deep, in one
// ls-tree call.
//
// ls-tree given a directory with a trailing slash lists its entries, and with
// no path at all lists the root. It runs at the root, so the paths it is given
// and the paths it prints are project paths. A directory the base lacks lists
// nothing. --literal-pathspecs keeps a leading colon from being read as
// pathspec magic, as it does in loadOIDs: without it a directory named :cdir
// lists nothing.
//
// A blob is a file, a symlink among them, since the reader serves a link's
// target text. A gitlink is left out, because its contents belong to another
// repository.
func (r *Reader) listBase(ctx context.Context, dirs []string) ([]DirEntry, error) {
	var listed []DirEntry
	for _, chunk := range chunks(dirs, r.batch) {
		r.lookups++
		args := []string{"--literal-pathspecs", "ls-tree", "-z", r.base}
		if len(chunk) != 1 || chunk[0] != "" {
			args = append(args, "--")
			for _, d := range chunk {
				args = append(args, d+"/")
			}
		}
		out, err := run(ctx, r.root, args...)
		if err != nil {
			return nil, err
		}
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
// A symlink is listed as a file and opens on its target text. What git
// ignores is the caller's to drop, in one batch for every directory listed.
func (r *Reader) listDisk(clean string) ([]DirEntry, error) {
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
	return listed, nil
}

// listCapture merges what the round's capture holds under a directory into a
// listing, over whatever the other source listed.
//
// Each captured path claims its next segment below the directory, as a file,
// a deletion or a directory, and the highest claim on a name decides it, by
// the rule Locate applies to an address. A deletion counts, so a file the
// round deleted is listed as deleted and the directory holding one still
// lists.
func (r *Reader) listCapture(entries map[string]DirEntry, clean string) {
	prefix := ""
	if clean != "" {
		prefix = clean + "/"
	}
	best := map[string]claim{}
	for p, f := range r.files {
		rest, under := strings.CutPrefix(p, prefix)
		if !under {
			continue
		}
		name, _, _ := strings.Cut(rest, "/")
		best[name] = max(best[name], claimOn(prefix+name, f))
	}
	for name, c := range best {
		e := DirEntry{Name: name, Path: prefix + name}
		switch c {
		case claimDeleted:
			e.Deleted = true
		case claimDirBelow, claimDeletedBelow:
			e.Dir = true
		}
		entries[name] = e
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
