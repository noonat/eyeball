---
status: done
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

- [x] The five steps leave the bar against the bottom edge while scrolling
- [x] A review opened from the queue still shows the bar at the bottom, and a
      short page shows it under its content

**Departures, iteration 4.**

- The fix changed during this iteration. The owner did not accept a page that
  stays wrong until the first tap, so the cause was found and a reproduction
  built before anything else was written, as "Where the investigation stands"
  records. The reset now runs on the first touch as well as on a tap, since iOS
  honors a focus in the touch that starts a scroll. Before that touch, the page
  scrolls past the reader's place and back, which puts the header and the bar at
  the screen's edges while the viewport is still short.
- That corrects iteration 3's cost. No keyboard was seen at any reset on the
  phone in this iteration, where iteration 3 saw one flash up once.
- Laying the header and the bar out against the shortfall was tried and dropped.
  iOS drew the bar off the screen where the page measured it on it.
- The five steps are not the reproduction. The phone check uses the one under
  "What the trials showed": a notification tapped over an app that has had its
  keyboard up, which an agent can send on request.
- At the very end of a review the scroll past and back leaves the header off the
  top and blank space under the last card until the first touch. Three tries did
  not move it. The bar there sits in its place under the content, so the owner
  accepted it.
- A page with no keyboard of its own showing is left alone for a second after a
  field loses focus, since a real keyboard is still going away then.
- The browser tests fail with each part removed: the first-touch reset, the wait
  after a field loses focus, the realign, its second try, and its stopping at
  the first touch. `TestServer_Surface_browserJump` fails now and then on master
  too: 3 times in 50 runs alone. The two failures whose value was kept had the
  target 3482 pixels from where it belongs, the value it shows when it fails on
  this branch.

> **Completed** 2026-10-21 09:48 UTC
>
> - acks: phone-checked
> - `make check` — 5.8s
> - `go test -race ./...` — 4.0s
> - `go test -count=1 -tags browser ./internal/server` — 45.2s


## Where the investigation stands

Written on 2026-10-19 so a fresh session could pick it up, and carried on
2026-10-20 until the cause was read, the fault reproduced on request, and the
fix measured on the phone.

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

### What the trials showed

Run on 2026-10-20 with the probe, then with eyeball on this branch's build.

- Coming back from the terminal app while its keyboard is up shrinks the visual
  viewport to 576, the fault's exact height, about 50 ms after the page turns
  visible, in the installed app and in a Safari tab alike. With nothing focused.
  In the probe it always came back 0.3 to 1 second later. Notes' keyboard never
  shrank it, so prediction 1 could not be tested: only the terminal app's
  keyboard reaches the page at all, for a reason not known.
- A reload during that dip does not clear it. The new page loads whole and
  shrinks within 40 ms, so the state belongs to the native web view.
- A scroll during the dip gives the fault's layout: `offsetTop` 318, and
  everything pinned to the layout viewport 318 points up the screen.
- The probe was never left short. Its own reloads at once, after 300 ms, a busy
  main thread and a scroll all recovered. The probe's script stops about 3
  seconds after it is hidden, measured once.

**A reproduction.** In eyeball, with a review open and scrolled partway:

1. Switch to the terminal app and use it with the keyboard up for a minute.
2. Have an agent open a new review, so its notification arrives.
3. Tap the notification with the terminal's keyboard still up.

It left the page short both times it was tried, reviews 108 and 109. On 108 the
page loaded by navigation at 894, turned visible at 894 at 150 ms, fell to 576
at 188 ms and stayed there, with `offsetTop` 312 and the bar at 422 to 582 when
scrolled. On 109 the phone was in landscape when tapped. The page loaded whole
at 440, and turned 576 tall on rotating to portrait 2.5 seconds later. Opening a
different review in the app cleared it both times.

On review 110 the terminal's keyboard was closed before the tap, and the page
was left short the same way: whole at 186 ms, 576 at 254 ms, `offsetTop` 318 and
the bar at 416 to 576. So the keyboard does not have to be on screen when the
notification is tapped. Whether it had been up earlier in that minute was not
recorded. Switching to the terminal and back through the app switcher cleared
it, which fits a later keyboard notification resetting the rectangle.

On review 111 about two minutes were spent in Photos, with no keyboard shown in
any app since eyeball was last whole. The tap left the page whole: 894 at 177 ms
and after. So the fault needs the terminal app, even with its keyboard closed at
the tap. A notification tap alone does not cause it.

With a minute in the terminal and its keyboard up, coming back through the app
switcher instead of a notification left the page whole once it settled. It fell
to 576 and came back 4.9 seconds later with no tap, far slower than the probe's
0.3 to 1 second. So the notification is part of the fault, and the terminal
alone is not.

A notification for the review already open left it short too. The service worker
then only focuses the window and does not navigate it, and the page reloaded
itself when its stream saw the new round: loaded whole by `reload` at 153 ms,
576 at 194 ms, `offsetTop` 213. So the service worker's navigation is not the
trigger.

A push that arrived while the terminal was in front, left untapped, did not
leave it short. Coming back through the app switcher ten seconds later, the page
was 894 with no dip at all. So the push waking eyeball in the background is not
the trigger. The tap on the banner is.

From Notes with its keyboard up, the tap left the page short once in two tries,
so the terminal app is not required. In both tries the app came forward on the
review already open, with no page load, so the tap did not reach the new review.
In the terminal runs the readout shows a page loaded fresh by navigation, but
which review it showed was not checked. The run that stuck fell to 576 15 ms
before the page turned visible, and showed `offsetTop` 260 and the bar at 474
to 634. So a page load is not required either. The run that stayed whole is not
explained. Notes' keyboard leaves the same 576 as the terminal's, which fits
both being the standard keyboard with its suggestion bar, 380 points tall on a
956-point screen.

What is left is a tap on the banner over an app that has had its keyboard up.
The keyboard did not have to be showing at the tap (review 110), and an app that
never showed one left it whole (review 111). Every other part has been taken
away once and the fault still followed.

iOS 26.6.2 does not honor `interactive-widget=overlays-content`. In a Safari
tab, the probe with only that key added to its viewport meta still shrank to 462
of 836 with its own field focused, as the probe without it does. So that fix is
closed on this release.

The short state can begin as eyeball is left, not as it comes back. With the
readout logging visibility, one run fell to 576 while still visible, 49 ms
before it was hidden, as the terminal app brought its keyboard up during the
switch, and stayed short while hidden. The banner tap then brought it back
short, and it cleared 0.7 seconds later. Every run from the terminal that stayed
short had a page load as the app came back: the service worker's navigation, or
the page reloading for a new round. The runs with no load cleared within about a
second, except the second Notes run. That is a pattern across about ten runs,
not a mechanism.

Laying out against the shortfall was tried and dropped. The header's sticky
`top`, a card header's, and the bar's sticky `bottom` each grew by
`visualViewport.offsetTop` while the viewport was short with nothing focused.
With the page short and `offsetTop` at 193, the header sat at the top of the
screen as intended, and the bar was nowhere on screen, though the page measured
it at 734 to 894. As `offsetTop` fell while scrolling up it slid into view from
below, and reached the bottom edge when `offsetTop` reached 0. So what the page
measures and where iOS draws a sticky element disagree in this state, and an
offset the page computes cannot place the bar.

iOS honors a focus in `touchstart`. With the reset armed on the first touch as
well as on a tap, a run left short by the reproduction above was reset by the
touch that began a scroll, at 1880 ms, and was whole 92 ms later. Whether a
keyboard showed was not recorded.

Scrolling past the reader's place by the shortfall and back up to it puts the
short visual viewport at the top of the layout viewport, where the header and
the bar are drawn at the screen's edges. Measured on one run of the
reproduction, reached that time through the home screen and the app switcher
rather than straight from eyeball to the terminal. The page loaded, fell to 576
at 235 ms with `offsetTop` 318, and was then scrolled past and back, first with
both scrolls in one task, then again with the second scroll a frame after the
first. Before any touch the readout showed 576 tall, `offsetTop` 0, the place
unchanged at 14617, and the bar at the bottom edge. A read one frame after each
try still said 318, so iOS applies a page's scroll later than the next frame,
and which try did the work is not known. The first touch then reset the
viewport, at 15.8 seconds.

At the very end of a review the scroll past and back did not reach 0. With the
reader's place 68 short of the furthest a whole viewport scrolls, the page
scrolled 17182, 17500, 17250, 17182 and stopped with `offsetTop` at 250. Nothing
in the page scrolls to 17250, which is exactly that furthest point, so iOS
appears to stop there on the way up. Bringing the place back to that point
first, and a third try by way of the very bottom with a wait between, changed
nothing. The first touch put it right each time.

Opening a different review probably cleared it through this branch's own fix,
not through the navigation. The fix was armed each time, and the tap on the link
to the other review is the tap it waits for. The readout cannot show the reset,
because the page that ran it was replaced. Inferred, not checked.

The tap reaches the page through the service worker, which navigates the window
already open with `navigate` (`openReview` in `web/ts/sw.ts`). That navigation
starts while iOS brings the app forward. Which part the fault needs, the
navigation, the notification, or the minute away, is not yet known.

### The code on this branch

- `web/ts/viewport.ts` is the fix. It counts the fault when the visual viewport
  is more than 100 points short of the window, nothing that takes typing is
  focused, and the page is not zoomed, and not within a second of a field losing
  focus. It then waits for the first touch or tap and focuses and blurs a hidden
  16px field inside it, at most twice per return to the foreground, with three
  seconds between tries for the viewport to settle. Until that touch, each time
  it finds the fault with `offsetTop` above 0, at most four times, it scrolls
  past the reader's place by the shortfall and back, once in one task and again
  across two frames. It checks on each viewport resize, each scroll, on load,
  and on `visibilitychange` and `pageshow`.
- `TestServer_Surface_browserViewport`, `…Realign` and `…Touch` make the
  viewport report the fault in Chromium, which has none, and count the hidden
  field arriving and the page's scrolls.
- The daemon on the owner's machine runs this branch's build. Test reviews in a
  scratch repository are open for checking.

### What is not solved

- Why iOS never tells the web view that the other app's keyboard went away when
  the app is opened from a notification. It happens inside iOS, between the
  other app and the system, and only Apple can answer it. A report to Apple can
  give the reproduction above.
- Why some returns cleared by themselves within a second, and one in Notes never
  stuck at all.
- The end of a review before the first touch, under "What the trials showed".
- Opening the review a notification names, which failed from Notes and from the
  terminal later in the day. It is in specs/TODO.md.

Rejected along the way, with the measurement that settled each:

- `interactive-widget=overlays-content`, which iOS 26.6.2 ignores.
- Laying out against the shortfall through the sticky offsets, which iOS drew
  off the screen.
- A `position: fixed` bar placed from `visualViewport` by script. Not tried. It
  would follow the visual viewport by design, the opposite of what is wanted,
  and the sticky offsets showed that where the page places an element in this
  state is not where iOS draws it.
