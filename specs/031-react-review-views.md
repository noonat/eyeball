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

- A passage's `html` is what is inside its block element, not the element. The
  app draws `<div class="blk changed" data-lines="7-7">` from the range and the
  changed flag, because a thread has to sit between two `.blk` elements, and
  app.css has sibling rules (`.blk + .blk`, `.blk:has(...)`, `.fold > .blk`) a
  wrapper around a run of passages would break. `render.Passage` gained `Body`,
  and `DocPassage.html` and every fixture holding a document changed with it.
  The template pages did not change. This corrects Iteration 1, which sent the
  whole element.
- The markup comparison leaves out what the template carried for its script and
  nothing draws: `hx-*` attributes, `data-level`, `data-target`, `data-cancel`,
  `data-retry`, `data-discard`, `data-body`, `data-path` and the `data-*` facts
  on `.page` that no selector reads (`data-stream`, `data-stamp`, `data-place`,
  `data-review`, `data-round`, `data-against`), hidden elements, `<template>`
  elements, hidden form fields, and the `style` the app sets on a skeleton.
  `data-acting` is compared, since app.css selects `.page[data-acting]` to make
  rows, passages and files tappable on iOS Safari. Both sides are reduced the
  same way, and the file pages' stories pass under the same rules.
- A fragment is compared as the top-level elements the route answered with,
  parsed as a template's content so a table row needs no table. The count a
  write's answer carries after its thread is the tally, which has its own story
  from the answer to a delete, so the add-comment stories compare the thread or
  list alone.
- axe fails a card's summary, which holds the link to the file, as nested
  interactive controls. The template had the same markup and the owner approved
  it, so the rule is off for `summary` on the review stories and sheets. It is a
  known fault the template also had. The large sheet also turns off
  `landmark-unique`, since it draws the list of files once for each theme in one
  document.
- The comparison's toolbar links, `?against=previous` and `?against=base`, are
  plain anchors. A query alone is relative to the page it is on, and the router
  resolved it against the story's root. The review has no route yet, so the next
  spec makes them router links.
- The composer is not in the view. Its words, chips and element id come from the
  anchor in `composer.ts`, and the id from the card's own id, since the server
  hashes a path into every id and the app cannot. The fragments check the words
  and the ids against the template route's answers.
- The states no template drew, as designed. A comment sending is the card with
  the word Sending beside its anchor and no actions. A comment not saved reuses
  the dashed `unsent` card the template kept for its script, with its text, a
  Retry now and a Delete. The decision bar says each reason it cannot send a
  verdict: unsent comments, a comment being written, offline. A card waiting for
  its body is the skeleton sized from the view's hunk and line counts. The
  template's script measured a row and a hunk header on the device first, which
  is not ported, so the skeleton uses the stylesheet's fallback heights. The
  screen before its view arrives is the arrival screen the other screens use.
- Sheets: four, for rounds, large rounds, comments and the bar. The review's
  stories are left out of the foundations catalog, as the other screens are. The
  fragments are stories with a play function and no sheet, since each is a part
  of a screen already on a sheet.
- `review-edit.html` shows a review-level comment edited to a sentence about a
  file. `tool/fixtures` names the comment it stored by taking the last one in
  the store's order, and that order is not insertion order. The baseline is
  committed, and the fragment's markup is right for what it holds, so it is left
  as it is.

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

- [x] The review page, its cards and their bodies are components whose markup
      equals the template page's for each state from Iteration 1.
- [x] The composer, the comment card and its edit form, and the decision bar are
      components whose markup equals the fragment the template route answers for
      each of their states.
- [x] States no template draws are designed here: the screen before its view
      arrives, a comment sending, a comment unsent with its text and a retry,
      and a card waiting for its body.
- [x] Screen sheets lay every state out light beside dark, with a story per
      state.
- [/] The owner judges the sheets on the phone against review.html, and every
  change asked for is made before the ack.
