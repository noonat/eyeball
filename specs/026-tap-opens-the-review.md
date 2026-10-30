---
status: blocked
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -count=1 -tags browser ./internal/server
---

# 026 A tap opens the review it names

**Blocked, 2026-10-29, until the fault is seen again.** The beacon from
Iteration 1 is merged, and six taps on the phone could not reproduce the fault
(Iteration 2, "Results"). There is no cause to fix, so Iterations 3 and 4 wait.
The owner reports the next tap that opens the wrong review, and the beacons it
left in `eyeball.log` say which branch of `openReview` ran. Iteration 2 then
closes on those beacons, its terminal todo with one run instead of two, and the
fix is written from them. The owner chose this over keeping the branch open
until the fault recurred, which would have held an approval that any other merge
cancels.

A notification tapped over Notes brought the installed app forward on the review
already open, twice, with no page load. Tapped over the terminal app, the page
loaded fresh by navigation, and later in the same day a tap from the terminal
also failed to show the named review. Both are recorded in 020 under "What the
trials showed". product.md says a notification is deep linked to the review and
opening it lands there, under Notification, so this is a defect.

The tap lands in `openReview` in `web/ts/sw.ts`, which has three branches: focus
a window already on the review, `navigate` the first window, or `openWindow`.
The comment above it says `openWindow` on iOS brings the one window forward
without loading the address. So "forward on the old review, no page load" fits
two paths equally: `navigate` rejected or resolved `null` and the code fell
through to `openWindow`, or `matchAll` returned no window and the code went
there directly. Commit `2221191` already fixed this tap once, and the fault is
seen only on the owner's phone. So this spec measures which branch runs before
it fixes anything, as the dev-workflow rule on two failed fixes asks.

One path is known to exist. `matchAll` passes `includeUncontrolled: true`, so
the first window can be one this worker does not control. Measured in headless
Chromium 2026-10-29: `navigate` on such a window rejects with "TypeError: This
service worker is not the client's active service worker.", and `postMessage` to
the same window arrives. Whether an eyeball window is ever uncontrolled on iOS
is not known. The worker claims every window when it activates.

**The worker reports each tap to the daemon.** A beacon is a `POST` from the
worker to a new surface route that writes one line to `eyeball.log`. Every
request passes Go's cross-origin check, which a same-origin worker fetch
satisfies, and the tailnet listener needs no token. The alternative, a readout
on the page as 020 used, was rejected because the page that would show it is the
one the tap fails to replace or replaces too early. The beacon stays after the
fix. It costs one route and a few log lines per tap, and it answers the next
question about a tap without a new instrument. Removing it before merge was
rejected, since it saves one route and leaves the next fault with nothing to
read. architecture.md says the worker handles push and taps, and it gains one
sentence naming the beacon.

## Review

Spec review pass: declined, because the spec adds one logging route and changes
one function, and the premise it rests on was probed in Chromium. End-of-branch
review pass: declined, because the review agent reads each iteration and the
phone check in the last iteration is the test that matters.

## Out of scope

- The keyboard fault 020 left open. It is a separate mechanism inside iOS.
- Showing that the page is refreshing when the app comes forward, which is its
  own TODO item.
- Which review a tap opens when the named review has been decided. It opens that
  review, as today.
- Rotating `eyeball.log`, already in specs/TODO.md. The beacon adds to it.

## Iteration 1: The beacon

- [x] A surface route, `POST /taps`, writes the JSON body it receives as one
      `eyeball.log` line at info level and answers 204. A body over 8 KB or not
      JSON is refused with 400 and not logged.
- [x] A Go test fails when the route logs nothing, logs a body it should refuse,
      or answers anything but 204 to a good body.
- [x] `openReview` sends a beacon when it starts and after each step, each
      inside `event.waitUntil`, so one step that never settles leaves the
      earlier beacons logged. A failed beacon never stops the tap.
- [x] The first beacon holds the notification's url and every window from two
      `matchAll` calls, with and without `includeUncontrolled`, each with its
      url, `visibilityState` and `focused`.
- [x] Later beacons name the branch taken and what `focus`, `navigate` and
      `openWindow` resolved to: a url, `null`, or the error's name and message.
- [x] Every beacon carries a stamp of the worker's build, and the daemon logs
      beside it the stamp of the `sw.js` it serves now, so a stale worker shows
      as two stamps that differ.
- [x] architecture.md, where it describes the service worker, names the beacon
      and why it stays.

> **Completed** 2026-10-30 00:46 UTC
>
> - `make check` — 6.2s
> - `go test -race ./...` — 3.8s
> - `go test -count=1 -tags browser ./internal/server` — 49.0s

## Iteration 2: Taps on the phone

```punchcard
required_acks:
  - taps-read
```

Worked by the orchestrator directly, since only the owner's phone shows the
fault. The daemon runs this branch's build. Before the first run, one beacon
sent from the phone is read in `eyeball.log`, since a beacon that never arrives
looks the same as a worker that never ran. Each result is written below as it
arrives.

- [x] Two taps over Notes with its keyboard up, each recording the beacons and
      which review the screen showed after the tap.
- [ ] Two taps over the terminal app, recorded the same way.
- [x] One tap with eyeball itself in front on a different review, as a control.
- [x] The branch each run took and why, written here, with the cause named or
      the runs that left it open.

### Results

The daemon ran `4dc4d87`, and the worker's stamp matched the served one,
`d6704ce669e6`, in every run below.

Opening the app from the queue after the deploy sent no beacon, as expected with
no tap.

Control, eyeball in front on review 166, tap on 167's notification: one window,
on 166, visible, focused and controlled. `navigate` resolved to `/reviews/167`
in 121 ms and the screen showed 167.

Run 1, eyeball left on 167, Notes in front with its keyboard up for under a
minute, tap on 169's notification: one window, on 167, controlled, and reported
visible and focused although Notes was in front. `navigate` resolved to
`/reviews/169` in 90 ms and the screen showed 169. The fault did not reproduce.

Run 2, the same with three minutes in Notes before the tap on 170's
notification: the same one controlled window, on 169. `navigate` resolved to
`/reviews/170` in 103 ms and the screen showed 170. The fault did not reproduce.
Every review open so far had one file of two lines, where the failed taps in 020
had a real branch review open. The next runs use long reviews, at the owner's
suggestion.

Run 3, eyeball in front on 171, 14 files and 11,713 lines, scrolled partway
down, tap on 172's notification, 26 files and 4,141 lines: one controlled
window, on 171. `navigate` resolved to `/reviews/172` in 259 ms and the screen
showed 172.

Run 4, Notes in front with its keyboard up for about two and a half minutes, tap
on 173's notification, the same 14 files as 171: one controlled window, reported
on `/reviews/171` though the owner had last been on 172. `navigate` resolved to
`/reviews/173` in 507 ms and the screen showed 173. The fault did not reproduce.
The window was on 171 because the owner had decided 172, which returns to the
queue, and opened 171 from there. So the window's url was right.

Run 5, the terminal app in front with its keyboard up for a few minutes, tap on
174's notification, 10,042 lines: one controlled window, on 173. `navigate`
resolved to `/reviews/174` in 549 ms and the screen showed 174. The fault did
not reproduce.

Over six taps, every one found one window this worker controls, and `navigate`
reached the named review. No run left a window uncontrolled or found none, so
neither suspect above has been seen. The longest time away was about three
minutes, and the worker had been installed minutes before the first run. The
failed taps in 020 came after hours of real use with a worker days old.

## Iteration 3: The fix

Written from iteration 2's results before this iteration starts. The spec is
corrected then and the correction recorded here. Candidates, each tied to what
the phone may show:

- Uncontrolled window: call `navigate` only on a controlled window, or post the
  url to the window and let the page set `location.href` itself. The Chromium
  probe above shows `postMessage` reaches an uncontrolled window.
- No window returned: the worker keeps the url, and the page asks for it when it
  becomes visible.
- `navigate` resolves on a controlled window and the screen still shows the old
  review: not explained by anything above, and the iteration is a second round
  of measurement.

- [ ] The fix the results chose, in `web/ts/sw.ts` and the page if it needs one.
- [ ] A browser test reproduces the cause in Chromium where Chromium can, and
      fails with the old `openReview` restored.

## Iteration 4: On the phone

```punchcard
required_acks:
  - tap-opens-review
```

- [ ] Taps over Notes and over the terminal each open the review the
      notification names, with the beacons showing the branch the fix expects.
