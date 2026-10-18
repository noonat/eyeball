---
status: draft
created: 2026-10-18T00:00:00Z
updated: 2026-10-18T00:00:00Z
required_commands:
  - cmd: make check
---

# 019 Long table rows

docs/conventions.md, under Tests, says a table row stays on one line only when
it has at most three fields and fits in about 80 characters, and otherwise puts
every field on its own line. The rule came from the review of 018, which asked
for a 139-character row in `cat_test.go` to be split. It is marked Judgment, and
175 rows in 18 test files break it. This spec reformats them and adds a check
that enforces the rule.

## Design

**The check, `row-lines`, applies the rule as two exact tests.** A row is what
`keyed-rows` already calls one: a struct literal written as an element of
another composite literal. In a `_test.go` file:

- A row on one line is flagged when it has more than three fields, or when the
  literal from `{` to `}` is more than 80 runes.
- A row on several lines is flagged when a field shares a line with the opening
  brace, with the field before it, or with the closing brace. Measured, none
  does today. Without this half, a row could pass by breaking once in the
  middle.

The width is the row's own text, not the line it sits on. A line's length
depends on how many tabs indent it and how wide a tab is drawn, and neither is
part of the row. "About 80" becomes 80 because a check needs one number. The
number moves the count more than anything else: 27 of the 175 rows have three
fields or fewer and are 81 to 90 runes wide, so a limit of 90 would leave them
alone. 29 of the 175 are at most 80 runes wide, with four or more fields. Those
break the rule as worded, and a six-row table of four short fields grows from 6
lines to 30.

**Test files only.** The rule sits under Tests, and the comment it came from was
about a test table. Applied to every Go file, it would also flag 15 rows in
`acting.go`, `project.go`, `rendered.go` and `tool/seed/main.go`, compact tables
of four or five short fields that read as a grid today.

**The reformat is done by a throwaway program, not by hand.** It parses each
test file, inserts a newline before every field of a flagged row and before its
closing brace, adds a trailing comma, and runs `go/format`. It then scans the
file before and after with `go/scanner`, comments included, and refuses to write
a file whose tokens differ in anything but commas and newlines. Run on a clone
of `master` at `520d698`, it reformatted all 175 rows, every file passed the
comparison, a re-survey found no row breaking either test, and `make check`
passed. The program is not committed. It runs once, and the check holds the rows
to the rule afterward. No test file carries a build tag that holds a flagged
row, so `make check` compiles every file the reformat touches.

## Review

Spec review pass: declined, because the check is one walk of the syntax tree
beside `keyed-rows`, and the reformat was probed before this was written.
End-of-branch review pass: declined, because the reformat is proved by the token
comparison and the check by its fixture and a planted violation.

## Out of scope

- Rows outside test files. The 15 named above stay as they are. specs/TODO.md
  records whether the rule should reach them.
- Struct literals that are not rows, such as an `api.RequestBody` passed to a
  call. The judgment rule for a struct literal that does not fit on one line
  still covers them.
- Committing the reformatting program.

## Iteration 1: reformat the rows

- [ ] Every one-line row in a `_test.go` file with more than three fields, or
      wider than 80 runes, has each field on its own line, with a trailing comma
      and the closing brace alone.
- [ ] The token comparison passes on every changed file, and the result is
      recorded here: files changed, rows reformatted.

## Iteration 2: enforce it

- [ ] `row-lines` in `internal/convention` flags the two cases in Design, in
      `_test.go` files only, and is registered in `Checks`.
- [ ] `testdata/row-lines/bad_test.go` holds a four-field row at most 80 runes
      wide, a two-field row over 80 runes, and a multi-line row with two fields
      on one line, written from the rule, and each is flagged.
- [ ] A violation planted in a real test file fails
      `Test_repoFollowsConventions`, and is then removed.
- [ ] docs/conventions.md's entry says Enforced by `internal/convention` and
      gives the exact rule: three fields, 80 runes from brace to brace.
