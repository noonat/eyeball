---
status: ready
created: 2026-10-11T00:00:00Z
updated: 2026-10-12T03:50:13.15717558Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -count=1 -tags browser ./internal/server
---

# 017 The sticky file header

On a review page, a long file's header scrolls away with the top of its card.
Partway down a long diff nothing on screen says which file it is. This spec
makes a card's header stay at the top of the screen while its file scrolls under
it, and leave with the card's bottom. It is the Unscheduled entry in
specs/TODO.md, and 016's out-of-scope list points at it.

## Design

**The card's `summary` is sticky, at the bottom of the page header.** 016 made
the document scroll and keeps the sticky page header's height on the root as
`--head-height`. The summary sticks at `top: var(--head-height, 0px)`. In a
tearout's phone frame the property is unset, so the summary sticks at the top of
the frame's `.scroll`. Its z-index sits above a card's code and below `.head`
and `.decide`, so the page header and the decision bar still cover it.

**`.file` clips with `overflow: clip`, not `overflow: hidden`.** `hidden` makes
the card a scroll container, and a sticky element sticks inside its nearest
scroll container. The summary then never sticks. Measured in headless Chromium
at 390 pixels, scrolled 1500 pixels into a 3000-pixel card: with `hidden` the
summary sat at -1389, off screen; with `clip` it sat at 50, the page header's
bottom. `clip` cuts children to the rounded corners as `hidden` does. The rule
is shared with the file page and every tearout, and should look the same in
each. Safari has supported `clip` since 16, from memory, not checked. A sticky
summary inside `details` in Safari is assumed to work, not measured. Iteration 3
checks both on the phone.

The rejected alternative is one bar in the page header naming the file under the
top of the screen. It needs a script tracking which card is there, and it would
copy the path, the fold control and the Whole file link into a second place. A
sticky summary keeps all three where they are. Either costs the same: a header's
height of the diff covered at the top of the screen, about 40 pixels, on top of
the page header.

**Folding a card from its stuck header keeps that header in place.** The stuck
header is the obvious place to fold a long file. When a card folds, its top
stays where it was in the document, far above the screen, and the scroll
position does not move. Measured in headless Chromium: folding a 3000-pixel card
from 1500 pixels down left the next card's header stuck at the top with the
reader 1400 pixels into its body. `holdPlace` in `web/ts/bodies.ts` does not
correct it, because it corrects a card that moved, and the folded card's top did
not move. So when a card folds with its top above the page header's bottom, the
page scrolls to put the card's top at the header's bottom, which shows the
folded header and the next card under it. `holdPlace` records its anchor again
after that scroll, so its own observer, which fires on the same resize, finds
nothing moved. Which runs first is not known yet, and a browser test pins the
result either way.

**A jump inside a card lands below its header.** `scroll-padding-top` covers the
page header only. A link to a comment, `#comment-N`, and the composer's
`scrollIntoView` after a tap would land a target under the stuck file header. A
target inside an open card gets a `scroll-margin-top` of the summary's height,
added to the page header's padding. The summary's height is one line of the
path and its padding, set in app.css, so it is a CSS value rather than a
measurement. A jump from the list of files lands on a card's own top, where its
header is not yet stuck, and needs nothing.

## Overlap with 015

015 is on its own branch, active, with its phone check open. It rewrites app.css
and redraws `docs/design/review.html`, both of which this spec edits. This spec
is executed on a branch from master after 015 merges, so it builds on 015's
app.css and tearouts and never merges against them.

## Review

Spec review pass: declined, because both premises it rests on were measured, and
the phone check covers the one that cannot be measured here. End-of-branch
review pass: declined, because the change is a few rules and one handler, each
held by a browser test that is broken to prove it can fail.

## Out of scope

- The file page. It shows one file, and its crumb names the file at the top.
- A hunk header naming the function it is in. That is its own Unscheduled entry,
  and needs a rule per language.
- Keeping a reader's place as a card and an offset rather than a scroll
  position. Its own Unscheduled entry.

## Iteration 1: The tearout

```punchcard
required_acks:
  - tearout-approved
```

- [ ] `.file` clipping with `overflow: clip`, and its summary sticky with a
      z-index under `.head` and `.decide`, in `docs/design/app.css`
- [ ] A frame in `docs/design/review.html` scrolled partway into a long file,
      its header stuck at the top of the frame
- [ ] Every frame and expanded view that shows a card screenshotted headless
      and looked at, for a card the change draws differently
- [ ] The owner has looked at the tearout on the phone and approved it

## Iteration 2: The review page

- [ ] The summary sticking at `--head-height` on a real page, and a browser test
      that it sits at the page header's bottom partway down a long card
- [ ] Folding a card whose top is above the page header's bottom scrolls its top
      to the header's bottom, with `holdPlace` recording its anchor after
- [ ] A browser test that folding from a stuck header leaves that header at the
      page header's bottom, and the next card under it
- [ ] `scroll-margin-top` of the summary's height on targets inside an open
      card, and a browser test that a comment jump lands below both headers
- [ ] Each of the three tests failing with its rule or handler removed, checked
      by removing it

## Iteration 3: On the phone

```punchcard
required_acks:
  - phone-checked
```

With the daemon running the branch's build, the owner checks, in the installed
app and in a Safari tab:

- [ ] A long file's header stays under the top bar as it scrolls, with Safari's
      toolbar showing and collapsed
- [ ] Folding from the stuck header shows the folded header and the next file
- [ ] A comment link and a tapped line's composer land below both headers
- [ ] The card's corners still clip, and the file page looks as it did
