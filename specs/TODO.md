# TODO

The work that is next, in order, and the work that is not scheduled. One or two
lines each. Detail belongs in a spec, and a spec is written when the work is
ready to build.

This file exists because neither a person nor an agent holds a project's
outstanding work in their head. A person forgets it between sessions and an
agent forgets it between contexts.

## Next

Each becomes `specs/NNN-*.md` when it reaches the front.

1. **001 Foundation.** The Go module, the package layout, and `make check` as
   the gate every iteration closes on. Includes the convention test that holds
   this repo to its own rules, and moving the three Python scripts under
   `docs/design/` to Go: the tearout checker becomes a test, the icon generator
   and the tearout server become `tool/` binaries.
2. **002 The store.** SQLite for reviews, rounds, comments and decisions, plus
   the content-addressed blob store for captured files. A comment is fsynced on
   write, because losing one is the failure this must not have.
3. **003 Capture and diff.** Git shell-out for the base commit and the dirty
   file set, a capture that holds only what differs, and the diff between two
   captures with word-level marking.
4. **004 The daemon and the CLI.** One process owns the store, the CLI is a thin
   client over a unix socket, and `wait` is a long poll rather than a loop.
   `--json` everywhere, flags in any position, `eyeball doc`.
5. **005 The surface.** Templates seeded from the tearouts: the queue, a review,
   widening, the project view. Server-rendered, htmx for the interactions, and
   the live stream that carries updates and doubles as the connection state.
   Every action reports its own failure, because a tap that does nothing is the
   worst thing this surface can do.
6. **006 Notification.** Web push with VAPID, the service worker that exists for
   it, the installed-page requirement, and the lapsed-subscription state the
   surface has to report.
7. **007 Rendered files.** Images, pages, media, and the sandbox that anything
   executable renders in.

## Unscheduled

- [ ] design: a tearout for the comment composer's line-range selection on a
      touch screen. The open question in product.md is whether ranges are worth
      the interaction cost at all.
- [ ] design: settle the expand-or-list threshold against real reviews. Ten
      files and a few hundred lines is a guess written down so it can be
      corrected.
- [ ] product: whether a review-level comment and a verdict note are one
      concept. They overlap almost completely and differ only in when they are
      written.
- [ ] product: what a marker file holds beyond naming a project root, if
      anything ever needs a second field.
- [ ] docs: a deployment document, once there is something to deploy and a
      decision about where it runs.
- [ ] tooling: whether the tearout server survives once the real binary can
      serve `docs/design/` in a dev mode.

## Open questions

Not repeated here. Product questions live in
[docs/product.md](../docs/product.md) under **Open questions**, and build
questions in [docs/architecture.md](../docs/architecture.md) under **Not decided
here**. Both are where they get answered, and a copy in a third place drifts.
