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
// The two readers are supplied rather than built here, so how many times this
// asks git can be counted from outside. Nothing in the changes it returns says
// whether it asked once or once per file.
//
// This is where the round's stored size comes from, and it is computed at
// capture time rather than when the queue renders. The queue re-renders on
// every live update, and diffing every waiting review's latest round each time
// is work that never stops.
func measure(ctx context.Context, opts Options, files []store.File, before, now *Reader) ([]store.Change, error) {
	if err := primedFor(ctx, opts, before, now, files); err != nil {
		return nil, err
	}

	var changes []store.Change
	for _, p := range moved(opts.Previous, files) {
		added, removed, err := countLines(ctx, before, now, p)
		if err != nil {
			return nil, err
		}
		// Every path that moved gets a row, including one that moved no lines.
		// A mode change, a checkout filter and a binary file all land here, and
		// leaving them out would make a first round's file count smaller than
		// the number of files it captured.
		changes = append(changes, store.Change{Path: p, Added: added, Removed: removed})
	}
	return changes, nil
}

// primedFor asks the base about every path a round will compare, in as few
// calls as the argument list allows.
//
// Left to the per-path lookup this is one git process per file, and on a first
// round every path resolves from the base, so it is one per captured file.
func primedFor(ctx context.Context, opts Options, before, now *Reader, files []store.File) error {
	paths := moved(opts.Previous, files)
	if opts.Base == "" || len(paths) == 0 {
		return nil
	}
	if err := before.loadOIDs(ctx, paths); err != nil {
		return err
	}
	return now.loadOIDs(ctx, paths)
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
			oldDigest, hadOld := was[p]
			newDigest, hasNew := is[p]
			if hadOld == hasNew && oldDigest == newDigest {
				continue
			}
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
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

// countLines diffs one path between the two rounds.
//
// Both sides are read frozen, never from the working copy, because a round is
// what is being measured and the working copy has moved on.
func countLines(ctx context.Context, before, now *Reader, p string) (int, int, error) {
	old, tooBig, err := readSide(ctx, before, p)
	if err != nil {
		return 0, 0, err
	}
	if tooBig {
		return 0, 0, nil
	}
	current, tooBig, err := readSide(ctx, now, p)
	if err != nil {
		return 0, 0, err
	}
	if tooBig {
		return 0, 0, nil
	}
	result := diff.Text(old, current)
	if result.TooLarge {
		// The line limit's own answer is the length of each side, which is
		// what a renderer shows for a file it will not diff. As a round's size
		// it would read as a one-line edit moving twenty-five thousand lines,
		// so it is dropped here rather than stored. The byte limit above says
		// the same thing by never reading the file.
		return 0, 0, nil
	}
	return result.Added, result.Removed, nil
}

// readSide reads one side of a comparison, reporting a file too large to diff
// rather than holding it.
//
// A path that does not exist as of that round reads as nothing, which is what
// makes an addition and a deletion fall out of the same comparison.
func readSide(ctx context.Context, r *Reader, p string) ([]byte, bool, error) {
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
