---
status: active
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -count=1 -tags browser ./internal/server
---

# 028 The queue in React

The queue becomes the first screen the React app draws, end to end: a JSON view
of it from the server with TypeScript types generated from Go, fixtures from the
same view, the queue's components as Storybook stories judged on the phone, and
the shell and router that serve it at `/`. The design is in
[docs/architecture.md](../docs/architecture.md) under The surface and The client
code. The screen is the one approved in `docs/design/queue.html`, so the tearout
reproduces it from components rather than redesigning it.

**The shell cache and the tap handling move to the next spec** (the service
worker's shell, 029). This reverses the order in specs/TODO.md. With no cache,
the daemon answers every navigation, so the worker cannot hand a template page
the shell. With no change to the tap, a tap on the React window gets `navigate`,
which reloads the app on a controlled window. So neither problem of the mixed
period appears in this spec. The cache also has a failure only the installed app
can show, a shell older than the daemon, which needs its own phone check. This
spec is large without them.

**The view carries its own stamp.** `handleQueue` reads the queue and then
`queueStamp` reads it again in a second transaction (internal/server/queue.go),
so a change between the two reads is missed until the next. The view's stamp is
computed from the rows it holds, and the app refetches when the stream's stamp
differs from the view's.

**Behaviors the template pages still run stay in one copy.** Theme, the loopback
token, worker registration, push setup, the verdict's blink mark, the build
check and the viewport fix are framework-free modules in `web/ts`, and several
were settled on the phone. The React app calls them. Where a module mixes its
logic with template DOM, as push.ts does with `#allow` and `#lapsed`, the logic
is split into a function both callers use, and the template keeps its wiring.
Rewriting them in React was rejected: two copies would have to agree on storage
keys, such as the blink mark and the token, with nothing checking that they do.
`App()` then embeds and stages `web/ts` as well as `web/src`.

**The generator is written here, with the standard library.** A reflection walk
over the response structs, about 80 lines, wrote types that passed every strict
flag (measured 2026-10-29). Reflection cannot see constants, so it reads the API
package with `go/types` and the standard library's source importer, which also
gives a named string type with constants a union of its values. Assumed, not
probed. An external generator was not an option: `go run ...@latest` is refused
in this environment, and the project's rule is the standard library first. Go
sends a nil slice or map as `null`, so those are typed `| null`, and a planted
`q.waiting.length` then failed with "possibly 'null'". A Go kind with no exact
TypeScript form fails the generator rather than becoming a guess: interfaces,
integers that can pass 2^53, maps without string keys.

**Two bundles, two addresses, while screens are mixed.** The templates keep
`/static/app.js`. The React app is served at `/static/react.js` until the
cleanup spec. The shell links `/static/app.css` and `/static/shell.css`, the
files the templates link, since `app.css` stays one file through the mixed
period, so the React bundle imports no stylesheet.

**Rows link to template pages with plain anchors.** A review is still a template
page, so tapping a row is a full page load. React Router intercepts only its own
`Link`, so a plain `<a>` does exactly that.

## Review

Spec review pass: requested, because the view and its stamp, and the storage
keys the two bundles share, are pairs that must agree with nothing yet checking
them. End-of-branch review pass: declined, because the review agent reads each
iteration and the browser tests below fail with the old route restored.

## Out of scope

- The worker's shell cache and the tap handling, in the next spec (029).
- Every other screen. A review, a file, the tree and the project view stay
  templates. The queue's row partials stay, since project.html uses them.
- Ages that count up on screen. `render.Age` stays server-side, so an age is as
  old as the last refetch, as it is today.

## Departures

Where the work went differently from the paragraphs above.

- The views live in a new package, `internal/view`, where the spec says "the API
  package". `internal/api` is the agent API's `--json` contract, and the manual
  lists every field of it, so surface views there would mix two contracts.
- A review's id is an int64 sent with json's `,string` option and typed
  `string`, so it never passes through a double. Round and the waiting count are
  int32. A nil section is sent as `null` and typed `Row[] | null`.
- The design frame's "Not requested yet" working row cannot be seeded. A row is
  Working only once a round has an answer, so no store state draws it.
- The view's waiting rows come out longest wait first, as the store orders them.
  The template queue page draws the same order today. The frames in
  `docs/design/queue.html` draw newest first. Iteration 3 has to settle which
  order the component uses, and specs/TODO.md already lists the question.
- Iteration 3's first item says the components' markup equals queue.html's.
  `docs/design/queue.html` is the design frame, which draws newest first and
  includes the "Not requested yet" row, so equality with it was never possible.
  The components were compared with the template queue page's render for each
  seeded state, which `tool/fixtures` wrote as `web/src/fixtures/queue-*.html`.
  Iteration 4 removes the template page, so those files are now committed as
  they were, with a comment saying so, and each data story still compares its
  markup with them. The comparison no longer follows a live template, which was
  its whole purpose while both existed. What it holds now is the markup the
  owner approved on the phone, and a change to it is made by hand with the
  component. Rendering the row partials that project.html still uses would have
  compared the rows alone, and left the sections, the empty state and the top
  bar unchecked.
- That comparison left out `#firstrun` and `#lapsed`, since `QueueSetup` takes
  those states as a prop. It also left out `data-stream` and `data-stamp` on the
  page's column. This corrects an earlier note here, that iteration 4's shell
  has to carry them. The app opens its own stream and takes the stamp from the
  view, so the shell prints none. A stamp in the shell would be a second read of
  the queue, which is the gap the view's stamp closes.
- The client-only states (after a verdict, not installed, notifications not
  allowed and a lapsed subscription) are compared with nothing beyond the text
  the template and the frames give them.
- The sheet's stylesheet had a rule for its own headings, `.sheet-body h2`, that
  also matched the queue's section headings inside the phone frames. The frames
  showed about 56px above Working and Settled and a 19px heading, where the real
  screen draws 20px. The owner asked for the gap to be halved, and the cause was
  that rule. It is removed, and `.sheet-body h1` is now `.sheet-body > h1`,
  since the top bar's heading matched it too. The sheet's own story now fails
  when a heading in a frame computes a different margin, size or letter spacing
  than in a screen with no sheet around it.
- The owner's approval of the sheet settled two open questions. Waiting rows are
  longest wait first, the order the store and the template page use, and the
  "Not requested yet" row stays out. The question in specs/TODO.md is removed.
- The template pages' wiring for the first-run steps and the blink was removed
  with the queue's template page, since no template page holds that markup any
  more and nothing would exercise it. `push.ts` keeps the logic the app calls
  (`pushState`, `allowPush`, `resubscribePush`, `installed`, `setBadge`), and
  `blink.ts` keeps `takeBlinkMark` and `blinks`. A template page still clears a
  stale blink mark on load, so it cannot blink a later queue.
- The stream's connection logic moved to `web/ts/connection.ts`, with no page in
  it. The template pages wire it to `#offline` and `#update`, and the app wires
  it to state. `update.ts` keeps `pageBuild`, `newerBuild` and `refreshOnTap`.
- The React bundle imports no stylesheet, so `AppBundle` has no `Stylesheet` and
  the build fails if an import collects one. The shell links `app.css` and
  `shell.css`, as the template pages do.
- The queue's template tests were rewritten over `GET /api/`, since the page
  they parsed is gone. The shell has a test of its own.

## Iteration 1: The documents

- [x] architecture.md names the generator: the standard library over the API
      package, nil slices and maps as `| null`, a kind it cannot map an error.
- [x] architecture.md says the React bundle is at `/static/react.js` during the
      mixed period, and that shared behaviors stay in `web/ts` and are called.
- [x] specs/TODO.md moves the shell cache and tap handling into an item of their
      own after this one, with the worker spec's notes below.

The worker spec's notes, for that item: the cache's version has to follow the
app bundle, since the worker's stamp hashes only the worker and an app-only
change leaves `sw.js` unchanged; whether Cache Storage keeps a response sent
with `no-store` is not probed; the answering listener goes in the template
bundle too, so `navigate` runs only for a page that does not answer; the
one-second wait for an answer is a guess, and the beacons log how long the
answer took and whether the fallback ran; `focus()` stays after an answer, since
a page cannot bring itself forward. Measured 2026-10-29 in headless Chromium: a
page answered on a `MessageChannel` in under a millisecond, and a page with no
listener gave nothing until the worker's one-second timeout.

> **Completed** 2026-10-30 08:24 UTC
>
> - `make check` — 15.5s
> - `go test -race ./...` — 5.4s
> - `go test -count=1 -tags browser ./internal/server` — 48.2s

## Iteration 2: The API

- [x] `tool/apitypes` writes `web/src/api/types.ts` from the API package with
      `go/types`, and `make check` fails when the committed file differs.
- [x] The generator fails on a kind it cannot map, proved by a planted interface
      field and a planted `map[int]string`.
- [x] A named string type with constants becomes a union of its values, proved
      by a planted constant that changes the union.
- [x] `GET /api/{$}` answers the queue's view as JSON: its three sections, the
      waiting count, and a stamp computed from the same rows.
- [x] A Go test fails when the view's stamp and `queueStamp` differ for the same
      store state, and when a section is sent as something other than its rows.
- [x] `tool/fixtures` writes the view for seeded states (populated, nothing
      waiting, the awkward week) with a fixed clock and fixed ids, into
      `web/src/fixtures/`, and `make check` fails when they differ.

> **Completed** 2026-10-30 08:34 UTC
>
> - `make check` — 14.8s
> - `go test -race ./...` — 45.6s
> - `go test -count=1 -tags browser ./internal/server` — 49.0s

## Iteration 3: The tearout

```punchcard
required_acks:
  - tearout-approved
```

- [x] The queue's row, section and screen are components in
      `web/src/components/` whose markup equals queue.html's for each state.
- [x] Stories cover each state queue.html draws: populated, after a verdict,
      nothing waiting, not installed, notifications not allowed, a lapsed
      subscription and the awkward week. Data states come from the fixtures, and
      client-only states from props.
- [x] A screen sheet lays those states out in 390 by 800 frames, light beside
      dark, each with a full-screen button, plus the wide layout as its own
      story.
- [x] The owner judges the sheet on the phone against queue.html, and every
      change asked for is made before the ack.

> **Completed** 2026-10-30 13:22 UTC
>
> - acks: tearout-approved
> - `make check` — 16.8s
> - `go test -race ./...` — 5.3s
> - `go test -count=1 -tags browser ./internal/server` — 47.5s

## Iteration 4: The shell

- [x] `GET /{$}` answers the shell: the head the layout writes,
      `/static/react.js` and a root element. Every other page stays a template.
- [x] The app draws the queue with React Router and TanStack Query from `/api/`,
      and refetches when the stream's stamp differs from the view's.
- [x] The app shows the offline notice, the update notice and the first-run,
      lapsed and blink states by calling the `web/ts` modules, split where a
      module mixed its logic with template DOM.
- [x] `App()` embeds and stages `web/ts`, and the module zip test covers it.
- [x] browserApp, browserBlink, browserFirstRun and browserUpdate run against
      the React queue, and each fails with `GET /{$}` answering the template.
- [x] A browser test opens a review through the agent socket while the queue is
      open, and the row appears with a value set on `window` still there.
- [x] queue.html's page block and `handleQueue`'s template path are removed.

> **Completed** 2026-10-30 13:41 UTC
>
> - `make check` — 13.9s
> - `go test -race ./...` — 4.1s
> - `go test -count=1 -tags browser ./internal/server` — 46.9s

## Iteration 5: On the phone

```punchcard
required_acks:
  - phone-checked
```

With the daemon running this branch's build, the owner checks in the installed
app:

- [ ] The queue opens and matches the approved sheet.
- [ ] A review an agent opens appears in the queue with no reload.
- [ ] A row opens its review, a verdict returns to the queue, and the eye
      blinks.
- [ ] Stopping the daemon shows the offline notice, and a rebuilt daemon shows
      the update notice.
