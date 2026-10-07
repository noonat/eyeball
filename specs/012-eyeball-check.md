---
status: done
created: 2026-10-07T00:00:00Z
updated: 2026-10-07T04:09:54.187952295Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
---

# 012 eyeball check

`eyeball check --base <old> <new>`, run inside a repository, exits 0 only when
an approved review in that project, by any agent, has a latest round whose base
is `<old>` and whose captured files match `<new>`'s tree file by file. A review
gate will run it before letting a default branch move from `<old>` to `<new>`,
so the eyeball approval becomes the only approval. It compares trees, so a
fast-forward and a merge commit are treated alike. It fails closed, and it never
acknowledges, changes or records anything.

Approved by the owner as review 15, round 1, on 2026-10-07. Three amendments
from the gate's design were added to the draft after round 1 was captured and
announced in dispatch before work began: the gate sets `EYEBALL_NO_START`, the
comparison reads commits only, and `not_approved` names the project root and the
base.

This work is reviewed once, as a whole branch, before it merges. That replaces
the autonomous-run arrangement recorded in 010 for this work.

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
fix says to run `eyeball start`. The gate will set it. Either way, no answer
means exit 1.

A daemon started from a hook inherits the hook's environment, where git sets
`GIT_DIR` to a worktree's git directory, as measured while designing the gate.
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

- [x] `capture.Match(ctx, root, old, new, files)` returning the first mismatch
      as a sentence naming the path, or none
- [x] `<old>` and `<new>` resolved with `rev-parse --verify <ref>^{commit}`, and
      the comparison done with `diff-tree`, `ls-tree` and `cat-file` against the
      two commits alone, never the index or the working tree. A merge holds
      `index.lock` while the gate runs, as measured while designing the gate
- [x] Tests on real repositories: a match, a fast-forward and a merge commit
      both accepted, and each of these refused: one file differing, a path in
      `<new>` not captured, a captured deletion `<new>` still holds, an in-scope
      stray `<new>` lacks, and an out-of-scope change `<new>` makes. Also a
      symlink matched, and an out-of-scope capture `<new>` leaves alone ignored

**Departures, iteration 1.**

- A review cannot claim `.`. `store.NormalizePath` refuses the project root as a
  path, so a whole-branch review names its files or directories, as review 13
  named its 19 files. Measured against the store: no review there claims `.`.
  This corrects the premise above that a review of `.` captures strays in scope.
  Every round captures every untracked file whatever its paths, as said above,
  so a stray is in scope only inside a path the review claims. The rule holds as
  written. It refuses fewer strays than the premise suggested.
- Tested on real repositories: a symlink's captured digest equals the digest of
  git's blob for it, which the plan assumed.
- A ref that names no commit is a third refusal, `bad_commit`, beside the plan's
  `not_approved` and `tree_differs`, since every sentinel the daemon can return
  maps to a code.
- A commit that adds a submodule is refused with its own sentence. No capture
  records one, so it would be refused as uncaptured anyway.

> **Completed** 2026-10-07 04:04 UTC
>
> - acks: reviewed
> - `make check` — 5.1s
> - `go test -race ./...` — 34.5s

## Iteration 2: The command

- [x] `GET /agent/check` taking the caller's directory, `base` and `new`,
      finding approved reviews whose latest round's base is `<old>`, and passing
      if any matches
- [x] `eyeball check --base <old> <new>`: exit 0 and one line naming the review,
      or exit 1 and one line saying why, with codes `not_approved` and
      `tree_differs` under `--json`. `not_approved` names the project root and
      the base it looked for, since a review opened from a worktree is another
      project and will not match a merge made in the main clone
- [x] The manual's command list, codes and exit statuses, and architecture.md's
      routes table and agent section
- [x] Breaks planted by hand and seen refused: a wrong base, one differing file,
      an open review with the same tree, and a stopped daemon under
      `EYEBALL_NO_START`. A real match accepted, also from a daemon started with
      `GIT_DIR` pointing at another repository
- [x] Installed to `~/.local/bin`, and the daemon restarted

**What iteration 2's breaks showed, on 2026-10-07**, with a built binary and a
daemon on its own state directory, approving through its loopback surface:

| Case                                            | Result                       |
| ----------------------------------------------- | ---------------------------- |
| The approved tree, fast-forward                 | exit 0, names review 1       |
| The approved tree, as a `--no-ff` merge commit  | exit 0, names review 2       |
| A base no review is measured from               | exit 1, `not_approved`       |
| One file changed after approval                 | exit 1, `tree_differs`       |
| An open review with the same tree               | exit 1, `not_approved`       |
| A stopped daemon under `EYEBALL_NO_START=1`     | exit 1, and no daemon starts |
| A daemon started with `GIT_DIR` at another repo | exit 0, names review 2       |

The handler's approved-state test, its base test and its match test were each
broken by hand, and `TestServer_check` failed each time, as it did with the
environment scrub in `internal/capture/git.go` removed.

> **Completed** 2026-10-07 04:09 UTC
>
> - acks: reviewed
> - `make check` — 5.1s
> - `go test -race ./...` — 33.8s

## Out of scope

- The gate itself, which lives outside eyeball.
- A project outside git, or one named by a marker file inside a repository. Its
  round is a walk with no base commit, so the check refuses it.
- Recording that a check passed. It reads, and nothing else.

## Review

Spec review pass: declined. The three decisions are stated above with their
reasons, for the reviewer to judge directly, and iteration 1's tests exercise
each case in both directions. End-of-branch review pass: requested. This is the
one check the gate enforces, and a defect lets unapproved work reach a default
branch.
