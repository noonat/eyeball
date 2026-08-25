---
status: active
created: 2026-08-25T00:00:00Z
updated: 2026-08-25T16:05:40.043145548Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
---

# 001 Foundation

The Go module, the layout the architecture document names, and the gate every
later iteration closes on. Then the checks that hold this repo to its own rules,
and the three Python scripts under `docs/design/` rewritten as Go.

Nothing here ships behavior. What it produces is a repository where the next
spec can be executed without deciding any of this again.

## Design

**`make check` is build, lint and test, and it is the only gate.** Every
iteration in every later spec closes on it, so it has to exist before anything
else and has to stay fast enough that nobody avoids it.

**Every check corresponds to an entry in
[conventions.md](../docs/conventions.md) marked "Enforced by
`internal/convention`".** A check that enforces an unwritten rule is a rule
nobody can look up, and a written rule with no check is one that gets violated.
The document and the package are written together or neither is worth much.

**The checks live in `internal/convention` whatever the file type.** Go source,
markdown prose and the tearout markup are all this repository holding itself to
rules it wrote down. Splitting them by file type would mean three packages
answering one question, and the architecture layout would grow two entries that
say the same thing.

**Every check gets a fixture that it flags.** A gate nobody has watched fail is
not a gate, and the proof belongs in the test rather than in a person's memory
of having tried it once. Fixtures live under `testdata/`, which the go tool
ignores, so a file that deliberately breaks a rule never has to compile.

**The generator is proved by reproducing what is committed.** `tool/icons`
rewrites a block in `app.css`, so running it and finding no diff is a real proof
that the port is faithful. That is a required command rather than a claim.

**Oxfmt owns prose width, so no check here does.** Formatting markdown to 80
columns with `proseWrap: always` is a formatter's job, and writing a Go check
for it would be a second implementation of one rule. `oxfmt --check` in `make
lint` fails on prose that has drifted, which is the same gate by a shorter
route.

That brings Node in earlier than the rest of the frontend toolchain, and the
reason it was going to wait no longer holds: there is now something to check.
Oxlint and `tsc` still arrive with the surface in 005, because there is still no
TypeScript. Node stays a development dependency either way, since `go install`
never runs it.

**Oxfmt formats `specs/` too.** backlog derives a todo's id by hashing its text,
so reformatting changes ids. So does editing a todo at all, which is why
backlog's own documentation says never to cache an id and to re-list when a
command reports one it cannot find. Excluding specs would trade a formatted
document for avoiding one `backlog spec list` call.

Nothing outside the spec file holds an id. State lives in the checkbox, so a
reformatted spec keeps every todo's state and only its ids move. A review
comment cannot detach either, because a round is frozen: a comment anchors to a
line in a capture rather than to text that is still being edited.

**Oxfmt covers markdown only for now.** It also formats CSS, and `app.css` holds
a block that `tool/icons` generates. A formatter and a generator writing the
same bytes is a fight to settle when the surface arrives and `app.css` moves,
not while the only consumer is a tearout.

**`app.css` does not move yet.** The architecture document has it shipping from
`web/`. Moving it before anything serves it would leave the tearouts pointing
across the repository at a directory with one file in it. It moves in 005, and
`tool/icons` gains a one-line path change then.

**Rejected: a single `check` script instead of a Makefile.** Every command here
is already one line, and a Makefile gives `make help` and per-target running for
free. A script would have to grow both.

## Iteration 1: The module and the gate

- [x] `go.mod` at `github.com/noonat/eyeball` on Go 1.26, with `cmd/eyeball`
  whose `main` calls one function and reports what it returns
- [x] The `internal/` packages the architecture layout names, each holding a
  `doc.go` with its package comment and nothing else yet
- [x] A `Makefile` whose default target is help, with `build`, `lint`, `test`,
  `fmt` and `check`, where `check` is build then lint then test
- [x] `gofmt`, `go vet` and `staticcheck` wired into `lint`, with staticcheck
  pinned by a `tool` directive rather than a version on the command line
- [x] `.gitignore` covering the binaries a bare `go build` drops in the working
  directory and in each command's own directory
- [x] Each linter made to fail once on a deliberate violation, and the violation
  removed, so the gate is known to be wired rather than assumed

> **Completed** 2026-08-25 16:19 UTC
>
> - acks: reviewed
> - `make check` — 290ms


## Iteration 2: The Go conventions this repo can check

```backlog
required_commands:
  - make check
  - go test ./internal/convention -run Test_eachCheckFlagsItsFixture
```

- [ ] `doc-comments`: every exported type, function, method, struct field and
  package-level value carries a comment starting with one of its names
- [ ] `brace-lines`: a declared function opens and closes its braces on
  different lines, with function literals exempt
- [ ] `argument-wrapping`: an argument list wraps all or nothing, and the first
  break falls after the open paren
- [ ] `range-literal`: never range over an anonymous literal, and `keyed-rows`:
  a table's rows name their fields one per line
- [ ] `named-gomega`: an assertion goes through a named gomega, and
  `gomega-in-subtest`: the closure creates its own rather than reaching out
- [ ] `packages-listed`: every package under `internal/` and `tool/` appears in
  the layout block in `docs/architecture.md`
- [ ] `test-names`: a test names a package-level identifier, a method of one, or
  the package, with any description segment starting lowercase
- [ ] `test-order`: tests for one subject sit together, ordered on `(X, Y, Z)`
  with an empty segment first, which is not the same as sorting the strings
- [ ] A `testdata/` fixture per check and a test asserting each check flags its
  own fixture, so no check is trusted without having been seen to fail

## Iteration 3: The prose rules, checked and formatted

```backlog
required_commands:
  - make check
  - go test ./internal/convention -run Test_eachCheckFlagsItsFixture
```

- [ ] Oxfmt wired into `lint` and `fmt` over every committed markdown file, at
  80 columns with `proseWrap: always`, specs included
- [ ] `prose-person`: no first or second person in any committed markdown, which
  is the voice rule most often broken by accident
- [ ] `prose-dashes`: no em dash in any committed markdown, which the voice
  document bans and which arrives without being typed
- [ ] A fixture per prose check, flagged by the same test that covers the Go
  checks, so both kinds are proved the same way

## Iteration 4: The tearout checks

```backlog
required_commands:
  - make check
  - go test ./internal/convention -run Test_eachCheckFlagsItsFixture
```

- [ ] A class used in a tearout is defined by a stylesheet, and a fragment link
  points at an id that is on the page
- [ ] Tags nest, an icon span carries a glyph class, and an icon span holds no
  text of its own
- [ ] `icons.txt` and the generated mask table in `app.css` name the same set,
  neither having an entry the other lacks
- [ ] The load-bearing declarations `app.css` must keep, so a generator that
  takes too much with it fails the build rather than the page
- [ ] No agent name appears in prose on a page that also displays it, which is
  the check that drifted twice before it existed
- [ ] A fixture per check, and `docs/design/check.py` deleted with the design
  readme naming the Go command in its place

## Iteration 5: The icon generator

```backlog
required_commands:
  - make check
  - go run ./tool/icons
  - git diff --exit-code docs/design/app.css
```

- [ ] `tool/icons` fetches each name in `icons.txt` from Lucide and rewrites the
  generated block in `app.css`, leaving everything outside it untouched
- [ ] It refuses a name that is not a Lucide icon, and refuses to drop one that
  a pseudo-element resolves through a root variable
- [ ] `docs/design/icons.py` deleted, with the design readme naming the Go
  command, having first confirmed the output is byte for byte the same

## Iteration 6: The tearout server

```backlog
required_acks:
  - reviewed
  - served-on-the-phone
```

- [ ] `tool/tearout` serves `docs/design` on the tailnet address, sending
  `Cache-Control: no-store` so an edited page is never shown stale
- [ ] It prints the URLs to open and falls back to all interfaces when no
  tailnet address is available
- [ ] `docs/design/serve.py` deleted, with the design readme and every command
  it names pointing at `tool/tearout`
