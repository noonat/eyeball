---
status: done
created: 2026-08-26T00:00:00Z
updated: 2026-08-26T02:45:29.107655267Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
  - cmd: go test ./internal/convention -run Test_checksFlagFixtures
---

# 002 Check gaps

Two rules this repository has written down and does not enforce. Both were found
during 001, by the rules failing to catch violations of themselves.

## Design

**A rule left to judgment gets broken.** Breaking a string constant across lines
is written in conventions.md under Judgment, and it was broken in five places
during 001. `argument-wrapping` and `signature-lines` sit beside it, enforced,
and have not been broken since their checks were written. The difference is the
check, not the rule.

**The prose checks report quoted examples that a wrap split.** They read one
line at a time, and the patterns exempting a quoted example and a code span
match neither across a line break, so `"I did` on one line and `this"` on the
next loses its exemption and the pronoun inside it is reported. Both checks
match a single token, a pronoun or an em dash, and oxfmt wraps at word
boundaries, so nothing is missed: this is a false positive rather than a blind
spot, which is the opposite of what this spec first claimed. Joining a
paragraph's lines before the spans are stripped fixes it, and the joined text
has to carry a map from each offset back to its line so a finding still names
one.

**The check needs no exemption, because the exception is gone.** conventions.md
carved out a long HTML fragment hoisted to a named `const` with its parts joined
by `+`. It was written before any HTML existed, nothing ever used it, and it was
removed rather than taught to a checker. When the surface arrives (006) and
there is real HTML to look at, the rule can be reopened against something
concrete.

**Neither check gets a repository-wide sweep by hand.** The check is the sweep.
Whatever it reports on the first run is fixed in the same iteration, which is
also the only proof that it reads real code rather than only its fixture.

## Iteration 1: The prose checks read whole paragraphs

- [x] A joined view of a markdown file that closes a wrapped line before the
      quoted and code spans are stripped, mapping each offset back to its line
- [x] `prose-person` and `prose-dashes` read that view, so a quoted example or a
      code span broken by a wrap keeps the exemption it has on one line
- [x] A test that both report nothing on a file whose every pronoun and em dash
      sits inside a wrapped quote or code span, which they flag today

> **Completed** 2026-08-26 02:39 UTC
>
> - acks: reviewed
> - `make check` — 1.6s
> - `go test ./internal/convention -run Test_checksFlagFixtures` — 64ms

## Iteration 2: The string-wrapping check

- [x] `string-wrapping`: a string constant is never split across lines by `+`,
      with a fixture and an entry in conventions.md marked enforced
- [x] Whatever the check reports across the repository, fixed

> **Completed** 2026-08-26 02:45 UTC
>
> - acks: reviewed
> - `make check` — 1.6s
> - `go test ./internal/convention -run Test_checksFlagFixtures` — 256ms
