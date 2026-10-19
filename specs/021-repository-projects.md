---
status: done
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -count=1 -tags browser ./internal/server
---

# 021 A project is a repository

Every git worktree an agent asks for review from becomes a project of its own,
and stays one after the worktree is removed. On 2026-10-18, five of the eleven
projects in the owner's store had a root that no longer existed, all of them
worktrees or build copies, and the queue's empty state lists every one with no
way to clear it. The same rule made `eyeball check` from the main checkout
refuse review 58, approved from a worktree, so the same tree needed a third
approval as review 59.

This spec makes a project the repository, and a worktree one of its working
copies. The design is in [docs/product.md](../docs/product.md), under One
instance, many projects, the project view and the queue's empty state, and in
[docs/architecture.md](../docs/architecture.md), under The project and the
agent, Storage and Checking a merge. Both name the earlier rule they correct.
The owner chose this over only hiding projects whose root is gone. Hiding is
cheaper, and it leaves a worktree's approval unable to pass the gate. Claims
apply per working copy and a review can move between working copies when the
agent passes its name, both the owner's choices.

The probes behind the identity rule were run on 2026-10-18 with git 2.55 and are
recorded in architecture.md. The common git directory names the repository in
every layout tried, and `git worktree list` does not.

## Review

Spec review pass: requested, because a migration and a startup merge move
reviews between projects, and a mistake there loses or misfiles history.
End-of-branch review pass: requested, for the same reason, and because every
live read moves from the project's root to the round's working copy.

## Out of scope

- Forgetting a project, or deleting a working copy that is gone. Hiding is
  enough for the empty state, and deleting cascades to reviews.
- Showing a worktree no agent has run eyeball in. Finding one needs
  `git worktree list`, which misnames the main checkout in some layouts.
- A default agent name shared across a repository's working copies. Crossing to
  another working copy passes `--agent`.

## Iteration 1: The tearout

```punchcard
required_acks:
  - tearout-approved
```

- [x] project.html drawing a project with its main checkout and one worktree,
      each working copy as its own section with its changed files
- [x] project.html drawing a worktree whose working copy is clean, and the root
      named in each section's heading as the topbar names it today
- [x] The tearout judged on the phone by the owner, with any change asked for
      made before the ack

Approved by the owner on 2026-10-18 with no changes. Each working copy is named
by its directory, with "Main checkout" or "Worktree" and its path under the
name, which is how the second todo's heading was drawn.

> **Completed** 2026-10-19 02:51 UTC
>
> - acks: tearout-approved
> - `make check` — 5.6s
> - `go test -race ./...` — 36.6s
> - `go test -count=1 -tags browser ./internal/server` — 32.4s

## Iteration 2: Working copies in the store

- [x] Migration 0010 adding `working_copies`, `projects.repository` with its
      partial unique index, and `rounds.working_copy_id` with the trigger that
      refuses a round without one, every existing round pointed at its copy
- [x] A migration test on a database holding two projects with reviews, reading
      back each round's working copy and each project's unchanged root
- [x] Store reads and writes for working copies, and `OpenReview` and
      `OpenRound` recording the working copy a round was captured from
- [x] The overlap refusal scoped to open reviews whose latest round is in the
      same working copy, with a test that the same path is accepted in another
- [x] `OpenRound` refusing a round from another working copy where an open
      review there claims one of its paths, with a test of the refusal

> **Completed** 2026-10-19 03:02 UTC
>
> - `make check` — 6.0s
> - `go test -race ./...` — 44.2s
> - `go test -count=1 -tags browser ./internal/server` — 34.2s

**Departures, iteration 2.**

- `EnsureProject` also registers the working copy at the project's root, and a
  request naming no working copy is recorded against that copy on a first round
  and against the previous round's on a later one. About forty callers and tests
  open reviews through `EnsureProject`, and none of them had a working copy to
  pass. Iteration 3's test that a worktree's round records the worktree is what
  catches a caller that forgets to name one.
- `store.ErrWrongProject` has a refusal mapping in the server, since a test
  requires one for every exported store error. It answers 403 with the existing
  `wrong_project` code.
- Each new test was checked by breaking what it covers: pointing rounds at the
  wrong copy in the migration, dropping the claim check when a round moves, and
  scoping claims to the project again. Each made its test fail.

## Iteration 3: Finding the project

- [x] `capture.Repository` returning a working copy's resolved common git
      directory, and nothing for a working copy found by its marker
- [x] `resolve` finding the project by repository and the working copy by root,
      registering either on first sight, and moving the project's root to a main
      checkout once one is seen
- [x] Tests that a main checkout and its worktree resolve to one project, and a
      submodule, a bare clone's worktree and a marker directory to their own
- [x] The startup fill of `projects.repository` for roots still on disk,
      finished before the agent socket accepts a request, merging two projects
      that share one
- [x] A merge test counting reviews, rounds and comments before and after, so a
      merge that deletes a project before moving its reviews fails
- [x] A request from a directory recreated at a gone root merging that root's
      old project into the repository's, with a test

**Departures, iteration 3.**

- git is asked about the caller's working copy on every request, not only on
  first sight, since a directory can be removed and made again as part of
  another repository. It is one `rev-parse`.
- A directory made again at a gone root is merged into the repository's project
  only when the old project names the same repository, or none. An old project
  git was never asked about is taken to be the same repository, an assumption
  nothing on disk can check. One that names a different repository is refused
  with an error naming both, rather than filed under the wrong project.
- When no project has the repository yet, the request registers one at its own
  root, which finds an old project at that root if there is one. So a worktree
  recreated before its main checkout is seen takes over its old project rather
  than merging it into anything.
- The fill runs between building the server and serving the agent socket, in
  `internal/daemon/run.go`. No test checks the order, since it is two adjacent
  lines.
- A merge publishes on the hub, so an open page of either project reloads. The
  other new writes do not, as `MarkInsideCheckout` does not, since requests make
  them.
- The tests were checked by breaking what they cover: dropping the working copy
  from a request, skipping each of the fill's two merge branches, and deleting
  before moving in a merge. The first version of the worktree test passed with
  the working copy dropped, because the copy a request falls back to happened to
  be the right one. It now asks from the main checkout first.

> **Completed** 2026-10-19 03:37 UTC
>
> - `make check` — 6.2s
> - `go test -race ./...` — 46.4s
> - `go test -count=1 -tags browser ./internal/server` — 34.7s

## Iteration 4: Reading the round's working copy

- [x] The review page, the file page, the raw route and the decision reading the
      round's working copy where they read `project.Root` today
- [x] `ownReview` accepting a round from another working copy of the project,
      with a test that the next round reads the new working copy
- [x] `eyeball check` and `eyeball cat` finding the project by repository, with
      a test that an approval asked for in a worktree passes `check` run from
      the main checkout
- [x] `api.Project.Root` documented as the caller's working copy, in
      `eyeball     doc` and the CLI tests that print it

**Departures, iteration 4.**

- `api.Project.Root` is the caller's working copy in a status, as planned, and
  in a review it is the working copy the review's latest round came from, since
  `show` and `wait` answer about a review from any working copy. The CLI tests
  needed no change: their fixtures have one working copy, where the two agree.
- The file page's and the tree's notices name the root they read, from the
  inspection, rather than the project's root.
- One test covers the live reads, on the file page: a file written in a worktree
  after its round, in a project whose root is the main checkout. The review
  page, the raw route and the verdict read through the same `copyRoot`. The
  first version wrote the file before the round, which captured it, so the page
  served the capture and the test passed against the old code. Each test in this
  iteration was checked by putting the old lookup back.
- The manual's three passages saying a worktree is its own project are
  rewritten, and it says how to ask for a round from another working copy.

> **Completed** 2026-10-19 03:43 UTC
>
> - `make check` — 6.5s
> - `go test -race ./...` — 44.6s
> - `go test -count=1 -tags browser ./internal/server` — 35.6s

## Iteration 5: The surface

- [x] The queue's empty state leaving out a project with no working copy on
      disk, with a test that a project with one gone and one present stays
- [x] The project view showing each working copy on disk as the approved tearout
      draws it, with claimed files marked from that copy's reviews
- [x] The project file route and the tree reading `?copy=<id>`, defaulting to
      the project's root, with a test that a worktree's file opens from it

**Departures, iteration 5.**

- A project with one working copy on disk is drawn as it was before, under
  "Working copy" with no name or hint, since that frame was approved too. Only a
  project with several on disk is drawn with a section per copy, and the notice
  that the rows are live appears once above them, as the tearout draws.
- There is no project tree route. "The tree" in the third todo was a mistake in
  the spec: the tree under a review reads its round's working copy since
  iteration 4. The project file route takes `copy`, and refuses one that does
  not exist or belongs to another project.
- A root that cannot be looked at for a reason other than being gone counts as
  on disk, so the queue still lists it, and the project view says why it cannot
  be read.
- The queue tests registered projects at made-up paths, which the new rule
  hides. The fixture now makes each root under a temporary directory, and the
  roots still read as the same paths.
- Each test was checked by removing what it covers: the claim filter per working
  copy, and the queue's filter.

> **Completed** 2026-10-19 03:50 UTC
>
> - `make check` — 7.2s
> - `go test -race ./...` — 43.9s
> - `go test -count=1 -tags browser ./internal/server` — 36.6s

## Iteration 6: On the phone

```punchcard
required_acks:
  - phone-checked
```

With the daemon running this branch's build, the owner checks in the installed
app:

- [x] The queue's empty state lists no project whose working copies are gone
- [x] The eyeball project view shows a worktree in use beside the main checkout

Checked by the owner on 2026-10-18, with the daemon running this branch's build
and 020's readout merged in uncommitted, after migration 0010 ran on the owner's
store with a copy kept first. The project view showed the main checkout and a
scratch worktree, each in its own section. The first check was not made: the
owner chose to remove the empty state's list of projects altogether, as 022,
since its sentence about agents at work says what eyeball cannot know.

> **Completed** 2026-10-19 06:46 UTC
>
> - acks: phone-checked
> - `make check` — 5.8s
> - `go test -race ./...` — 45.2s
> - `go test -count=1 -tags browser ./internal/server` — 33.9s
