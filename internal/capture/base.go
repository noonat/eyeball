package capture

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
)

// ErrBadBase reports a ref that does not name a commit, which covers a ref that
// no longer resolves and one that names a path or a tree.
var ErrBadBase = errors.New("base does not name a commit")

// headRef is what an empty ref means on a review's first round.
const headRef = "HEAD"

// ResolveBase turns ref into the commit id a round is captured against.
//
// An empty ref means HEAD, which is what a first round asks for when the agent
// named nothing. An empty return means there is nothing to measure against: a
// project with no git, or a repository with no commits yet.
//
// Those two answers are not the same thing and must not be folded together. An
// empty return says this review has no base. It is never a request to resolve
// HEAD again later, because a review's base is fixed at its first round and a
// later round is handed the commit already recorded on it.
func ResolveBase(ctx context.Context, root, ref string) (string, error) {
	git, err := isGitProject(ctx, root)
	if err != nil {
		return "", err
	}
	if !git {
		return "", nil
	}

	named := ref != ""
	if !named {
		ref = headRef
	}
	out, err := run(ctx, root, baseArgs(ref)...)
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	if named {
		return "", errors.Wrapf(ErrBadBase, "%q, so name a commit that exists or open a new review", ref)
	}

	// HEAD did not resolve. On a branch that has no commit yet that is the
	// expected answer and the base is empty. Anything else is a repository
	// whose HEAD is broken, where reporting no base would capture every
	// tracked file against the empty tree.
	if !headIsUnborn(ctx, root) {
		return "", errors.Wrapf(ErrBadBase, "HEAD in %s, so name a commit that exists", root)
	}
	return "", nil
}

// baseArgs is the rev-parse call that turns a ref into a commit id.
//
// --verify with the ^{commit} peel is the check, and rev-parse alone is not: it
// exits zero for a path in the working tree, printing the path back, and a tree
// id resolves to itself. Either would be stored as a base and handed to git
// diff, which reads a path as a pathspec and compares the index to the working
// tree instead of comparing against the base.
//
// --end-of-options separates the agent's string from the flags. It does not
// change what this call decides, because the appended peel stops a ref from
// matching an option exactly, and every spelling tried exits non-zero with or
// without it. What it stops is git reading an agent's string as an option at
// all: without it, git 2.43 parses --path-format=relative^{commit} as the
// option and complains about its value. The argv is built here so that
// separator cannot be dropped by an edit somewhere else.
func baseArgs(ref string) []string {
	return []string{"rev-parse", "--verify", "--quiet", "--end-of-options", ref + "^{commit}"}
}

// headIsUnborn reports whether HEAD names a branch that has no commit yet.
// Unborn is git's own word for it, used in git-diff, git-worktree and the v2
// protocol.
//
// It is HEAD that is unborn rather than the repository, which is not a
// distinction without a difference: after checkout --orphan the repository has
// branches and commits and this still answers true. Naming the function for the
// repository, as an emptiness check would, describes the wrong subject in
// exactly the case that separates the two.
//
// symbolic-ref answers it directly: on an unborn branch HEAD still points at
// refs/heads/<name> and the call succeeds, while a detached HEAD makes it fail.
// That separates a branch with no commit from a HEAD that is broken, and the
// two need different answers.
//
// A call that fails for any other reason is read as not unborn, which makes
// ResolveBase refuse. That is the safe direction: the alternative answer sends
// a capture at the empty tree, which is every tracked file in the repository.
func headIsUnborn(ctx context.Context, root string) bool {
	_, err := run(ctx, root, "symbolic-ref", "--quiet", headRef)
	return err == nil
}
