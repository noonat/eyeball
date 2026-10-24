---
status: done
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -count=1 -tags browser ./internal/server
---

# 023 Changed documents

A changed markdown document renders whole on the review page, with a rule beside
each changed passage. In a long spec the reviewer scrolls the whole document to
find a few edits, and then opens each changed passage to compare its old text
with the new by eye. The owner named this on 2026-10-23 as the thing that makes
reviewing documents hardest: the whole document renders where only the changed
sections are wanted.

The design is in [docs/product.md](../docs/product.md), under A changed document
shows what changed, and Changed words are marked in place. The mechanism and
what probing it found are in [docs/architecture.md](../docs/architecture.md),
under Marking a rendered document. The owner chose, on 2026-10-23:

- the nearest heading and one passage each side as context, over one passage
  each side alone, which leaves a change deep in a long section with no section
  name, and over the whole section, which brings a long section back for one
  edit
- a fold that opens in place, over a fold that only points at the whole file
- a list item as a passage, since `specs/TODO.md` is one list per section and
  every spec edits it
- folding and word marks in one spec

Word marks were also offered as a later spec of their own, which would have
fixed the scrolling sooner. Source as the default view was rejected, because a
paragraph is one long source line.

## Review

Spec review pass: requested, because folds, list items and word marks all change
what a passage is, and reading the first draft against the code found three gaps
the probes had not: where a list item's comments go, a changed HTML block, and a
ticked task box. The probes were then run again through eyeball's own renderer.
End-of-branch review pass: requested, because the word marks assemble HTML that
the page prints without escaping, and a mistake there breaks a page's markup or
passes text through unescaped.

The owner approved the spec as review 127 on 2026-10-23 without the spec review
pass, which the request note offered.

## Out of scope

- Opening folded lines in place in a code card. Code keeps its hunks.
- Marking a change to formatting alone, such as a word made bold. The rule shows
  it, and no word is marked.
- Removed text keeping its links and formatting. It shows as plain text.
- An item of a nested list, or a passage inside a block quote, as a passage of
  its own.
- Leaving marks off a passage that shares almost nothing with the one it
  replaced, already in specs/TODO.md, where it now covers documents too.
- Keeping the reader's place across a fold opened before a reload, which the
  item in specs/TODO.md on keeping a place by card and offset covers.

A passage comment left on a whole list before this ships matches no item's lines
afterward, so on a review still open it is listed with the file's own comments
instead of under a passage.

## Iteration 1: The tearout

```punchcard
required_acks:
  - tearout-approved
```

- [x] review.html drawing a long document's card on a second round: two changed
      passages, each under its heading with one passage of context, and fold
      lines between them giving a count
- [x] review.html drawing one fold opened in place, and the passages it held
      shown as they are in the whole document
- [x] review.html drawing word marks in a paragraph, inside a link and inside a
      code span, distinguishable without hue in both themes
- [x] review.html drawing a changed item in a long list with the other items
      folded, each item a one-item list in its own passage as architecture.md
      says, and an ordered list numbered on past a fold
- [x] review.html drawing a run replaced by a different number of passages, a
      removed passage, a ticked task box, a changed HTML block, and a passage
      changed only in formatting
- [x] foundations.html's changed-passage specimens matching, with the styles
      added to app.css and the old ones kept until iteration 3 removes them
- [x] widening.html drawing a markdown file's own page in both themes: the whole
      document with nothing folded, marked with rules and word marks as
      review.html's second round is
- [x] The tearout judged on the phone by the owner, with any change asked for
      made before the ack

The todo for the file's own page was added at this iteration's review. The owner
approved the tearout on the phone on 2026-10-24 with no changes.

> **Completed** 2026-10-24 13:51 UTC
>
> - acks: tearout-approved
> - `make check` — 20.1s
> - `go test -race ./...` — 46.6s
> - `go test -count=1 -tags browser ./internal/server` — 50.3s

## Iteration 2: Passages and folds

- [x] internal/render keying each item of a top-level list as a passage, written
      as a one-item list in its own `.blk` with its own line range
- [x] A test that one changed item in a list marks only that item, and that a
      change from a tight list to a loose one marks every item
- [x] `render.Document` folding on request: changed passages, their nearest
      heading and one passage each side shown, and runs of two or more folded
- [x] A passage whose `After` writes a comment is shown, never folded, with
      `After` called once per passage, and a test that one between two folds
      stays visible
- [x] An ordered list's items numbered by `start`, with a test that the item
      after a fold keeps its number, and a passage comment on an item quoting it
- [x] The review page's document card folding, and the file's own page and the
      fresh layout showing the whole document, each with a test

Built beyond the todos: a draft reopened on a passage inside a fold opens the
fold, in `web/ts/drafts.ts`, with a browser test. A passage with nothing saved
under it folds again on reload, and the composer reopened there was hidden.

Where the code differs from the tearout until iteration 3: a replaced passage
keeps today's details element, so a replaced list item's list sits inside its
summary. The stylesheet's rule for consecutive one-item lists does not reach it
there, and it is spaced as a list of its own.

> **Completed** 2026-10-24 14:10 UTC
>
> - `make check` — 5.9s
> - `go test -race ./...` — 44.1s
> - `go test -count=1 -tags browser ./internal/server` — 47.8s

## Iteration 3: Word marks

- [x] internal/render marking added and removed words in a passage that replaced
      exactly one other, with `<ins>` and `<del>` in its rendered HTML
- [x] The four rules in architecture.md, each with a test that fails with its
      rule removed: an entity, whitespace joining, same block tags, the task box
- [x] A test that escaped text and an attribute in the new passage come through
      byte for byte, and that removed text is written escaped
- [x] A document's own strikethrough written as `<s>` by `shownRenderer`, so
      `~~text~~` is not a removed-word mark, as architecture.md says, with a
      test
- [x] An unequal run, a removed passage and a changed HTML block showing the old
      text struck through, always visible, and a passage added with nothing
      replaced highlighted whole, as the tearout draws them
- [x] The details element, its two cues and their styles removed from
      document.go and app.css, with no class left that no page uses, and the
      amber passage state no longer cited in foundations.html or app.css

Built as follows, where the todos left it open. Words are cut by internal/diff's
own tokenizer and compared by its own comparison, exported as `diff.Words` and
`diff.Compare`, with each character reference split off first as one token. A
pair of passages past 5,000 tokens between them is shown as a rewrite rather
than compared. An unequal run's old text now goes above its first new passage,
as the tearout draws it, where the details element held it on the last. A
changed passage the comparison pairs one to one is marked word by word even when
the two share almost nothing, which the item already in specs/TODO.md covers.

Decided at iteration 1's review: a document's own strikethrough is written as
`<s>`, a removed task box is not written, a passage added with nothing replaced
is highlighted whole, and the wording that still cites the amber passage state
is corrected with the rest of the old styles. That is foundations.html's "The
dark highlighter leans toward lemon, away from the amber that marks a passage
showing what it replaced." and the dark `--hl` comment in app.css.

> **Completed** 2026-10-24 14:30 UTC
>
> - `make check` — 7.9s
> - `go test -race ./...` — 44.7s
> - `go test -count=1 -tags browser ./internal/server` — 46.6s

## Iteration 4: On the phone

```punchcard
required_acks:
  - phone-checked
```

- [x] The owner reads a review of a long changed document on the phone, with the
      daemon running this branch's build, and any change asked for is made

The owner read review 137 on the phone on 2026-10-24, with the daemon running
this branch's build at 5f10cf8: this branch's changes to docs/product.md,
docs/architecture.md and specs/TODO.md against master. It was approved with no
changes asked.

> **Completed** 2026-10-24 14:43 UTC
>
> - acks: phone-checked
> - `make check` — 19.7s
> - `go test -race ./...` — 44.0s
> - `go test -count=1 -tags browser ./internal/server` — 46.0s

## End-of-branch review pass

One review pass read the branch's diff on 2026-10-24 and re-ran its claims in a
scratch copy. It found one display defect, three guards no test exercised, and
some wording. Nothing failed at runtime. Each code fix below has a test that
fails with the fix removed. The wording and the stylesheet have none, and the
stylesheet was checked in a headless render.

- Removed words were placed by the innermost block that had opened, and a
  closing tag never returned to the enclosing one. Text removed after a code
  block inside a tight list item was written inside the code block, and text
  removed from an emptied code block between `<pre>` and `<code>`. Words are now
  placed by the stretch between one block tag and the next, after as many of its
  inline tags as preceded them. The cases are in `TestDocument_words`.
- Whitespace that only lays out block tags is copied through, so no mark lands
  between two table cells. A case changing every cell of a row now fails without
  that guard.
- Removed words with nothing new left in their stretch go where they stood in
  it. Writing them after the last token instead put them in the wrong cell. A
  case emptying one cell, and the code block cases, now fail that way.
- The 5,000-token limit had no test: raising it passed everything.
  `TestDocument_wordLimit` marks 1,240 repeated words on each side and shows
  1,260 as a rewrite.
- `TestDocument_wordRules` said it held all four rules and held three. Its
  comment now names the fourth's test.
- product.md said every changed HTML block shows its old source. Only a
  top-level one does, and the sentence now says a nested one shows the rule
  alone.
- The stylesheet's spacing for consecutive one-item lists did not reach an item
  tinted whole, whose list sits inside its tint. It does now.

A second pass checked these fixes with fuzzing and found one regression from the
new placement. Removed text that was only whitespace, in a stretch with no new
words, was trimmed to nothing and written as an empty `<del>`, a thin tinted
sliver, as in `- [x] ` turned into `- `. It is now skipped, and a case in
`TestDocument_words` fails without the skip.

Deferred to specs/TODO.md: a changed HTML block nested in a list item or a block
quote showing its old source; a block quote that held an HTML block, rewritten
as text, striking the placeholder as if it were the author's words; a task box
and its text changed together joining into one change, with a box removed from
`- [x] Done` shown only as a struck space; and, added to the item on passages
that share almost nothing, positional pairing in an equal-length run.
