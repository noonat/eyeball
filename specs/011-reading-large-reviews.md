---
status: draft
created: 2026-10-07T00:00:00Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
---

# 011 Reading a large review

A review past ten files or three hundred lines opens today as a list of
collapsed files, each loading its body on a tap. The owner hit it on review 13,
nineteen files and 301 lines, and named the problem: reviewing means reading
every file, so a collapsed list turns reading nineteen files into nineteen taps.
This spec opens every file, loads bodies as they near the screen, adds a list of
files at the top of a large review, defers a single very large file behind a
link, and makes folding the reviewer's own mark. It was not on specs/TODO.md's
Next list, which was empty. It corrects product.md's expand-or-list rule.

## Design

### Why the list existed, and what replaces each reason

product.md gave one reason for the list: "scrolling through four thousand lines
to find the interesting one." 008 added a second, page weight: a deferred card
sends no body until opened. Both are real, and collapsing served both by taxing
the task the surface exists for. Each gets its own answer, decided with the
owner on 2026-10-07:

| Reason                   | Was                   | Now                                   |
| ------------------------ | --------------------- | ------------------------------------- |
| Reading every file       | a tap per file        | every card open, at any size          |
| Page weight on a phone   | a body loads on a tap | a body loads as it nears the screen   |
| The shape, and jumping   | the collapsed list    | a list of files at the top, as links  |
| One huge, generated file | buried among the rest | its body deferred behind a link       |
| Knowing what was read    | nothing               | folding a card, remembered per device |

**Every card opens.** Scrolling is reading at any size. A 4,000-line review is a
long page, and that is what it is; the list at the top is how a reviewer jumps.

**A body loads when its card nears the screen.** A large review's cards render
open with their header and an empty body, which htmx fetches with
`hx-trigger="intersect once"` from the cards route 008 built.
IntersectionObserver with no root measures against the viewport and accounts for
the clipping of the page's own scroller, so a card inside `.scroll` triggers as
it scrolls in. That is assumed from the specification and confirmed by a browser
test in iteration 2. The cost: on a slow connection a card can show its header
before its body. A small review, at or under the threshold, still sends every
body with the page, since a few hundred lines cost less than a round trip each.

**A list of files tops a review past ten files**: each path with its counts,
linking to its card by an id. Ten files is the threshold the old rule used; the
line count no longer matters, since the list is for finding a file and every
file is open regardless. The list is a plain list of links, and the card it
jumps to loads its body as it arrives on screen.

**A file past 500 changed lines defers its body**, behind the line naming its
size and a Load link, as a file past the 20 MB send limit does. The owner
decided that on 2026-10-07: a generated file or a lock file is usually skimmed,
and loading it by accident costs the scroll of everything after it. Five
hundred, counting added and removed lines together, is a guess written down to
be corrected. The diff's own limits, 2 MB and 20,000 lines, still apply beyond
it. Load fetches the same body the cards route serves.

**Folding is the reviewer's mark.** Tapping a card's header folds it, which is
how the owner will mark a file read, as GitHub's Viewed does. No count or other
state is kept; the owner declined one, and more behavior would be its own spec.
The page already keeps a reader's place per device in `localStorage`. It now
stores the folded paths rather than the open ones, since every card opens by
default and a new file in the list should open too.

## Out of scope

- **A count of files read**, or folding that reaches the server or the agent.
  The owner declined it for now; specs/TODO.md holds it.
- **The file page.** It shows one file whole and already has the diff's limits.

## Not converged

No review pass has run on this spec. What is least certain: whether a body
loading as it scrolls in reads as smooth on the phone, or as the page jumping
under the reader's thumb as bodies arrive above where they are reading. Bodies
arrive below the reader on a downward scroll, so it is expected to read as
smooth; iteration 5 measures it on the phone.

## Iteration 1: The tearout for a large review

```backlog
required_acks:
  - reviewed
  - phone-approved
```

- [ ] review.html's #large section redrawn: the list of files at the top, every
      card open, one card folded, and one file past 500 lines with its body
      deferred behind Load
- [ ] Its notes saying why, replacing the collapsed list's
- [ ] README's review.html row naming the change

## Iteration 2: Every card open, bodies as they near the screen

- [ ] Cards render open at any size, a large review's bodies empty and fetched
      with `intersect once` from the cards route
- [ ] The place stored as folded paths, and an earlier entry of open paths read
      once and discarded
- [ ] product.md's expand-or-list paragraph and open question, and
      architecture.md's cards route row, saying what replaced the list
- [ ] Tests: every card open past the threshold, and a browser test that a card
      scrolled to inside `.scroll` loads its body and one far below does not

## Iteration 3: The list of files

- [ ] A list of files above the cards of a review past ten files, each path with
      its counts, linking to its card's id
- [ ] Tests: the list present past ten files and absent at ten, and each link
      naming a card on the page

## Iteration 4: A very large file, deferred

- [ ] A card whose file moved more than 500 lines rendering the line naming its
      size and a Load link, which fetches its body from the cards route
- [ ] The limit as a named constant beside the threshold, with why
- [ ] Tests: the line at 501 lines and a body at 500, and a browser test that
      Load fills the card

## Iteration 5: On the phone

```backlog
required_acks:
  - reviewed
  - phone-checked
```

This iteration is the owner's. `phone-checked` is given only by the owner.

- [ ] Review 13, or a seeded review past ten files, read top to bottom on the
      phone, with bodies arriving as cards scroll in, and what was found
      recorded in this spec
- [ ] A folded card staying folded after a reload
