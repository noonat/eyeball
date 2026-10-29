---
status: active
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -count=1 -tags browser ./internal/server
---

# 025 After the verdict

Three things the owner ran into on the phone, all on the review page around the
verdict. Submitting a verdict reloads the page to the decided round, where the
owner wanted to be back on the queue. On a decided round a tap on a line does
nothing, and the owner spent a while tapping lines on approved reviews before
finding that only open rounds open a composer. The decision bar's notice while
eyeball cannot be reached wraps to two lines on the phone.

The design is in [docs/product.md](../docs/product.md), under Deciding, changed
in this spec's first commit.

**A verdict returns to the queue.** `handleDecide` answers `HX-Refresh` today,
and will answer `HX-Redirect: /`. `HX-Location` was rejected because it swaps
the queue in without a page load, so `blinkOnLoad`, which runs on load, never
would. The confirmation is the eye blinking to its new count and the review's
row out of Waiting, the owner's choice over a one-line notice on the queue,
which would be a new part of the queue page. Today only Approve blinks.
Requesting changes lowers the waiting count too, so both verdicts blink. The
guard against a stale mark moves with the blink: on the queue the eye blinks
when the marked review has a row and that row is not under Waiting. Approved
reviews land in Settled and changes requested in Working, both read from
`queue.go`.

**A decided round takes a tap.** product.md already allowed a comment on a
closed round, and the server already stores one at any level as late and sends
it at once (`handleAddComment`, and the level-agnostic `lateExpr` in
`internal/store/delivery.go`). Only the client stops it: `acting()` in
`web/ts/composer.ts` returns nothing unless `data-acting` is `open`. The owner
chose taps over a sign that lines are closed, since a sign would explain a limit
product.md never asked for. `acting()` also gates `keepDrafts`, and a late
comment is sent, never drafted, so taps get their own check and `acting()` stays
as it is. A composer opened on a decided round reads "Sends immediately" and
"Send", as the late review box does. `handleComposer` builds every anchored
composer as "Saved as you type" and "Add" today, with no knowledge of the
round's state, so it has to learn it.

**The offline notice fits one line.** The sentence is "A verdict needs eyeball.
Both become available when the connection is back." The owner offered a shorter
sentence or a layout that allows two lines. A shorter sentence is proposed,
because the narrow layout already puts the notice on a line of its own above the
buttons (`.decide .why-not` in `app.css`), and a second line makes the pinned
bar taller over the page it covers. "Offline. A verdict needs eyeball." is the
starting candidate, and the tearout settles the words.

## Review

Spec review pass: declined, because the spec changes no schema and no write, and
every server premise it rests on was read in the code. End-of-branch review
pass: declined, because the review agent checks each iteration and the browser
tests below fail with the old behavior restored.

## Out of scope

- Taps on an earlier round, one a newer round has replaced. Its page is built
  with no acting view, so it offers no composer at all. Added to specs/TODO.md.
- Drafts for a late comment. It is sent at once, so there is nothing to keep.
- A notice on the queue naming the verdict, which the owner declined.
- A phone check of the built screens. The tearout is judged on the phone, and
  the browser tests cover the redirect, the blink and the tap.

## Iteration 1: The tearout

```punchcard
required_acks:
  - tearout-approved
```

Corrections found while building the tearout:

- The opening says the narrow layout already puts the notice on a line of its
  own above the buttons. That was wrong.
  `.decide .why-not { order: -1; flex-basis: 100% }` sits inside
  `@media (min-width: 720px)`, so on the phone the notice sits under the
  buttons, and above them only at 720px and wider. The shorter sentence is still
  what fixes the wrap. Measured at 390px: 172px of text in a 356px box, one
  line, where the old sentence took two.
- The decided-round composer frame draws the hint and the level chips together.
  `composerform` in `web/templates/acting.html` draws one or the other
  (`{{if .Chips}}...{{else}}hint{{end}}`). Iteration 3 has to change
  `composerform` to draw both, not only set Hint as `acting()` does for the
  review box.

- [x] review.html's Approve section landing on a queue.html frame after the tap,
      with its paragraph and notes saying the blink plays on the queue
- [x] queue.html frame after a verdict: the review's row under Settled, the eye
      at its new count, and a note that a changes request moves it to Working
- [x] review.html frame of a decided round with a line tapped, its composer
      reading "Sends immediately" and "Send", and a late comment under a line
- [x] The offline notice reworded in foundations.html, rendered headless at 390
      pixels in both themes on one line, with the width measured
- [x] docs/design/README.md's table naming the new frames and no reload
- [ ] The tearout judged on the phone by the owner, with any change asked for
      made before the ack

## Iteration 2: Back to the queue

- [ ] `handleDecide` answering `HX-Redirect: /` and no `HX-Refresh`, with
      acting_test.go asserting both, failing with `HX-Refresh` restored
- [ ] blink.ts marking both verdicts, and blinking on the queue when the marked
      review's row is outside Waiting, its comments saying why
- [ ] The browser test after Approve finding the location `/`, the eye blinking
      once and the mark cleared, failing with the reload restored
- [ ] A browser test that Request changes lands on the queue and blinks
- [ ] The stale-mark browser test kept: a mark naming a review still under
      Waiting does not blink on the queue

## Iteration 3: Taps on a decided round

- [ ] A tap on a line or passage of a decided current round opening its
      composer, through a check of its own beside `acting()`
- [ ] `keepDrafts` still saving nothing on a decided round, with a browser test
      that fails if `acting()` is widened instead
- [ ] `handleComposer` reading the round's state, and every composer on a
      decided round reading "Sends immediately" and "Send", tested per level
- [ ] A file-level late comment reachable from a decided round's file card, as
      the open round's is
- [ ] A late line comment shown under its line marked "after the verdict", with
      no Edit or Delete, tested in review_test.go
- [ ] A browser test: tap a line on an approved review, send, and find the
      comment delivered to the agent as late

## Iteration 4: The notice

- [ ] web/templates/acting.html carrying the sentence the tearout settled, and
      the browser test at the offline bar asserting its text
