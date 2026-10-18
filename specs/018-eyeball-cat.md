---
status: done
created: 2026-10-11T00:00:00Z
updated: 2026-10-18T00:00:00Z
required_commands:
  - cmd: make check
---

# 018 eyeball cat

A merge gate that runs `eyeball check` lets a default branch move only to a tree
that matches an approved round byte for byte. Some files change after the
approval that authorizes the change. A spec whose status flips to ready once the
reviewer approves it is the common case. The gate then refuses the merge, and
the reviewer approves the same file a second time.

A gate can allow such a change when it can prove the change is the only
difference: replay it on the approved text and compare. To do that, it needs the
approved bytes. This spec gives it `eyeball cat`.

## Design

**`eyeball cat --base <old> <path>` prints the bytes of a file from an approved
review's latest round measured from `<old>`.** It refuses with a code when no
approved review holds the path, or when two approved reviews hold different
bytes for it. The gate asks eyeball rather than reading the blob store from
disk, because the gate's trust rests on eyeball's answer, and a file on disk is
not one. How it answers, and why in JSON, is in
[docs/architecture.md](../docs/architecture.md), under "Reading the approved
bytes".

**The gate checks a commit that no ref names.** It builds a throwaway commit
whose tree is the merged tree with the approved bytes put back, and runs
`eyeball check` on it. `handleCheck` and `Match` read only `rev-parse` and
`diff-tree`, so it should be accepted. That was assumed, not measured, when the
plan was written, so the iteration pins it with a test.

## Review

Spec review pass: declined, because the design is one read of bytes the store
already holds. End-of-branch review pass: requested, because a gate built on
this decides what reaches a default branch. It ran once, over this diff and the
gate's together.

## Out of scope

- A reviewer-facing note in eyeball saying a gate covered a change after the
  approval. It goes in specs/TODO.md if it proves wanted.
- Replaying a change. That is the gate's job, and eyeball knows nothing of what
  a file means.

## Iteration 1: eyeball cat

- [x] `eyeball cat --base <old> <path>` prints an approved review's captured
      bytes for the path, and refuses with a code when none exists or two
      approved reviews hold different bytes.
- [x] A test that `eyeball check` accepts a commit named by no ref, with one
      blob swapped for the approved bytes. It must fail when the swap is
      skipped.
- [x] `eyeball doc` lists `cat`, and `docs/` records why it exists.

**Departures, iteration 1.**

- `eyeball cat` answers in JSON with a SHA-256 digest, not raw bytes, and the
  command checks the digest. The plan said it prints the bytes. A raw 200 body
  failed in the first CLI test: the client treats a body that is not JSON as a
  dropped connection, so that a cut-short answer cannot pass as a real one. A
  raw file would lose that protection, and could reach the gate truncated.
  Plainly, the command still prints only the bytes. It recomputes the digest
  first and prints nothing on a mismatch, and a test damages the stored blob to
  prove it. `--json` prints the answer instead. The cost is base64 and a whole
  file in the daemon's memory, which suits a spec and not an artifact.
- The premise is measured. `TestServer_checkAcceptsAnUnreferencedCommit` shows
  `eyeball check` accepts a commit named by no ref. With the merged blob left in
  place of the approved one, the same test fails with `tree_differs`.
- `make check` failed on `master` before any change. 016's last close left a
  blank line at the end of the spec, which `oxfmt --check` rejects. Commit
  `de8c97f` drops it.
- The end-of-branch review pass read this diff and found no defect in it. It
  checked that `cat` and `check` use the same approved-review and base filters,
  so a swapped tree passes `check` only when one approved review holds that
  digest and mode for the path. It confirmed by hand that
  `eyeball cat --base <old> -- <path>` works, the form a gate passes. No test in
  `cli_test.go` covers it.

> **Completed** 2026-10-12 03:58 UTC
>
> - `make check` — 5.5s
