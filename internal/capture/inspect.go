package capture

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/blob"
	"github.com/noonat/eyeball/internal/store"
)

// Live says whether a round's project allows a live read, a read of a file
// nothing captured, and what keeps a path out of one.
type Live string

const (
	// LiveOpen allows any regular file under the root. It is a project outside
	// git that has never been seen inside a checkout, so nothing says what to
	// ignore.
	LiveOpen Live = "open"
	// LiveIgnoreRules allows a path the repository's ignore rules do not
	// name. It is a git project, or a marker project inside a checkout that
	// does not ignore it.
	LiveIgnoreRules Live = "ignore_rules"
	// LiveRootMissing refuses every live read, because the root is gone.
	LiveRootMissing Live = "root_missing"
	// LiveLost refuses every live read, because the project's repository is
	// gone from its root and git would answer for another one, or fail.
	LiveLost Live = "lost"
	// LiveIgnored refuses every live read in a marker project the outer
	// checkout ignores as a whole.
	LiveIgnored Live = "ignored"
	// LiveCheckoutGone refuses every live read in a marker project that was
	// seen inside a checkout whose .git is no longer above it.
	LiveCheckoutGone Live = "checkout_gone"
	// LiveRulesUnreadable refuses every live read, because git failed to say
	// which paths it ignores.
	LiveRulesUnreadable Live = "rules_unreadable"
)

// Refused reports whether l allows no live read at all. A value it does not
// know refuses, so a state added later fails closed until it is handled here.
func (l Live) Refused() bool {
	switch l {
	case LiveOpen, LiveIgnoreRules:
		return false
	}
	return true
}

// State is what Inspect found about a round's project, settled once per
// request so every rule that reads it agrees.
type State struct {
	// Root is the project root Inspect was asked about, resolved.
	Root string
	// Base is the round's base commit as recorded, empty where there is none.
	Base string
	// Live is whether a live read is allowed, and under which rules.
	Live Live
	// BaseGone reports a base the repository no longer holds, as after a
	// rebase and a garbage collection. It is false wherever the base was not
	// asked about: no base, a missing root, or a lost repository.
	BaseGone bool
	// FoundCheckout reports a .git entry at or above a root that holds a
	// marker file. The caller records it on the project, since capture holds
	// no store handle.
	FoundCheckout bool
	// RulesErr is why the ignore rules could not be read, when Live is
	// LiveRulesUnreadable. It is for a log, never for a page.
	RulesErr error
}

// Inspect settles the state of the project at root for one request, given the
// round's base and whether the store has seen the project inside a checkout.
//
// It answers four questions in order: whether the root exists, whether the
// project has lost its repository, whether the base still exists, and which
// ignore rules apply to a live read. A missing root or a lost repository skips
// the rest, because every git call after them fails or answers for a
// different repository.
//
// The answers a page shows are values, not errors. A git failure while reading
// the ignore rules is LiveRulesUnreadable, which refuses only live reads, so a
// captured file still opens. A failure to ask about the base is an error,
// since without that answer nothing says which reader to build.
func Inspect(ctx context.Context, root, base string, insideCheckout bool) (State, error) {
	st := State{Root: root, Base: base}
	info, err := os.Stat(root)
	if err != nil && !absent(err) {
		return State{}, errors.Wrapf(err, "stat %s", root)
	}
	if err != nil || !info.IsDir() {
		st.Live = LiveRootMissing
		return st, nil
	}

	lost, git, err := lostRepository(root, base)
	if err != nil {
		return State{}, err
	}
	if lost {
		st.Live = LiveLost
		return st, nil
	}

	if base != "" {
		if st.BaseGone, err = BaseGone(ctx, root, base); err != nil {
			return State{}, err
		}
	}

	if st.FoundCheckout, err = CheckoutAround(root); err != nil {
		return State{}, err
	}
	switch {
	case git:
		st.Live = LiveIgnoreRules
	case st.FoundCheckout:
		st.Live, st.RulesErr = rootRules(ctx, root)
	case insideCheckout:
		st.Live = LiveCheckoutGone
	default:
		st.Live = LiveOpen
	}
	return st, nil
}

// Reader reads files as of the round the state was inspected for, from the
// sources the state allows.
//
// A gone base, a lost repository and a missing root leave the reader with no
// base, since git cannot read one. A refused state refuses every read of the
// working copy, so the reader answers from the capture alone, or from the
// capture and the base.
func (s State) Reader(blobs *blob.Store, files []store.File) *Reader {
	base := s.Base
	if s.BaseGone || s.Live == LiveLost || s.Live == LiveRootMissing {
		base = ""
	}
	r := NewReader(s.Root, base, blobs, files)
	r.live = s.Live
	return r
}

// BaseGone reports whether the repository at root no longer holds the commit
// base, which happens after a rebase and a garbage collection.
//
// cat-file -e exits 0 for an object that exists, 1 for one that does not, and
// 128 for anything else, such as a root that is no repository. Only the 1 is
// an answer. Reading 128 as gone would turn a broken repository into a live
// read of its whole working copy.
func BaseGone(ctx context.Context, root, base string) (bool, error) {
	_, err := run(ctx, root, "cat-file", "-e", "--end-of-options", base)
	if err == nil {
		return false, nil
	}
	if exitCode(err) == 1 {
		return true, nil
	}
	return false, err
}

// CheckoutAround reports whether root holds a marker file and a .git entry
// sits at root or in a directory above it.
//
// The root itself counts. A root holding both a .git and a marker on a branch
// with no commit records no base, and once its .git is moved away it looks
// like a marker project with nothing around it, so the sighting has to be
// recorded while the .git is still there.
//
// The walk reads the filesystem and never asks git, so it starts from the
// resolved root the store keys on.
func CheckoutAround(root string) (bool, error) {
	marker, err := holds(root, Marker)
	if err != nil || !marker {
		return false, err
	}
	for d := root; ; d = filepath.Dir(d) {
		found, err := holds(d, gitDir)
		if err != nil || found {
			return found, err
		}
		if d == filepath.Dir(d) {
			return false, nil
		}
	}
}

// lostRepository reports whether the project at root has lost its repository,
// and whether its root holds a .git entry, from the filesystem alone.
//
// FindRoot only records a root that held a .git entry or a marker, so a root
// holding neither has lost what it had. A round with a base was captured in a
// git project, so a root with a base and no .git has lost it even with a
// marker beside it, since a marker outlives a .git moved from beside it. A .git
// file naming a git directory that is gone has lost it as well: in a linked
// worktree whose directory was removed, every git call exits 128 and none
// falls back to an outer repository.
func lostRepository(root, base string) (lost, git bool, err error) {
	git, err = holds(root, gitDir)
	if err != nil {
		return false, false, err
	}
	marker, err := holds(root, Marker)
	if err != nil {
		return false, false, err
	}
	if !git {
		return !marker || base != "", false, nil
	}
	usable, err := gitDirUsable(root)
	if err != nil {
		return false, false, err
	}
	return !usable, true, nil
}

// gitDirUsable reports whether the .git entry at root leads to a git
// directory that exists.
//
// A directory does. A file is a linked worktree's or a submodule's pointer,
// "gitdir: <path>", with a relative path read against the root, which is how
// a submodule writes it. A pointer git could not parse is as unusable as one
// whose target is gone, since git refuses it with "invalid gitfile format".
func gitDirUsable(root string) (bool, error) {
	full := filepath.Join(root, gitDir)
	info, err := os.Stat(full)
	if err != nil {
		if absent(err) {
			// A symlink whose target is gone. git walks past it and answers
			// for whatever repository sits above the root.
			return false, nil
		}
		return false, errors.Wrapf(err, "stat %s", full)
	}
	if info.IsDir() {
		return true, nil
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	pointer, err := os.ReadFile(full)
	if err != nil {
		return false, errors.Wrapf(err, "read %s", full)
	}
	target, found := strings.CutPrefix(strings.TrimSpace(string(pointer)), "gitdir: ")
	if !found || target == "" {
		return false, nil
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	info, err = os.Stat(target)
	if err != nil {
		if absent(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "stat %s", target)
	}
	return info.IsDir(), nil
}

// rootRules asks the outer checkout whether it ignores a marker project as a
// whole.
//
// check-ignore exits 0 for ./ in a directory the checkout ignores and 1 for
// one it does not. Anything else is a failure to answer, such as
// GIT_CEILING_DIRECTORIES stopping git short of the .git found above or a
// filesystem boundary. That refuses live reads rather than skipping the check,
// since skipping it is what would serve the files the rules exist to hide.
func rootRules(ctx context.Context, root string) (Live, error) {
	_, err := run(ctx, root, "check-ignore", "-q", "--", "./")
	if err == nil {
		return LiveIgnored, nil
	}
	if exitCode(err) == 1 {
		return LiveIgnoreRules, nil
	}
	return LiveRulesUnreadable, err
}

// holds reports whether dir holds an entry named name.
//
// An entry that cannot be read for lack of permission counts as absent, so a
// walk up through a directory it cannot search carries on past it. FindRoot
// walks this way, and every walk that has to agree with it reads entries
// through here.
func holds(dir, name string) (bool, error) {
	_, err := os.Lstat(filepath.Join(dir, name))
	if err == nil {
		return true, nil
	}
	if absent(err) || errors.Is(err, os.ErrPermission) {
		return false, nil
	}
	return false, errors.Wrapf(err, "look for %s in %s", name, dir)
}

// absent reports an error that means the path is not there: it does not
// exist, or a component above it is a file.
func absent(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// exitCode is the status git exited with, or -1 where it did not run to an
// exit, as when it is not on PATH.
func exitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}
