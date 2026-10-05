# TODO

The work that is next, in order, and the work that is not scheduled. One or two
lines each. Detail belongs in a spec, and a spec is written when the work is
ready to build.

This file exists because neither a person nor an agent holds a project's
outstanding work in their head. A person forgets it between sessions and an
agent forgets it between contexts.

## Next

Each becomes `specs/NNN-*.md` when it reaches the front.

1. **005 The daemon and the CLI.** One process owns the store, the CLI is a thin
   client over a unix socket, and `wait` is a long poll rather than a loop.
   `--json` everywhere, flags in any position, `eyeball doc`.
2. **006 The surface.** Templates seeded from the tearouts: the queue, a review,
   widening, the project view. Server-rendered, htmx for the interactions, and
   the live stream that carries updates and doubles as the connection state.
   Every action reports its own failure, because a tap that does nothing is the
   worst thing this surface can do.
3. **007 Notification.** Web push with VAPID, the service worker that exists for
   it, the installed-page requirement, and the lapsed-subscription state the
   surface has to report.
4. **008 Rendered files.** Images, pages, media, and the sandbox that anything
   executable renders in.

## Unscheduled

- [ ] design: a tearout for the comment composer's line-range selection on a
      touch screen. The open question in product.md is whether ranges are worth
      the interaction cost at all.
- [ ] design: settle the expand-or-list threshold against real reviews. Ten
      files and a few hundred lines is a guess written down so it can be
      corrected.
- [ ] design: where an insertion should attach when a run of identical lines
      makes several placements equally minimal. 004 pins one tie-break and GNU
      diff shifts boundaries with heuristics of its own, so the two differ on
      about a fifth of diffs. Which reads better needs real reviews.
- [ ] design: whether word marking should be suppressed where a paired line
      shares almost nothing with the one it replaced. Marking it end to end is
      noise, and the proportion to cut at needs real reviews.
- [ ] product: whether a review's base should follow a branch that moves under
      it. 004 fixes the base at the first round, so a merge landing mid-review
      arrives inside the review as work the agent did not do.
- [ ] product: whether a marker file inside a git repository should get the
      no-git treatment. 004 decides a project is git by whether its root holds a
      `.git` entry, so a monorepo package named by a marker file is captured by
      walking: whole files, no base, and only the paths the review named, inside
      a repository that could have answered better.
- [ ] product: whether a rename should read as a rename. 004 captures it as a
      delete and an add, which is what a path-to-content list holds either way.
- [ ] product: file modes. Nothing records them, so a change that only sets a
      bit shows as a file with no changed lines.
- [ ] product: whether a file too large to send to a phone should be captured at
      all. Today its bytes are stored and the surface refuses to send them,
      which costs disk for something nobody opens.
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
