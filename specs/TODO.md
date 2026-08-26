# TODO

The work that is next, in order, and the work that is not scheduled. One or two
lines each. Detail belongs in a spec, and a spec is written when the work is
ready to build.

This file exists because neither a person nor an agent holds a project's
outstanding work in their head. A person forgets it between sessions and an
agent forgets it between contexts.

## Next

Each becomes `specs/NNN-*.md` when it reaches the front.

1. **004 Capture and diff.** Git shell-out for the base commit and the dirty
   file set, a capture that holds only what differs, and the diff between two
   captures with word-level marking.
2. **005 The daemon and the CLI.** One process owns the store, the CLI is a thin
   client over a unix socket, and `wait` is a long poll rather than a loop.
   `--json` everywhere, flags in any position, `eyeball doc`.
3. **006 The surface.** Templates seeded from the tearouts: the queue, a review,
   widening, the project view. Server-rendered, htmx for the interactions, and
   the live stream that carries updates and doubles as the connection state.
   Every action reports its own failure, because a tap that does nothing is the
   worst thing this surface can do.
4. **007 Notification.** Web push with VAPID, the service worker that exists for
   it, the installed-page requirement, and the lapsed-subscription state the
   surface has to report.
5. **008 Rendered files.** Images, pages, media, and the sandbox that anything
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
- [ ] product: whether the daemon exits when nothing is outstanding, meaning no
      open reviews and no connected agents. architecture.md names it as a
      smaller rule than running until stopped, and does not adopt it.
- [ ] product: what a marker file holds beyond naming a project root, if
      anything ever needs a second field.
- [ ] product: say what an agent declares as its base commit once it has
      committed its own work. A capture is the delta from the base, so a base of
      `HEAD` after committing is an empty review and a base of the branch point
      is not.
- [ ] docs: a deployment document, once there is something to deploy and a
      decision about where it runs.
- [ ] tooling: whether the tearout server survives once the real binary can
      serve `docs/design/` in a dev mode.
- [ ] tooling: whether the surface needs a JS build step beyond esbuild's
      default bundling.
- [ ] tooling: whether the spec-reference rule can be checked. A three-digit
      spec number outside parentheses is mechanical to find. Whether a name sits
      beside it is not.

## Open questions

Not repeated here. Product questions live in
[docs/product.md](../docs/product.md) under **Open questions**, and build
questions in [docs/architecture.md](../docs/architecture.md) under **Not decided
here**. Both are where they get answered, and a copy in a third place drifts.
