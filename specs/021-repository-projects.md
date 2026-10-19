---
status: ready
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

- [ ] project.html drawing a project with its main checkout and one worktree,
      each working copy as its own section with its changed files
- [ ] project.html drawing a worktree whose working copy is clean, and the root
      named in each section's heading as the topbar names it today
- [ ] The tearout judged on the phone by the owner, with any change asked for
      made before the ack

## Iteration 2: Working copies in the store

- [ ] Migration 0010 adding `working_copies`, `projects.repository` with its
      partial unique index, and `rounds.working_copy_id` with the trigger that
      refuses a round without one, every existing round pointed at its copy
- [ ] A migration test on a database holding two projects with reviews, reading
      back each round's working copy and each project's unchanged root
- [ ] Store reads and writes for working copies, and `OpenReview` and
      `OpenRound` recording the working copy a round was captured from
- [ ] The overlap refusal scoped to open reviews whose latest round is in the
      same working copy, with a test that the same path is accepted in another
- [ ] `OpenRound` refusing a round from another working copy where an open
      review there claims one of its paths, with a test of the refusal

## Iteration 3: Finding the project

- [ ] `capture.Repository` returning a working copy's resolved common git
      directory, and nothing for a working copy found by its marker
- [ ] `resolve` finding the project by repository and the working copy by root,
      registering either on first sight, and moving the project's root to a main
      checkout once one is seen
- [ ] Tests that a main checkout and its worktree resolve to one project, and a
      submodule, a bare clone's worktree and a marker directory to their own
- [ ] The startup fill of `projects.repository` for roots still on disk,
      finished before the agent socket accepts a request, merging two projects
      that share one
- [ ] A merge test counting reviews, rounds and comments before and after, so a
      merge that deletes a project before moving its reviews fails
- [ ] A request from a directory recreated at a gone root merging that root's
      old project into the repository's, with a test

## Iteration 4: Reading the round's working copy

- [ ] The review page, the file page, the raw route and the decision reading the
      round's working copy where they read `project.Root` today
- [ ] `ownReview` accepting a round from another working copy of the project,
      with a test that the next round reads the new working copy
- [ ] `eyeball check` and `eyeball cat` finding the project by repository, with
      a test that an approval asked for in a worktree passes `check` run from
      the main checkout
- [ ] `api.Project.Root` documented as the caller's working copy, in
      `eyeball     doc` and the CLI tests that print it

## Iteration 5: The surface

- [ ] The queue's empty state leaving out a project with no working copy on
      disk, with a test that a project with one gone and one present stays
- [ ] The project view showing each working copy on disk as the approved tearout
      draws it, with claimed files marked from that copy's reviews
- [ ] The project file route and the tree reading `?copy=<id>`, defaulting to
      the project's root, with a test that a worktree's file opens from it

## Iteration 6: On the phone

```punchcard
required_acks:
  - phone-checked
```

With the daemon running this branch's build, the owner checks in the installed
app:

- [ ] The queue's empty state lists no project whose working copies are gone
- [ ] The eyeball project view shows a worktree in use beside the main checkout
