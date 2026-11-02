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

### Iteration 4

- The review's screen holds its own stream, `/events?review=N` from the view's
  `stream`, and the build stream is held only while the view arrives or is
  missing. Each stamp is compared with the cached round's, once for each stamp,
  as the queue's is.
- A refetch can change a round under the reader: a verdict given elsewhere takes
  the bar and its note away and turns an open composer late. The template's
  reload waited until nothing typed was unsaved, so nothing typed went with the
  page. The app waits twice. The round is not fetched while text in a composer,
  an edit form, the review's box or the note is held, a comment is on its way or
  one did not save, and a round that was fetched is not shown until none is,
  since text may be typed while the fetch is under way. What is shown is state
  of the screen, and the reader's own writes go into it at once. `browserStream`
  records a verdict elsewhere with the review's box holding text, then the note
  alone, then the note and a composer, and finds each text and the open bar kept
  until it is cleared or sent. The composer sends its comment as the late one it
  is.
- The verdict guard counts what the template's did. Text in a composer, an edit
  form or the review's box, a comment on its way, a comment that did not save,
  and the stream being down hold a verdict back. The note does not, as the
  template's did not: it holds a refetch but not a verdict. A comment on its way
  counts because its text stayed in the template's form until the answer came,
  which `browserActingVerdict` tries with a write held in flight. The buttons
  are `aria-disabled` while held, and the handler drops the verdict as well,
  since `aria-disabled` does not stop a click.
- The verdict is sent to the address the view's `bar.action` names and
  answers 204. A 422 is the bar's refused state, and any other refusal or a
  failure to arrive is a failed line at the foot of the bar with the note kept
  and the blink mark cleared. A stale verdict's sentence is shown, and the
  stream's next stamp fetches the round.
- `blink.ts` exports `setBlinkMark` and `clearBlinkMark`, which the template's
  own handlers call too, so one module writes the key. The mark's count is the
  view's `waiting_count`, which is the number the template read from the page's
  eye. A verdict stored moves the router to the queue, which draws from its
  cache and again when the stream's fetch arrives, and the blink waits for the
  row to leave Waiting as it always did.
- The `deciding` flag the template's stream kept in `state.ts` is state of the
  screen here. While a verdict is in flight no refetch is made and none is
  shown.
- The round's bodies are not refetched with it: no card holds a fetched body
  until Iteration 5.
- The failed line in the bar is the one new state, `BarFailed` in `ReviewStates`
  and on the bar sheet, for the owner's phone check in Iteration 7.
- The five template verdict tests are the app's: `browserActingVerdict`,
  `browserFailedVerdict`, `browserBlink`, `browserRefusedChangeRequest` and
  `browserStream`. The template page's verdict has no browser test now and ships
  until Iteration 6. `browserBlink` reaches each review from the queue by a
  click that moves the router.
- A verdict stored drops the cached queue before the router moves, so the queue
  is fetched fresh. Drawn from the view cached before the verdict, it showed the
  old count and the row under Waiting first, and under reduced motion, where the
  new count shows at once, the old one showed before it. Patching the cached
  view to look as the server would is a guess. `browserBlink` holds that nothing
  the eye shows after the verdict is the old count under reduced motion, and
  that the blink starts with the old count from the mark otherwise.
- A write's answer cancels a fetch of the round already in flight and fetches
  again. A fetch that read the store before the write would otherwise take the
  comment off the screen when it arrived and the round was taken up. `fetchView`
  passes the query's signal on, so the cancelled request ends: measured in
  Chromium, a second request for an address waited 20 seconds behind a first
  that never answered, on the browser's cache lock. `browserStaleFetch` holds a
  fetch back across a write.
- A stamp whose fetch did not arrive is asked for again. The stamp asked for is
  forgotten when the connection goes offline and when the fetch fails, since the
  stream that reconnects sends the same stamp. `browserStreamFailedFetch` fails
  the fetch and its three retries, and `browserStreamReconnect` leaves a fetch
  unanswered while the stream drops and returns.
- The verdict guard is defined once, as `blocked` in the screen's hook, and the
  bar reads it as `held` for its buttons. The hook is the one place that drops a
  verdict, so a mutation of the definition or of the drop is caught by one test.

### Iteration 5

- The option is gone, and so is the template page's handler. The two review
  routes are flagged `shell` and use `handleApp` directly. With the option off
  by default nothing reached the handler's template path, so deleting it moved
  that part of Iteration 6's first todo here. The form routes, the fragments and
  the templates stay until Iteration 6. `TestServer_Surface_reviewInApp` is
  `TestServer_Surface_reviewShell`, which asks both addresses and a review that
  is not there for the shell. `TestServer_reviewViewNotFound` reads the shell at
  the page's address, where it read the template's 404.
- A premise of Iteration 2 was wrong for the end of this iteration. It held that
  the template page is served, and `make api-check` compares its markup, until
  Iteration 6. With no option no route serves the template review page, so
  `tool/fixtures` could not read it. The markup of the review pages,
  `review-*.html`, is frozen now where Iteration 6 would have frozen it: it
  stays as it last came out and is neither written nor checked, as the other
  pages' markup is, and a story still compares its component with it. The raw
  files a page points at are read from the view's rendered cards, and the set is
  the same. The answers of the form routes are still written and checked until
  Iteration 6. `tool/fixtures/markup.go` went with the page reader. Iteration
  6's last todo is done for the pages and open for what is left.
- `browserShellTemplatePage` used the review page as its example of a template
  page, which Iteration 6 would have changed. It reads the error page, from a
  raw address with nothing at it. `browserPreviewSandbox` still posts to the
  form route, which stays until Iteration 6, and loads the review page, which is
  now the app's and needs no change.
- `browserFonts`, `browserViewport`, `browserViewportRealign` and
  `browserViewportTouch` load the review page and had no option. They now get
  the app and wait for it to draw. `browserFonts` also waits for its three
  families to load, since `document.fonts.ready` resolved before the app had
  drawn any text and the test read one family.
- Fetched bodies are cached views of their own, one query for each card, keyed
  by the cards route's address with its query. The first version of this
  iteration held them in the screen's state beside the round, on the grounds
  that a fetch of the round replaces the cached view with one that has no body
  and that a body arriving through `change` would cancel a round fetch in
  flight. Review found that wrong for what it cost: server data then lived in
  two places, and a review the reader returned to drew skeletons until its
  bodies were fetched again. Bodies in their own queries answer both reasons,
  since a round fetch cannot touch a body key and a body arrives through its own
  entry. A write is an update of the shown round's own state, and of each body's
  entry from the entry's own latest value, so two answers handled in one tick
  each build on the one before. An earlier version built both from a ref fed by
  an effect, which lost the first of two answers in one tick.
  `browserBodiesTogether` and `browserWritesTogether` hold two answers let go
  together. The todo's "in the cache" is therefore met. A Vitest test places a
  comment in a body and reads it back after a newer round, and
  `browserBodiesRefreshed` does it in the browser. `browserBodiesReturn` goes to
  a file page and back with every read held, and finds the bodies drawn at once.
- Iteration 4 left the card bodies of a stream refetch to this iteration, and
  this closes it. When the round moves, every body of the review and round is
  invalidated by a predicate on its key. Active ones refetch and keep their old
  data until the new arrives, and the rest are marked stale. A verdict given
  elsewhere would otherwise leave a held body with Edit and Delete on comments
  that can no longer change. The cost is a request for each card the reader
  scrolled past. Dropping the bodies far from the reader was rejected: a body
  above the reader turning back into a skeleton moves what they are reading.
- A body read before a write of the reader's lacks the comment that write
  stored. The write's answer cancels a body's fetch in flight and fetches it
  again, as it does for the round, which replaced a counter of writes and a
  retry loop. `browserBodiesStale` holds a read across a comment. A comment on a
  passage of a fetched document invalidates that card's key, since whether a
  passage sits in a fold is the server's rule, and `passagePath` replaces
  `onPassage` to name the card. `browserFoldCommentDeferred` is
  `browserFoldComment` over a fetched document. A body's request ends when the
  screen goes, which `browserBodiesCancelled` holds.
- A held card's Load that fails draws the failed line with a Retry, which the
  template did not: its line sat after the link with no retry, and the link was
  the way to ask again. The spec asks for one on both. A deferred card's line is
  after its summary and above the skeleton, as the template drew it, and a held
  card's is in its row after the link. Neither card asks again by itself. The
  failed line component takes an optional retry, and `BodyFailed` and
  `LoadFailed` are on the large sheet for the phone in Iteration 7.
- 031's `bodies` prop of `ReviewScreen` and `fetched` prop of `ReviewCard` are
  gone. The screen merges bodies into the round, so nothing passes them.
- A draft anchored in a deferred card waits for its body with no time limit and
  does not load it. The template polled for the row for three seconds and gave
  up, and its next save then wrote the draft out without it. Here the composer
  is screen state, so the draft stays saved while its row is away, and the
  composer opens when the card comes near and the body arrives. A body far below
  the reader is not fetched for a draft. `browserDraftsDeferred` plants one on
  the 58th of 60 files. An edit form of a comment inside a body not yet fetched
  is still dropped when the screen opens, as the template dropped it. That is
  open.
- The place is saved only after the reader has acted or three seconds have
  passed, where the template's listener saved from the start. The router's
  scroll to the top and a restore that a page too short clamps would be saved
  over the place being restored. `browserRemembers` holds that no place is
  written in the first second and that the next scroll after a tap is. The
  scroll saved as the screen goes is the last one its handler saw, since the
  router may have moved the page by then.
- A jump is a router move to an address with a hash, or a page load with a hash
  and no saved place. The spec names only the router move. The template page got
  the page load from the browser, which jumped to an element in the page's HTML,
  and the app's HTML has none. A page load with a saved place keeps the place,
  as a reload after a jump would otherwise return to the card the jump named. A
  link to a comment in a card whose body is not fetched finds nothing and
  scrolls nowhere, as the template's did. `browserCommentJump` does both, from a
  scrolled queue by a notification's tap and by a page load.
- The list of files keeps `#` and the card's id as its address, which the
  stories compare, and moves the router on a plain click with the query kept.
  The query is part of the address, and a `navigate({ hash })` alone dropped it.
- `holdPlace` holds nothing at the top of the page. This is not in the template.
  The first card moved 26px when the letter's font arrived after the app drew,
  and the hold scrolled the page down by that much from the top, which
  `browserReviewScreen` read as a scroll of 26. Browsers' own anchoring is off
  at the top for the same reason. `browserHoldPlace` grows what is above the
  first card with the page at the top, where nothing moves, and scrolled, where
  the page follows.
- The skeleton's heights are measured in a layout effect when a round has a
  deferred card, on the first open card, and only when the root has no `--row-h`
  yet, which is once for a run of the app. `browserBodies` reads the three
  properties.
- Each browser test of the long review reaches the review by a page load for
  part of its steps, since a router move draws the app whatever the option said.
  `browserJump` reaches it from a queue of 15 reviews scrolled down by a router
  move and reopens it by a page load. It delays each body by 400ms: the app
  draws a body before the jump lands, where the template page's took longer, and
  no skeleton was left to count the hunks of. The jump test was run 20 times
  with `-count=20` and passed 20. The entry in specs/TODO.md says the port is a
  different test and stays open.
- The browser tests of the long review were run with the option forced off by a
  temporary edit of `handleReviewPage`, as in Iteration 3, and each failed at
  the wait for the app's page: `browserBodies`, `browserBodiesFailed`,
  `browserBodiesRefreshed`, `browserCommentJump`, `browserDocumentScrolls`,
  `browserDraftsDeferred`, `browserHeld`, `browserJump`, `browserRemembers` and
  `browserStickyHeader`. The option was then removed. The tests written after
  that, `browserBodiesStale`, `browserFoldCommentDeferred`, `browserHoldPlace`,
  `browserRemembersByMove` and `browserRemembersPastTheSkeleton`, were run with
  the two review routes pointed at the error page for one run, and with the
  other ten, 14 of 15 failed at the same wait. `browserRemembersByMove` passed,
  since it entered by a move of the router, which draws the app whatever a page
  load answers. It now starts from a page load. `browserJump` reaches the review
  by a router move first and fails at its page load. The error page stands in
  for the template page there, which is weaker than the page itself, since a
  test that waits for the app's page fails on any page that is not the app's. A
  second run did the same for `browserFonts`, `browserViewport`,
  `browserViewportRealign` and `browserViewportTouch`, which now wait for the
  app, and for `browserRemembersByMove` from its page load, and all five failed.
- Mutation check. 29 violations were planted one at a time, each edit checked to
  have landed, and each run against the long review's browser tests or the
  Vitest tests. Caught first time: the margin of the observer
  (`browserCommentJump`), a folded card that loads (`browserBodies`), a fold not
  reported (`browserBodies`, `browserRemembers`), no hold (`browserHoldPlace`,
  `browserJump`), no fold fix (`browserStickyHeader`), a hold at the top
  (`browserHoldPlace`, `browserReviewScreen`), Chromium's anchoring left on
  (`browserJump`, `browserStickyHeader`), no measuring (`browserBodies`,
  `browserJump`), the place saved at once (`browserRemembers`), no save on
  pagehide (`browserRemembers`), folds not seeded (`browserRemembers`), no jump,
  and a page load's hash ignored (`browserCommentJump`), the plain anchor in the
  list of files (`browserJump`), no read again on a stamp
  (`browserBodiesRefreshed`, `browserBodiesStale`), no read again after a
  passage comment (`browserFoldCommentDeferred`), no Retry
  (`browserBodiesFailed`), a page's fold counting as the reader's
  (`browserStickyHeader`), a click that does not end the restore
  (`browserJump`), no skeleton counts (`browserJump`, `browserRemembers`,
  `browserStickyHeader`), and four in the Vitest tests: a place with no number,
  the key of a place, a body kept in the round and a merge that overwrites a
  card's own body. Five survived or were missed, and each got a test. A body
  read again after a write was caught only once `browserBodiesStale` held the
  first card's read, where it first held whichever card asked first. No save as
  the screen goes survived, and `browserRemembersByMove` leaves before the
  scroll's own save is due. No set again as bodies arrive survived, and
  `browserRemembersPastTheSkeleton` leaves the reader below what the skeletons
  measure, in front of a document taller than its skeleton. A failure kept after
  Retry survived, since the body replaced the line anyway, and
  `browserBodiesFailed` holds the read so the line is seen to go while it waits.
  A settle time of zero survived, since no scroll came after it, and
  `browserRemembers` now scrolls inside and then outside the three seconds. The
  restore in a layout effect, which runs before the router's scroll, was caught
  by `browserRemembersByMove`, which returns to a cached view.
- Mutation check of the bodies as queries. Review moved the bodies from screen
  state to queries of their own, which replaced the code the plain-state
  mutations of a body's read and its re-read touched, so those were planted
  again against the new code, 11 violations one at a time. A query never
  enabled, and one not enabled for a card the cache holds, failed
  `browserBodies` and the others that fetch, and `browserBodiesReturn`. The
  query's signal dropped failed `browserBodiesCancelled`. No refetch on a stamp
  failed `browserBodiesRefreshed` and `browserBodiesStale`, a wrong round in the
  predicate the same two, no re-read after a passage comment
  `browserFoldCommentDeferred`, a body not written back to its entry
  `browserBodiesRefreshed`, a fetch in flight not cancelled by a write
  `browserBodiesStale`, and retries left on `browserBodiesFailed`. A key that
  ignores the query survived the first tests, and `browserBodiesAgainst` holds a
  body of the other comparison from being drawn. A guard that kept a failure
  from showing while its card asked again survived and was removed: a query with
  no data starts over, so it guarded nothing a reader could see.

### Iteration 6

Most of its first, third and last todos were done in Iteration 5, which the
Departures there record: the review handler's template path,
`browserShellTemplatePage` on the error page and the frozen `review-*.html`
pages. Each was checked before it was marked.

- `review.html` and `acting.html` are deleted. `file.html` keeps `notice`, which
  `error.html` reaches, and loses `rendered` and `modeline`, which only the
  review template used. The file keeps its name. A rename to `notice.html` was
  not made, since it would change nothing a reader could see.
- The routes gone are `POST`, `PUT` and `DELETE /reviews/{review}/comments`,
  `GET /reviews/{review}/comments/{comment}`, `GET /reviews/{review}/composer`,
  `POST /reviews/{review}/decision` and the template cards route, each with its
  handler. What only they used went too: the composer, the fragment and the form
  readers, the tally's out-of-band flag, the composer and chip types, and the
  `Composer` of the acting view. `storeComment`, `newComment`, `changeComment`,
  `removeComment`, `recordVerdict` and the checks stay, since the JSON routes
  call them. `barView` keeps an action and a count. `tally` returns the count.
- The document's markup is no longer joined with a thread after each passage,
  which was the template's. Its one other effect was to mark which comments a
  passage placed, so a comment whose range is no passage is listed with the
  others. That is now done by walking the document's parts, and the review tests
  for placement pass unchanged.
- The tests of the form routes are deleted:
  `TestServer_Surface_actingFailureIsPlain`, `addComment`, `addCommentLate`,
  `addCommentRefused`, `addCommentRepeated`, `comment`, `composer`,
  `composerDecided`, `decide` and `editAndDeleteComment`, and
  `TestServer_addCommentJSONRefusedLikeForm`. Each fact they held is held by a
  JSON test that existed before: the stored fields of each level by
  `addCommentJSON`, the refusals by `addCommentJSONRefused`, a key sent twice by
  `addCommentJSONSubmission`, an edit or delete of another review's comment, of
  one already gone and of a released one by `editCommentJSON`,
  `deleteCommentJSON` and `editCommentJSONGone`, and the verdict's refusals and
  stale round by `decideJSON`. The sentences a composer shows on a decided round
  are the components' own, held by the stories and by `composerText`. The form
  block inside `addCommentJSONSubmission` and `editCommentJSONGone` is gone, and
  the four form entries in `refusesEveryWrite` with them. The daemon's loopback
  test sends its comment and verdict to the JSON routes.
- `TestServer_Surface_reviewPassageAcrossTwo` sends a passage comment on lines
  that span two passages of a document, which the JSON route accepts since it
  checks only that the lines exist. The comment is listed once with the file's
  comments and sits under no passage, and a comment on exactly one passage is
  under it and not listed. Leaving out the walk that places comments, and
  placing every comment of a document, each fail it.
- `TestServer_Surface_removedTemplateRoutes` holds each removed address to a 404
  that stores nothing, and `TestServer_Surface_templateBundle` holds the
  template bundle free of the review page's scripts.
- `tool/fixtures` no longer writes the answers of the form and fragment routes,
  `review-*.html` and `review-*.txt` beside the pages, or `composers.ts`, which
  was built from the queries of those requests. All are kept as they last came
  out, and a story and the Vitest test of the composer's request still compare
  with them. The header of `main.go` says so, as for the pages, and
  `composers.ts` says so in its own. It is formatted again now that nothing
  writes it, so the Makefile no longer excludes it. The `forms` review that the
  writes were tried on is still seeded, since removing it renumbers the reviews
  after it and every address in the fixtures that holds one.
- `web/ts/composer.ts`, `drafts.ts`, `unsent.ts`, `bodies.ts`, `place.ts`,
  `failures.ts`, `events.ts` and `state.ts` are deleted. Each was imported by
  `main.ts` or by another of them and by no file of the app. What the template
  bundle still builds is the theme, the token, the worker, the update banner,
  the stream, the viewport fix, the tap, the head and the blink read. The
  functions that only the deleted modules called went with them: `sendToken`,
  `keepBlink`, the stream's reload and its stamp, `unsavedText`, `isDeciding`,
  and `setOffline`, whose only effect was an event nothing listens to, and every
  entry of `classes` but `sel`. In Go, the `refuser` type and the `refuse`
  parameter of `writeReview`, `findRound` and `reviewComment` are gone, since
  the JSON routes were the one caller left, and `sendForm` in the tests lost its
  htmx flag. The `globals.d.ts` declaration of `htmx` stays, since the vendored
  file it names is htmx's removal, which is the next item. So do the htmx
  branches of the refusal and the failure page, and `render.Join`'s after hook,
  which the render tests use, and the `/static/htmx.js` route. specs/TODO.md
  lists them under the htmx item.
- The class check of the templates, `template-classes`, now reads `layout.html`,
  `error.html` and the notice. Every class the review's two templates used is
  read by a component, a script or a remaining template, except the `i-*` names
  of the icons, which `Icon` builds as `i-${name}` and `icons-listed` holds to
  the stylesheet. No class of the two templates is left in `app.css` with no
  reader, so no CSS was removed. The review's markup is held by
  `component-classes`.
- `browserPreviewSandbox` posts a JSON body to `/api/reviews/{review}/decision`,
  which the sandboxed frame sends without a token and the surface answers 403.
- Nothing reaches what was removed: a search of the source, the docs, the
  Makefile and `internal/convention` finds no name of a removed handler, module
  or route outside the specs and the frozen fixtures. Each route's return was
  planted, as `handleApp` for `POST` comments, the composer, a decision and the
  cards route, and each failed `TestServer_Surface_removedTemplateRoutes`. A
  string of the removed scripts planted in `main.ts`, `hx-get` and `keepPlace`,
  failed `TestServer_Surface_templateBundle`. architecture.md and conventions.md
  are corrected where they named the form routes or the review scripts.

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

- [x] The bar holds the verdict back while text is unsaved, a comment is unsent
      or the device is offline, as the template's guard does.
- [x] A verdict sets the blink mark through functions `blink.ts` exports, clears
      it if the verdict fails, and moves the router to the queue once stored.
- [x] A change request with no comments draws the refused bar, and a failed
      verdict draws the failed line in the bar with its note kept, with a story.
- [x] The screen follows `/events?review=N` and refetches the round and the
      bodies on screen when its stamp moves, once nothing typed is unsaved, as
      the template's reload waits.
- [x] The browser tests for the verdict, the blink, the refused change request,
      the stream and a failed verdict run against the app's screen, and each
      fails with the option off. The blink test reaches the review from the
      queue by a router move.
- [x] The verdict guard counts a comment that is sending, as the template's did,
      since its text stayed in the form during the request, and a browser test
      sends a verdict while a comment is in flight.

> **Completed** 2026-11-02 00:54 UTC
>
> - `make check` — 35.7s
> - `go test -race ./...` — 59.1s
> - `go test -count=1 -tags browser ./internal/server` — 79.3s

## Iteration 5: The long review

- [x] A deferred card asks for its body once it is open and within a screen of
      view, and a folded card once it is unfolded.
- [x] A body or a held card's Load that fails draws the failed line with a
      retry, each with a story.
- [x] A skeleton's heights are measured once on the device, as `measureRows`
      measures them.
- [x] The card at the top of the screen stays put when a card above it changes
      height, with Chromium's scroll anchoring off, and a card folded from its
      stuck header puts its header back at the top.
- [x] The reader's place is read when the screen mounts and saved as it scrolls,
      folds and unmounts, and a scroll before the saved place is applied writes
      nothing over it.
- [x] The place and folds are applied after the router's scroll and again as
      bodies arrive, until a pointer down, click, wheel, touch move or key, or 3
      s, as the template's are, and a Vitest test restores a place in the
      template page's shape.
- [x] A draft anchored in a deferred card waits for its body, as the template's
      does.
- [x] The list of files and a link to a comment move to their target once it is
      drawn, below its card's stuck header, after the review was reached by a
      router move from a scrolled queue.
- [x] The browser tests for bodies, held cards, the jump, the comment jump, the
      sticky header and the reader's place run against the app's screen, each
      fails with the option off, and the jump test reaches the review from a
      scrolled queue.
- [x] The jump test is run 20 times, and its entry in specs/TODO.md records the
      result.
- [x] The review routes answer the shell with no option, and the option is
      removed.
- [x] A comment written on a deferred card's fetched body is placed in that body
      in the cache as it is in the round's, and a test saves one there.

> **Completed** 2026-11-02 02:06 UTC
>
> - `make check` — 20.6s
> - `go test -race ./...` — 58.3s
> - `go test -count=1 -tags browser ./internal/server` — 108.2s

## Iteration 6: The templates go

- [x] `review.html`, `acting.html` and the parts of `file.html` only they use,
      the form and fragment routes, the template cards route and the review
      handler's template path are removed.
- [x] The `web/ts` modules only the review template loaded are removed, and the
      template bundle still builds for the error page.
- [x] Browser tests that used the review page as their example of a template
      page use the error page instead, and the preview sandbox test posts to the
      `/api` verdict route.
- [x] The template markup files are committed as they last came out, with a
      comment saying so, as 030 did.

> **Completed** 2026-11-02 02:48 UTC
>
> - `make check` — 20.2s
> - `go test -race ./...` — 53.0s
> - `go test -count=1 -tags browser ./internal/server` — 110.8s

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
- [ ] A review opened at the top of the page, with the letter's font arriving,
      moves nothing on the phone.
- [ ] The first draw and a fling on a long review are measured as the probe
      measured them, and recorded beside the probe's numbers.
