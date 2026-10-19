---
status: active
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

- [x] A script, on this branch only, that draws a small panel on a review page
      showing `innerHeight`, the root's `clientHeight`, `visualViewport` height
      and `offsetTop`, `scrollY`, and the bar's top and bottom
- [x] The panel also showing `document.visibilityState` and the navigation type,
      so a page that reloaded itself reads differently from one opened
- [x] The panel updating on scroll, on resize and on `visualViewport`'s resize
      and scroll events, and readable over both themes

> **Completed** 2026-10-19 02:21 UTC
>
> - `make check` — 21.6s
> - `go test -race ./...` — 44.8s
> - `go test -count=1 -tags browser ./internal/server` — 34.0s

**Departures, iteration 1.**

- The panel follows the visual viewport, 70 pixels below the top of what is on
  screen, rather than sitting under the page header. The owner saw that in the
  fault the page header and each file's header are off screen too, as if the
  whole view had moved up, and come back as the page is scrolled up. A panel
  pinned under the header would have been off screen with it. It also shows the
  visual viewport's `pageTop` beside `scrollY`, so the two can be compared.

## Iteration 2: Read on the phone

```punchcard
required_acks:
  - readout-read
```

With the daemon running this branch's build, the owner follows the five steps
above in the installed app and sends screenshots:

- [x] The readout with the bar misbehaving, at the top of the page and scrolled
      halfway down, saying whether a verdict note was typed
- [x] The readout on the same review, reached again from the queue, saying
      whether the bar sits at the bottom there

**Readings so far, iteration 2.** Taken by the owner on 2026-10-18 on test
reviews in a scratch repository, with this branch's build.

- In the fault the visual viewport is 576 points tall in a 894-point window,
  with nothing focused. Scrolled down, its `offsetTop` is 318; at the top of the
  page it is 0. In a good state it is 894. The page header, each card's header
  and the bar all move together, because the browser places them against a
  visual viewport 318 points short.
- With a real keyboard open on the verdict note, the visual viewport is 508, 386
  points short. The fault's 318 is 68 points less, about the height of the bar
  iOS adds above a web field's keyboard, so the phantom keyboard is likely not
  one of the page's own fields. That is inferred from one pair of numbers.
- Going to the queue and back left it short. Only closing the app cleared it, so
  the state lives in the web view and not in the page.
- It followed a reload by the live stream four times, one of them inferred
  rather than seen, and one with the page left untouched while a round was
  requested. A reload after a verdict alone stayed good three times. Once it
  broke with no reload from the agent's side, and its readout was not captured.
- The keyboard being up when a verdict is sent is not the trigger: the owner
  could not reproduce it that way, with or without closing the keyboard first.

The readout now also logs each change in the visual viewport's height, with the
time since the page began loading and what had focus, so the next capture shows
whether the page loaded short or shrank after. The trigger is intermittent, so
iteration 2 stays open while the owner uses this build normally and captures the
next occurrence.

A capture on 2026-10-19 with that log settles part of it. The page was opened by
a navigation, not a reload, from the home screen as far as the owner remembers,
with no keyboard open. The visual viewport grew 854, 875, 884 in steps 25 ms
apart, which looks like the app's opening animation, and 230 ms later fell to
576 with nothing focused. So a reload is not needed for the fault, and the page
asks for no keyboard: iOS applies a keyboard-sized viewport to the page on its
own as the app comes forward.

That matches neither expected reading, and the owner agreed to an experiment
first. On seeing the fault, the page focuses a hidden field and blurs it at
once, and if the viewport is still short, does the same inside the reader's next
tap, where iOS treats the focus as the reader's own. The readout logs each
attempt and the height that followed.

The experiment's capture, the same night, names the trigger and the fix. The
owner came from a terminal app with its keyboard open and tapped a notification.
The page loaded 318 short with nothing focused, a keyboard without the bar iOS
adds over a web field, which is the other app's keyboard carried into this one.
The focus with no tap changed nothing. The focus inside the next tap brought a
real keyboard up, to 508, and once it went away the viewport was 894 again, 2.8
seconds after the tap.

> **Completed** 2026-10-19 07:39 UTC
>
> - acks: readout-read
> - `make check` — 5.8s
> - `go test -race ./...` — 1.7s
> - `go test -count=1 -tags browser ./internal/server` — 32.9s

## Iteration 3: The fix

The readout picks the fix. The two readings expected, and their fixes:

- **The viewport numbers are short after the reload and right after reopening.**
  The reload ran before iOS settled the window's size. `followStream` waits to
  reload until the page is visible and `innerHeight` has held for two frames.
- **The numbers are right and the bar is wrong.** WebKit kept a stale sticky
  position. The bar is laid out again once the reloaded page settles, by
  dropping and restoring its `position`.

A reading that matches neither goes to the owner as a question before any fix is
written.

- [x] The fix the readout picked, with a comment naming the five steps and the
      readout that showed the cause
- [x] A browser test that fails with the fix removed, or a departure saying why
      Chromium cannot show the condition
- [x] The readout script removed, and nothing left of it in the tree

**Departures, iteration 3.**

- The readout picked neither expected fix. The fault is iOS loading the page
  with another app's keyboard height taken off the viewport, with nothing
  focused, so neither waiting to reload nor laying the bar out again would have
  helped. The fix is the one the experiment showed working: on the next tap, a
  hidden field is focused and blurred, which has iOS show a keyboard and take it
  away. It lives in `web/ts/viewport.ts`, tries at most twice per page, and
  waits three seconds after each try before judging again, since the keyboard a
  try brings up took 2.8 seconds to go on the phone.
- The cost is a keyboard that flickers up once, on the first tap after the
  fault. A focus with no tap would have had none, and the phone showed it does
  nothing.
- The browser test makes the visual viewport report the fault, since Chromium
  has none, and counts the hidden field arriving rather than focus events, which
  a headless page without focus does not fire. It fails with the fix switched
  off and with the settling wait removed.
- The browser suite failed once while closing iteration 2, and passed the four
  runs after. Which test failed was not captured, as in 016 and 017.

> **Completed** 2026-10-19 07:44 UTC
>
> - `make check` — 5.8s
> - `go test -race ./...` — 43.0s
> - `go test -count=1 -tags browser ./internal/server` — 40.1s

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
