---
status: done
created: 2026-10-11T00:00:00Z
updated: 2026-10-12T03:38:31.124924547Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -count=1 -tags browser ./internal/server
---

# 016 The status bar tap

On an iPhone, a tap on the status bar scrolls a page to its top. In eyeball it
does nothing. The owner hit this in the installed app. It was on specs/TODO.md's
Unscheduled list.

Approved by the owner as review 49, round 1, on 2026-10-11.

The cause is the layout. Every page is `.app`, a flex column the height of the
window, holding the top bar and a `.scroll` element that scrolls with
`overflow-y: auto`. The document itself never scrolls. iOS sends the status bar
tap to the document's scroller and not to an element scrolling inside it.
Observed by the owner on 2026-10-11 in Safari on the phone, on a tearout sheet:
the tap scrolled the sheet to its top and left a phone frame's inner `.scroll`
where it was. The installed app was not tested this way. It is assumed to behave
the same, and iteration 2 checks it.

The inner scroller came from the tearouts. A tearout draws the app inside a
phone frame, a fixed-height box, where only an inner element can scroll. The
real page inherited the layout. `web/static/shell.css` already holds the rules a
real page needs and a tearout frame does not, so the fix goes there.

## Design

**The document scrolls on a real page.** shell.css lets `.app` grow with its
content and stops `.scroll` from scrolling. A tearout's frame keeps the inner
scroller, since it still has a fixed height. Nothing changes on screen, so no
tearout changes.

The rejected alternative is a script that scrolls `.scroll` to the top on a tap.
iOS handles the status bar tap itself and sends the page no event, so a script
could react only to a tap on eyeball's own top bar, a gesture iOS does not have.

**A sticky header.** The top bar and the two notices under it, `#update` and
`#offline`, move into one `.head` element that is sticky at the top. Each is
sticky at `top: 0` today, which does nothing inside a column that never scrolls.
Once the document scrolls, the notices would sit on top of the bar. `.head` is
defined in app.css, since the `template-classes` check requires it. In a
tearout's frame it is a plain block.

**The top of the screen is the bottom of `.head`.** A script measures `.head`
whenever its size changes and sets its height on the root element as a custom
property. Its height varies with the safe area and with which notice is showing.
Two things read it:

- `scroll-padding-top` on the root, so a jump from the list of files lands below
  the header rather than under it.
- `holdPlace` in `web/ts/bodies.ts`, which picks the card at the top of the
  screen and keeps it there as bodies above it load.

**The scripts follow the document.** `loadBodies` observes cards against the
viewport instead of `.scroll`, and `holdPlace` and `keepPlace` read and set the
window's scroll position instead of `.scroll`'s. `holdPlace` turns off
Chromium's scroll anchoring on the root, where today it turns it off on
`.scroll`. Without that, Chromium's anchoring would cover for `holdPlace`, and
the browser tests would stop running the code Safari runs.

**What changes in a Safari tab.** Scrolling the document lets Safari collapse
its toolbars and turns on pull to refresh. Neither happens in the installed app,
which has no toolbars and no pull to refresh. Pull to refresh stays on. A
refresh is harmless here, since a review page reloads to the same place. Whether
the sticky decision bar sits correctly while Safari's toolbar collapses is not
known until it is seen on the phone.

## Out of scope

- A card's file header that stays at the top of the screen as its file scrolls.
  It is an Unscheduled entry of its own, and its sticky offset would be the same
  header height this spec measures.
- Changing the tearouts. Their frames keep the inner scroller.

## Overlap with 015

015 is on its own branch and rewrites app.css and the templates. This spec adds
one class to app.css and one element to `web/templates/layout.html`. Whichever
of the two merges second merges master into its branch first and asks for a
fresh review, and its note says what was already read.

## Review

Spec review pass: declined, because the iOS premise was checked on the phone and
the browser tests cover the scripts. End-of-branch review pass: declined, for
the same reason, and because the owner reads the branch in eyeball.

## Iteration 1: The document scrolls

- [x] shell.css letting `.app` grow with its content and `.scroll` not scroll,
      with a comment saying why a real page differs from a tearout's frame
- [x] `.head` in app.css and `web/templates/layout.html` holding the top bar,
      `#update` and `#offline`, sticky at the top of a real page
- [x] The header's height kept on the root as a custom property, and read by
      `scroll-padding-top` and by `holdPlace` as the top of the screen
- [x] `loadBodies`, `holdPlace` and `keepPlace` working on the document, with
      Chromium's scroll anchoring off on the root
- [x] The browser tests that use `.scroll` changed to the document, and still
      failing with `holdPlace` disabled, checked by disabling it
- [x] A browser test that on a long review the document scrolls and `.scroll`
      does not, and that a jump lands the card's top at the header's bottom

**Departures, iteration 1.**

- The browser tests build only under the `browser` tag, which neither of the
  spec's commands set, so as approved nothing would have run them.
  `go test -count=1 -tags browser ./internal/server` is now a required command.
  The suite failed once in eight runs here, and passed the other seven. Which
  test failed was not captured.
- shell.css does not stop `.scroll` scrolling. Once `.app` grows with its
  content, `.scroll` has nothing to overflow, and a rule setting its overflow
  changed no test's result. The `.app` rule is the one a test catches.
- `holdPlace` reads the header's bottom from the header itself, not from the
  custom property. The two agree while the header sits at the top, and the
  element's own box needs no parsing.
- Each piece was checked by breaking it. With `holdPlace` off and Chromium's
  anchoring off on the root, the jump test failed. With `holdPlace` off and
  Chromium's anchoring left on, it passed, which is why the anchoring is turned
  off. Without `scroll-padding-top` the jump test failed, and without the `.app`
  rule the document scroll test failed.

> **Completed** 2026-10-12 03:25 UTC
>
> - acks: reviewed
> - `make check` — 5.6s
> - `go test -race ./...` — 3.8s
> - `go test -count=1 -tags browser ./internal/server` — 28.9s

## Iteration 2: On the phone

```punchcard
required_acks:
  - phone-checked
```

With the daemon running the branch's build, the owner checks, in the installed
app and in a Safari tab:

- [x] The status bar tap scrolls a long review to its top
- [x] The top bar and the decision bar stay in place, with Safari's toolbar
      showing and collapsed
- [x] A jump from the list of files lands below the header, and the card stays
      there as bodies above it load
- [x] A review closed partway through reopens at the same place

Checked by the owner on 2026-10-11 on review 13, which has 19 files, and
reported as passing. No review was waiting, so the decision bar had no page to
show on. It is checked on the branch's own review in eyeball, before that review
is approved.

> **Completed** 2026-10-12 03:38 UTC
>
> - acks: reviewed, phone-checked
> - `make check` — 6.1s
> - `go test -race ./...` — 4.0s
> - `go test -count=1 -tags browser ./internal/server` — 29.9s

