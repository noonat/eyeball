---
status: active
created: 2026-10-12T00:00:00Z
updated: 2026-10-12T03:02:46.828652223Z
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
---

# 015 Applying the reading desk

013 chose a visual direction on the phone: the reading desk, with the agent's
note as a letter, drawn in `docs/design/mix.html`. docs/design/README.md records
the choice and the reasons the owner gave for it. This spec makes it the
surface's visual language. `app.css` takes the direction, every tearout is
redrawn in it, the binary serves its fonts, and the templates gain the eye, the
highlighter and the letter. It also fixes the one shortfall 013 measured in the
shipped surface: white Approve text on the light theme's green is 3.53 to 1,
under the 4.5 a 14px label needs. These are the Next item and one Unscheduled
item in specs/TODO.md.

## Design

What the direction looks like is settled in `mix.html` and is not reopened here.
This section covers what drawing it in the shipped surface decides that the
sheet did not.

**`app.css` absorbs the three direction stylesheets.** `directions.css`,
`desk.css` and `letter.css` override rules in `app.css`, which is how a sheet
could try a direction without touching what ships. Folded in, each override
replaces the rule it overrode rather than following it, so a reader of `app.css`
finds one rule per thing. Appending them unchanged was rejected: two rules for
every pill and row, with the second winning by order alone, is the cascade a
reader has to run in their head to know what renders. The rules that drive a
sheet's Approve moment by `:target`, `.moment`, `.is-before`, `.is-after` and
`.verdict`, are tearout plumbing, and move to `tearout.css` rather than ship.
The shipped blink is a class the client adds, `.eye.blink`, which `app.css`
animates and `web/ts/classes.ts` names. Then `mix.html` and the three
stylesheets go, and the five sheets are redrawn in the new language.

**The fonts come from the binary, with a cache.** `app.css` names them by a path
relative to itself, `fonts/...`, which resolves beside the tearouts and, served
at `/static/app.css`, to `/static/fonts/...`. A route serves the `.woff2` files
of `docs/design/fonts` from the embedded files and answers 404 for anything else
there, which is each family's `OFL.txt`. The licenses stay in the repository
beside the fonts. The fonts are 215 KB, sent on every page load under the
surface's `no-store`, so they are the one response that gets a cache: a strong
ETag from the bytes and `Cache-Control: no-cache`, so a load costs a request and
a 304. That corrects architecture.md, which says nothing here is large enough to
cache. Long-lived caching under content-hashed names was rejected: it changes
how every static file is addressed, which is its own item in specs/TODO.md, and
a revalidation on the tailnet is assumed cheap, not measured.

**The eye is on every page, so every page knows the waiting count.** The
layout's topbar draws it, so the page the layout prints carries the count, read
with one query when the page renders. Its definition has to match the queue's
Waiting section, and a test holds the two together. Where the count cannot be
read, as on a failure page whose cause is the database, the topbar draws no eye,
because a shut eye would say nothing waits. On a page other than the queue, the
count is as of that page's load and does not follow the stream. The queue's own
page reloads on its stamp, as it does today.

**A waiting row carries the first paragraph of its note**, clamped to two lines
by CSS. The note is required on every round and stored on it, and the queue's
query already joins the latest round, so the paragraph comes from that join. It
is split by `paragraphs`, the function the review page splits the letter with,
so a note reads the same in both places. The fixture's single sentences were
written to fit. A real note's first paragraph can run long, which the clamp is
for.

**The agent's note on the review page is the letter, first, and signed.** The
reviewer's verdict note on a decided round is the reviewer's own words, so it
stays in the chrome face, as a plain note after the letter. A decided round's
notice and its earlier rounds keep their places below the brief. These are the
decisions the sheet did not have to make, so the tearouts in iteration 1 draw
each of them, and the owner judges them there.

**The blink plays after the reload.** Approving answers with `HX-Refresh`, so
the page that blinks is the one that loads next: a decided round, with no
decision bar, its Approved notice and pill, and the eye at the new count.
review.html draws the moment that way. `mix.html`'s bar settling in place is not
drawn, because the live app reloads instead. The client sets a mark naming the
review when an Approve request starts, and clears it when the request fails. On
load, the page adds `.blink` to the eye once if the mark names the review on
screen and that review is approved, then clears the mark. Setting it when the
request starts means nothing depends on whether htmx raises an event between
reading `HX-Refresh` and reloading, which the minified source did not settle. A
server-side flag in the redirected URL was rejected, because the address would
then replay the blink on every reload and every shared link. Under
`prefers-reduced-motion` there is no blink, and the count is already right.

### Measured and assumed

- Measured in 013: the fonts load in iOS Safari from the tearout server, and
  white Approve text is 3.53 to 1 on `#1a9b6c` and 4.92 on `#15805a`.
- Read from `internal/server/surface.go`, not exercised: the surface's policy,
  `default-src 'self'`, admits a same-origin font, and `img-src 'self' data:`
  admits the eye's data-URI masks as it does the icons'. Iteration 3's browser
  tests exercise both.
- Assumed: a revalidated font is cheap on the tailnet. Not measured.
- Assumed: a stale count on a page other than the queue is acceptable, because a
  reviewer reads it on arrival and the queue corrects it. Judged on the phone in
  iteration 5.

## Review

Spec review pass: declined, because the design was settled on the phone in 013
and what remains is small and named above.

End-of-branch review pass: requested, because folding three stylesheets into
`app.css` touches every rule the surface ships, and a rule lost in the fold
renders as something plausible on a screen no test looks at.

## Out of scope

- Content-hashed addresses under `/static/`. specs/TODO.md holds it.
- The eye's count following the stream on pages other than the queue. Added to
  specs/TODO.md in iteration 3.
- A manual theme override. specs/TODO.md holds it.

## Iteration 1: The tearouts in the new language

```punchcard
required_acks:
  - approved-on-phone
```

> **Departures.** The fold removed the middle dot, so the `dot-sep` class had no
> rule left, and the template class check refused `web/templates/queue.html` for
> using it. The class went from that template and the sheets here rather than
> staying as an empty rule, which touched one template ahead of iteration 3. The
> empty state's large heading, a shared fix in 013, now applies only to an empty
> state holding the eye. The project view's "Nothing changed on disk" took the
> 26px heading and read as a headline, so ordinary empty states keep the dashed
> box. One queue frame, not installed yet, showed no waiting pill although a
> review waited below it. The live layout shows the count whenever anything
> waits, so that frame's eye is open.

> **Departure, from the owner's look on the phone, 2026-10-12.** The first-run
> section still drew the installed icon as the letter e on the accent, which the
> fold had turned into an ink square. The owner asked for it to take the new
> language, so the specimen is the eye on the paper with a highlighter iris, in
> three fixed colors, since a home screen icon does not follow the theme. That
> moves the installed icon's redraw, out of scope in the approved spec, into
> iteration 3: `tool/appicon` reads the accent tokens, and run now it would draw
> an ink square.

- [x] app.css holding the direction, each override folded into the rule it
      replaced, and the light Approve green at `#15805a`
- [x] The `:target` moment's rules in tearout.css, and `.eye.blink` in app.css
- [x] mix.html, directions.css, desk.css and letter.css removed, and nothing
      linking them
- [x] queue.html, review.html, widening.html and project.html redrawn: the eye
      in every topbar, marked waiting titles, note lines and letters
- [x] review.html drawing a decided round's verdict note and notice, earlier
      rounds under a letter, and Approve as the reloaded decided round blinking
- [x] foundations.html's specimens redrawn: palette, type scale, both faces,
      pills, rows, the highlighter, the eye open and shut, the letter
- [x] docs/design/README.md's rules, table and direction section describing the
      language as shipped, with every rule the change made false corrected
- [x] Every frame and every expanded view screenshotted headless and looked at,
      and every text tone measured at 4.5 to 1 or better
- [x] specs/TODO.md's Approve contrast entry removed

> **Completed** 2026-10-12 03:17 UTC
>
> - acks: approved-on-phone
> - `make check` — 19.6s
> - `go test -race ./...` — 38.9s

## Iteration 2: The fonts from the binary

- [x] `docs/design/fonts` embedded, and `GET /static/fonts/{path...}` serving
      each `.woff2` as `font/woff2` with a strong ETag and `no-cache`
- [x] A request with a matching `If-None-Match` answered 304 with no body
- [x] Tests through `SurfaceHandler`, so the surface's own headers apply: each
      font with its cache headers, the 304, `OFL.txt` and an unknown path a 404,
      and every `url()` in app.css naming a file the route serves
- [x] architecture.md's route table and its caching paragraph corrected

> **Completed** 2026-10-12 03:22 UTC
>
> - `make check` — 17.6s
> - `go test -race ./...` — 38.0s

## Iteration 3: The templates

```punchcard
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -tags browser ./internal/server/...
  - cmd: go vet -tags browser ./internal/server/...
  - cmd: go tool staticcheck -tags browser ./internal/server/...
```

- [x] The layout's topbar drawing the eye on every page from a waiting count
      read when the page renders, shut when nothing waits, absent when the count
      cannot be read
- [x] A test holding the waiting count to the queue's Waiting section
- [x] Waiting titles marked, and each waiting row carrying the first paragraph
      of its latest round's note, split by `paragraphs`
- [x] The review page's note as the signed letter before the brief, with the
      verdict note after it as a plain note
- [x] The empty queue's shut eye, and the layout's theme-color matching the new
      paper
- [x] Tests: the note's first paragraph on a row, the letter's order and
      signature, and the eye open, shut, and absent on a count error
- [x] A browser test loading a page with the fonts and the eye under the
      surface's own policy, with no violation reported
- [x] specs/TODO.md entry for the count following the stream off the queue
- [x] `tool/appicon` drawing the eye from `.badgeicon`'s three fixed colors, the
      PNGs under `web/static/icons` regenerated, and the TODO entry for the icon
      removed

> **Completed** 2026-10-12 03:34 UTC
>
> - `make check` — 6.6s
> - `go test -race ./...` — 3.2s
> - `go test -tags browser ./internal/server/...` — 1.9s
> - `go vet -tags browser ./internal/server/...` — 80ms
> - `go tool staticcheck -tags browser ./internal/server/...` — 192ms

## Iteration 4: The blink after Approve

```punchcard
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -tags browser ./internal/server/...
  - cmd: go vet -tags browser ./internal/server/...
  - cmd: go tool staticcheck -tags browser ./internal/server/...
```

- [ ] The mark set when an Approve request starts, cleared when it fails, and
      read once on the next load
- [ ] The blink played only on the approved review the mark names, by adding
      `classes.blink` to the eye, and never under reduced motion
- [ ] A browser test: approving blinks once, a reload after it does not, and a
      failed approve leaves no mark

## Iteration 5: On the phone

```punchcard
required_acks:
  - checked-on-phone
```

The owner uses the installed surface on real reviews, served by a daemon built
from this branch. One daemon serves every project, so restarting it holds up
every agent waiting on a review, and the restart is confirmed with the owner
first.

- [ ] The daemon rebuilt from this branch and restarted, once the owner agrees
- [ ] The fonts loading in the installed page
- [ ] The eye's count, the highlighted rows, the letter and the blink on real
      reviews, with anything found recorded here or in specs/TODO.md
