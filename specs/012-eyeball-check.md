---
status: ready
created: 2026-10-07T00:00:00Z
updated: 2026-10-07T03:59:20.027964507Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
---

# 012 eyeball check

`eyeball check --base <old> <new>`, run inside a repository, exits 0 only when
an approved review in that project, by any agent, has a latest round whose base
is `<old>` and whose captured files match `<new>`'s tree file by file. The
dev-review gate in `~/Work/machine` (its spec 014) will run it before letting a
default branch move from `<old>` to `<new>`, so the eyeball approval becomes the
only approval. It compares trees, so a fast-forward and a merge commit are
treated alike. It fails closed, and it never acknowledges, changes or records
anything.

Approved by the owner as review 15, round 1, on 2026-10-07. Three amendments
from machine spec 014's session were added to the draft after round 1 was
captured and announced in dispatch before work began: the gate sets
`EYEBALL_NO_START`, the comparison reads commits only, and `not_approved` names
the project root and the base.

This work follows machine spec 014's workflow. That workflow replaces the
autonomous-run arrangement recorded in 010 for this work.

## What was settled from the code

**1. What "match" means.** Read from `internal/capture/freeze.go` and
`changed.go`. A git round captures every path that differs from the base:
`git diff --raw --no-renames <base>` against the working tree, plus untracked
files that are not ignored. The review's paths do not limit what is captured.
They set `in_scope` on each file. A deletion is recorded with an empty digest. A
symlink's content is its target, which is also what git stores as its blob, so
the two digests agree. That is assumed from the code and tested in iteration 1.
Submodules and non-regular files are never captured.

The check passes when two things hold:

- Every path where `<new>` differs from `<old>` was captured in scope, with the
  digest of `<new>`'s bytes, or with an empty digest where `<new>` lacks the
  path.
- Every path captured in scope is such a path. A captured file differs from the
  base by definition, so one that `<new>` leaves as `<old>` had it is approved
  work that does not merge.

So a stray untracked file captured by a review of `.` fails the check, by name,
when `<new>` does not hold it. The reviewer saw it in the review, so what was
approved and what merges differ. Leaving strays out would also let a new file
the agent forgot to commit pass, and the default branch would break. The cost is
that an agent must delete or ignore strays before asking. A file captured out of
scope that `<new>` leaves as `<old>` had it is ignored, since nobody approved it
and nothing of it merges. One that `<new>` changes fails the first rule, for the
same reason.

**2. The daemon.** The check is a route on the agent socket, as every command is
(architecture.md, "The CLI speaks HTTP over the unix socket"). The daemon owns
the store and already reads git and blobs. Reading `eyeball.db` from the CLI was
the alternative. It works with the daemon stopped, but it would add a second
reader of the schema and a second code path. Like any command, `check` starts a
stopped daemon. With `EYEBALL_NO_START` set, a stopped daemon is a refusal whose
fix says to run `eyeball start`. The gate will set it, as machine spec 014's
session decided. Either way, no answer means exit 1.

A daemon started from a hook inherits the hook's environment, where git sets
`GIT_DIR` to a worktree's git directory, as machine spec 014's session measured.
Read from `internal/capture/git.go`: every git command the daemon runs already
drops the variables `git rev-parse --local-env-vars` names, `GIT_DIR` and
`GIT_INDEX_FILE` among them, so an inherited `GIT_DIR` reaches no read.
Iteration 2 tests it by planting `GIT_DIR` in the starter's environment.

**3. Where it sits.** `eyeball check` in `internal/cli`, a `GET /agent/check`
route in `internal/server`, and the comparison in `internal/capture`, where git
shell-outs live. The route finds the project with `ProjectByRoot` and never
registers one, so a repository eyeball has not seen is refused, not recorded.
architecture.md's routes table and its agent section name the route and the
match rule, in the change that adds them.

## Iteration 1: The comparison

- [ ] `capture.Match(ctx, root, old, new, files)` returning the first mismatch
      as a sentence naming the path, or none
- [ ] `<old>` and `<new>` resolved with `rev-parse --verify <ref>^{commit}`, and
      the comparison done with `diff-tree`, `ls-tree` and `cat-file` against the
      two commits alone, never the index or the working tree. A merge holds
      `index.lock` while the gate runs, as machine spec 014's session measured
- [ ] Tests on real repositories: a match, a fast-forward and a merge commit
      both accepted, and each of these refused: one file differing, a path in
      `<new>` not captured, a captured deletion `<new>` still holds, an in-scope
      stray `<new>` lacks, and an out-of-scope change `<new>` makes. Also a
      symlink matched, and an out-of-scope capture `<new>` leaves alone ignored

## Iteration 2: The command

- [ ] `GET /agent/check` taking the caller's directory, `base` and `new`,
      finding approved reviews whose latest round's base is `<old>`, and passing
      if any matches
- [ ] `eyeball check --base <old> <new>`: exit 0 and one line naming the review,
      or exit 1 and one line saying why, with codes `not_approved` and
      `tree_differs` under `--json`. `not_approved` names the project root and
      the base it looked for, since a review opened from a worktree is another
      project and will not match a merge made in the main clone
- [ ] The manual's command list, codes and exit statuses, and architecture.md's
      routes table and agent section
- [ ] Breaks planted by hand and seen refused: a wrong base, one differing file,
      an open review with the same tree, and a stopped daemon under
      `EYEBALL_NO_START`. A real match accepted, also from a daemon started with
      `GIT_DIR` pointing at another repository
- [ ] Installed to `~/.local/bin`, and the daemon restarted

## Out of scope

- The gate itself, which is machine spec 014's iteration 2.
- A project outside git, or one named by a marker file inside a repository. Its
  round is a walk with no base commit, so the check refuses it.
- Recording that a check passed. It reads, and nothing else.

## Review

Spec review pass: declined. The three decisions are stated above with their
reasons, for the reviewer to judge directly, and iteration 1's tests exercise
each case in both directions. End-of-branch review pass: requested. This is the
one check the gate enforces, and a defect lets unapproved work reach a default
branch.
