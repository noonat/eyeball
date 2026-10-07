package capture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/store"
)

// ErrBadCommit reports a ref a check was given that names no commit.
var ErrBadCommit = errors.New("not a commit")

// ResolveCommit is a ref as the full name of the commit it points at.
func ResolveCommit(ctx context.Context, root, ref string) (string, error) {
	out, err := run(ctx, root, baseArgs(ref)...)
	if err != nil {
		return "", errors.Wrapf(ErrBadCommit, "%q", ref)
	}
	return strings.TrimSpace(string(out)), nil
}

// Match compares a round's capture with the tree of the commit target, both
// measured from the commit base, and returns the first difference as a
// sentence naming its path, or an empty string where they match.
//
// They match when every path target changes from base was captured in scope
// with target's bytes, or as a deletion where target lacks it, and every path
// captured in scope is one target changes. A captured file differs from the
// base by definition, so one target leaves as base had it is approved work
// that does not arrive. A file captured out of scope that target leaves alone
// is ignored, since nobody approved it and nothing of it arrives.
//
// Only commits are read, never the index or the working tree: a merge holds
// index.lock while a reference-transaction hook runs.
func Match(ctx context.Context, root, base, target string, files []store.RoundFile) (string, error) {
	changed, err := changedBetween(ctx, root, base, target)
	if err != nil {
		return "", err
	}
	captured := make(map[string]store.RoundFile, len(files))
	for _, f := range files {
		captured[f.Path] = f
	}

	paths := make([]string, 0, len(changed)+len(files))
	seen := map[string]bool{}
	for p := range changed {
		paths, seen[p] = append(paths, p), true
	}
	for _, f := range files {
		if f.InScope && !seen[f.Path] {
			paths, seen[f.Path] = append(paths, f.Path), true
		}
	}
	sort.Strings(paths)

	for _, p := range paths {
		change, moved := changed[p]
		f, held := captured[p]
		switch {
		case !moved && f.Digest == "":
			return fmt.Sprintf("the approved round deletes %s, and the new tree keeps it", p), nil
		case !moved:
			holds, err := treeHolds(ctx, root, target, p)
			if err != nil {
				return "", err
			}
			if !holds {
				return fmt.Sprintf("the approved round holds %s, and the new tree does not", p), nil
			}
			return fmt.Sprintf("the approved round changes %s, and the new tree does not", p), nil
		case change.gitlink:
			return fmt.Sprintf("%s is a submodule, which no round captures", p), nil
		case !held:
			return fmt.Sprintf("%s changes in the new tree and is not in the approved round", p), nil
		case !f.InScope:
			return fmt.Sprintf("%s changes in the new tree and is outside the approved review's paths", p), nil
		case change.deleted && f.Digest != "":
			return fmt.Sprintf("the new tree deletes %s, and the approved round keeps it", p), nil
		case change.deleted:
			continue
		case f.Digest == "":
			return fmt.Sprintf("the approved round deletes %s, and the new tree keeps it", p), nil
		}
		digest, err := blobDigest(ctx, root, change.object)
		if err != nil {
			return "", err
		}
		if digest != f.Digest {
			return fmt.Sprintf("%s differs from the approved round", p), nil
		}
	}
	return "", nil
}

// treeChange is one path two commits' trees disagree on.
type treeChange struct {
	// object is the blob the later tree holds, empty where it deletes the
	// path.
	object string
	// deleted says the later tree lacks the path.
	deleted bool
	// gitlink says either side is a submodule, whose content is another
	// repository's and which no capture records.
	gitlink bool
}

// changedBetween is every path whose entry differs between two commits'
// trees, by diff-tree, which compares objects and reads neither the index nor
// the working tree. Renames are two paths, as a capture records them.
func changedBetween(ctx context.Context, root, base, target string) (map[string]treeChange, error) {
	out, err := run(ctx, root, "diff-tree", "-r", "-z", "--no-renames", "--raw", base, target)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	changes := map[string]treeChange{}
	for i := 0; i+1 < len(fields); i += 2 {
		meta, p := fields[i], fields[i+1]
		// :oldmode newmode oldsha newsha status
		parts := strings.Fields(strings.TrimPrefix(meta, ":"))
		if len(parts) != 5 {
			return nil, errors.Newf("diff-tree entry %q has %d fields, not 5", meta, len(parts))
		}
		c := treeChange{object: parts[3], deleted: parts[4] == "D"}
		if parts[0] == submoduleMode || parts[1] == submoduleMode {
			c.gitlink = true
		}
		if c.deleted {
			c.object = ""
		}
		changes[clean(p)] = c
	}
	return changes, nil
}

// treeHolds reports whether a commit's tree has an entry at a path. A stray
// file a capture took and the commit lacks is told apart from a change the
// commit left out, so the sentence names the right fix.
func treeHolds(ctx context.Context, root, commit, p string) (bool, error) {
	out, err := run(ctx, root, "--literal-pathspecs", "ls-tree", "-z", commit, "--", p)
	if err != nil {
		return false, err
	}
	return len(out) > 0, nil
}

// blobDigest is the SHA-256 of one blob's bytes, as the blob store names
// content. A symlink's blob is the target it names, which is also what a
// capture stores for one.
func blobDigest(ctx context.Context, root, object string) (string, error) {
	cmd := gitCommand(ctx, root, []string{"cat-file", "blob", object})
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.Wrapf(err, "read object %s", object)
	}
	if err := cmd.Start(); err != nil {
		return "", errors.Wrapf(err, "read object %s", object)
	}
	sum := sha256.New()
	_, copyErr := io.Copy(sum, stdout)
	if err := cmd.Wait(); err != nil {
		return "", errors.Wrapf(err, "read object %s", object)
	}
	if copyErr != nil {
		return "", errors.Wrapf(copyErr, "read object %s", object)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}
