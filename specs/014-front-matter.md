---
status: done
created: 2026-10-11T00:00:00Z
updated: 2026-10-12T02:48:03.756200104Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
---

# 014 Front matter

A markdown file that opens with YAML front matter renders it as a code block at
the top of the document. Today goldmark reads the opening `---` as a rule and
the closing one as a rule or a heading underline. Measured against goldmark
1.8.6: `---`, `status: draft`, `---` renders as `<hr>` and
`<h2>status: draft</h2>`, and 011's front matter renders as a rule, a paragraph,
a two-item list made from `- reviewed` and `- cmd: make check`, and a rule.
Every spec in this repository opens with front matter, so every spec reviewed on
the phone opens with this. It was on specs/TODO.md's Unscheduled list, and was
picked from there because the Next list is empty.

Approved by the owner as review 45, round 1, on 2026-10-11.

## Design

**What counts as front matter.** The file's first line is `---`, and a later
line is `---`. Trailing whitespace and a CRLF line ending are allowed on both.
Anything else renders as today. An opening `---` with no closing line is a rule,
as now, since a document that starts with a rule must not become one code block.

**How it shows.** A `<pre><code>` holding the front matter as written, both
delimiter lines included, highlighted with chroma's YAML lexer. It is the
element a fenced code block in a document already renders, which
foundations.html draws and app.css styles, so no tearout and no stylesheet
changes. 013 is redrawing every sheet, and this stays out of its way.

Two alternatives lost. A table of keys and values, as GitHub draws it, flattens
nesting, and `required_commands` is a list of maps. It also needs a YAML parser
as a new dependency and a fallback for YAML that does not parse. Hiding the
front matter loses a spec's status and acks, which a reviewer reads.

**One block.** The front matter is one top-level block, so it is marked and
commented on like any other. A change to `status` marks it, with what it
replaced. Its line range runs from the opening delimiter to the closing one, so
a comment can sit under it, and an empty front matter still has a range.

**The mechanism.** A goldmark block parser, registered in `extensions()` at a
priority ahead of the thematic break, so the shown parse and the key parse find
the same blocks. It opens only on line 0. Before opening, it scans the source
ahead for a closing line, because a block parser cannot give back lines it has
taken. Measured in a probe without the scan: an unclosed front matter swallowed
the rest of the document. With the scan, it rendered exactly as today.

Both the shown renderer and the key renderer register a function for the new
node kind. Measured: goldmark's renderer panics with an index out of range on a
node kind it has no function for, which would fail every spec's page.

Only documents are affected. `render.Document` is the only caller of the
markdown parser, and Source view already shows the file as code.

## Out of scope

- TOML front matter between `+++` lines, as Hugo writes it. Nothing in this
  repository uses it.
- `...` as the closing line, which YAML allows and Jekyll does not.
- A byte order mark before the opening `---`. goldmark keeps the mark as text
  today, so such a file's first line is not `---` to any parser here.

Each has an Unscheduled entry in specs/TODO.md.

## Review

Spec review pass: declined, because the change is one block parser and the
probes above cover the two ways it can break the page. End-of-branch review
pass: declined, for the same reason, and because the owner reads the branch in
eyeball.

## Iteration 1: Front matter as a code block

- [x] A block parser in `internal/render` opening only on a document's first
      line when it is `---` and a later line is `---`, registered in
      `extensions()` ahead of the thematic break
- [x] The block rendered by the shown renderer as `<pre><code>` with both
      delimiter lines, highlighted by chroma's YAML lexer, and by the key
      renderer as its source lines
- [x] Tests that render front matter: one code block followed by the heading, a
      changed `status` marked with what it replaced, a range from the first line
      to the closing one, an empty front matter, and CRLF endings
- [x] Tests that keep today's rendering: an unclosed opening `---`, `---` first
      appearing below line 1, a `...` closing line, and a byte order mark before
      `---`
- [x] The package comment in `internal/render/doc.go` saying front matter is one
      block and what counts as front matter

**Departures, iteration 1.**

- Highlighted as YAML, a front matter shows few colors. The highlighter maps
  five token kinds to the roles app.css defines: keyword, string, comment,
  number and function. chroma's YAML lexer gives a key as a tag and a plain
  value as a literal, which match none, so `status: draft` is plain and the `2`
  of `rounds: 2` is a number. A `.yaml` file in a diff shows the same. Coloring
  keys would be a new role, which is a design change and not made here.
- The replaced text of a changed front matter needed a case in `plainText`,
  which no todo named. Without it, a changed `status` was marked with nothing
  under it.
- The design's reason for requiring a closing line holds only in part. A
  document that opens with a rule and has another `---` line further down, such
  as a second rule, becomes one code block up to that line, since the rule here
  is any later `---`. Front matter cannot be told from that by its delimiters
  alone.
- Every new test was checked by planting a defect: no scan ahead, no key
  renderer, no replaced-text case, no extension, opening on any line, and the
  opening delimiter left out of the block's lines. Each made at least one test
  fail.

> **Completed** 2026-10-12 02:48 UTC
>
> - acks: reviewed
> - `make check` — 5.5s
> - `go test -race ./...` — 3.0s
