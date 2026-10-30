# Design tearouts

A **tearout** is a single screen torn out for inspection: the screen's real
components on fixture data, with every state that matters as a story in
Storybook. Look-and-feel is settled there, before any plumbing.

**Design comes first.** Any work with a user-visible surface opens with a
tearout, before any plumbing. This is an ordering, not a preference: find a
design worth building, then build it. A change to a surface that already exists
starts by changing the stories it was approved against.

**The tearout is the component that ships.** The first tearouts were static HTML
and CSS, and the templates were seeded from their markup. Nothing kept the two
in step afterward, and [specs/TODO.md](../../specs/TODO.md) holds five places
where an approved frame and the shipped screen disagree, and six states the
shipped screen draws that no frame does. A story renders the component itself,
so there is no second copy to drift. What this gives up is a page that opens
from the filesystem with no build step and no JavaScript. Storybook needs Node
and a build, both development only.

**Fixtures are views the server produced.** `tool/seed` writes a populated state
directory, and the fixtures are the JSON the API returns for it, so a story
shows the shapes and lengths the real surface gets. A fixture written by hand
for a story is a guess at the API, and the API's types already say what it
sends.

## Judge it on the phone

eyeball is read on a phone in whatever minutes are available, so a desktop
browser at 390 pixels is not where this gets judged.

```sh
go run ./tool/tearout               # builds Storybook and prints the tailnet URLs to open
```

**Each screen has a sheet story**: its states in 390 by 800 frames, a real
iPhone viewport rather than a shrunken preview, each light frame beside its dark
one. Every frame links to the state's own story at full screen. Storybook's own
interface is cramped on a phone, and typing a story's address there is the step
that gets skipped, so nothing on the phone needs it.

**The foundations sheet is a specimen page, like `foundations.html`.** It has
one section per primitive, with the explanation that goes with it, and every
state of the primitive together in one panel, light beside dark. A primitive is
judged beside its other states and its reasons, and alone in a phone frame it is
a small mark on a large empty screen. Under each pair a small link per state
opens that state at full screen, so the phone still needs no typed address. The
phone frame is for a whole screen, where the frame is the thing being judged.

`python3 -m http.server` is the wrong tool, and so is Storybook's dev server
reached across the tailnet. iOS Safari shows a stale copy of a page that sends
only `Last-Modified`, which reads as "the change did not work" rather than as a
cache. `tool/tearout` sends `Cache-Control: no-store` and binds the tailnet
address, so the page opens on a phone that is not on this LAN.

**The wide layout gets its own story**, since it cannot be shown beside a second
copy of itself. Open it before calling a tearout done.

## The tearouts

| Tearout                                  | Covers                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| ---------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [`foundations.html`](./foundations.html) | The shell, type scale, palette in both themes, and every reused primitive: state pills and rows, the eye, the highlighter and the letter, the unified diff with word marks and its five syntax roles, rendered prose with changed-passage rules, word marks and folds, comment cards and the composer, level chips, buttons, the decision bar, notices, empty state, first-run steps, the icon badge                                                                                                                                                                                                                                                                    |
| [`queue.html`](./queue.html)             | The home screen: populated across three projects, as it reads after a verdict, nothing waiting, not installed yet, installed with notifications not yet allowed, a lapsed subscription, and the awkward week that breaks it                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| [`review.html`](./review.html)           | The review page: a second round of a long document folded to what changed, a fold opened, a changed list item, a rewrite, a removal, a ticked box and a changed HTML block, a large change with every file open under a list of them, one folded and one deferred, with a comment being written, a first round against an empty base, a refused verdict, a round already decided, a line tapped on a decided round, Approve and the queue it returns to, a passage selected for a comment, the review box and the verdict note, a verdict that could not be sent, rendered files in their cards, files that changed mode, and an earlier round with the agent's replies |
| [`widening.html`](./widening.html)       | Three steps out from a hunk: the whole file with the change still marked, a document's own page whole and marked, any file in the project at that round, the tree, a file too big to render, a page in its sandboxed frame, a changed image before over after, a file that changed mode, and new and deleted images, SVG, video, audio and a PDF                                                                                                                                                                                                                                                                                                                        |
| [`project.html`](./project.html)         | One project: its open reviews over its live working copy, a main checkout beside a worktree, a clean tree, and a directory git cannot see                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |

These are the static sheets every screen was approved against. They are the
reference while the surface moves to React. A screen's sheet is deleted once its
stories are approved against it on the phone, and the stories are the tearout
from then on. A new screen gets stories before it gets code.

## The visual direction

The visual language is the reading desk, with the agent's note as a letter. Spec
013 drew two directions against these fixtures, and the owner chose between them
on the phone on 2026-10-11. Spec 015 applied the choice to `app.css`, these
sheets and the templates.

The reading desk won over letters from agents for the reasons the owner gave.
Less color on the screen was less distracting. The highlight was a great idea.
The eye in the top bar was excellent, especially closing when nothing waits and
blinking on Approve. Letters kept the earlier design's state hues and spent its
one new color, a violet, on links and selection.

Seen on the desk's paper, the letter was preferred to the desk alone. So the
note leads the review page in Literata, signed, and its first paragraph is on
the queue row. Letters' violet tokens and its reply on Approve were dropped. Git
holds every sheet as it was judged, under spec 013's commits.

## How to work on these

- **The components' stylesheets are the only styling a story uses.** Each
  component imports its own, and the tokens and the reading-desk language are
  shared by all of them, as `app.css` is today. The sheet's frames and labels
  are chrome with a stylesheet of their own, which nothing in a component may
  use. A style written for one story is a style the real surface does not have.

  The fonts live beside the stylesheets and are served at `/static/fonts/`. Each
  family's license sits beside its files.

- **Both themes, side by side.** Tokens are defined on `[data-theme="light"]`
  and `[data-theme="dark"]`, so a tearout stamps the attribute on each frame and
  shows both at once instead of depending on the viewer's OS setting. The real
  surface puts `data-theme` on `<html>`, defaulting from `prefers-color-scheme`
  with a manual override that wins in both directions.

- **Two tones per semantic hue.** `--ok` and `--ok-vivid`, `--warn` and
  `--warn-vivid`, and so on: the text tone for labels and copy, the vivid tone
  for rules, borders and tints. One value cannot do both in light mode, because
  an amber dark enough to read as text is brown.

- **Hue is never the only signal.** Additions are teal and removals are clay,
  which separate on the blue-yellow axis and survive both common forms of
  red-green color blindness. On top of that a changed row carries a sign in the
  gutter and a tinted background, so the distinction survives a grayscale
  screenshot and a phone in sunlight.

- **Show the states, not the happy path.** Empty, populated, not yet set up;
  every review state; a comment that has not sent; a verdict that is refused; a
  round already decided. A state that exists only as prose is a state that has
  not been designed.

- **Fixture content is real in shape, and names no other repository.** Real
  paths, agent names that look like the working copies agents actually run in,
  review titles of the length a real one runs to, Go that looks like this
  project's own. Lorem ipsum hides exactly the problems these pages exist to
  find: a title that wraps to three lines, an agent name longer than its column,
  a change of eleven files, a review waiting four days, two titles that open
  with an identifier rather than a capital. The projects a fixture names are
  invented, because these pages outlive any particular week's work and a
  screenshot travels further than the repository it came from.

- **Icons are [Lucide](https://lucide.dev)**, ISC licensed, drawn on a 24 unit
  grid with a 2 unit stroke. Each one is inlined into `app.css` as a CSS mask,
  so `background-color: currentColor` paints it and it takes the color of
  whatever text it sits beside. Eighteen icons come to about 5.7KB of
  stylesheet, with no font file, no second request and no load state. The names
  in `icons.txt` are the names on lucide.dev, and the class at a use site is
  `i-<that name>`.

  The eye in the top bar is drawn the same way, as two masks in `app.css`, and
  is not in `icons.txt`: Lucide has no shut eye with lashes, and the open one
  has to hold the waiting count.

  **A mask rather than a font, deliberately.** A Lucide webfont would have to
  ship a thousand icons or add a subsetting step, and there is no server-side
  subsetting service for it. More to the point, a ligature icon font fails
  badly: where the shaper does not apply `liga`, the literal word `check`
  renders in the middle of a sentence, and it looks correct in every engine it
  was tested in. A mask needs no shaping at all.

  **Inline SVG at the use site was the other candidate.** It is correct, and it
  puts eighteen blocks of path data into the markup. A mask keeps one table in
  the stylesheet and a readable class where it is used.

  Presentation attributes are stripped from each SVG and declared once in the
  `.i` rule, so stroke weight is one number rather than eighteen. Two masks are
  also bound to `:root`, because a file's disclosure caret is drawn by generated
  content, which cannot carry a class.

  To change the set, edit `icons.txt`, then regenerate the table:

  ```sh
  go run ./tool/icons                 # rewrites the mask table in app.css
  ```

  `make check` fails when `icons.txt` and the table disagree, when a span
  carries `i` without an `i-` class, and when one holds text. All three render
  as something plausible.

- **Prose never repeats an identifier the page already displays.** A note that
  says the verdict went to a named agent is a second copy of a name shown three
  lines above it, and the two drift the moment either changes. They did.
  `make check` fails on an agent name appearing anywhere outside its own span.

- **Labels are sentence case. Identifiers keep their own case.** A status pill
  reads `Waiting`, a hint reads `Saved as you type`. A project name, an agent
  name, a file path, a flag and a line of source are addresses rather than
  labels, so `docs`, `internal/upload/worker.go` and `busy_timeout` stay exactly
  as they are written elsewhere.

- **The phone layout is the design.** A wider screen gets a wider column and the
  line-number gutter back, never a second layout to keep in step. Everything is
  fluid, so a smaller viewport is a smaller viewport rather than a broken one.

- **The code block owns its horizontal overflow, never the page.** A page that
  scrolls sideways takes the decision bar with it.

- **A bar spans the window, its contents do not.** Stopping a sticky bar's
  background at the reading column looks like a broken element. Letting its
  contents run to the window edges leaves a narrow body between two wide bars,
  which is what it looked like before anybody opened these on a desktop. The
  bars pad themselves to center the same column the page uses, so the two align
  to the pixel and cannot drift, because the width has one name.

- **Sticky inside the scroll container, not pinned to the shell.** A long
  document keeps the decision bar against the bottom edge as it scrolls, which
  is the point: approving one must not require reaching its end. A short
  document on a wide screen lets the bar sit directly under the content instead
  of floating at the foot of an empty window with the review stranded above it.
  Pinning to the shell gives the first behavior and never the second.

- **A phone layout stretched to 860px is not a desktop layout.** Two verdict
  buttons splitting the width in half are right under a thumb and absurd at
  410px each. The wide layout puts them at their own size, at the end of a
  single row.

- **Open the expanded view before calling a tearout done.** Every sheet has one
  and it is the only place the wide layout appears. Both of the problems above
  survived several rounds of review at 390px.

- **A browser default is a layout decision nobody made.** `figure` carries
  `margin: 1em 40px`, which indented every phone frame and put 80px between the
  pairs on top of their gap. The page looked deliberate for as long as nobody
  measured it. `tearout.css` resets the chrome elements only: anything inside
  `.app` keeps the default it would have in the real surface, or this sheet
  stops showing what it certifies.

- **A default has to weigh nothing, so write it in `:where()`.** A plain
  `.app a` rule outranks `.row`, `.treerow` and `.chip`, each of which sets its
  own color, and every card on the queue turns into a blue link. A default that
  beats the things it is a default for is not a default.

- **A notice is for what could not have been prevented.** A refused verdict, a
  lapsed push subscription, a comment that has not reached the server. It is
  never a refusal of something the screen offered and then declined, which is
  why the refused-verdict notice appears beside the bar rather than replacing
  it.

- **Controls meant for an agent stay on the command line.** Absent from every
  screen: the coaching line that tells an agent what to do next, and anything
  that drives an agent's own state. This is about what the surface puts in front
  of a person, not about removing capability.

## What is checked rather than trusted

```sh
make check
```

`internal/convention` reads the remaining static sheets and holds them to the
rules under Tearouts in [docs/conventions.md](../conventions.md). Each story
runs as a test in a real browser, through Storybook's Vitest add-on: its
interaction test, if it has one, and an accessibility audit. The checks the
static sheets have, such as a class no stylesheet defines or an icon with no
glyph, move to the rendered stories as the sheets go.

Everything else here is judged rather than asserted. There is no test that says
a screen looks right.

**The scaffolding is Go, except Storybook itself.** The checker is
`internal/convention`, run by `make check`. The generator is `tool/icons`. The
server is `tool/tearout`, which builds Storybook with Node and serves the
result.

The split is the same one that decides where anything goes: a check that must
pass before work closes is a test, and a generator or a server that runs when a
person decides to run it is a tool.

## Approval

A tearout is approved by a person looking at its stories on a phone. That
approval is what lets the plumbing start, and the screen is compared back
against the approved stories rather than against a description of them.

When the design itself changes, the tearout changes with it. A reference that
cannot render the thing it certifies has stopped being one.
