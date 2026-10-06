package capture

import (
	"context"
	"io"
	"sort"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/diff"
	"github.com/noonat/eyeball/internal/store"
)

// measure works out what moved between the previous round and this one.
//
// This is where the round's stored size comes from, and it is computed at
// capture time rather than when the queue renders. The queue re-renders on
// every live update, and diffing every waiting review's latest round each time
// is work that never stops.
func measure(ctx context.Context, before, now *Reader) ([]store.Change, error) {
	results, err := Compare(ctx, before, now)
	if err != nil {
		return nil, err
	}
	// Every path that moved gets a row, including one that moved no lines. A
	// mode change, a checkout filter and a binary file all land here, and
	// leaving them out would make a first round's file count smaller than the
	// number of files it captured.
	changes := make([]store.Change, 0, len(results))
	for _, m := range results {
		changes = append(changes, store.Change{Path: m.Path, Added: m.Result.Added, Removed: m.Result.Removed})
	}
	return changes, nil
}

// Moved is one path whose state differs between two captures, and what moved.
type Moved struct {
	// Path is the path.
	Path string
	// Result is the comparison of the path as of each capture. A too-large
	// result carries no line counts.
	Result diff.Result
}

// Compare diffs, path by path, every path whose state differs between the
// capture before reads and the capture now reads, in path order.
//
// A round's stored size is summed from this, and the surface renders it, so
// what the page shows and what the queue counts cannot disagree. A reader
// holding no capture compares a round with its base.
//
// The two readers are supplied rather than built here, so how many times this
// asks git can be counted from outside. Nothing in the results says whether it
// asked once or once per file.
func Compare(ctx context.Context, before, now *Reader) ([]Moved, error) {
	return ComparePaths(ctx, before, now, moved(before.capture(), now.capture()))
}

// ComparePaths diffs each of paths as before reads it against how now reads
// it, in the order given.
//
// It is Compare for a caller that already knows which paths it wants, such as
// a tree page counting the few rows it shows, where comparing everything that
// moved would diff the whole capture to label a handful of rows.
func ComparePaths(ctx context.Context, before, now *Reader, paths []string) ([]Moved, error) {
	// Left to the per-path lookup this is one git process per file, and on a
	// first round every path resolves from the base, so it is one per captured
	// file. Asking about every path first takes as few calls as the argument
	// list allows.
	if len(paths) > 0 {
		if before.base != "" {
			if err := before.loadOIDs(ctx, paths); err != nil {
				return nil, err
			}
		}
		if now.base != "" {
			if err := now.loadOIDs(ctx, paths); err != nil {
				return nil, err
			}
		}
	}
	out := make([]Moved, 0, len(paths))
	for _, p := range paths {
		result, err := compareFile(ctx, before, now, p)
		if err != nil {
			return nil, err
		}
		out = append(out, Moved{Path: p, Result: result})
	}
	return out, nil
}

// moved lists the paths whose state differs between two captures, in order.
//
// A capture holds what differs from the base, so a path it does not name is a
// path matching the base. That is a third state, distinct from a path the
// capture names as deleted, and a comparison that folded them together would
// miss a file the agent removed and then put back.
//
// Comparing digests is what keeps this cheap: a file that still differs from
// the base but has not moved since the last round is most of a later round's
// capture, and it is skipped without being read.
func moved(previous, current []store.File) []string {
	was := states(previous)
	is := states(current)
	seen := make(map[string]struct{}, len(was)+len(is))
	var out []string
	sides := []map[string]string{was, is}
	for _, side := range sides {
		for p := range side {
			if _, done := seen[p]; done {
				continue
			}
			seen[p] = struct{}{}
			if !differs(was, is, p) {
				continue
			}
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// Differs reports whether p's state differs between two captures, by the rule
// Compare uses to choose what it diffs: present in one and not the other, or
// held by both with different content.
//
// It is how a page asks whether one path moved without diffing every path
// that did, so a file's page and the review page agree on what moved.
func Differs(previous, current []store.File, p string) bool {
	return differs(states(previous), states(current), p)
}

// differs is Differs over captures already mapped by states.
func differs(was, is map[string]string, p string) bool {
	oldDigest, hadOld := was[p]
	newDigest, hasNew := is[p]
	return hadOld != hasNew || oldDigest != newDigest
}

// states maps each captured path to its digest, which is empty for a path the
// capture names as deleted.
func states(files []store.File) map[string]string {
	out := make(map[string]string, len(files))
	for _, f := range files {
		out[f.Path] = f.Digest
	}
	return out
}

// compareFile diffs one path between the two rounds.
//
// Both sides are read frozen, never from the working copy, because a round is
// what is being compared and the working copy has moved on.
func compareFile(ctx context.Context, before, now *Reader, p string) (diff.Result, error) {
	old, tooBig, err := before.Side(ctx, p)
	if err != nil {
		return diff.Result{}, err
	}
	if tooBig {
		return diff.Result{TooLarge: true}, nil
	}
	current, tooBig, err := now.Side(ctx, p)
	if err != nil {
		return diff.Result{}, err
	}
	if tooBig {
		return diff.Result{TooLarge: true}, nil
	}
	result := diff.Text(old, current)
	if result.TooLarge {
		// The line limit's own answer is the length of each side. As a round's
		// size it would read as a one-line edit moving twenty-five thousand
		// lines, so it is dropped here, and the byte limit above says the same
		// thing by never reading the file.
		result.Added, result.Removed = 0, 0
	}
	return result, nil
}

// Side reads p as one side of a comparison, from the reader's frozen sources,
// reporting a file too large to diff rather than holding it.
//
// A path that does not exist as of that round reads as nothing, which is what
// makes an addition and a deletion fall out of the same comparison.
func (r *Reader) Side(ctx context.Context, p string) ([]byte, bool, error) {
	rc, _, err := r.Frozen(ctx, p)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer func() { _ = rc.Close() }()

	// One byte past the limit is enough to know the diff will refuse it, and
	// stopping there is what keeps a large file out of memory.
	content, err := io.ReadAll(io.LimitReader(rc, diff.MaxBytes+1))
	if err != nil {
		return nil, false, errors.Wrapf(err, "read %s", p)
	}
	if len(content) > diff.MaxBytes {
		return nil, true, nil
	}
	return content, false, nil
}
