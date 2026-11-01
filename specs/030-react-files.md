---
status: ready
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

## Iteration 1: The API

- [ ] `internal/view` holds the project view, the tree and file view with its
      `kind`, and the code row, and `tool/apitypes` writes their types.
- [ ] Each page route in scope has its view at the same path under `/api`,
      answering 404 where the page does, and a Go test walks the route table to
      show it.
- [ ] Each view carries its title, its back link and label, and the waiting
      count, as `renderPage` fills them today.
- [ ] The document field is a named type, and a Go test that reads the package's
      source fails on a conversion to it outside the one function.
- [ ] `tool/fixtures` writes each state's view and its template page: the
      project view's populated, worktree and edge frames, the tree's root and a
      subdirectory, and the file page's frames in widening.html.

## Iteration 2: The tearout

```punchcard
required_acks:
  - tearout-approved
```

- [ ] The project view, the tree and the file page are components whose markup
      equals the template page's for each state from Iteration 1.
- [ ] Two states no template draws are designed here: a screen whose view has
      not arrived, and an address the view answers 404.
- [ ] A screen sheet lays every state out light beside dark, as the queue's
      does, with a story per state.
- [ ] The owner judges the sheet on the phone against project.html and
      widening.html, and every change asked for is made before the ack.

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
