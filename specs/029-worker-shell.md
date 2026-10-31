---
status: active
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -count=1 -tags browser ./internal/server
---

# 029 The worker keeps the shell, and a tap moves the open app

The service worker keeps the React app's shell, so the installed app opens when
the daemon cannot answer, and a notification tap moves the open app to the
review instead of reloading it. The design is in
[docs/architecture.md](../docs/architecture.md) under The live connection, which
this spec's first commit corrects. Today the worker intercepts no fetches, so a
relaunch with the daemon down shows a black screen, measured on the phone on
2026-10-30.

**The daemon is asked for the shell first, and the cache answers when it
cannot.** The owner chose this on 2026-10-30 over serving the shell from the
cache first, which architecture.md said until now. The daemon's build changes
with every binary, so a cache-first shell would run the old shell against the
new daemon on the first launch after every rebuild, and the update notice would
show each time. Asking first costs one round trip for a small page on each
launch. The one-second timeout before the cache answers is a guess.

**Premises probed on 2026-10-30**, in Chromium 156 and the system's WebKitGTK
2.54.1, with a scratch server sending the surface's headers. Playwright's WebKit
build does not run on this machine: Playwright 1.64.1 wants WebKit revision
2370, the installed 2092 needs ICU 66, and Arch has 78. iOS is tested only by
the phone in the last iteration.

- `cache.addAll` keeps responses sent with `Cache-Control: no-store`, and later
  loads are served from the cache.
- With the server killed, a cached `/` drew. The stream stayed in CONNECTING and
  retried every 3 s, and `navigator.onLine` stayed true. A page the worker does
  not answer failed with a refused connection in 4 ms in Chromium.
- A daemon restarted between the fetch of `sw.js` and the fill of the cache left
  the cache named for the old build holding the new build's files. A check
  during install that fails on a mismatch left the old worker in control.
- A reload timed to land as a new worker installed drew the old HTML with the
  new script and stylesheet, in 5 of 27 trials in Chromium and 4 of 37 in
  WebKitGTK. Addressing each file by build is the fix architecture.md now
  describes.

**An address carries a token of the build, not the build.** The build string
holds a space and an `@`, as in `d763c54… 66398103@1793371291`, so the head
would escape it in a link, and the worker would have to build the same escaped
address from its list for the cache to match. A short hex hash of the build is
the same in both, and also names the cache. The page's `eyeball-build` meta and
the stream's `build` event keep the build itself.

**Reads need no token on the loopback listener** (`LoopbackSurface`), so the
install's fetches pass both listeners, and the 409 for another build's fetch
applies on both.

**The tap posts the address to the window and waits for an answer.**
specs/TODO.md held the reasoning: a page answered on a `MessageChannel` in under
a millisecond in headless Chromium on 2026-10-29, and a page with no listener
gave nothing until the timeout. Spec 026 stays blocked. This change may remove
the fault it waits for, and the beacons still show which branch ran if it comes
back.

## Review

Spec review pass: requested, because the worker's list of files, the addresses
the head writes and the daemon's refusal of another build's fetch must agree,
and a mismatch shows only as a blank launch on the phone. End-of-branch review
pass: declined, because the review agent reads each iteration and the browser
tests below fail with each mechanism removed.

## Out of scope

- Caching data. architecture.md says why under The live connection.
- A template page with the daemon down. The worker does not answer it, so it
  fails as it does today, until its screen moves to React.
- Showing that the page is refreshing when the app comes forward, already in
  specs/TODO.md.
- The fix 026 waits on. It stays blocked until a tap opens the wrong review.

## Iteration 1: The tearout

```punchcard
required_acks:
  - tearout-approved
```

The owner chose on 2026-10-30 to see this state before the worker is built. It
existed before only as a flash before the first view arrived.

- [x] The queue's state before its first view is a component of its own, which
      `QueueRoute` draws and a story can.
- [x] The queue sheet shows it twice, light and dark: with the offline notice,
      as a launch with the daemon down draws it, and without, as the moment
      before the view arrives.
- [x] The owner approves the sheet on the phone.

> **Completed** 2026-10-31 01:48 UTC
>
> - acks: tearout-approved
> - `make check` — 14.5s
> - `go test -race ./...` — 45.5s
> - `go test -count=1 -tags browser ./internal/server` — 50.1s

## Iteration 2: The shell cache

- [x] The head writes the build's token into the address of each script and
      stylesheet it links, for the shell and the template pages, and the daemon
      ignores it.
- [x] `sw.js` is served with the daemon's build and the list of the shell's
      files written in: `/`, the bundle, both stylesheets, every font, the icons
      and the manifest. The worker's stamp still hashes its code alone.
- [x] A Go test fails when an address the shell's head links, or a `url()` in
      `app.css`, is missing from the list written into `sw.js`.
- [x] A surface request whose `Eyeball-Build` header names another build's token
      is answered 409, on both listeners. A Go test fails when that request is
      answered 200.
- [x] The install fetches each file with that header into a cache named by the
      build's token. Activation deletes every other build's cache.
- [x] A navigation to `/` goes to the daemon and falls back to the cache on a
      failure or after one second. A shell file is answered from any cache
      holding its exact address. The worker answers nothing else.
- [x] Browser test: with the daemon stopped, a reload of `/` draws the state
      from Iteration 1 with the offline notice. It fails with the fetch handler
      removed.
- [x] Browser test: the daemon started again, that page draws the queue with no
      reload. The stream's stamp on reconnect differs from the empty one the
      page holds, which refetches the view.
- [x] Browser test: a template page's address gets the template, not the shell,
      with the worker in control.
- [x] Browser test: a daemon restarted on the same address with another build
      serves its build on the next reload, with no second reload. It fails with
      the shell answered from the cache first.
- [x] Browser test: an install whose fetch is answered 409 leaves the old worker
      in control and no cache for the new build.
- [x] The comment atop `web/ts/sw.ts` says what the worker answers.

> **Completed** 2026-10-31 01:57 UTC
>
> - `make check` — 14.0s
> - `go test -race ./...` — 45.9s
> - `go test -count=1 -tags browser ./internal/server` — 61.8s

## Iteration 3: The tap

The React router draws only `/` until the review screen moves, and every
notification names a review. So in this spec the React window answers each tap
by setting `location`, which is a full load, and the router's branch is first
used by the spec that moves the review screen.

- [x] Both bundles answer the worker's message on its port. The React app moves
      its router to an address it draws and sets `location` for any other. A
      template page sets `location`.
- [x] `openReview` posts the address to the window it finds, waits up to one
      second, and calls `focus()` on an answer. With no answer it calls
      `navigate`, then `openWindow`, as today.
- [x] The beacons log how long the answer took and whether `navigate` ran.
- [x] Browser test: the React window on the queue answers a tap naming a review,
      loads that review itself, and `navigate` does not run. It fails with the
      old `openReview` restored, which calls `navigate`.
- [x] Browser test: a window with no listener gets `navigate` after the wait.

> **Completed** 2026-10-31 02:09 UTC
>
> - `make check` — 31.2s
> - `go test -race ./...` — 53.1s
> - `go test -count=1 -tags browser ./internal/server` — 67.0s

## Iteration 4: The connecting state

```punchcard
required_acks:
  - connecting-approved
```

Added on 2026-10-30, during the phone check below. With the daemon stopped, the
relaunched app drew the state from Iteration 1 with no offline notice, and the
owner asked for a different state before the first view: the standard
background, a large eye in the middle of the screen that blinks after a random
delay of one to five seconds, each time, and the words "Eyeball is connecting…"
in small type below it. On the sheet the owner then asked for the app's own eye,
with its lashes when shut and a 0 as its pupil, a blink that moves as the bar's
does, and a shut eye held for a random time. The hold's range, 0.2 to 1 s, is a
guess.

The notices leave this state. The phone cannot tell a daemon that is down from
one that is slow, since a tailnet node that is offline drops packets and nothing
refuses the connection, and "connecting" is true of both. The first view
replaces this state, and any notice due then draws with the queue. A stream that
never connects showing no notice anywhere is recorded in specs/TODO.md.

- [x] `QueueLoading` draws the large eye centered on the background, with the
      words below it, and no bar or notices.
- [x] The eye blinks after a random delay of one to five seconds, drawn afresh
      for each blink, and does not blink when the reader asks for reduced
      motion.
- [x] Each blink holds the shut eye for a random time from 0.2 to 1 s, drawn
      afresh for each blink.
- [x] The browser test from Iteration 2 that reloads `/` with the daemon stopped
      expects the connecting state, and still fails with the fetch handler
      removed.
- [x] The queue's stories and the queue sheet show the state light and dark, and
      the offline variant is gone.
- [x] The owner approves the sheet on the phone.

> **Completed** 2026-10-31 06:40 UTC
>
> - acks: connecting-approved
> - `make check` — 14.3s
> - `go test -race ./...` — 45.6s
> - `go test -count=1 -tags browser ./internal/server` — 64.4s

## Iteration 5: On the phone

```punchcard
required_acks:
  - phone-checked
```

Worked by the orchestrator, with the daemon running this branch's build. Each
result is written here as it arrives.

**Daemon stopped, app relaunched, 2026-10-30.** The cached shell drew the
queue's state before its first view, with no offline notice. The notice waits
for the stream's `error` event, and none came. With the daemon's tailnet node
offline, a request to its address from the desktop had no answer after 30 s: the
packets are dropped, and nothing refuses the connection. The refused connection
in 4 ms in the probes was on a local address. The shell drew anyway because the
worker answers a navigation from the cache after one second. With the daemon
started again and the app left open, the queue drew on its own. The owner then
asked for the state in Iteration 4.

- [ ] With the daemon stopped, the installed app relaunched draws the connecting
      state from Iteration 4, and the queue once the daemon is back.
- [ ] A daemon rebuilt while the app was closed: the next launch opens the new
      build with no update notice.
- [ ] A page open across a rebuild shows the update notice, and one Refresh
      reaches the new build.
- [ ] Taps over Notes and over the terminal open the named review, and the
      beacons show that the page answered.
