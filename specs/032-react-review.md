---
status: active
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

**The route switches behind a server option until every test is ported.** Every
iteration is gated on the browser tests, and about 25 of them read the template
page by its markup. Switching the route in one iteration would fail the ones a
later iteration ports. A server option answers the review routes with the shell,
the tests ported in Iterations 3 to 5 set it, and Iteration 5 makes it the
default and removes it. Porting every review browser test in one iteration was
rejected as one review too large to read.

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
requested, because Iteration 2 rewrites about 235 markup reads in the Go tests
and Iteration 6 removes what they read, and a check lost between the two fails
nothing, where the review agent reads one iteration at a time.

The owner approved the spec on 2026-11-01, and after Iteration 1 had started
agreed to both review passes, the spec's to be run against this draft. The spec
pass found 16 problems, among them an Iteration 3 that could not close, a jump
from the list of files that lands at the queue's old scroll after a router move,
and template behaviors no iteration ported. Iterations 2 to 7 are its fixes,
made before Iteration 2 started. Its findings on Iteration 1's code are todos in
Iteration 3, where the app first uses those routes.

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

## Departures

Where the work went differently from the paragraphs above.

- `DELETE /api/reviews/{review}/comments/{comment}` takes no body. The address
  names the comment, and a delete with a JSON body is a request some proxies
  drop. It answers `CommentDeleted`, the count. The edit takes `CommentEdit`,
  and the add and the verdict take `CommentRequest` and `DecisionRequest`.
- A verdict the server refuses as an empty change request answers 422 with a
  `Refusal` that holds the template's sentence, so the rule that a refusal is a
  `Refusal` has no exception. The bar draws its own refused state from the
  status.
- The count in a write's answer is the comments on the review's latest round,
  the number the bar shows, as the template's tally was. The template sent it
  only while the round was open. The JSON answer always carries it, and the app
  ignores it once the bar is gone.
- A write's body must be declared `application/json`, is bounded at a megabyte,
  and is refused when it holds a field the type does not or has data after it.
  The type check is not in the spec. It answers a body not declared as JSON with
  415 and a `Refusal` rather than a decode failure, and sits behind the
  cross-origin check every surface request passes. It has no reason beyond being
  explicit. A store failure in a write under `/api` answers 500 with `Refusal`
  where the error page would have answered, since the app cannot read a page.
- The cards route still builds the card with the round's own code. It gives
  `roundView` the path, so one file is compared with `capture.ComparePaths` and
  the comments are placed as the round places them. It also needed a new
  `capture.MovedPaths`, which lists the paths that moved from the captures'
  digests with no diff. The route reads the count of files the round moved from
  it, since a document is drawn as the page itself only when it is the round's
  one file, and a card built alone always looks like the only one. The same list
  answers 404 for a path that did not move. This corrects 031's departure that
  the route and the view share a function by construction: they share
  `cardBody`, and the comments each holds come from the same code, but the
  comparison is now separate. The Go test covers a later round, `?against=base`,
  a fresh document alone and beside other files, and a round whose other file
  has no blob left, which only a route that compares one file can answer.
- The Vitest test runs in a second project, `checks`, of the same Chromium,
  since it reads the answers with the browser's DOM parser. `make stories` runs
  both projects. The request is built by `commentRequest` in `composer.ts`. The
  template's `submission` field is always empty, since its script draws the key,
  so the test leaves it out of both sides.

### Iteration 2

Each Go test that read the review page now reads the round's view, the cards
route or a JSON write. A fact only the page's markup held is listed here with
the story that checks it. Every story named compares a component with the
committed template markup, which `make api-check` keeps equal to what the
template routes send, so the template page is still held while it is served. The
four new stories, `TooLarge`, `Binary`, `ChangesRequested` and `RenderedBox`,
are in `ReviewChecks.stories.tsx` and assert what they draw. They are not on the
sheets, since the sheets are the pages the owner approved.

- `review_test.go`, `TestServer_Surface_review`: the note is the letter, with
  the class `note letter`, a signature, no `from` label and first on the page.
  The `Code` story, from `review-code.html`. The decision bar's buttons and the
  card's `open` attribute are in the same story.
- `TestServer_Surface_reviewAgainst`: the control's labels, its `?against=`
  links and `aria-current`. The `RoundTwo` and `WholeReview` stories, from
  `review-doc.html` and `review-doc-whole.html`. The Go test holds the label and
  the flag in the view.
- `TestServer_Surface_reviewChangesRequested`: the notice "Changes requested."
  ahead of the sentence. No committed page holds a current round with changes
  requested, so the new `ChangesRequested` story asserts it. The `Approved`
  story holds the approved form.
- `TestServer_Surface_reviewComments`: the verdict note under the notice, one
  box with "Sends immediately" for a late comment, and no actions on a released
  comment. The `Approved` story, and the `ComposerLate`, `CommentAfterVerdict`
  and `CommentReleased` stories. The "What was done" section is the
  `RoundOneClosed` story. The Go test holds that the same replies are in the
  view of an approved current round.
- `TestServer_Surface_reviewDocument`: the Raw diff link's label. The `RoundTwo`
  story. The scan of the whole page for scripts, styles, images and event
  attributes now covers the passages' HTML, the one field the app inserts as
  markup. Every other string reaches the page through React's escaping, and the
  source test that keeps `DocumentHTML` to one conversion holds the rest.
- `TestServer_Surface_reviewEarlierRounds`: the order of the letter, the
  sections, the verdict note and the toolbar on a closed round, and the
  sections' headings. The `RoundOneClosed` story. The 404 sentence is the
  `NotFoundScreen` text the arrival stories draw.
- `TestServer_Surface_reviewFilesWithNoRows`: the lines "Too large to show as a
  diff." and "Binary file, not shown as a diff." in place of rows. No committed
  page held a card of either kind, so the new `TooLarge` and `Binary` stories
  assert each. A binary file from git is a rendered facts line in the view, as
  the `Rendered` story's cards are.
- `TestServer_Surface_reviewFresh`: the toolbar's "1 file, new Raw diff", the
  small button, the prose directly in the page and no card. The `Fresh` story.
  No page holds a fresh document beside the comparison control. The toolbar
  draws the control from `against` alone, so the Go test holds that both are in
  the view.
- `TestServer_Surface_reviewHoldsAFilePastFiveHundred`: a held card sends no
  `hx-trigger`, shows "501 changed lines" and a Load anyway link equal to the
  file's own. The `Held` story, except the `hx-` attributes, which the markup
  comparison leaves out. The view holds the same fact as `deferred` false with
  no body. The app's own behavior is a browser test in the long review's
  iteration.
- `TestServer_Surface_reviewSendsBodiesUpToItsLimits`: the card's `hx-get` and
  `hx-trigger` of "eyeball:near once". These are the template script's wiring
  and no story compares them. The browser tests that run against the template
  page still exercise them until the templates go, and the long review's
  iteration tests the app's own fetch. The skeleton's `data-hunks` and
  `data-lines` are `skeleton_hunks` and `skeleton_lines` in the view.
- `TestServer_Surface_reviewListsFilesPastTen`: each row's link to `#` and the
  card's id, its name and its plus count. The `Listed` story.
- `TestServer_Surface_reviewLostRepository`: the warning's second sentence,
  "Each file is shown as this round captured it.", and no counts on the card.
  The `Unmeasurable` story.
- `TestServer_Surface_reviewLateLineComment`: a slot under a late comment's
  file. `Approved` holds a `slot-` id, and the view's `slot` is asserted.
- `TestServer_Surface_reviewMarksAChangedWord`: the classes `k`, `n` and `w`
  around the keyword, the number and the changed word. The tokens' roles and
  marks are asserted in the view, and the `Code` story holds the class each role
  draws as.
- `TestServer_Surface_reviewModeChange`: the mode line's markup. The view's
  `mode`, and the `File` story's `file-mode.html` for the line itself.
- `TestServer_Surface_reviewRendered`: a rendered card in a box of its own, and
  a page in a sandboxed frame. The new `RenderedBox` story, and the file page's
  frame stories. The card's sides, frame and facts are asserted in the view.
- `TestServer_Surface_reviewTotalsMatchTheStoredSize`: the toolbar's "N files +X
  −Y" as one string. The view's three fields are compared with the stored size,
  and the `Code` story holds how they are written out.
- `acting_test.go`: the form routes' answers are no longer parsed. A comment's
  anchor, quote and actions, the tally and the bar being left alone are asserted
  by the JSON routes' tests and drawn by the `AnswerLine`, `AnswerLate` and
  `TallyAfterDelete` stories. The tests that remain on the form routes read the
  stored comment, the status and the headers, and look for the answer's list or
  row id as a substring, until the routes go.
- The composer, comment and edit form routes' tests read their answers as
  substrings for the same reason. The chips' order and which is pressed, the
  anchor line, the row a line's composer opens in, and the hint on a line above
  the chips on a decided round are the `ComposerLine`, `ComposerLate` and
  `CommentEditing` stories, and `composerText` is what the app draws the words
  from. The refused bar's notice and its single textarea are `BarRefused` and
  `BarRefusedNote`.
- `surface_test.go`: the eye on the review page is the view's `waiting_count`,
  and the error page keeps the parse of its eye. The htmx configuration test
  reads the error page, the one template page left.
- `file_test.go` and `tree_test.go` read a card's counts, its whole-file link
  and the All files link from the view. The "Whole file" label, the link sitting
  in the card's summary and the "All files" row's text are in the `Code` story.
- `tool/seed`'s link crawler also parsed the template review page, which the
  review pass did not list. It now follows the links the round's view holds,
  drawn into a page of anchors in the places the template put them, since two of
  its checks read where a link sits. A review card's frame Open link and Load
  anyway link are not followed. The file pages' links still reach the raw route.
- The guard is in the helpers: `parse` fails on a body that holds the template
  review page's `data-stamp` or `data-review`, and the crawler fails on the same
  mark. A test that fetches the page and parses it fails at once, which was
  checked with a test added and then removed. It found four tests in
  `file_test.go` and `tree_test.go` that a search for the review address had
  missed.
- `documentDoc` and `docFromView` in the tests send a review card's passages
  through the file page's existing `viewdoc.Page` drawing, so the tests read
  each block by the old class names. This is not the extension of `viewdoc` to
  the review page that the spec rejected. `viewdoc` is unchanged, only a card's
  passages are drawn that way, and a passage's thread and comments are read from
  the view. The cost is that this reading of a card is held by the file page's
  markup comparison, `file-document.html` and the stories that compare it, and
  not by one of a review card's.
- Mutation check. Five facts chosen across files and kinds of check were broken
  in the view builder one at a time, each from an assertion the old tests held:
  added rows read as context (`TestServer_Surface_review`'s rows, a row check),
  late comments counted with the verdict (`TestServer_Surface_reviewComments`, a
  sentence), a card held at 500 lines and not 501
  (`TestServer_Surface_reviewHoldsAFilePastFiveHundred`, a threshold), the stamp
  left out of the view (`TestServer_handleEvents`, which read `data-stamp`) and
  the project link dropped (`TestServer_Surface_projectFromAReview`, which read
  `.who .bit`). Each failed the rewritten test named. Ten more were caught by
  the tests that follow their facts: released comments editable, a fresh
  document that ignores its old side's blocks, the eye's count, the All files
  link, swapped counts, earlier rounds in the wrong order, the whole-file link,
  the quote read from the wrong lines, a passage's thread dropped and the mode
  line dropped. Three component changes failed the new `TooLarge`,
  `ChangesRequested` and `RenderedBox` stories. None survived.

### Iteration 3

- The spec says an edit or a delete of a comment already gone answers 500 now.
  Only the edit did. A delete of a comment already gone answers 200 with the
  count, and `TestServer_deleteCommentJSON` and the template's own handler hold
  that as deliberate: a delete sent twice must not report a failure. The edit
  now answers 404 with `Refusal`, from the one function the form route shares,
  and the delete is unchanged.
- The server option is the unexported field `reviewInApp`, which a test sets.
  The two review GET routes are flagged `shell`, so the test that holds the
  route table to the app's patterns passes with the review's two added, and the
  test that asks each shell route for the shell sets the field first. The field
  goes with the template page's handler, at the close of Iteration 5.
- The view's comment `href` and the bar's `action` name the `/api` routes, added
  in the view's conversion and nowhere else, so the template's `hx-put`,
  `hx-post` and the committed `review-*.html` answers are byte for byte as they
  were. Only the `review-*.json` views and `scenes.ts` moved.
- `components/composer.ts` is `composerAt.ts`. Once the app's bundle reached it,
  esbuild refused `./composer` beside `Composer.tsx`, since it resolved the
  import to the capital file and a case-insensitive file system would have
  agreed. The cases the file holds are an anchor and what it sends.
- The route, the taps, the sends and the drafts are `app/ViewRoutes.tsx`,
  `useReview.ts`, `useTaps.ts`, `reviewWrites.ts`, `reviewCache.ts` and
  `reviewDrafts.ts`. The components stay controlled and send nothing. A
  composer's text, a chosen level, each open edit form's text, the review's box
  and the verdict's note are the route's state, which the components read
  through `ReviewUi`.
- A tap is one document listener that reads the click's target, as the
  template's did, and the selection is a `sel` class drawn from the open
  composer. The template's `preventDefault` for a passage that is a `<details>`
  is not ported: the app draws a passage as a `div`. A tap on a link or the hint
  inside a passage is still left alone, and a passage reads its path from the
  nearest element that carries one, so the round's one new document, which no
  card holds, takes comments too. It drew none before: the fresh layout was a
  bare document with no threads and no composer.
- The composer is scrolled into view with `block: "nearest"` and the field
  focused with `preventScroll`, once as it opens and not as text is typed. A
  composer that opens in a fold opens the fold, where the template did that only
  for a draft reopened there. The reasons are the template's: the call order and
  the two options were settled with the keyboard fix, and the template recorded
  no more, so none is invented here.
- The comparison's links keep `?against=` as their address and call
  `navigate({ search })` on a plain click. A router link would write the
  resolved address into the markup, which the stories compare. Moving between
  the two draws the arrival screen until the other view comes.
- A comment sent from a composer closes the composer at once and shows the
  comment as sending. The template kept the form open until the answer. A
  refusal puts the text back in the composer with the server's sentence. If
  something was typed elsewhere meanwhile, the refused comment waits as an
  unsent card with the sentence beside its actions, so nothing is lost for
  putting it back.
- A chosen level is state beside the anchor. A line, a passage or a removed row
  sent as the file or the review sends only the fields that level takes, and a
  removed row sent as the file is still a removed row, as the template's form
  was. The comparison goes with a removed row alone. The template's
  `review-composer-whole` answer, a file composer with a comparison, is not
  something a tap opens on either page, so `tool/fixtures` writes
  `review-composer-removed-base` in its place, the `ComposerWholeReview` story
  is `ComposerRemovedWhole`, and `composers.ts` holds the query of each composer
  answer for the Vitest test to build its anchor from. The file is generated and
  left out of the format check as `scenes.ts` is.
- An answer is placed from the anchor it was sent to, since the comment carries
  no level or path. The template's answer to a comment on a file replaced the
  file's list with every comment at the path, line comments included. The app
  puts a line comment under its line, where a reload does, so
  `browserDecidedTap` finds one comment in the file's list, not two.
- A comment that is sending carries `data-sending`, so a test can tell it from
  the stored one that replaces it. It has no counterpart in a template page.
- The failed line is drawn by the one `Failed` component, after a form's foot or
  after a card's actions. Its sentence is the server's `Refusal` where there is
  one, and one of the template's three otherwise. A request that failed to
  arrive, timed out at ten seconds or was failed by the server leaves an unsent
  card with no line beside it, as the template's did, and a retry that fails
  again leaves the line. The four new states are in `ReviewStates` and on the
  comments sheet: a composer refused, an edit that failed, a delete that failed,
  and a retry that failed. The owner judges them on the phone in Iteration 7.
- A draft is saved as the state changes, where the template saved after an
  input, a click or a swap. A decided round saves none and restores none. The
  edit forms restored are those of comments the round still has. Deferred and
  held cards show their skeleton or their size in the app until Iteration 5, and
  a review reached from another screen is the app's whatever the option says, so
  the browser tests of a long review are not in this iteration.
- Verdicts stay on the template page. `browserActing` is split, and its verdict
  half is `browserActingVerdict`. The card half of the template's failed-save
  test is the app's `browserFailedSave`, and the template's whole test stays as
  `browserFailedVerdict` for the verdict's failure and the guard that waits on
  an unsent card. `typeInto` sets a field through the element's own setter,
  since React drops an input event after a value set the plain way.
- Each ported test was run with the option forced off by a temporary edit of
  `handleReviewPage`, and each failed: `browserActing`, `browserDecidedTap`,
  `browserDrafts`, `browserFailedSave`, `browserFold`, `browserPassages`,
  `browserRenderedCard` and `browserReviewScreen`, by 5 seconds waiting for the
  review the shell did not draw. The two template tests, `browserActingVerdict`
  and `browserFailedVerdict`, passed unchanged.
- A comment on a passage of a document is placed from the answer so it shows at
  once, and the round is fetched again after one is added or deleted. Whether a
  passage sits in a fold depends on whether it has a comment, which is
  `render`'s rule. Copying it into TypeScript would leave two copies with
  nothing checking that they agree, and without the fetch a commented passage
  stays in its fold with a summary count that is wrong, until the next load.
  `browserFoldComment` saves a comment on a passage inside a fold and deletes
  it, and fails with the fetch removed.
- A tap on the place the composer is already open at opens it again, as the
  template's script did: the field is focused and scrolled into view, and the
  level the chips chose and the line a failed send left are cleared. The typed
  text stays. The first version of the app ignored that tap. On iOS it is how a
  reader gets the keyboard back after dismissing it.

## Iteration 1: The API

- [x] `POST`, `PUT` and `DELETE /api/reviews/{review}/comments` and
      `POST /api/reviews/{review}/decision` take JSON bodies typed in
      `internal/view`, and `tool/apitypes` writes the request types.
- [x] A comment write answers the stored comment and the count, a delete the
      count, and a verdict nothing, and a refusal answers 4xx with `Refusal`
      wherever the template route refuses.
- [x] The JSON routes take the same submission key the form routes do, and a Go
      test sends one comment twice under one key and finds it stored once.
- [x] The cards route compares only its own file with `capture.ComparePaths`,
      and the test that builds every body both ways still finds them equal, for
      the latest round and with `?against=base`.
- [x] A Vitest test builds the app's request for each committed composer answer
      and finds its fields equal to that answer's hidden form fields.

> **Completed** 2026-11-01 23:02 UTC
>
> - `make check` — 35.5s
> - `go test -race ./...` — 58.7s
> - `go test -count=1 -tags browser ./internal/server` — 73.1s

## Iteration 2: The Go tests over the views

- [x] Every Go test that reads the review page's markup reads the round's view,
      the cards route or a JSON write instead, among them the stamp test in
      `stream_test.go`, the eye and htmx configuration tests in
      `surface_test.go` and the brief's project link in `project_test.go`.
- [x] The htmx configuration test reads the error page, the one template page
      left after the review's.
- [x] No Go test outside `tool/fixtures` parses the template review page.
- [x] A fact an old test checked that only markup holds is listed in this spec's
      Departures with the story that now checks it.
- [x] Five facts the old tests checked, chosen across the rewritten files, are
      broken in the view builder one at a time, and each fails a rewritten test.

> **Completed** 2026-11-01 23:32 UTC
>
> - `make check` — 41.4s
> - `go test -race ./...` — 64.4s
> - `go test -count=1 -tags browser ./internal/server` — 72.8s

## Iteration 3: The screen and its comments

- [x] A server option answers `/reviews/{review}` and
      `/reviews/{review}/rounds/{round}` with the shell, and the app draws the
      review from its view, with the view's title. The browser tests ported here
      and in the next two iterations set it, and the rest still read the
      template until Iteration 5 makes it the default.
- [x] The review's patterns are in the app's routes, so a link to a review from
      another screen moves the router, and the link test's last block says so.
- [x] The toolbar's comparison links and the back link are router links.
- [x] The view's comment and bar addresses name the `/api` routes, and a write
      sends the loopback token, whose refusal answers a write under `/api` with
      `Refusal`.
- [x] A tap opens the composer on a row, a removed row, a passage, a file's
      image or its line saying why there is no body, as the template's script
      does, with the selection drawn and the composer scrolled into view.
- [x] A composer moved to another anchor keeps its typed text, as the template's
      does, and the comment in `composer.ts` that says otherwise is corrected.
- [x] A composer's level chips switch what it sends, and a test sends a line, a
      passage and a removed row switched to file and to review through the JSON
      route and finds each stored.
- [x] `tool/fixtures` writes a removed row's composer under `?against=base`, and
      the composer test takes each anchor from the scene that wrote the fixture.
- [x] A comment is added, edited, deleted and retried through the JSON routes,
      shown as sending and then as stored, and the tally follows each answer. A
      retry sends the first try's submission key, and an edit or delete of a
      comment already gone answers 404 with `Refusal` where it answers 500 now.
- [x] A failed request draws the failed line beside its control in the composer,
      the edit form and a comment's actions, each with a story.
- [x] Drafts are saved as typed through `web/ts/storage.ts` and restored on
      reload, and none is kept on a decided round. A draft inside a folded run
      of passages opens the fold.
- [x] A Vitest test stores a draft with all five of the template page's parts
      through `remember` and finds each restored, and the unsent card's retry
      reusing its stored key.
- [x] The app prunes expired stored entries, as the template bundle does.
- [x] The browser tests for comments, passages, folds, rendered cards, a decided
      round's tap, a failed save and a draft surviving a reload run against the
      app's screen, and each fails with the option off.

> **Completed** 2026-11-02 00:16 UTC
>
> - `make check` — 41.8s
> - `go test -race ./...` — 58.9s
> - `go test -count=1 -tags browser ./internal/server` — 75.0s

## Iteration 4: The verdict and the stream

- [ ] The bar holds the verdict back while text is unsaved, a comment is unsent
      or the device is offline, as the template's guard does.
- [ ] A verdict sets the blink mark through functions `blink.ts` exports, clears
      it if the verdict fails, and moves the router to the queue once stored.
- [ ] A change request with no comments draws the refused bar, and a failed
      verdict draws the failed line in the bar with its note kept, with a story.
- [ ] The screen follows `/events?review=N` and refetches the round and the
      bodies on screen when its stamp moves, once nothing typed is unsaved, as
      the template's reload waits.
- [ ] The browser tests for the verdict, the blink, the refused change request,
      the stream and a failed verdict run against the app's screen, and each
      fails with the option off. The blink test reaches the review from the
      queue by a router move.
- [ ] The verdict guard counts a comment that is sending, as the template's did,
      since its text stayed in the form during the request, and a browser test
      sends a verdict while a comment is in flight.

## Iteration 5: The long review

- [ ] A deferred card asks for its body once it is open and within a screen of
      view, and a folded card once it is unfolded.
- [ ] A body or a held card's Load that fails draws the failed line with a
      retry, each with a story.
- [ ] A skeleton's heights are measured once on the device, as `measureRows`
      measures them.
- [ ] The card at the top of the screen stays put when a card above it changes
      height, with Chromium's scroll anchoring off, and a card folded from its
      stuck header puts its header back at the top.
- [ ] The reader's place is read when the screen mounts and saved as it scrolls,
      folds and unmounts, and a scroll before the saved place is applied writes
      nothing over it.
- [ ] The place and folds are applied after the router's scroll and again as
      bodies arrive, until a pointer down, click, wheel, touch move or key, or 3
      s, as the template's are, and a Vitest test restores a place in the
      template page's shape.
- [ ] A draft anchored in a deferred card waits for its body, as the template's
      does.
- [ ] The list of files and a link to a comment move to their target once it is
      drawn, below its card's stuck header, after the review was reached by a
      router move from a scrolled queue.
- [ ] The browser tests for bodies, held cards, the jump, the comment jump, the
      sticky header and the reader's place run against the app's screen, each
      fails with the option off, and the jump test reaches the review from a
      scrolled queue.
- [ ] The jump test is run 20 times, and its entry in specs/TODO.md records the
      result.
- [ ] The review routes answer the shell with no option, and the option is
      removed.
- [ ] A comment written on a deferred card's fetched body is placed in that body
      in the cache as it is in the round's, and a test saves one there.

## Iteration 6: The templates go

- [ ] `review.html`, `acting.html` and the parts of `file.html` only they use,
      the form and fragment routes, the template cards route and the review
      handler's template path are removed.
- [ ] The `web/ts` modules only the review template loaded are removed, and the
      template bundle still builds for the error page.
- [ ] Browser tests that used the review page as their example of a template
      page use the error page instead, and the preview sandbox test posts to the
      `/api` verdict route.
- [ ] The template markup files are committed as they last came out, with a
      comment saying so, as 030 did.

## Iteration 7: On the phone

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
