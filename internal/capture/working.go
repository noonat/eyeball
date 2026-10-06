package capture

import (
	"context"
	"os"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/diff"
)

// WorkingCopy is what differs on disk from HEAD in a project, as it is now.
//
// It reuses the capture's listing and stores nothing: no Freeze, no blob. Its
// files are read off disk when asked for, and each read can see a change made
// after the listing, since an agent may be editing the tree that second.
type WorkingCopy struct {
	// State is what Inspect found about the project, with no round's base.
	State State
	// Listed reports that git listed what changed. It is false for a state
	// that refuses every live read, and for a project with no repository of
	// its own at its root, whose changes git cannot list against its root.
	Listed bool
	// Head is the commit HEAD names, empty on a branch with no commit yet,
	// where every file reads as added.
	Head string
	// Entries is every path that differs from HEAD, tracked or untracked, in
	// path order. Ignored files are left out, and so is a file the live read
	// refuses, such as one inside a directory holding a repository of its own.
	Entries []Entry
	// ListErr is why git could not list a git project's changes, and nil
	// where it listed them or was never asked. Only ErrBadBase lands here: a
	// HEAD that names no commit and is not a branch waiting for its first,
	// which gives git nothing to compare the disk with. Any other failure
	// fails the call.
	ListErr error

	// head reads HEAD with no capture, so its Frozen reads the commit.
	head *Reader
	// listed maps each entry's path to the entry.
	listed map[string]Entry
}

// ReadWorkingCopy lists what differs on disk from HEAD in the project at root,
// given whether the store has seen the project inside a checkout.
//
// Inspect answers first. ResolveBase returns an empty base both for a project
// outside git and for a branch with no commit, and the two need different
// answers, while a missing root, a lost repository or a refused live read
// cannot be listed at all. Only a git project is listed: at a marker root
// inside a checkout, git diff prints paths from the outer repository's top
// level and ls-files from the marker root, so the listing would be wrong.
func ReadWorkingCopy(ctx context.Context, root string, insideCheckout bool) (*WorkingCopy, error) {
	st, err := Inspect(ctx, root, "", insideCheckout)
	if err != nil {
		return nil, err
	}
	wc := &WorkingCopy{State: st}
	if st.Live.Refused() || !st.Reader(nil, nil).git {
		return wc, nil
	}
	head, err := ResolveBase(ctx, st.Root, "")
	if errors.Is(err, ErrBadBase) {
		wc.ListErr = err
		return wc, nil
	}
	if err != nil {
		return nil, err
	}
	entries, err := changed(ctx, st.Root, head, true)
	if err != nil {
		return nil, err
	}
	if entries, err = dropRefused(st.Root, entries); err != nil {
		return nil, err
	}
	at := st
	at.Base = head
	wc.Listed, wc.Head, wc.Entries, wc.head = true, head, entries, at.Reader(nil, nil)
	wc.listed = make(map[string]Entry, len(entries))
	for _, e := range entries {
		wc.listed[e.Path] = e
	}
	return wc, nil
}

// dropRefused leaves out a present entry whose parents the live read refuses.
//
// git diff still lists a tracked file below a directory that has since gained
// a .git of its own, and the live read refuses it as another repository's
// file. Listed, it would read as deleted on disk and show every line removed,
// a wrong count shown as a right one, and its link would answer the 404 page.
// A deletion is kept, since nothing reads its disk side.
func dropRefused(root string, entries []Entry) ([]Entry, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, errors.Wrapf(err, "open the project root %s", root)
	}
	defer func() { _ = r.Close() }()
	out := entries[:0]
	for _, e := range entries {
		if !e.Deleted {
			err := checkParents(r, e.Path)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// Holds reports whether the listing holds p, and the entry it holds.
func (w *WorkingCopy) Holds(p string) (Entry, bool) {
	e, held := w.listed[p]
	return e, held
}

// Compare diffs each of paths as HEAD holds it against the disk now, in the
// order given.
//
// HEAD's object ids for every path are loaded in one batch first, since left
// to itself each read runs an ls-tree of its own. Each path then costs one
// cat-file for HEAD's side and one check-ignore for the disk's.
func (w *WorkingCopy) Compare(ctx context.Context, paths []string) ([]Moved, error) {
	if len(paths) > 0 && w.head.base != "" {
		if err := w.head.loadOIDs(ctx, paths); err != nil {
			return nil, err
		}
	}
	out := make([]Moved, 0, len(paths))
	for _, p := range paths {
		head, disk, tooLarge, err := w.Sides(ctx, p)
		if err != nil {
			return nil, err
		}
		out = append(out, Moved{Path: p, Result: measured(head, disk, tooLarge)})
	}
	return out, nil
}

// Sides reads p as HEAD holds it and as the disk holds it now, each stopping
// one byte past the diff's limit, and reports a side past it rather than
// holding it.
//
// HEAD's side reads through a reader with no capture, and the disk's through
// ReadLive, which reads a link as its target text, as HEAD stores one. A path
// absent from either side reads as nothing, which is what makes an addition
// and a deletion fall out of one comparison.
func (w *WorkingCopy) Sides(ctx context.Context, p string) ([]byte, []byte, bool, error) {
	e, held := w.Holds(p)
	if !held {
		return nil, nil, false, errors.Wrapf(ErrNotFound, "%s is not in the working copy's listing", p)
	}
	head, tooLarge, err := w.head.Side(ctx, p)
	if err != nil || tooLarge {
		return nil, nil, tooLarge, err
	}
	if e.Deleted {
		return head, nil, false, nil
	}
	rc, err := ReadLive(ctx, w.State, p)
	switch {
	case errors.Is(err, ErrRulesUnreadable):
		// Not an absence. Reading it as one would show every line removed.
		return nil, nil, false, err
	case errors.Is(err, ErrNotFound):
		// Gone since the listing, which is a deletion as of now.
		return head, nil, false, nil
	case err != nil:
		return nil, nil, false, err
	}
	disk, tooLarge, err := readSide(rc, p)
	if err != nil || tooLarge {
		return nil, nil, tooLarge, err
	}
	return head, disk, false, nil
}

// measured is the comparison of two sides as a count, the rule every count of
// a change shares: a side past the byte limit is too large, and a result past
// the line limit carries no counts.
func measured(old, current []byte, tooLarge bool) diff.Result {
	if tooLarge {
		return diff.Result{TooLarge: true}
	}
	result := diff.Text(old, current)
	if result.TooLarge {
		// The line limit's own answer is the length of each side. As a size
		// it would read as a one-line edit moving twenty-five thousand lines,
		// so it is dropped here, and the byte limit says the same thing by
		// never reading the file.
		result.Added, result.Removed = 0, 0
	}
	return result
}
