package capture

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
)

// submoduleMode is the file mode git reports for a gitlink. The worktree entry
// under it is a directory, so reading it as a file fails, and its contents
// belong to another repository rather than to this review.
const submoduleMode = "160000"

// ErrVanished reports a path an enumeration found and the filesystem no longer
// has. Recording it as deleted would put a deletion the agent did not make into
// a frozen round.
var ErrVanished = errors.New("path disappeared during capture")

// Entry is one path the enumeration found, before any content is read.
type Entry struct {
	// Path is relative to the project root, cleaned and slash separated.
	Path string
	// Deleted is whether the path is gone from the working copy. Nothing reads
	// its content, and the round records it with an empty digest so a deletion
	// stays distinguishable from a file that never changed.
	Deleted bool
	// Symlink is whether the path is a symbolic link, whose content is the
	// target it names rather than the bytes at the other end.
	Symlink bool
}

// describe fills in what only the filesystem knows about a present path, and
// reports whether the path is worth capturing at all.
//
// Every path goes through here, whichever call reported it. A raw record's mode
// 120000 is not enough, because an untracked symlink arrives from ls-files with
// no mode at all, and a link pointing at a directory is indistinguishable from
// one pointing at a file in that output. Reading such a link with os.ReadFile
// fails with EISDIR and takes the whole capture with it.
//
// Anything that is neither a regular file nor a symlink is skipped, so a socket
// or a device node left in a working tree is never opened.
func describe(root string, e Entry) (Entry, bool, error) {
	if e.Deleted {
		return e, true, nil
	}
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(e.Path)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return e, false, errors.Wrapf(ErrVanished, "%s, so the working copy moved during capture", e.Path)
		}
		return e, false, errors.Wrapf(err, "stat %s", e.Path)
	}
	mode := info.Mode()
	if mode&os.ModeSymlink != 0 {
		e.Symlink = true
		return e, true, nil
	}
	return e, mode.IsRegular(), nil
}

// Changed lists every path that differs between base and the working copy at
// root, together with every untracked file that is not ignored.
//
// An empty base means the repository has no commits yet, and the comparison
// runs against the empty tree, which reports every tracked file as added.
//
// The result is sorted by path and holds one entry per path. It is exported
// because the project view's working copy listing is the same question asked
// against HEAD, with nothing written.
func Changed(ctx context.Context, root, base string) ([]Entry, error) {
	if base == "" {
		empty, err := emptyTree(ctx, root)
		if err != nil {
			return nil, err
		}
		base = empty
	}

	tracked, err := trackedChanges(ctx, root, base)
	if err != nil {
		return nil, err
	}
	untracked, err := untrackedFiles(ctx, root)
	if err != nil {
		return nil, err
	}

	// By path after both calls return, rather than by filtering one through
	// the other, so the rule holds whichever order git reports things in. The
	// untracked answer wins: after git rm --cached the raw diff calls the path
	// deleted and ls-files calls it present, and the file is on disk, so its
	// content is what the agent has. Two rows for one path would be a
	// duplicate the store refuses outright.
	found := make(map[string]Entry, len(tracked)+len(untracked))
	for _, e := range tracked {
		found[e.Path] = e
	}
	for _, e := range untracked {
		found[e.Path] = e
	}
	return keep(root, found)
}

// keep asks the filesystem about each merged entry and drops the ones nothing
// can read, returning what is left in a stable order.
func keep(root string, found map[string]Entry) ([]Entry, error) {
	entries := make([]Entry, 0, len(found))
	for _, e := range found {
		filled, ok, err := describe(root, e)
		if err != nil {
			return nil, err
		}
		if ok {
			entries = append(entries, filled)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// emptyTree is the id of the tree with nothing in it.
//
// It is asked for rather than written down. 4b825dc642cb6eb9a060e54bf8d69288fbee4904
// is the SHA-1 value, and a repository created with --object-format=sha256 has
// a different one and rejects that constant outright.
func emptyTree(ctx context.Context, root string) (string, error) {
	out, err := run(ctx, root, "hash-object", "-t", "tree", "/dev/null")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// trackedChanges lists what git diff reports between base and the working tree.
//
// --raw rather than --name-status, because a raw record carries the file modes
// and nothing else in the output tells a submodule from a file. Both object ids
// are ignored: the destination one is all zeros only where the working copy
// differs from the index, so a staged path carries a real blob id, and reading
// content from git there would capture the staged copy rather than the file on
// disk.
func trackedChanges(ctx context.Context, root, base string) ([]Entry, error) {
	out, err := run(ctx, root, changedArgs(base)...)
	if err != nil {
		return nil, err
	}
	return parseRaw(string(out))
}

// changedArgs is the raw diff call, built here so its flags can be asserted.
//
// --no-renames does not change the entries this produces: rename detection is
// on by default, and parseRaw reads the two-path record it emits as a delete
// and an add, which is exactly what the flag makes git emit as two records.
// What the flag buys is git not doing the detection work at all, on a diff that
// can be the whole of a large change.
func changedArgs(base string) []string {
	return []string{"diff", "--raw", "-z", "--no-renames", base}
}

// parseRaw reads the records of a NUL separated raw diff.
//
// Each record is metadata, then its path or paths. A rename or copy carries two
// paths and is read as a delete of the first and an add of the second, which is
// what a path-to-content list holds either way.
func parseRaw(out string) ([]Entry, error) {
	fields := splitNUL(out)
	var entries []Entry
	for i := 0; i < len(fields); {
		meta := fields[i]
		i++
		src, dst, status, err := parseRecord(meta)
		if err != nil {
			return nil, err
		}
		paths, err := takePaths(fields, &i, status, meta)
		if err != nil {
			return nil, err
		}
		if src == submoduleMode || dst == submoduleMode {
			// Either side, not just the destination: a removed submodule has a
			// source mode of 160000 and a destination mode of zero, so a check
			// on the destination alone reads it as an ordinary deleted file.
			continue
		}
		entries = append(entries, records(status, paths)...)
	}
	return entries, nil
}

// parseRecord takes the source mode, destination mode and status letter out of
// one raw record's metadata.
func parseRecord(meta string) (string, string, byte, error) {
	if !strings.HasPrefix(meta, ":") {
		return "", "", 0, errors.Newf("raw diff record %q does not start with a colon", meta)
	}
	parts := strings.Fields(meta[1:])
	if len(parts) < 5 {
		return "", "", 0, errors.Newf("raw diff record %q has %d fields, not 5", meta, len(parts))
	}
	status := parts[4]
	if status == "" {
		return "", "", 0, errors.Newf("raw diff record %q has no status", meta)
	}
	return parts[0], parts[1], status[0], nil
}

// takePaths reads the one or two paths belonging to a record and advances i
// past them.
func takePaths(fields []string, i *int, status byte, meta string) ([]string, error) {
	want := 1
	if status == 'R' || status == 'C' {
		want = 2
	}
	if *i+want > len(fields) {
		return nil, errors.Newf("raw diff record %q names %d paths and the output holds %d", meta, want, len(fields)-*i)
	}
	paths := fields[*i : *i+want]
	*i += want
	return paths, nil
}

// records turns one parsed raw record into the entries it stands for.
func records(status byte, paths []string) []Entry {
	if len(paths) == 2 {
		return []Entry{
			{Path: clean(paths[0]), Deleted: true},
			{Path: clean(paths[1])},
		}
	}
	// D is the only status with no file to read. U is an unmerged path, whose
	// working copy holds the conflict markers, and that is what the agent is
	// looking at.
	return []Entry{{Path: clean(paths[0]), Deleted: status == 'D'}}
}

// untrackedFiles lists what git considers untracked and not ignored.
//
// An entry ending in a slash is an untracked directory holding its own .git,
// which git reports as one entry rather than as the files inside it. Reading it
// would open a directory as a file, and its contents belong to another
// repository.
func untrackedFiles(ctx context.Context, root string) ([]Entry, error) {
	out, err := run(ctx, root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	return parseUntracked(string(out)), nil
}

// parseUntracked reads the paths of a NUL separated ls-files listing.
//
// The trailing slash is dropped here rather than left to the lstat every path
// gets later. Both would keep the directory out of a capture, but a repository
// removed between the listing and the stat would come back as a vanished path
// and refuse the whole round, over something that was never review material.
func parseUntracked(out string) []Entry {
	var entries []Entry
	for _, p := range splitNUL(out) {
		if strings.HasSuffix(p, "/") {
			continue
		}
		entries = append(entries, Entry{Path: clean(p)})
	}
	return entries
}

// splitNUL breaks git's -z output into its records, dropping the empty piece
// after the final separator.
func splitNUL(out string) []string {
	out = strings.TrimSuffix(out, "\x00")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\x00")
}

// clean normalizes a path git printed into the form the store keeps.
func clean(p string) string {
	return path.Clean(strings.TrimPrefix(p, "./"))
}
