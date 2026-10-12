---
status: active
created: 2026-10-11T00:00:00Z
updated: 2026-10-12T00:16:04.703998275Z
required_commands:
  - cmd: make check
---

# 013 Two directions for how eyeball looks

The owner said on 2026-10-11 that the surface looks good and is not enjoyable to
use, and reads as generic. This spec draws two directions for its visual
identity as tearout sheets, so the owner can judge them on the phone beside the
current design and choose. It ends at that choice. Applying the chosen direction
to every tearout, `app.css` and the templates is a later spec, because its size
depends on which direction wins and how much markup it changes. This was not on
specs/TODO.md's Next list, which was empty.

## Why it reads as generic

Almost nothing in the current look was chosen. Each part is a sensible default,
and together they are the stock issue-tracker kit:

- **No typeface.** `--prose` is the system stack, which on an iPhone is SF Pro,
  the face of Settings and Reminders. Hierarchy comes from weight alone (580,
  620, 640) on a scale of 11 to 21 pixels, so nothing on a screen leads.
- **Nothing of the product's subject.** The wordmark is 17px bold system text.
  The product is one person's attention and a verdict that releases an agent,
  and no shape, color or behavior says so.
- **Two themes at two temperatures.** Light is a warm off-white with a stock
  link blue, `#2b52c9`. Dark is a cool slate, `#131519`, with periwinkle.
- **Template habits.** One 10px rounded card for every kind of thing, tracked
  all-caps section heads, meta joined with middle dots, monospace for labels
  that are not code (`Note`), and a dot inside every pill.
- **Waiting does not lead.** product.md says waiting is the top of the screen
  and nothing competes with it. A settled row has the same card, weight and size
  as a waiting one, and five state hues share the screen.
- **The payoff has no moment.** Approving releases an agent, and nothing marks
  it. The empty queue, the state a reviewer is working toward, is a dashed box.

## Design

### What both directions share

The template habits are fixed the same way in both sheets, so the two differ
only in type, color and what leads. A comparison of either sheet against the
current one mixes these shared fixes with the direction itself, and the notes
under each sheet say so.

- Section heads in sentence case, at a size that reads as a heading.
- No middle-dot separators, no monospace outside code, paths and agent names,
  and no dot inside a pill.
- Radius and weight by importance: waiting rows are cards, working and settled
  rows are lines between hairlines.
- One neutral family per direction, at one temperature in both themes.
- One moment on Approve, answering the tap, which becomes an instant change of
  state under `prefers-reduced-motion`. A tearout has no script, so the moment
  is a `:target` frame that plays when its link is followed, as Expand already
  works.

  > **Departure, before iteration 1.** The approved line read "and still under
  > `prefers-reduced-motion`", which says the motion plays when a reader has
  > asked for none. The intent was the reverse.

- Everything that is already right stays: teal and clay for add and remove, two
  tones per hue, hue never the only signal, the phone layout as the design, and
  the code block owning its overflow.

### A: The reading desk

Identity from reading and marking up text.

- **Type.** Atkinson Hyperlegible Next for everything, and Atkinson Hyperlegible
  Mono for code, paths and agent names. The Braille Institute drew it for
  low-vision legibility, and it separates `l`, `I` and `1`, and `0` and `O`,
  which an agent name or a path needs. Scale 12, 14, 16, 20, 26, with 16 as the
  body of a document.
- **Color.** One attention color, a highlighter yellow, used only as a fill
  behind ink: the waiting rows, the waiting count, and a line or passage
  selected for a comment, where it replaces the 2px blue outline. Working and
  settled fall back to ink tones. Links are underlined ink. Starting tokens:
  paper `#f3f4f1`, ink `#1a1c22`, highlighter `#ffe14d` with text tone
  `#6b5300`. Dark: paper `#17181c`, highlighter `#e8c93a`.
- **The one bold element.** An eye in the top bar that shows the queue's state:
  open, with the waiting count as its pupil, while anything waits, and closed
  when nothing does. Approve plays a blink. It is drawn in the direction's own
  stylesheet under a class outside the `i-` icon set, so the shipped mask table
  in `app.css` does not change.

### B: Letters from agents

Identity from the agent's note, which product.md calls the reviewer's brief.

- **Type.** Literata for the agent's words and for rendered documents, a text
  face TypeTogether drew for reading on screens. The chrome keeps the system
  sans, so the serif marks speech and reading and nothing else.
- **Lead.** On the review page the note comes first, at reading size, signed
  with the agent's name. A queue row carries the note's first line under its
  title, so a row says what the agent wants as well as what it changed. The
  queue does not have that line today; the sheet's fixture supplies it, and
  serving it belongs to the later spec.
- **Color.** Ink on a cool paper, mostly monochrome, with an accent that is not
  blue, since blue is what the current design uses and a blue accent would blur
  the comparison. Starting tokens: paper `#eef1f4`, ink `#1d2430`, accent an
  iron-gall violet near `#4a3aa8`. Dark: paper `#161a20`, ink `#dfe4ea`.
- **Risk.** A serif on warm paper is one of the most common generated looks, and
  the removal clay sits near its usual accent. The paper stays cool for that
  reason.

### How a direction is drawn

Each direction is a stylesheet beside `app.css`, `desk.css` and `letters.css`,
loaded after it, holding token overrides and the rules the direction adds. Each
has a sheet, `desk.html` and `letters.html`, linking `tearout.css`, `app.css`
and its own stylesheet. The fonts live under `docs/design/fonts/` with their
`OFL.txt`, as Latin-subset woff2.

> **Departure, before iteration 1.** The approved spec missed that Expand and
> the Approve moment both use `:target`, and a page has one target at a time.
> Following Approve inside an expanded view would close the view. So
> `tearout.css` shows `.full` under `:has(:target)` as well as `:target`, and an
> Approve link inside an expanded view targets an id inside it.

The rejected alternative was editing `app.css` and judging on the live surface.
`app.css` is embedded in the binary, so an exploratory edit reaches the surface
on the next build, and only one direction could be seen at a time. Copying all
five sheets per direction was also rejected: about 3,600 lines of fixture each,
mostly states that test layout rather than identity.

Each sheet draws the same frames, in both themes, with fixture content copied
from queue.html and review.html so the three compare row for row: the populated
queue, the queue with nothing waiting, a second round of a document, a large
code change with a line selected and the composer open, and the Approve moment.

The check change: `ReadTearout` joins only `app.css` and `tearout.css`, so a
class defined in `desk.css` reads as undefined. It will join every `*.css` in
the directory, and `AppCSS` stays `app.css` alone, which `template-classes`
reads, so a template still cannot use a class that does not ship. Checking each
page against the stylesheets it links was rejected: no fixture page under
`internal/convention/testdata` carries a `<link>`, so every fixture's findings
would change. The cost is that a direction's class used on queue.html passes.

### Measured and assumed

- Measured: headless Chromium loads a font from `file://` in a subdirectory of
  the page, and Go's `http.FileServer`, which `tool/tearout` uses, serves
  `.woff2` as `font/woff2` by extension and by sniffing.
- Assumed: iOS Safari loads the same fonts through `tool/tearout`. Iteration 4
  confirms it on the phone.
- Assumed: Atkinson Hyperlegible Mono is OFL, as Next and Literata are.
  Confirmed from its `OFL.txt` on download. If it is not, IBM Plex Mono replaces
  it.
- Not measured: the woff2 sizes. Recorded when the files are added.

  > **Measured, iteration 2.** The Mono is OFL-1.1 and variable over 200 to 800,
  > per its package's LICENSE and metadata. The desk's Latin woff2 files are 34,
  > 38, 18 and 19 KB. Literata's are 52 KB upright and 54 KB italic.

- Not measured: contrast of the highlighter tones. Measured in iteration 2.
- The current sheets' headless screenshots render Linux fallback fonts rather
  than SF Pro, so a fair comparison with the current design is on the phone.

## Review

Spec review pass: declined, because the code change is one check with a fixture,
and the rest is a question of taste only the owner can judge on the phone.

End-of-branch review pass: declined, for the same reason, and because the owner
reads both sheets on the phone before the branch is reviewed.

## Out of scope

- Applying the chosen direction to app.css, every tearout and the templates. The
  later spec, sized by the choice.
- Serving fonts from the binary: a route under `/static/` and its caching.
- The installed page's icon redrawn to match, through `tool/appicon`.
- The queue row's note line in direction B, served for real.

## Iteration 1: Stylesheets beside app.css

> **Departure.** `css-declarations` and `icons-listed` also read the joined
> stylesheets, and the spec did not name them. Both describe `app.css`, and a
> direction's stylesheet redeclares `--ink:` and the other tokens, so a joined
> read would hide `app.css` losing one. Both now read `AppCSS`, with a test
> each.

- [x] `ReadTearout` joins every `*.css` in the tearout directory, with `AppCSS`
      still `app.css` alone
- [x] A `tearout-classes` fixture whose class is defined only in a third
      stylesheet, passing, beside its existing failing case
- [x] docs/conventions.md's `tearout-classes` entry naming every stylesheet in
      the directory

> **Completed** 2026-10-12 00:19 UTC
>
> - `make check` — 5.3s

## Iteration 2: The reading desk

> **Departure.** The shared fixes live in a third stylesheet, `directions.css`,
> which both sheets link between `app.css` and their own, rather than in
> `desk.css` and a copy in `letters.css`. One file makes the two sheets' shared
> rules identical by construction. It stays until the spec that applies the
> choice. The fonts came from Fontsource 5.3.0's npm packages, whose LICENSE
> files are the OFL texts, and the desk's four files come to 109 KB. Contrast
> found one shortfall that is not this direction's: `app.css`'s light Approve
> green holds white text at 3.53 to 1. The desk darkens it, and specs/TODO.md
> holds the fix for the shipped surface.

- [x] Atkinson Hyperlegible Next and Mono under `docs/design/fonts/` as Latin
      woff2 with their `OFL.txt`, and their sizes written in the sheet's notes
- [x] `desk.css` holding the shared fixes, the tokens in both themes, the type
      scale and the eye
- [x] `desk.html` drawing the five frames in both themes, with Expand on each
      pair, and notes saying why under each
- [x] Contrast of every text tone against its background measured, and each
      below 4.5 to 1 changed or named in the notes
- [x] Every frame and every expanded view screenshotted headless and looked at
      before the iteration closes

> **Completed** 2026-10-12 00:31 UTC
>
> - `make check` — 5.4s

## Iteration 3: Letters from agents

- [ ] Literata under `docs/design/fonts/` as Latin woff2 with its `OFL.txt`, and
      its size written in the sheet's notes
- [ ] `letters.css` holding the same shared fixes as `desk.css`, the tokens in
      both themes, and the note as the lead
- [ ] `letters.html` drawing the same five frames in both themes, with the same
      fixture content, Expand, and notes
- [ ] Contrast measured as in iteration 2, and every frame and expanded view
      screenshotted and looked at
- [ ] README's table of tearouts and every sheet's navigation naming both
      directions as exploratory, and its two-shared-stylesheets rule naming the
      direction stylesheets

## Iteration 4: On the phone

```punchcard
required_acks:
  - chosen
```

The owner opens queue.html, review.html, desk.html and letters.html through
`go run ./tool/tearout` and chooses: one direction, a mix with which parts come
from which, or neither.

> **Departure, before iteration 1.** Two todos changed from the approved spec.
> The removal todo now says what happens on a mix, which it did not. The TODO
> todo moves the Unscheduled entry this spec added, where the approved wording
> would have written a second copy. Iteration 3's README todo also gained the
> README's rule that every tearout links two shared stylesheets, which these
> sheets break and no todo corrected.

- [ ] The fonts confirmed loading in iOS Safari, or the failure recorded here
- [ ] The choice and its reason recorded in docs/design/README.md
- [ ] The sheet, stylesheet and fonts of each direction not chosen removed. On a
      mix, both sheets stay until the spec that applies the choice
- [ ] specs/TODO.md's Unscheduled entry for applying the choice moved to Next,
      naming what was chosen
