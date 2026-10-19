---
status: draft
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -count=1 -tags browser ./internal/server
---

# 020 The decision bar after a resume

In the installed app the decision bar sometimes floats partway up the screen.
Scrolling down moves it up until it stops about halfway, and scrolling up moves
it back down to the bottom edge, with the review's text showing under it. It is
the first entry under Next in specs/TODO.md.

The owner reproduced it on the phone on 2026-10-18 with these steps:

1. An agent opens a review.
2. The owner requests changes, then sends the app to the background.
3. The agent requests a second round.
4. The owner brings the app back while it still shows the first round.
5. The page reloads to show round 2, and the bar misbehaves from then on.

The reload in step 5 is `followStream` in `web/ts/stream.ts`. On reconnecting,
the stream sends a new stamp, and the page calls `location.reload()` at once,
during the resume. `keepPlace` then scrolls back to the saved position. Round 1
was decided and had no bar, so the reloaded page is the first time the bar
appears.

**The cause is not known.** The bar is `position: sticky; bottom: 0` at the end
of `.scroll` (`.decide` in `docs/design/app.css`), and since 016 the document
scrolls. Headless Chromium at 390 by 844 pixels, on a seeded review of ten
files, keeps the bar against the bottom edge at every scroll offset, measured
2026-10-18. No WebKit runs on the development machine. Playwright's WebKit build
needs an older ICU than the system has, and WebKitGTK loads the page but runs
none of its scripts. In a screenshot of the fault, the bar's bottom sits about
310 points above the screen's bottom, close to the keyboard's height. That
suggests iOS gave the reloaded page a short viewport, but the suggestion is not
measured. Whether the owner typed a verdict note in step 2 was not recorded.

So the spec measures first, on the phone, and then fixes what the measurement
shows. The rejected alternative is guessing a fix now, such as pinning the bar
with `position: fixed`. The fault needs a particular sequence, so one try that
does not reproduce it proves nothing. `fixed` also gives up what the comment on
`.decide` says sticky was chosen for: a short page's bar sits under its content
and not at the foot of an empty desktop window.

## Review

Spec review pass: declined, because the cause is found on the phone and no
reading of the draft can stand in for that. End-of-branch review pass: declined,
because the fix is small, the owner reads the branch in eyeball, and only the
phone can show whether the fault is gone.

## Out of scope

- The keyboard leaving the viewport short outside a reload, if the readout shows
  that as a separate fault. It goes to specs/TODO.md under Unscheduled.
- Showing that the page is refreshing when the app comes back, already an
  Unscheduled entry.

## Iteration 1: A readout

- [ ] A script, on this branch only, that draws a small panel on a review page
      showing `innerHeight`, the root's `clientHeight`, `visualViewport` height
      and `offsetTop`, `scrollY`, and the bar's top and bottom
- [ ] The panel also showing `document.visibilityState` and the navigation type,
      so a page that reloaded itself reads differently from one opened
- [ ] The panel updating on scroll, on resize and on `visualViewport`'s resize
      and scroll events, and readable over both themes

## Iteration 2: Read on the phone

```punchcard
required_acks:
  - readout-read
```

With the daemon running this branch's build, the owner follows the five steps
above in the installed app and sends screenshots through dispatch:

- [ ] The readout with the bar misbehaving, at the top of the page and scrolled
      halfway down, saying whether a verdict note was typed
- [ ] The readout on the same review, reached again from the queue, saying
      whether the bar sits at the bottom there

## Iteration 3: The fix

The readout picks the fix. The two readings expected, and their fixes:

- **The viewport numbers are short after the reload and right after reopening.**
  The reload ran before iOS settled the window's size. `followStream` waits to
  reload until the page is visible and `innerHeight` has held for two frames.
- **The numbers are right and the bar is wrong.** WebKit kept a stale sticky
  position. The bar is laid out again once the reloaded page settles, by
  dropping and restoring its `position`.

A reading that matches neither goes to the owner through `dispatch ask` before
any fix is written.

- [ ] The fix the readout picked, with a comment naming the five steps and the
      readout that showed the cause
- [ ] A browser test that fails with the fix removed, or a departure saying why
      Chromium cannot show the condition
- [ ] The readout script removed, and nothing left of it in the tree

## Iteration 4: On the phone

```punchcard
required_acks:
  - phone-checked
```

With the daemon running this branch's build, the owner checks in the installed
app:

- [ ] The five steps leave the bar against the bottom edge while scrolling
- [ ] A review opened from the queue still shows the bar at the bottom, and a
      short page shows it under its content
