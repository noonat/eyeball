package capture

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/blob"
	"github.com/noonat/eyeball/internal/store"
)

// ErrNothingCaptured reports a first round whose capture holds no files. The
// agent named the wrong paths, forgot to write a file, or resolved a base that
// already contains its work.
var ErrNothingCaptured = errors.New("nothing differs from the base")

// ErrBaseGained reports a review opened before its repository had any commits,
// on a round taken after one landed.
var ErrBaseGained = errors.New("the repository has a commit now")

// Options is what a round is captured from.
type Options struct {
	// Root is the project root, which for a git project is the repository's
	// top level.
	Root string
	// Base is the commit to measure against, empty where there is none. It is
	// the review's own base rather than a ref to resolve, because a review's
	// base is fixed at its first round.
	Base string
	// Paths are the review's paths. They decide what a capture holds only
	// where there is no git, since git is asked what changed instead.
	Paths []string
	// Round is which round this is, counting from one. It is not inferred from
	// Previous: a later round may legally capture nothing, so an empty file
	// list is not the same fact as there being no round before this one.
	Round int
	// Previous is the previous round's file list, and is empty on a first
	// round and on a later round that captured nothing.
	Previous []store.File
	// Blobs is where captured content is written.
	Blobs *blob.Store
}

// Freeze captures a round and returns what the store needs to record it.
//
// The note is left empty for the caller to fill. Capture does not read it and
// the store refuses a request without one, so a caller that forgets fails
// loudly rather than recording a round with no brief.
func Freeze(ctx context.Context, opts Options) (store.Request, error) {
	if opts.Round < 1 {
		return store.Request{}, errors.Newf("a capture needs its round number, counting from one, and got %d", opts.Round)
	}
	git, err := isGitProject(ctx, opts.Root)
	if err != nil {
		return store.Request{}, err
	}
	if err := checkBase(ctx, opts, git); err != nil {
		return store.Request{}, err
	}

	entries, err := enumerate(ctx, opts, git)
	if err != nil {
		return store.Request{}, err
	}
	files, err := freezeAll(opts, entries)
	if err != nil {
		return store.Request{}, err
	}
	if len(files) == 0 && opts.Round == 1 {
		return store.Request{}, errors.Wrapf(ErrNothingCaptured, "%s, so name the paths the work is in", describeBase(opts.Base))
	}

	before := NewReader(opts.Root, opts.Base, opts.Blobs, opts.Previous)
	now := NewReader(opts.Root, opts.Base, opts.Blobs, files)
	changes, err := measure(ctx, opts, files, before, now)
	if err != nil {
		return store.Request{}, err
	}
	return store.Request{BaseCommit: opts.Base, Files: files, Changes: changes}, nil
}

// checkBase refuses a later round whose review has no base in a repository that
// has since gained a commit.
//
// An empty base resolves to the empty tree on every round, and what the empty
// tree is compared against grows as the repository does, so such a round would
// capture every tracked file rather than the agent's work. Nothing else catches
// it, because an empty base always resolves and the missing-base refusal never
// fires. Re-resolving the base instead is the moving base this design rejects,
// arriving through the one door left open.
func checkBase(ctx context.Context, opts Options, git bool) error {
	if !git || opts.Base != "" || opts.Round == 1 {
		return nil
	}
	now, err := ResolveBase(ctx, opts.Root, "")
	if err != nil {
		return err
	}
	if now == "" {
		return nil
	}
	return errors.Wrapf(ErrBaseGained, "%s, so open a new review against it", now)
}

// enumerate asks git what changed, or walks the review's paths where there is
// no git to ask.
func enumerate(ctx context.Context, opts Options, git bool) ([]Entry, error) {
	if git {
		return Changed(ctx, opts.Root, opts.Base)
	}
	return Walk(ctx, opts.Root, opts.Paths)
}

// freezeAll writes the content of every entry and returns the round's files.
func freezeAll(opts Options, entries []Entry) ([]store.File, error) {
	files := make([]store.File, 0, len(entries))
	for _, e := range entries {
		f, err := freezeFile(opts.Root, opts.Blobs, e)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

// freezeFile puts one entry's content in the blob store.
func freezeFile(root string, blobs *blob.Store, e Entry) (store.File, error) {
	if e.Deleted {
		// A path the agent removed differs from the base and has no content to
		// store. Leaving it out would make a deletion indistinguishable from a
		// file that never changed.
		return store.File{Path: e.Path}, nil
	}
	full := filepath.Join(root, filepath.FromSlash(e.Path))
	if e.Symlink {
		return freezeLink(blobs, e.Path, full)
	}
	return storeContent(blobs, e.Path, func() (io.ReadCloser, error) {
		return open(e.Path, full)
	})
}

// opener yields a path's bytes. It exists because the miss path reads twice,
// and the two reads are what can disagree.
type opener func() (io.ReadCloser, error)

// storeContent hashes a path, asks the store whether it already holds the
// content, and writes it only on a miss.
//
// Hash, ask, then write: a digest is known only after the whole stream has been
// read, so Put cannot make this saving itself. Round N+1 recaptures every file
// that still differs from the base, and most are byte for byte what round N
// captured.
//
// Put does refuse to duplicate content, so skipping the question would still
// leave one blob. What it would cost is a second read of the file and the
// temporary Put writes and then removes, once per unchanged file per round.
//
// The digest recorded is the one describing the bytes the store now has. On a
// miss that is Put's answer rather than the hashing pass's, because a file
// rewritten between the two reads makes them differ, and recording the first
// would leave a round naming content the store does not hold. On a hit nothing
// is written, so the hashing pass supplies both the digest and the size.
func storeContent(blobs *blob.Store, path string, next opener) (store.File, error) {
	digest, size, err := hashOnce(path, next)
	if err != nil {
		return store.File{}, err
	}
	held, err := blobs.Has(digest)
	if err != nil {
		return store.File{}, errors.Wrapf(err, "look for %s in the blob store", path)
	}
	if held {
		return store.File{Path: path, Digest: digest, Size: size}, nil
	}

	r, err := next()
	if err != nil {
		return store.File{}, err
	}
	defer func() { _ = r.Close() }()
	digest, size, err = blobs.Put(r)
	if err != nil {
		return store.File{}, errors.Wrapf(err, "store %s", path)
	}
	return store.File{Path: path, Digest: digest, Size: size}, nil
}

// freezeLink stores a symbolic link, whose content is the path it names.
//
// Following the link instead would capture whatever is at the other end, or
// fail where that is a directory or is missing, and neither is what the agent
// changed.
func freezeLink(blobs *blob.Store, path, full string) (store.File, error) {
	target, err := os.Readlink(full)
	if err != nil {
		return store.File{}, vanished(path, err)
	}
	digest, size, err := blobs.Put(bytes.NewReader([]byte(target)))
	if err != nil {
		return store.File{}, errors.Wrapf(err, "store %s", path)
	}
	return store.File{Path: path, Digest: digest, Size: size}, nil
}

// hashOnce streams one read and returns the digest and length of what it saw.
func hashOnce(path string, next opener) (string, int64, error) {
	r, err := next()
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = r.Close() }()
	sum := sha256.New()
	size, err := io.Copy(sum, r)
	if err != nil {
		return "", 0, errors.Wrapf(err, "read %s", path)
	}
	return hex.EncodeToString(sum.Sum(nil)), size, nil
}

// open reads a file the enumeration named, reporting a path that has gone as a
// vanished one rather than as an ordinary failure.
func open(path, full string) (io.ReadCloser, error) {
	f, err := os.Open(full)
	if err != nil {
		return nil, vanished(path, err)
	}
	return f, nil
}

// vanished turns a missing file into the refusal that names what happened.
//
// The gap between the enumeration and the read is small and not zero. Recording
// the path as deleted would put a deletion the agent did not make into a frozen
// round, so this refuses instead, and the agent can take the round again.
func vanished(path string, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return errors.Wrapf(ErrVanished, "%s, so the working copy moved during capture", path)
	}
	return errors.Wrapf(err, "read %s", path)
}

// describeBase names a base in a refusal, including the one that is not there.
func describeBase(base string) string {
	if base == "" {
		return "no base to measure against and no files under the review's paths"
	}
	return "nothing differs from " + base
}
