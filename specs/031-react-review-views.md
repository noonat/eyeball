---
status: active
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -count=1 -tags browser ./internal/server
---

# 031 The review page's views and components

The review page gets its JSON views, the API for one card's body, and the
components that draw it, judged on the phone as a tearout. The template page
stays the page the surface serves. The next spec in specs/TODO.md moves the
route to the app, with the writes, the deferred bodies, the reader's place, the
stream and the browser tests. The design is in
[docs/architecture.md](../docs/architecture.md) under The surface and Client
state, which this spec's first commit extends. The screens are the ones approved
in `docs/design/review.html`, so the tearout reproduces them from components
rather than redesigning them.

**The page moves in two specs.** The review page is the largest screen: code,
documents, rendered files, comments, the composers, the decision bar and the
deferred bodies, with 36 Go tests and about 25 browser tests reading it. One
branch for all of it is too large to read on a phone. The split is where a
reviewer's judgment changes: this spec's work is judged by looking at it, and
the next one's by using it. The cost is that the views are settled before
anything uses them, so the next spec may change a field this one adds.

**One function builds a card's body.** The round's view holds each body up to
the sending thresholds, and the cards route answers one body past them. Both
call the same function on one file and its comments. The template's cards route
builds the whole round and keeps one card, so its body and the round's agree by
construction. A body built twice could drift, so a Go test builds every body of
a round both ways and compares them.

**A document becomes a list of passages.** A thread is a component with a
composer and an edit form, so it cannot sit inside HTML inserted as it is.
render returns the passages, each with its HTML and line range, and the folds
that hold runs of them, and the template page joins the list with its threads
between, as `After` writes them today. The template page's markup for every
state is unchanged, which `tool/fixtures` shows by writing the same files before
and after. The file page moves to the same list, so one component draws a
document.

**Only a passage's HTML is trusted markup.** Each passage's HTML is a
`DocumentHTML`, converted in the one function that converts it today. A comment,
a quote, a reply and a note are strings, which the app escapes.

**The surface compresses.** A view and the bundle are sent gzipped when the
request accepts it. A long review's rows were 4.8 MB as JSON and 258 KB gzipped,
measured 2026-11-01, and it lands here because this spec's views are the large
ones.

**The tests are assertions over the views' fields.** `internal/viewdoc` draws
the file pages as their templates did, so the old tests could keep their class
lookups. The review page's template is still served, so its tests keep reading
it, and the new views get tests of their own over their fields. Extending
`viewdoc` to the review page was rejected: it would be a third drawing of the
page to keep in step with the template and the components.

## Review

Spec review pass: declined, because the one invariant with nothing checking it
yet, the two routes building the same body, gets a Go test in Iteration 1, and
the markup comparison checks the components against the template. End-of-branch
review pass: declined, because the review agent reads each iteration, and the
template page, which nothing here removes, keeps its tests.

The owner approved the spec on 2026-11-01, with both review passes declined.

## Out of scope

- The route switch, the writes, the deferred bodies as the app fetches them, the
  reader's place, the stream, the browser tests, removing the review templates,
  and the long review measured on the phone in the real page. Each is the next
  item in specs/TODO.md.
- Comments in the stamp. The template's stamp leaves them out, and the next spec
  decides whether the app needs them.
- Commenting from a file page, still an open question in specs/TODO.md.
- Every open design item in specs/TODO.md about the review page. Each state
  keeps today's behavior.

## Departures

Where the work went differently from the paragraphs above.

- The cards route builds the whole round and keeps one card, as the template
  route does, and calls `cardBody` on it. The Go test therefore compares the
  route's body with the body the round's view holds for a small round, and with
  `cardBody` over the same round's cards for a deferred one. The route and the
  view share the function by construction, so the test cannot catch a second
  function appearing, only a change to what the one function sends.
- A bundle has no ETag. Only the fonts do, and they are not compressed. The gzip
  test checks that a font keeps its ETag and its 304 with gzip accepted, and
  that the wrapper passes a 304 or a 204 through with no Content-Encoding.
  Giving the bundle an ETag is not part of this spec.
- The gzip flag is also set on the stylesheet `shell.css`, the queue view and
  the project and file views, since all are views or stylesheets. A Go test
  lists the flagged routes, so a route added to or dropped from the list fails
  it.
- `view.File` has `passages` in place of `document`, so every file view fixture
  changed. The file page's component joins the passages and inserts them as the
  one `.prose` element, which keeps the markup the story compares.
- The view has no composer. A composer is built from the anchor the reader taps,
  so the view sends the lists' ids, the line threads and the bar's address and
  count, and the next spec builds the rest. The bar's verdict note and its
  refusal are answers to a write, so they are not in the view.
- `tool/fixtures` also writes each page state's view as `review-*.json`, typed
  in `scenes.ts`, and the template routes' answers as `review-*.html` and
  `review-*.txt`, committed first as the baseline. It has its own world for the
  states with comments, a decided round, a deferred round, a held card, a list
  of files, a lost base and an abandoned review, and reads the cards and the
  history from the week world.
- The Makefile leaves `web/src/fixtures/review-*.json` out of the format check.
  A view with a one-paragraph note has a one-entry array, which oxfmt writes on
  one line and the server's encoder does not, and `make api-check` compares the
  files byte for byte.
- The template cards route answers one plain sentence, "There is no such review
  or round.", where it answered two, because both now come from one function
  that reads the round.
- The fold test checks the list against expected outlines with a comment at the
  first passage, in the middle and at the end of a run. The old `After` path is
  gone, so there is nothing left to compare it with.

## Iteration 1: The API

- [x] `internal/view` holds the round's view, the card and its body, the
      comment, the thread, the decision bar's state and the document's passages,
      and `tool/apitypes` writes their types.
- [x] `GET /api/reviews/{review}` and `GET /api/reviews/{review}/rounds/{round}`
      answer the round, with `?against=base`, and 404 with `NotFound` where the
      page answers 404.
- [x] `GET /api/reviews/{review}/rounds/{round}/cards/{path...}` answers one
      card's body, and a Go test builds every body of a deferred round from both
      routes and finds them equal.
- [x] Before render changes, `tool/fixtures` writes each review state's template
      page, and those files are committed with the spec's first code commit as
      the baseline.
- [x] render returns a document as passages and folds. The template page joins
      them with its threads, and regenerating the fixtures leaves every
      committed template page unchanged byte for byte.
- [x] The file page's view sends its document as the same list.
- [x] Each passage's HTML is a `DocumentHTML`, and the source test that keeps
      the conversion to one function still passes with no new exception.
- [x] The round's view carries the stamp `/events?review=N` sends.
- [x] The surface sends a view, the bundle and the stylesheet gzipped at level 6
      when the request accepts it, with `Vary: Accept-Encoding`. `/events` and
      the raw route are sent as they are, since gzip would buffer the stream and
      break a video's range requests. A Go test reads a view both ways, and
      finds the stream and a raw range unwrapped.
- [x] `tool/fixtures` writes each state's view and its template page: every
      frame in review.html, a deferred round, a held card, a round with the list
      of files, a whole-review comparison, a round whose base git cannot read,
      and an abandoned review.

> **Completed** 2026-11-01 18:33 UTC
>
> - `make check` — 18.7s
> - `go test -race ./...` — 5.3s
> - `go test -count=1 -tags browser ./internal/server` — 74.6s

## Iteration 2: The tearout

```punchcard
required_acks:
  - tearout-approved
```

- [ ] The review page, its cards and their bodies are components whose markup
      equals the template page's for each state from Iteration 1.
- [ ] The composer, the comment card and its edit form, and the decision bar are
      components whose markup equals the fragment the template route answers for
      each of their states.
- [ ] States no template draws are designed here: the screen before its view
      arrives, a comment sending, a comment unsent with its text and a retry,
      and a card waiting for its body.
- [ ] Screen sheets lay every state out light beside dark, with a story per
      state.
- [ ] The owner judges the sheets on the phone against review.html, and every
      change asked for is made before the ack.
