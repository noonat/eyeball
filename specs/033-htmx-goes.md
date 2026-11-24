---
status: done
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -count=1 -tags browser ./internal/server
---

# 033 htmx and the error page go

Every screen is the React app's since spec [032](done/032-react-review.md). What
is left of the template surface is the error page, the layout that wraps it,
htmx, and the template bundle at `/static/app.js` that the error page loads.
This spec removes them, and the React bundle takes the address `/static/app.js`,
as [architecture.md](../docs/architecture.md) says under "Until the last
template goes". The shell stays a template, and is the only one left.

**A failure answers the person with the app and anything else with text.** The
error page is drawn for five failures: an address the mux has no route for
(404), a method an address does not answer (405), a raw file past the send limit
(413), a handler's 500, and a write outside `/api` without the token (403). A
person sees it only when the browser loads the address as a page: a reload, a
typed address, or a raw file opened in a tab of its own. The same failures also
reach an `<img>`, a frame, a `fetch` and curl, where a page is the wrong answer.

The browser says which it is. A page load sends `Sec-Fetch-Dest: document`, and
an image, a frame and a fetch send `image`, `iframe` and `empty`. Measured in
headless Chromium on 2026-11-22. A frame sends the same `Accept: text/html`
header as a page, so `Accept` cannot tell the two apart. iOS Safari sends the
header too. Measured on an iPhone on 2026-11-23. A browser that sends no header
gets the text answer, which is correct and plain.

So a failure with `Sec-Fetch-Dest: document` answers the shell, with the
failure's status, and the shell carries the failure in a
`<meta name="eyeball-error">` tag: its heading, its sentence and the waiting
count for the eye. The app reads the tag before it routes, and draws an error
screen in place of any route. Everything else answers `text/plain` with the
status and the sentence. The tag is needed because a raw file's address has no
route in the app, so the app alone would draw "Nothing here" for a file that
exists and is too large.

Two alternatives lost. A scriptless template page keeps an HTML page for every
client, but the bundle is what sets the theme, so it shows light on a dark
phone, and a browser loading it in a frame gets a page inside a card. Answering
every failure with the shell sends a frame and curl an app they cannot use.

**The worker's cached shell does not change how this works.** The worker answers
a navigation from the network and falls back to the cached shell only for `/`
when the daemon cannot answer, so the cached copy never carries an error tag.

**The rename relies on the worker's exact-match cache.** An installed phone's
old shell asks for `/static/react.js?b=<old token>`, which its old worker holds
by that exact address until the new worker takes over. No alias for `react.js`
is kept. Reasoned from `shellFiles` and the worker, not measured, so the phone
iteration checks it.

## Review

Spec review pass: declined, because the design is one header test and a meta
tag, and the probes above settled its one premise that can be checked here.
End-of-branch review pass: requested, because the branch deletes code across the
server, the bundles and the docs, and a pass is what finds prose that still
describes a removed route.

## Out of scope

- Porting the markup checks in `internal/convention` from the static sheets to
  the rendered stories, deleting the sheets in `docs/design/`, and moving
  `shell.css` into the bundled stylesheet. `shell.css` sets a height on `body`
  that would change how the sheets lay out, so it waits for them. That is the
  next spec, listed in [TODO.md](TODO.md).
- The frozen template markup in `web/src/fixtures/review-*.html`, which still
  holds `hx-` attributes. Spec 032 kept the comparisons that read them, and the
  next spec decides their fate with the other checks.
- `internal/viewdoc` and the Go tests that read old class names, an unscheduled
  item of its own.
- The phrase "server-rendered `html/template` with htmx" in architecture.md's
  record of the rejected alternative, which records a decision and stays.

## Departures

Where the work went differently from the paragraphs above.

- `AppLink` renders a plain anchor outside a router. The failure branch mounts
  no router, and the error screen's "Queue" link, as a router `Link`, threw
  because it had no router context to read. A plain anchor is also right there,
  since the link has to load the queue and there is no router to move.
- `TestServer_Surface_htmxConfig` was deleted in Iteration 2, not 3. It read the
  htmx configuration meta through the error page, which no longer renders the
  layout, so it could not pass.
- `renderPage` and the `message` type were deleted in Iteration 2. Once `fail`
  replaced every caller, staticcheck flagged both as unused. Iteration 3's todo
  about `renderPage` is partly done. `error.html`, `file.html` and the `layout`
  template remain for it.
- `pageText` was removed, as nothing reads an error page as HTML any more.
  `TestServer_Surface_browserShellTemplatePage` became
  `TestServer_Surface_browserShellFailurePage`, which asserts the shell carrying
  the failure with the worker in control. The surface tests that read an error
  page use `loadPage` to send `Sec-Fetch-Dest: document` and `failureTag` to
  read the tag back.
- The `/../` cases of `TestServer_Surface_fileNotFound` check the shell's tag
  through `loadPage`. They no longer assert that no notice banner is drawn,
  which does not apply to a failure.
- `TestServer_LoopbackSurface` and its `apiRefusal` test expect `text/plain` for
  a refused write that sends no `Sec-Fetch-Dest`. A native form post sends
  `document` and gets the shell.
- The Iteration 2 todo for the Go tests says four failures. The tests cover the
  five paths, the 404 and the 405 separately, each as a page, an image, a fetch,
  a frame and with no header.
- The `page` type keeps its `Back`, `BackLabel` and `Body` fields, which the
  view builders still read, and loses only `Eye` and the `eye` type. The spec
  did not name them, and removing `renderPage` left them unused.
- `TestServer_Surface_templateBundle` was deleted. It held the template bundle
  at `/static/app.js` to omit the review page's scripts, which says nothing of
  the React bundle that now has that address.
- `TestBundle`, `build` and the `bundleOnce` state in `bundle.go` went with
  `eyeball.Bundle`. Only `buildWith` remains, for the worker.
- No test asserts that `/static/htmx.js` answers 404. The spec's last todo for
  this iteration requires `git grep -i htmx` to match nothing else, and that
  assertion would have been one more match. The route is gone from the table.
- `docs/design/review.html` named htmx in two lines of a to-do item. They now
  say "the template script", since the spec's grep allows no other match in
  `docs/design/`. `specs/TODO.md` still names htmx in the item for this spec,
  which leaves with the spec when it moves to `specs/done/`.
- `render.Join`'s three tests lose their `after` assertions. The two fold tests
  still fail when the fold's closing tag or a passage is not written. `TestJoin`
  checks the passages are written in order and with no `aside`, and fails when
  none is written, but a fold fault alone cannot fail it, since its input has no
  unchanged run.
- `/static/app.js` is served gzipped, as `/static/react.js` was, since the
  bundle is the React one.
- Iteration 4 ran on 2026-11-23 against this branch's build, deployed as the
  running daemon. An unknown address opened in Safari on the iPhone drew the
  error screen, so iOS Safari sends `Sec-Fetch-Dest: document`. The installed
  app has no address bar, so only the Safari half of the first todo was done.
  The app uses the same WebKit, so the result is assumed to hold there, not
  measured.
- The installed app, opened first after the deploy, loaded, and no update banner
  appeared, so the Refresh half of the second todo did not run. A cold launch is
  expected to show no banner. The worker answers `/` from the network when the
  daemon answers in time, so the phone is expected to have got the new shell,
  which names `/static/app.js`, and the banner shows only on a page already open
  when the build changes. Both are reasoned from the worker, not observed. The
  path where an old shell asks the old worker's cache for `/static/react.js` did
  not run, and stays reasoned. A launch with the daemon unreachable is the
  expected way to reach it.
- Measured from the desktop against the deployed daemon, `/no/such/page`
  answered 404 as `text/plain` with no header, and as the shell with the
  `eyeball-error` tag with `Sec-Fetch-Dest: document`. `/static/react.js`
  answered 404.
- The binary `seed` at the repository root is tracked and holds the string htmx
  from an older build. `git grep -i htmx` reports it as a binary match, and this
  spec leaves it. That one binary breaks the todo's list of allowed matches.
  Removing it is the unscheduled item in `specs/TODO.md`.

## Iteration 1: The error screen

- [x] An `ErrorScreen` component draws a heading and a sentence in the top bar's
      "Queue" back link layout that `NotFoundScreen` uses, with the eye from a
      waiting count that may be null.
- [x] A sheet story shows it light beside dark for the 404, 405, 413 and 500
      sentences the server sends, with a story per state.
- [x] The story run fails when the heading or sentence is not drawn, checked by
      breaking the component once and reverting.

> **Completed** 2026-11-23 02:34 UTC
>
> - `make check` — 21.0s
> - `go test -race ./...` — 54.6s
> - `go test -count=1 -tags browser ./internal/server` — 126.3s

## Iteration 2: A failure answers by who asked

- [x] One function answers every failure the error page answered today: the
      shell with an `eyeball-error` meta tag when `Sec-Fetch-Dest` is
      `document`, and `text/plain` with the status otherwise.
- [x] The tag holds JSON of a type in `internal/view`, so `make api` generates
      its TypeScript type and the app reads it with no hand-written shape.
- [x] Go tests cover each of the four failures with `document`, with `image`,
      with `empty` and with no header, asserting the status, content type and
      body.
- [x] The app draws `ErrorScreen` when the tag is present, before the router,
      and the router is untouched when it is absent. A Vitest test covers both.
- [x] A browser test loads an unknown path in Chromium and finds the error
      screen with a 404 status, and loads a too-large raw file in an `<img>` and
      finds a 413 with a plain body.

> **Completed** 2026-11-23 02:59 UTC
>
> - `make check` — 37.1s
> - `go test -race ./...` — 53.6s
> - `go test -count=1 -tags browser ./internal/server` — 126.6s

## Iteration 3: htmx and the template bundle go

- [x] `web/templates/error.html`, `file.html` and the `layout` template are
      deleted, with `renderPage` and every caller moved to the function from
      Iteration 2. `layout.html` keeps `headmeta` and `shell` only.
- [x] The `HX-Request` branch of `refuseWithoutToken`, its test case, and the
      htmx configuration test in `surface_test.go` are gone.
- [x] `/static/htmx.js`, `eyeball.Htmx`, `Test_htmxIsTheVendoredRelease` and
      `web/vendor/` are gone, with the `web/vendor/` ignore patterns in
      `.oxfmtrc.json`, `.oxlintrc.json` and the Makefile.
- [x] `web/ts/main.ts`, `web/ts/stream.ts`, the `htmx` declaration in
      `web/ts/globals.d.ts` and `eyeball.Bundle` are gone. Every other `web/ts`
      module stays, since the app imports it.
- [x] The React bundle is served at `/static/app.js`, in the route, the shell's
      head and `shellFiles`, and `/static/react.js` answers 404.
- [x] `render.Join` loses its `after` hook, and the three tests that passed it
      assert the same outlines without it.
- [x] `git grep -i htmx` matches only this spec, finished specs, the fixtures
      named in Out of scope, and architecture.md's rejected alternative.
- [x] architecture.md's routes table and the paragraphs on the screens' move,
      the two bundles and `web/vendor/`, the import-order example in
      conventions.md, and the comments in `assets.go` describe what is left.

> **Completed** 2026-11-23 03:11 UTC
>
> - `make check` — 38.7s
> - `go test -race ./...` — 55.4s
> - `go test -count=1 -tags browser ./internal/server` — 126.1s

## Iteration 4: On the phone

```punchcard
required_acks:
  - phone-checked
```

- [x] The owner opens an unknown address in the installed app and in Safari, and
      sees the error screen in the device's theme. If it shows plain text
      instead, iOS sends no `Sec-Fetch-Dest`, and the spec records that here.
- [x] The owner opens the installed app first after the deploy that renames the
      bundle, and the app loads, then loads again after the update banner's
      Refresh.

> **Completed** 2026-11-24 03:17 UTC
>
> - acks: phone-checked
> - `make check` — 37.7s
> - `go test -race ./...` — 50.9s
> - `go test -count=1 -tags browser ./internal/server` — 126.3s
