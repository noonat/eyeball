package capture

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

// ErrNoGit reports a project whose root holds a .git entry on a machine with no
// git on PATH. Capture refuses rather than falling back to the walk, which
// would show whole files with no base and report none of it as unusual.
var ErrNoGit = errors.New("git is not on PATH")

// ErrNotTopLevel reports a project root that sits below its repository's top
// level. The two enumerations disagree about what a path is relative to and how
// much of the repository they cover, so nothing below the top level can be
// captured correctly.
var ErrNotTopLevel = errors.New("project root is not the repository top level")

// gitDir is the entry that makes a root a git project. It is a directory in an
// ordinary checkout and a file in a linked worktree or a submodule, and either
// counts.
const gitDir = ".git"

// run executes one git command in root and returns its standard output.
//
// Every call goes through here, so the flags that belong on all of them are in
// one place. --no-optional-locks stops an enumeration from taking index.lock,
// which the agent's own git may be holding. The context cancels the process,
// which is what stops a capture outliving the request that asked for it.
//
// The agent's configuration is deliberately left alone: no GIT_CONFIG_NOSYSTEM
// and no -c overrides. The repository is whatever the agent's git made, and
// reading it through different settings shows content the agent does not have.
//
// Output is buffered rather than streamed. The largest thing read this way is a
// raw diff of a very large working tree, about a megabyte, and file content
// goes through the reader instead.
func run(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", runArgs(root, args)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrapf(err, "git %s: %s", strings.Join(args, " "), message(&stderr))
	}
	return out, nil
}

// runArgs is the full argument list for one git call, built here so the flags
// that belong on every call cannot be dropped from one of them.
//
// Neither flag has an effect a test can observe, which is why they are asserted
// on the list rather than through behavior: -C decides the directory, and
// --no-optional-locks only matters when another git holds index.lock.
func runArgs(root string, args []string) []string {
	return append([]string{"-C", root, "--no-optional-locks"}, args...)
}

// message is what git said on stderr, trimmed to one line for an error string.
func message(stderr *bytes.Buffer) string {
	text := strings.TrimSpace(stderr.String())
	if text == "" {
		return "no output on stderr"
	}
	if line, _, cut := strings.Cut(text, "\n"); cut {
		return line
	}
	return text
}

// isGitProject reports whether root is a git project capture can enumerate.
//
// The .git entry is read from the filesystem before any subprocess, because
// asking git is circular: with git missing from PATH the call cannot run, so
// its failure cannot separate a repository whose tool is absent from a
// directory that was never one. Reading the entry first is what makes the
// ErrNoGit refusal reachable at all.
//
// A directory inside a repository with no .git of its own is not a git project
// here. That is the marker-file project someone put inside a checkout, and it
// gets the behavior of a project without git: whole files, no base, and only
// the paths the review named.
func isGitProject(ctx context.Context, root string) (bool, error) {
	if _, err := os.Lstat(filepath.Join(root, gitDir)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, errors.Wrapf(err, "look for %s in %s", gitDir, root)
	}
	if _, err := exec.LookPath("git"); err != nil {
		return false, errors.Wrapf(ErrNoGit, "%s is a git repository, so install git or put it on PATH", root)
	}
	if err := requireTopLevel(ctx, root); err != nil {
		return false, err
	}
	return true, nil
}

// requireTopLevel refuses a root that is not its repository's top level.
//
// It should hold by construction, since the root was chosen by walking up to a
// .git entry. Where it does not, the two enumerations return paths relative to
// different directories and cover different amounts of the repository, and the
// capture that results names files the root does not contain.
//
// rev-parse --is-inside-work-tree cannot be this check: it answers true from
// every subdirectory.
func requireTopLevel(ctx context.Context, root string) error {
	out, err := run(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	top := strings.TrimSpace(string(out))
	// git prints the top level with symlinks already resolved, so only this
	// side needs resolving. Comparing the two spellings would refuse every
	// project whose path runs through a symlink, which on some systems is
	// every project under a temporary directory.
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return errors.Wrapf(err, "resolve %s", root)
	}
	if real != top {
		return errors.Wrapf(ErrNotTopLevel, "%s is below %s, so capture the project at %s instead", root, top, top)
	}
	return nil
}
