---
status: active
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -count=1 -tags browser ./internal/server
---

# 030 The project view, the tree and the file pages in React

The project view, a review's tree and every file page become screens the React
app draws, from JSON views the server builds. The design is in
[docs/architecture.md](../docs/architecture.md) under The surface, The client
code and Client state, which this spec's first commit extends. The screens are
the ones approved in `docs/design/project.html` and `docs/design/widening.html`,
so the tearout reproduces them from components rather than redesigning them. The
review page stays a template, and so does its composer.

**The three screens move together.** The tree links into the file page, and the
project view's rows link into the same file page by `?copy=`. Moving one alone
would put a full page load between screens a reader moves through in one
sitting, so splitting them was rejected. The cost is size: the file page draws
code, documents, rendered pages and media. The review is read one iteration at a
time.

**A code row is sent as its tokens, each with its role, and the review page
reuses that type.** No file page in scope has a comment or a composer, so the
row is the part of this spec the next one builds on, and its shape is settled
here.

**A review's file address answers one view for both a tree and a file.** The
route draws a tree for a directory or an empty path and a file otherwise, and
the generator maps no interface. So the view has a `kind`, a string type whose
constants the generator makes a union, and one part per kind that is `null` for
the other. Two API addresses were rejected, since the page address alone decides
which it is, and the API path is the page path under `/api`.

**Only the document field is trusted markup.** In the handlers for these pages,
`template.HTML` is used for one field, the file page's `Document`, filled by
`render.Document`. A rendered page's frame is an address. architecture.md says
how the field is kept to one component.

**Each view carries the waiting count**, as each template page reads
`db.Waiting` for the top bar's eye. The eye still does not follow the stream on
these screens, as specs/TODO.md records.

**The template markup the components are compared with is committed when the
templates go.** Iteration 1 has `tool/fixtures` write each state's template page
beside its view, and each story compares its markup with it. Iteration 3 removes
the templates, so the files are kept as they last came out, with a comment
saying so, as 028 did for the queue.

## Review

Spec review pass: requested, because three pairs must agree with nothing yet
checking them: the routes the server answers with the shell and the addresses
the client function says the app draws, the hrefs Go writes and the router's
routes, and the trusted field and the one component that inserts it.
End-of-branch review pass: declined, because the review agent reads each
iteration and the browser tests below fail with the template routes restored.

The owner approved the spec on 2026-10-31 without the spec review pass.

## Out of scope

- The review page, its composer and its deferred bodies, in the next item of
  specs/TODO.md. Commenting from a file page stays an open question there.
- Stamps per view, with the review page.
- The worker answering a moved screen's navigation with the daemon down, in
  specs/TODO.md.
- Every open design item in specs/TODO.md about these screens: the back links'
  labels, the order of rows, a toolbar with no counts, a binary file on the live
  file page, and the eye that does not follow the stream. Each screen keeps
  today's behavior.

## Departures

Where the work went differently from the paragraphs above.

- An address with nothing at it answers its view with 404 and a body,
  `NotFound`: the waiting count and, where a live read was refused, the banner
  saying why. The spec says only that the view answers 404. The page carries
  that banner today, and a body with none would leave the screen Iteration 2
  designs for a missing address unable to draw it.
- A file's view has a `body`, a union that says which of its fields holds what
  the page shows: rendered, deleted, document, binary, too large or rows. The
  template chooses by testing fields in that order, and the app would repeat the
  order in a component. Go decides it once and sends the answer.
- The code row's kind and role are named, not the template's classes. A row is
  `hunk`, `context`, `add` or `del`, and a token's role is `keyword`, `string`,
  `comment`, `number`, `function` or `plain`. The component maps a role to its
  class. `internal/view` does not import `render`, since the generator would
  then load goldmark, so `internal/server` maps the classes `render` writes to
  the roles, and a test proves each role comes out of the highlighter.
- `tool/fixtures` builds its own repositories and does not read `tool/seed`. The
  seed is a `main` package, so nothing imports it, and it ages rows from the
  real clock, so its output differs on every run. The worlds copy the seed's
  files where a frame needs them. Every frame in project.html and widening.html
  that is in scope has a state, light and dark being one state in two themes.
  The content is smaller than the frames': one file's counts, the document's
  length and the working copy's rows differ, and the frame for a document of 184
  passages is a short one. The frames' `plate-04.jpg` is a PNG here.
- `tool/fixtures` writes no markup file for the queue's states, as before. Only
  the new states get one beside their view.
- Iteration 2's first todo says the components' markup equals the template
  page's for each state. It does, with one attribute the template never had: the
  code table is `tabindex="0"`. It scrolls sideways, and axe fails a scrolling
  region that a keyboard cannot reach (`scrollable-region-focusable`) on every
  file story. The comparison drops `tabindex` from `.code` on both sides, and
  the first file story asserts the attribute is there. The template page has the
  same fault and is not changed, since Iteration 3 removes it.
- The view fixtures are also written as `web/src/fixtures/scenes.ts`, typed
  constants, beside their JSON. TypeScript reads a JSON import's strings as
  `string`, so `kind` and every other union failed a typed assignment, and a
  cast would have checked nothing. An object literal keeps its strings as
  literals and is checked against the type, so a field the types do not allow
  fails the build. The Makefile leaves the file out of the format check, since
  `make api-check` compares it byte for byte.
- A story's markup is compared with the template page for the screen's `.topbar`
  and `.scroll` only, as the queue's is. A state the template never drew has no
  markup to compare: the screen before its view arrives, and a 404. Their
  stories assert what the design decided instead.
- The screen before its view arrives shows the queue's wordmark bar with no eye,
  over the queue's connecting eye. The view's title, way back and count are not
  known, and a label drawn now would change when the view came. A 404 draws the
  template error page's bar and box, with the view's banner above the box where
  a live read was refused.
- The sheets show real media, and this corrects an earlier bullet that said they
  served no file bytes. A frame, an image or a player points at the raw route's
  address, which only the daemon answers. `tool/fixtures` now reads each such
  address through the server's own raw route and writes the bytes under
  `web/src/fixtures/_raw/` at the same path, so the views and the markup are
  unchanged. Storybook's `staticDirs` serves the directory at the root for
  `storybook dev` and the stories, and `tool/tearout` serves `/reviews/` from
  it, since the phone opens Storybook under `/storybook/` and the addresses are
  absolute. The directory starts with an underscore so `go:embed` leaves it out
  of the binary. The files are real and small: a two-page PDF with text written
  by Go, a different tone on each side of the chime, tool/seed's clip as a copy,
  and the PNGs and SVGs. Their sizes changed three captions, and the template
  comparison passes with the regenerated files. The empty tree has no state,
  since no store produces one.
- The states are on four sheets, in four files, since one sheet of 29 states is
  too long to read on a phone: the project view with the two states no template
  draws, the tree, code and documents, and media. `ScreenSheet` leaves the wide
  layout out where a screen has none.

## Iteration 1: The API

- [x] `internal/view` holds the project view, the tree and file view with its
      `kind`, and the code row, and `tool/apitypes` writes their types.
- [x] Each page route in scope has its view at the same path under `/api`,
      answering 404 where the page does, and a Go test walks the route table to
      show it.
- [x] Each view carries its title, its back link and label, and the waiting
      count, as `renderPage` fills them today.
- [x] The document field is a named type, and a Go test that reads the package's
      source fails on a conversion to it outside the one function.
- [x] `tool/fixtures` writes each state's view and its template page: the
      project view's populated, worktree and edge frames, the tree's root and a
      subdirectory, and the file page's frames in widening.html.

> **Completed** 2026-11-01 03:56 UTC
>
> - `make check` — 30.7s
> - `go test -race ./...` — 49.3s
> - `go test -count=1 -tags browser ./internal/server` — 65.0s

## Iteration 2: The tearout

```punchcard
required_acks:
  - tearout-approved
```

- [x] The project view, the tree and the file page are components whose markup
      equals the template page's for each state from Iteration 1.
- [x] Two states no template draws are designed here: a screen whose view has
      not arrived, and an address the view answers 404.
- [x] A screen sheet lays every state out light beside dark, as the queue's
      does, with a story per state.
- [x] The owner judges the sheet on the phone against project.html and
      widening.html, and every change asked for is made before the ack.

> **Completed** 2026-11-01 13:26 UTC
>
> - acks: tearout-approved
> - `make check` — 18.2s
> - `go test -race ./...` — 47.5s
> - `go test -count=1 -tags browser ./internal/server` — 64.7s

## Iteration 3: The shell

- [ ] Each page route in scope answers the shell, and the app draws its screen
      from the view, with the view's title.
- [ ] The app moves to a data router, and one client function says which
      addresses it draws. The link component and the tap listener both call it.
- [ ] A link to a screen the app draws moves the router, and any other link is a
      full load. A browser test fails with every link a plain anchor.
- [ ] `react/no-danger` is on, with an exception for the one component.
- [ ] Each screen opens its own stream for the build and connection notices, and
      a browser test shows the offline notice on a file page.
- [ ] Back from a file returns to the scroll the tree or the project view had,
      and a browser test fails without `ScrollRestoration`.
- [ ] Browser tests for these pages run against the React screens, and each
      fails with its route answering the template.
- [ ] The templates, the handlers' template paths and the queue's row partials,
      which only the project page used, are removed, and the template markup
      files are committed as they last came out.

## Iteration 4: On the phone

```punchcard
required_acks:
  - phone-checked
```

Worked by the orchestrator, with the daemon running this branch's build. Each
result is written here as it arrives.

- [ ] The project view, a tree and file pages of each kind match the approved
      sheet.
- [ ] Moving from the project view to a file and back, and through the tree,
      loads no page and returns to the place in the list.
- [ ] A review's "All files" opens the tree, and a link from these screens to
      the review page loads it.
- [ ] A worktree's live file opens by `?copy=`, and a change on disk shows when
      the app comes back to the foreground.
