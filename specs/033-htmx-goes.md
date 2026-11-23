---
status: ready
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
error page is drawn for four failures: an address the mux has no route for (404
and 405), a raw file past the send limit (413), a handler's 500, and a write
outside `/api` without the token (403). A person sees it only when the browser
loads the address as a page: a reload, a typed address, or a raw file opened in
a tab of its own. The same failures also reach an `<img>`, a frame, a `fetch`
and curl, where a page is the wrong answer.

The browser says which it is. A page load sends `Sec-Fetch-Dest: document`, and
an image, a frame and a fetch send `image`, `iframe` and `empty`. Measured in
headless Chromium on 2026-11-22. A frame sends the same `Accept: text/html`
header as a page, so `Accept` cannot tell the two apart. iOS Safari is assumed,
not measured, to send the header, as WebKit has since Safari 16.4. A browser
that sends no header gets the text answer, which is correct and plain.

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
by that exact address until the new worker takes over. No alias for
`react.js` is kept. Reasoned from `shellFiles` and the worker, not measured, so
the phone iteration checks it.

## Review

Spec review pass: declined, because the design is one header test and a meta
tag, and the probes above settled its one premise that can be checked here.
End-of-branch review pass: requested, because the branch deletes code across
the server, the bundles and the docs, and a pass is what finds prose that still
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

## Iteration 1: The error screen

- [ ] An `ErrorScreen` component draws a heading and a sentence in the top bar's
      "Queue" back link layout that `NotFoundScreen` uses, with the eye from a
      waiting count that may be null.
- [ ] A sheet story shows it light beside dark for the 404, 405, 413 and 500
      sentences the server sends, with a story per state.
- [ ] The story run fails when the heading or sentence is not drawn, checked by
      breaking the component once and reverting.

## Iteration 2: A failure answers by who asked

- [ ] One function answers every failure the error page answered today: the
      shell with an `eyeball-error` meta tag when `Sec-Fetch-Dest` is
      `document`, and `text/plain` with the status otherwise.
- [ ] The tag holds JSON of a type in `internal/view`, so `make api` generates
      its TypeScript type and the app reads it with no hand-written shape.
- [ ] Go tests cover each of the four failures with `document`, with `image`,
      with `empty` and with no header, asserting the status, content type and
      body.
- [ ] The app draws `ErrorScreen` when the tag is present, before the router,
      and the router is untouched when it is absent. A Vitest test covers both.
- [ ] A browser test loads an unknown path in Chromium and finds the error
      screen with a 404 status, and loads a too-large raw file in an `<img>`
      and finds a 413 with a plain body.

## Iteration 3: htmx and the template bundle go

- [ ] `web/templates/error.html`, `file.html` and the `layout` template are
      deleted, with `renderPage` and every caller moved to the function from
      Iteration 2. `layout.html` keeps `headmeta` and `shell` only.
- [ ] The `HX-Request` branch of `refuseWithoutToken`, its test case, and the
      htmx configuration test in `surface_test.go` are gone.
- [ ] `/static/htmx.js`, `eyeball.Htmx`, `Test_htmxIsTheVendoredRelease` and
      `web/vendor/` are gone, with the `web/vendor/` ignore patterns in
      `.oxfmtrc.json`, `.oxlintrc.json` and the Makefile.
- [ ] `web/ts/main.ts`, `web/ts/stream.ts`, the `htmx` declaration in
      `web/ts/globals.d.ts` and `eyeball.Bundle` are gone. Every other `web/ts`
      module stays, since the app imports it.
- [ ] The React bundle is served at `/static/app.js`, in the route, the shell's
      head and `shellFiles`, and `/static/react.js` answers 404.
- [ ] `render.Join` loses its `after` hook, and the three tests that passed it
      assert the same outlines without it.
- [ ] `git grep -i htmx` matches only this spec, finished specs, the fixtures
      named in Out of scope, and architecture.md's rejected alternative.
- [ ] architecture.md's routes table and the paragraphs on the screens' move,
      the two bundles and `web/vendor/`, the import-order example in
      conventions.md, and the comments in `assets.go` describe what is left.

## Iteration 4: On the phone

```punchcard
required_acks:
  - phone-checked
```

- [ ] The owner opens an unknown address in the installed app and in Safari,
      and sees the error screen in the device's theme. If it shows plain text
      instead, iOS sends no `Sec-Fetch-Dest`, and the spec records that here.
- [ ] The owner opens the installed app first after the deploy that renames the
      bundle, and the app loads, then loads again after the update banner's
      Refresh.
