---
status: ready
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -count=1 -tags browser ./internal/server
---

# 032 The review page in React

The review page becomes a screen the React app draws, from the views and
components spec [031](done/031-react-review-views.md) built and the owner
approved. Its writes move under `/api` as JSON, a long review's bodies arrive as
their cards near view, the reader's place and drafts carry over, the stream
refetches what it names, and the review templates go. The design is in
[docs/architecture.md](../docs/architecture.md) under The surface, The client
code and Client state, which this spec's first commit extends. The screens are
the approved sheets, so nothing here is redesigned.

**The writes become JSON, and their request types come from Go.** The template
routes take a form and answer a fragment. The app holds typed data and draws
every fragment itself, so the routes under `/api` take a JSON body typed in
`internal/view` and answer with the stored comment, the count or nothing, as
architecture.md describes under "A write sends JSON". The app puts each answer
into the cached view, so a comment never waits on a refetch of the round.

**The cards route compares one file.** Today it builds the whole round and keeps
one card, so each body a long review fetches compares every file in the round
again. It calls `capture.ComparePaths` on its own file instead, and a Go test
still finds every body equal to the one the round's view holds.

**The Go tests move to the views before the route does.** About 36 tests in
`review_test.go` and `acting_test.go` read the template page's markup, roughly
235 reads by class. The route switch would leave them nothing to parse, so they
are rewritten over the view's fields and the JSON writes first, while the
template page is still served and `make api-check` still compares its markup
byte for byte. Extending `internal/viewdoc` to the review page was rejected, as
in 031: it would be one more drawing of the page to keep in step, where the
stories already compare the components with the committed template markup.

**The long review gets an iteration of its own.** The deferred bodies, the
skeleton heights measured on the device, the card held at the top of the screen
while bodies arrive, the reader's place with its folds, and a jump to a comment
are each a fix settled on the phone, and each is ported from `web/ts` rather
than redrawn. They are separated from the screen's writes because a fault in one
shows only on a long review, and its browser tests are the ones that measure
scroll.

**Two checks outlive the templates they compare with.** A composer's anchor
sends the level, path, range, old line and comparison the template's hidden
fields send: the committed composer answers hold those fields, and a Vitest test
builds the app's request for each and compares them, so the check survives the
templates. A draft or place the template page saved before the switch is read by
the app: a Vitest test stores one in the template's shape and finds it restored.

**The failed line is ported, and judged on the phone.** The template's script
writes a failed line beside a control whose request failed, with the server's
sentence or one of three fixed ones, and review.html draws it in the bar. No
component draws it yet. It is drawn as the script draws it, with a story per
control, and judged in the last iteration with the rest. A tearout iteration
first was rejected, since the frame and the stylesheet already decide how it
looks.

**The stamp still leaves comments out.** 031 left the question here.
architecture.md answers it under "The stream says when to refetch": the reader's
own comments reach the cache from the server's answers, so a stamp counting them
would refetch the round for every comment the reader writes. A comment written
on a second device shows on the first after a refetch, as specs/TODO.md records.

## Review

Spec review pass: requested, because five pairs must agree with only this spec's
checks between them: the app's request and the template's hidden fields, the
template's storage shapes and the app's reader, the router's scroll and the
screen's, the cards route's one-file comparison and the round's, and the blink
mark across a router move instead of a page load. End-of-branch review pass:
declined, because the review agent reads each iteration and each browser test
fails with the route answering the template.

## Out of scope

- htmx, the error page's template, `layout.html`, the template bundle at
  `/static/app.js` and the `web/ts` modules the error page still loads. The next
  item in specs/TODO.md removes them.
- A tapped notification landing on the review it names. Spec
  [026](026-tap-opens-the-review.md) is blocked, and the tap tests' targets
  change here only as far as the review route answers the shell.
- Comments in the stamp, and a comment from a second device showing without a
  refetch, as above.
- Commenting from a file page, and every open design item in specs/TODO.md about
  the review page. Each state keeps today's behavior.
- The cause of the flaky `TestServer_Surface_browserJump`. Its port is run 20
  times, and its entry in specs/TODO.md records whether it still fails.

## Iteration 1: The API

- [ ] `POST`, `PUT` and `DELETE /api/reviews/{review}/comments` and
      `POST /api/reviews/{review}/decision` take JSON bodies typed in
      `internal/view`, and `tool/apitypes` writes the request types.
- [ ] A comment write answers the stored comment and the count, a delete the
      count, and a verdict nothing, and a refusal answers 4xx with `Refusal`
      wherever the template route refuses.
- [ ] The JSON routes take the same submission key the form routes do, and a Go
      test sends one comment twice under one key and finds it stored once.
- [ ] The cards route compares only its own file with `capture.ComparePaths`,
      and the test that builds every body both ways still finds them equal, for
      the latest round and with `?against=base`.
- [ ] A Vitest test builds the app's request for each committed composer answer
      and finds its fields equal to that answer's hidden form fields.

## Iteration 2: The Go tests over the views

- [ ] Every Go test that reads the review page's markup reads the round's view,
      the cards route or a JSON write instead, and no Go test outside
      `tool/fixtures` parses the template page.
- [ ] A fact an old test checked that only markup holds is listed in this spec's
      Departures with the story that now checks it.
- [ ] Five facts the old tests checked, chosen across the rewritten files, are
      broken in the view builder one at a time, and each fails a rewritten test.

## Iteration 3: The screen

- [ ] `/reviews/{review}` and `/reviews/{review}/rounds/{round}` answer the
      shell, and the app draws the review from its view, with the view's title.
- [ ] The toolbar's comparison links and the back link are router links.
- [ ] A comment is added, edited, deleted and retried through the JSON routes,
      shown as sending and then as stored, and the tally follows each answer.
- [ ] A failed request draws the failed line beside its control in the composer,
      the edit form, a comment's actions and the bar, each with a story.
- [ ] A verdict sets the blink mark, moves the router to the queue once stored,
      and draws the refused bar on 422. A browser test fails with the mark
      removed.
- [ ] Drafts are saved as typed and restored on reload, and a Vitest test
      restores a draft in the template page's shape.
- [ ] The screen follows `/events?review=N`, refetches the round and the bodies
      on screen when its stamp moves, and keeps a composer's typed text.
- [ ] The browser tests for these behaviors run against the app's screen, and
      each fails with the route answering the template.

## Iteration 4: The long review

- [ ] A deferred card asks for its body once it is open and within a screen of
      view, and a folded card once it is unfolded.
- [ ] A skeleton's heights are measured once on the device, as `measureRows`
      measures them.
- [ ] The card at the top of the screen stays put when a card above it changes
      height, with Chromium's scroll anchoring off.
- [ ] The reader's place and folds are restored after the router's scroll and as
      bodies arrive, until the first tap, scroll or key, and a Vitest test
      restores a place in the template page's shape.
- [ ] A link to a comment scrolls to it once it is drawn, below its card's stuck
      header.
- [ ] The browser tests for bodies, held cards, the jump, the comment jump and
      the reader's place run against the app's screen, and each fails with the
      route answering the template.
- [ ] The jump test is run 20 times, and its entry in specs/TODO.md records the
      result.

## Iteration 5: The templates go

- [ ] `review.html`, `acting.html` and the parts of `file.html` only they use,
      the form and fragment routes, the template cards route and the review
      handler's template path are removed.
- [ ] The `web/ts` modules only the review template loaded are removed, and the
      template bundle still builds for the error page.
- [ ] Browser tests that used the review page as their example of a template
      page use the error page instead, and a link to the review from the app's
      screens moves the router.
- [ ] The template markup files are committed as they last came out, with a
      comment saying so, as 030 did.

## Iteration 6: On the phone

```punchcard
required_acks:
  - phone-checked
```

Worked by the orchestrator, with the daemon running this branch's build. Each
result is written here as it arrives.

- [ ] A review of each kind matches the approved sheets, and the failed line in
      each control looks right.
- [ ] A comment on a line, a passage, a file and the review is added, edited and
      deleted, a draft survives closing the app, and a verdict blinks the queue.
- [ ] A long review opens where it was left, bodies arriving above the reader
      move nothing, and a jump from the list of files lands on its card.
- [ ] The keyboard fix and the bar after a resume behave as before, and a tap on
      a row of a decided round opens its composer.
- [ ] The first draw and a fling on a long review are measured as the probe
      measured them, and recorded beside the probe's numbers.
