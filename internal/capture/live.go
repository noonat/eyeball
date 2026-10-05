package capture

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/cockroachdb/errors"
)

// ErrRulesUnreadable reports a live read refused because git failed to say
// whether it ignores the path. An error carrying it is ErrNotFound as well, so
// a caller that knows only ErrNotFound still refuses the read, and one that
// knows both can say why.
var ErrRulesUnreadable = errors.New("the ignore rules could not be read")

// escapeText is what os.Root says when a path leaves the root. Go 1.26 has no
// exported sentinel for it, so it is matched by text. The text is Go's own,
// not the operating system's or the locale's.
const escapeText = "path escapes from parent"

// fromWorking reads a path off disk as it is now, under the rules the reader's
// Live value names.
//
// The read goes in a fixed order. The parents are checked first, then git is
// asked whether it ignores the path, then the path itself is examined and
// read. Every step resolves the path through an os.Root opened on the project
// root, which refuses in the kernel a path that leaves it, so a directory
// swapped for a link after it was checked still cannot lead outside.
func (r *Reader) fromWorking(ctx context.Context, clean string) (io.ReadCloser, Source, error) {
	if r.live.Refused() {
		err := errors.Wrapf(ErrNotFound, "%s is in no frozen source, and the project allows no live read (%s)", clean, r.live)
		if r.live == LiveRulesUnreadable {
			err = errors.Mark(err, ErrRulesUnreadable)
		}
		return nil, "", err
	}
	root, err := os.OpenRoot(r.root)
	if err != nil {
		return nil, "", notFoundOr(err, "open the project root "+r.root)
	}
	// A file opened through the root keeps its own descriptor, so closing the
	// root here leaves the returned file readable.
	defer func() { _ = root.Close() }()

	if err := checkParents(root, clean); err != nil {
		return nil, "", err
	}
	if r.live == LiveIgnoreRules {
		if err := refuseIgnored(ctx, r.root, clean); err != nil {
			return nil, "", err
		}
	}
	r.step("checked")

	name := filepath.FromSlash(clean)
	info, err := root.Lstat(name)
	if err != nil {
		return nil, "", notFoundOr(err, "stat "+clean)
	}
	r.step("stat")
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		// A link reads as the text it points at, which is what git stores for
		// one and what a capture freezes, so a live link compares cleanly
		// against the base. Readlink never follows it, so a target outside
		// the root is only text.
		target, err := root.Readlink(name)
		if err != nil {
			return nil, "", notFoundOr(err, "read the link "+clean)
		}
		return io.NopCloser(strings.NewReader(target)), SourceWorking, nil
	case info.Mode().IsRegular():
		f, err := openRegular(root, name, info)
		if err != nil {
			return nil, "", err
		}
		return f, SourceWorking, nil
	}
	// A directory, a socket or a FIFO. Nothing here opens one, for the same
	// reasons a capture does not.
	return nil, "", errors.Wrapf(ErrNotFound, "%s is not a regular file", clean)
}

// step runs the test's pause, if there is one.
func (r *Reader) step(name string) {
	if r.pause != nil {
		r.pause(name)
	}
}

// checkParents refuses a path whose parent directories lead somewhere the
// capture and the tree never list: through a symlink, past a file, or into a
// directory holding another repository.
//
// os.Root follows a link that stays inside the root, so it does not keep this
// rule on its own. It runs before check-ignore as well, since git exits 128
// for a path beyond a symlink, and a path inside a nested repository is one
// git does not ignore.
func checkParents(root *os.Root, clean string) error {
	parts := strings.Split(clean, "/")
	for i := 1; i < len(parts); i++ {
		dir := strings.Join(parts[:i], "/")
		info, err := root.Lstat(filepath.FromSlash(dir))
		if err != nil {
			return notFoundOr(err, "stat "+dir)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return errors.Wrapf(ErrNotFound, "%s runs through %s, a symlink, so the working copy is not read for it", clean, dir)
		}
		if !info.IsDir() {
			return errors.Wrapf(ErrNotFound, "%s runs through %s, which is not a directory", clean, dir)
		}
		_, err = root.Lstat(filepath.FromSlash(dir + "/" + gitDir))
		if err == nil {
			return errors.Wrapf(ErrNotFound, "%s is inside %s, which holds a repository of its own", clean, dir)
		}
		if !missing(err) {
			return errors.Wrapf(err, "look for %s in %s", gitDir, dir)
		}
	}
	return nil
}

// refuseIgnored asks git whether it ignores a path, and refuses one it does.
//
// check-ignore exits 0 for an ignored path and 1 for one that is not. The path
// goes as ./<path>, because a bare leading colon is read as pathspec magic: an
// ignored :other.txt then exits 1 and reads as not ignored. 128 for a path
// inside a submodule is read as not found, since a submodule's files belong to
// another repository, as they do in the capture and the tree. That is decided
// from git's message, so the call pins the locale: under LANGUAGE=de git
// writes the message in German. Any other failure refuses the read with
// ErrRulesUnreadable.
func refuseIgnored(ctx context.Context, root, clean string) error {
	cmd := gitCommand(ctx, root, []string{"check-ignore", "-q", "--", "./" + clean})
	// exec keeps the last of two entries for one variable, so these win over
	// whatever the daemon inherited.
	cmd.Env = append(cmd.Env, "LC_ALL=C", "LANGUAGE=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return errors.Wrapf(ErrNotFound, "%s is ignored by git", clean)
	}
	code := exitCode(err)
	if code == 1 {
		return nil
	}
	if code == 128 && inSubmodule(stderr.String()) {
		return errors.Wrapf(ErrNotFound, "%s is inside a submodule", clean)
	}
	unreadable := errors.Wrapf(ErrRulesUnreadable, "git check-ignore %s: %s", clean, message(&stderr))
	return errors.Mark(errors.WithSecondaryError(unreadable, err), ErrNotFound)
}

// inSubmodule reports whether git's message says a path is inside a
// submodule, as "fatal: Pathspec './mod/m.go' is in submodule 'mod'".
func inSubmodule(stderr string) bool {
	return strings.HasPrefix(stderr, "fatal: Pathspec '") && strings.Contains(stderr, "' is in submodule '")
}

// openRegular opens a file that Lstat found to be regular, and refuses it if
// what opened is not that file.
//
// Something can be swapped in between the Lstat and the open. O_NONBLOCK keeps
// a FIFO swapped in from blocking the open until a writer appears, and
// SameFile refuses anything other than the file the Lstat saw. The regular
// check covers an inode freed and reused by a FIFO in that window, which
// SameFile alone would pass.
func openRegular(root *os.Root, name string, info fs.FileInfo) (*os.File, error) {
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, notFoundOr(err, "read "+name)
	}
	opened, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, errors.Wrapf(err, "stat %s", name)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		_ = f.Close()
		return nil, errors.Wrapf(ErrNotFound, "%s changed while it was being read", name)
	}
	return f, nil
}

// notFoundOr wraps err as ErrNotFound where it means the path is not there,
// and as itself otherwise. A real read failure stays an error, since it is not
// a page that does not exist.
func notFoundOr(err error, op string) error {
	if missing(err) {
		return errors.Wrap(errors.Mark(err, ErrNotFound), op)
	}
	return errors.Wrap(err, op)
}

// missing reports an error from a working-copy read that means the path is
// not there to read.
//
// Beyond a path that does not exist, Lstat through os.Root returns ENOTDIR
// below a file, ENAMETOOLONG for a segment past the system's limit, ELOOP
// through a link to itself, and its own error for a path that leaves the root.
// None of them is os.ErrNotExist, and each would otherwise answer a typed
// address with a server error.
func missing(err error) bool {
	if errors.Is(err, os.ErrNotExist) ||
		errors.Is(err, syscall.ENOTDIR) ||
		errors.Is(err, syscall.ENAMETOOLONG) ||
		errors.Is(err, syscall.ELOOP) {
		return true
	}
	var pathErr *fs.PathError
	return errors.As(err, &pathErr) && pathErr.Err != nil && pathErr.Err.Error() == escapeText
}
