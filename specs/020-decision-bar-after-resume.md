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

- [/] The five steps leave the bar against the bottom edge while scrolling
- [ ] A review opened from the queue still shows the bar at the bottom, and a
      short page shows it under its content

## Where the investigation stands

Written on 2026-10-19 so a fresh session can pick it up. Iteration 4 is open.
The owner is not satisfied with the fix as it stands, for the reason under "What
is not solved".

### The fault, as measured

- The installed app shows the page through a visual viewport 318 points shorter
  than the 894-point window, with nothing focused. `window.innerHeight` and the
  root's `clientHeight` stay 894. `visualViewport.height` is 576, and its
  `offsetTop` is 318 when scrolled down and 0 at the top of the page.
- Everything placed against the visual viewport moves together: the sticky page
  header, each card's sticky header, and the decision bar, whose bottom sits
  at 576. Scrolling to the top makes it look right, since `offsetTop` is then 0.
- With a real keyboard open on the verdict note the viewport is 508, 386 short.
  The fault's 318 is 68 less, the height of the bar iOS adds above a web field's
  keyboard. So the phantom keyboard is not one of the page's own fields. That is
  inferred from these two numbers, not measured another way.
- It survives the page: going to the queue and back, or a reload, leaves it
  short. Closing the app clears it. So does a real keyboard shown and hidden,
  which is what the fix does.

### When it happens

Seen, with the readout running:

- The app opened from a notification while a terminal app had its keyboard up.
  The page loaded short with nothing focused.
- The app brought back with the app switcher while another app had its keyboard
  up, with the review page already open and scrolled. No reload: the page came
  back to the foreground at 894 and 55 ms later was 576.
- Once on a fresh open from what the owner thinks was the home screen with no
  keyboard. The viewport grew 854, 875, 884 in 25 ms steps, which looks like the
  app's opening animation, then fell to 576 230 ms later. The owner is not sure
  no keyboard was up.

Seen before the readout, and consistent with the above:

- After the page reloaded itself for a new round as the app came back from the
  background. These were early reports, and the owner had been typing in other
  apps between them.

Not a trigger:

- Sending a verdict with the note's keyboard up, or after closing it first.
- The reload that follows a verdict, on its own.

Not known:

- Whether a Safari tab shows it, or only the installed app.
- Whether other installed web apps show it, which would make it WebKit's alone.
  No WebKit bug matches it, searched on 2026-10-20. Another installed web app's
  tracker reports a footer stuck mid-screen after a return from the home screen,
  with no steps that reproduce it (cybersemics/em issue 5972).

### What has been tried

- A hidden field focused and blurred with no tap: no effect at all. iOS ignores
  a focus that a tap did not start.
- The same inside the reader's tap: restores the viewport. The first time it
  took 2.8 seconds and a real keyboard flashed up, to 508, and down. The second
  time it took 112 ms, and the owner saw no keyboard.
- The same on `touchend`: a touch that ends a scroll fires it too, and iOS does
  not honor a focus there, so scrolling spent both tries and nothing changed.
  The fix now waits for `click`, which fires only for a tap.

### The cause, as read in WebKit's source

Read on 2026-10-20 in `Source/WebKit/UIProcess/API/ios/WKWebViewIOS.mm` on
WebKit's `main`. The phone runs iOS 26.6.2. Apple's branch for that release is
not public, so that it holds the same code is inferred from the history below,
not seen.

- The web view listens for UIKit's keyboard notifications with no filter on
  which app's keyboard sent them.
- On a will-show or will-change-frame notification,
  `_shouldUpdateKeyboardWithInfo` keeps the keyboard's frame when a page element
  is being focused, or when `UIKeyboardIsLocalUserInfoKey` is false, which means
  the keyboard belongs to another app. The frame is stored in
  `_inputViewBoundsInWindow`. A did-change-frame notification stores its frame
  with no check.
- `_inputViewBoundsForViewportCalculations` takes that rectangle off the area
  the page sees, which is what `visualViewport` reports. Only a later will-hide,
  or a frame that ends off screen, clears it. Loading a page does not.
- The rule for another app's keyboard came in April 2015 with "Non-local
  keyboards don't update scroll view parameters" (WebKit bug 143791), for an
  iPad app sharing the screen with another app's keyboard. It last changed in
  2022, and no commit to the file since June 2025 touches the keyboard handlers.

So the hypothesis is this. When the app comes forward while another app's
keyboard is on screen, the web view is sent that keyboard's frame and keeps it.
The will-hide sent as the other app goes to the background never reaches it. The
rectangle then stays until a keyboard of the app's own is shown and hidden.

It accounts for each reading above. Nothing is focused, because no element of
the page asked for a keyboard. The 318 lacks the bar WebKit adds over a web
field, because the keyboard was another app's. It survives the queue and a
reload, because the rectangle belongs to the native web view and not to a page.
Closing the app builds a new web view. The fix's own keyboard ends in a
will-hide, which clears it.

Why the will-hide is missed is inferred, not read. How UIKit delivers another
app's keyboard notifications is not public source.

The viewport meta's `interactive-widget=overlays-content` makes
`_inputViewBoundsForViewportCalculations` return an empty rectangle, so a stuck
one would no longer shorten the viewport. WebKit parses the key only when the
setting `metaViewportInteractiveWidgetEnabled` is on, and its value in iOS
26.6.2 is not known. The cost is that the page's own keyboard would cover the
verdict note instead of shortening the viewport.

### The trials that test it

A probe page outside eyeball runs them: a sticky header, a long body, a sticky
footer and no eyeball script, added to the home screen. It sends each change in
the visual viewport, each visibility and focus change, and samples at fixed
delays after each return to the foreground, to a log on the development machine.
That separates WebKit from eyeball's code, and replaces screenshots with exact
numbers and times. It stays out of the tree, since it exists only to answer
this.

Each prediction that fails counts against the hypothesis:

1. The shortfall is the other app's keyboard height. A terminal app's keyboard,
   Notes' keyboard with its suggestion bar and the emoji keyboard each give a
   different number.
2. With another app's keyboard up, coming back through the app switcher or
   through Spotlight search leaves the probe short. With it down, never.
3. Replying to a notification while the probe stays in front shrinks the
   viewport and gives it back, since the probe is in front to receive the
   will-hide.
4. A Safari tab shows it too, since Safari uses the same web view code.

Coming back from a notification banner needs web push, which the probe does not
have, so that way back is checked on eyeball with the readout.

### The code on this branch

- `web/ts/viewport.ts` is the fix. It counts the fault when the visual viewport
  is more than 100 points short of the window, nothing that takes typing is
  focused, and the page is not zoomed. It then waits for a click and focuses and
  blurs a hidden 16px field inside it, at most twice per return to the
  foreground, with three seconds between tries for the viewport to settle. It
  checks on each viewport resize, on load, and on `visibilitychange` and
  `pageshow`.
- `web/ts/readout.ts` is back, temporarily, and the fix reports to it through an
  `eyeball:viewport` event: `visible`, `armed`, `reset N`, and each height. Both
  the readout and the reporting have to go before the branch is reviewed.
- `TestServer_Surface_browserViewport` makes the viewport report the fault in
  Chromium, which has none, and counts the hidden field arriving. It fails with
  the fix off, without the settling wait, and without the foreground check.
- The daemon on the owner's machine runs this branch, merged with master, with
  the readout. Test reviews in a scratch repository are open for checking.

### What is not solved

Until the reader's first tap, the page stays wrong. A scroll cannot fix it,
because iOS honors a focus only during a tap, and a reader's first gesture on a
review is usually a scroll. The owner does not accept "fixed on the first tap".

No workaround is written until the trials settle the cause. The options once
they do, roughly from cheapest:

- `interactive-widget=overlays-content`, under "The cause, as read in WebKit's
  source" above, if iOS 26.6.2 honors it.
- Laying out against the shortfall while it lasts. The gap is known at every
  moment: `innerHeight - visualViewport.height - visualViewport.offsetTop` below
  the visual viewport. The page could move the bar down and the header up by
  that much, through custom properties the sticky rules read, and stop once the
  viewport is whole. The risk is that WebKit already places sticky elements
  against the visual viewport, so a correction may double-move them, and the
  amount changes with every scroll.
- A `position: fixed` bar placed from `visualViewport` by script, as some web
  apps do to sit above a keyboard. It would follow the visual viewport by
  design, which is the opposite of what is wanted here, unless the script places
  it against the window instead.
