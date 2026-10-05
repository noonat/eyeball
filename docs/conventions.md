# Conventions

How code in this repository is written. Each entry says whether it is
**enforced** or left to **judgment**, because the difference decides how much it
can be relied on.

A convention that is only written down gets violated. So where a rule can be
checked mechanically it is, and this file says which.

Prose is [voice.md](voice.md). Anything with a screen starts at
[design/README.md](design/README.md).

## Go

**Enforced by `make lint`.** `gofmt`, `go vet`, `staticcheck`. No exceptions and
no suppression comments. If staticcheck is wrong, say why in the code.

**Enforced by `internal/convention`.** Every exported type, function, method,
struct field and package-level value carries a doc comment starting with its
name.

One declaration can bind several names, and a comment starting with any of them
satisfies the rule, since starting with all of them is impossible. Two fields
declared separately are two declarations and need a comment each.

A function the testing toolchain calls is exempt: `Test`, `Benchmark`, `Fuzz`
and `Example`. Its name is already required to say what is under test, so a
comment would be a second copy of that to keep in step.

```go
// Old and New are the line numbers, blank where the line exists on one side.
Old, New int

// Base is the commit a round's capture was taken against.
Base string
// Files maps a path to the hash of its content at that round.
Files map[string]string
```

**Enforced by `internal/convention`.** A declared function opens and closes its
braces on different lines. A one-line body reads as a value rather than as code,
so the next person adds a statement and reformats the whole thing, and the diff
hides what changed. Function literals are exempt: a small transform passed as an
argument is the one place the compact form is clearer.

**Enforced by `internal/convention`.** An argument list wraps all or nothing. If
a newline falls between two arguments, every argument goes on its own line and
the first break is after the open paren. The half-wrapped form is what this
rules out: the call's name and an argument share a line, so a reader has to find
where the list starts, and adding an argument reflows the call.

A newline _inside_ an argument does not count, which keeps the common case
legal:

```go
rounds = append(rounds, round{
    id:   id,
    base: base,
})
```

This is stricter than it looks. Grouping the values onto a continuation line
reads well and is still a violation, because the writer and the format string go
on sharing a line with the call:

```go
// Bad
fmt.Fprintf(&b, `<span class="meta">round %d, %d files</span>`,
    n, len(files))

// Good
fmt.Fprintf(
    &b,
    `<span class="meta">round %d, %d files</span>`,
    n,
    len(files),
)
```

**Enforced by `internal/convention`.** A new package is listed in the layout
block in [architecture.md](architecture.md) in the change that creates it, with
a one-line description. A list missing entries stops being read as a list of
what exists.

**Judgment.** Each package carries a `doc.go` with a terse package comment:
purpose and package-scoped invariants, pointing at its counterpart in `docs/`
for the narrative. The prose lives in `docs/`, not in `doc.go`.

**Judgment.** `main` calls one function and reports what it returns:

```go
func main() {
    if err := run(); err != nil {
        fmt.Fprintf(os.Stderr, "eyeball: %+v\n", err)
        os.Exit(1)
    }
}
```

`os.Exit` from wherever a call happened to fail skips every deferred close, so a
half-written capture stays where a whole one was expected. It also scatters the
decision about what a failure looks like across a dozen sites, one of which will
drift.

**Judgment.** A doc comment sits on the thing it describes, not on the const
block above it, where a reader looking at the function never sees it.

**Judgment.** Several variables declared at their zero value go in one
parenthesized `var` block, not in consecutive `var` lines. gofmt aligns the
types into a column, so the set reads as one declaration of what the function is
about to fill in, and adding another touches one line. A single variable stays a
single line.

**Judgment.** A struct literal that does not fit on one line puts every field on
its own line, keyed, with a trailing comma and the brace alone. The keyed form
lets gofmt align the values, adding a field touches one line instead of
reflowing the literal, and the closing brace shows where the value ends.

**Judgment.** `if err := f(); err != nil` earns its compactness on a short call
and loses it on a long one. Past about a hundred characters the reader has to
find the semicolon before knowing what is being tested, so assign on one line
and test on the next. The compact form stays where nothing is hidden, as in a
close whose error is checked on the same line.

**Judgment.** Do not wrap a call whose only arguments are a context and a string
literal. `db.ExecContext(ctx, "UPDATE ...")` is one statement and reads as one
line. Wrapping starts to pay once there are parameters to line up under the
query.

**Judgment.** Do not wrap to hit a margin. Go tolerates long lines, and what
matters is whether a reader can scan the line, not how many columns it occupies.

**Enforced by `internal/convention`.** Never continue a string constant on the
next line with `+`. The reader has to reassemble the message before knowing what
it says, and a search for a phrase inside it finds nothing. This rule was left
to judgment first and broken in five places, while the rules beside it that
carry a check were not broken once.

A line that is genuinely hard to read is hard because of chaining or nesting,
not because of its length, and the fix for that is to split the logic into named
steps. Folding the same expression onto more lines makes it longer without
making it simpler.

**Enforced by `internal/convention`.** Never wrap a function definition. A
signature too long to read on one line has too many parameters, and it needs a
different signature rather than more lines. A callback parameter is the usual
cause; giving it a named type shortens every definition that takes it and gives
the parameter somewhere to be documented.

```go
// reporter receives one declaration for the doc-comment check.
type reporter func(pos token.Pos, kind string, names []string, doc *ast.CommentGroup)

func reportGen(d *ast.GenDecl, report reporter) {
```

**Judgment.** A composite literal goes on one line, or gives every field its own
line. The half-wrapped form, where some fields share a line inside a literal
that is already broken, fails for the same reason a half-wrapped argument list
does.

**Judgment.** Name an argument rather than wrapping the call it sits in. A long
expression inside an assertion reads better as two statements:

```go
docPath := filepath.Join(doc, "architecture.md")
g.Expect(os.WriteFile(docPath, []byte(layout), 0o644)).To(Succeed())
```

When a chain does have to wrap, break after the dot with the chain indented,
never by breaking the argument list.

**Judgment.** A set of values one field can hold gets a defined type, and
everything holding one uses it: `type Verdict string`, with
`VerdictApprove Verdict = "approve"`, and `Decide` taking a `Verdict` rather
than a `string`. The signature then says what it wants, a `Level` cannot be
passed where a `Verdict` goes, and a string arriving from JSON or a flag needs
an explicit conversion, which is where it gets checked.

Defined, not aliased. `type Verdict = string` is the same type under a second
name and buys none of that. The limit is worth knowing as well: Go converts an
untyped constant implicitly, so `Decide(id, "maybe", "")` still compiles. The
type documents and separates, and a runtime check is still what rejects a bad
value.

This covers a value set, not constants that happen to sit together. `openMarker`
and `closeMarker` delimit one region and nothing takes either as an argument, so
they stay plain strings. That distinction is why no check enforces this: a check
over a const group of strings would flag them.

**Judgment.** An enum's values carry its name: `KindMarkdown`, not `Markdown`.
At the point of use a bare `Markdown` reads as a variable, and nothing says
which of several sets it belongs to. Prose in comments keeps the name a reader
will see elsewhere: the wire format writes `not_found`, so a comment says
`not_found` and the constant is `CodeNotFound`.

**Judgment.** A function that returns HTML leads with a verb. A constant holding
a fragment ends in `HTML`. So `renderRow` builds the markup and `rowHTML` is the
template it formats. No check enforces this: telling a noun phrase from a verb
phrase needs a word list, and a check built on one is wrong at the edges.

**Judgment.** Do not name a thing for its position in the code. `first` and
`second` holding two captures say only which line declared them. Name what
differs: `base` and `head`, `stored` and `incoming`. Where nothing differs,
number them: `pass1`, `pass2`. An ordinal is fine when position is the meaning.

**Judgment.** A set is `map[T]struct{}`, not `map[T]bool`. A bool implies that
`false` means something, and a reader has to work out whether an absent key and
a `false` value differ.

**Judgment.** No `any` or `interface{}` in domain code. Generics where they fit.

**Judgment.** Narrow interfaces declared at the point of use. The server
declares the interface its handlers need, listing only the methods they call,
satisfied by the concrete store. A package-wide store interface grows to the
union of every caller and stops saying what any one handler depends on.

## Dependencies and errors

**Stdlib first.** `net/http` and `http.ServeMux` over a router framework,
`database/sql` over an ORM, `encoding/json` over an external JSON library. The
bar is "does this earn its keep", not "is this stdlib". Pure-Go dependencies
keep the binary static and CGO free, which is what makes `go install` produce
something that works.

The dependency table lives in [architecture.md](architecture.md), with one line
per entry saying what it buys. A dependency added without an entry there is a
dependency nobody weighed.

**Package-qualified, de-stuttered names.** `store.Open`, not `store.OpenStore`.
The exception is a package's namesake type, which keeps the name: `blob.Store`,
the `context.Context` idiom.

**`cockroachdb/errors`, not stdlib `errors` or `fmt.Errorf`.** A stack at the
earliest possible origination point, exactly once.

For a foreign error that point is the boundary where it enters this code: wrap
it on the first line, with `errors.Wrapf` when there is context worth adding and
`errors.WithStack` when the error already names its operation. Context is
lowercase and unpunctuated, as in `errors.Wrap(err, "open blob store")`, so a
wrapped chain reads as a sentence.

For an error originating here, that point is where the return chain starts.
Attach the stack where the error is constructed, not where it is finally
handled. By then the frames that would say which of eleven return points
produced it are gone.

Bare-return anything that already carries a stack, and re-wrap only to add
context. Sentinels are `Err`-prefixed package vars. Branch with `errors.Is` and
`errors.As`.

**Handlers map failures to a response** with the right code. A store failure is
`500`, a malformed request is `400`. Never leak a raw error string to a client.

## Tests

**Enforced by `internal/convention`.** A table test runs each row in its own
`t.Run`, and creates its gomega instance **inside** the closure:

```go
for _, c := range cases {
    t.Run(c.name, func(t *testing.T) {
        g := NewWithT(t)
        g.Expect(Kind(c.path)).To(Equal(c.want))
    })
}
```

Binding gomega to the parent `t` attributes the failure to the function instead
of the row, hides the row's name, and stops the table at the first failure. The
closure fixes all three for one line.

What the check reports is a closure **reaching for** a gomega bound outside it.
Binding one outside the loop is allowed when it is used out there, which setup
running once before the loop needs.

**Enforced by `internal/convention`.** An assertion goes through a named gomega,
not one made in the same expression. `NewWithT(t).Expect(x)` reads as one thing
and is two, and the next assertion has to either repeat the construction or
rewrite the line.

**Enforced by `internal/convention`.** Never range over an anonymous literal.
Assign the slice or map to a variable first. The values otherwise sit between
`range` and the loop body, so reading the loop means reading past them, and the
loop's subject has no name to refer to. This holds for every literal, not only a
table.

```go
// Bad
for _, name := range []string{"a.md", "b.go", "c.png"} {

// Good
paths := []string{"a.md", "b.go", "c.png"}
for _, path := range paths {
```

**Enforced by `internal/convention`.** A table's rows name their fields, one per
line. Positional fields stop being readable past two of them, and adding a field
silently reassigns every existing value in every row. The struct type is
declared inline rather than as a named type. This is the one place an anonymous
struct is preferred.

**Enforced by `internal/convention`.** A test is named for what it tests, in the
shape `go vet` already enforces for examples:

| Name                         | Tests                                                  |
| ---------------------------- | ------------------------------------------------------ |
| `TestStore`                  | the package-level type or function `Store`             |
| `TestStore_Freeze`           | the method `Freeze` on `Store`                         |
| `TestStore_emptyBase`        | `Store`, one narrow case the broad test does not cover |
| `TestStore_Freeze_emptyBase` | `Freeze`, one narrow case                              |
| `Test_replayIsDeterministic` | the package itself, where there is no identifier       |

`X` must resolve to a package-level type or function in the package under test,
and `Y` to a method of `X`. A third segment is a description and starts
lowercase, which is what keeps it from being read as a method:
`TestStore_Freeze` names a method and `TestStore_freeze` names a case.

**Judgment.** A description is as few words as will do, not a sentence. It is
read in a list of failures, where the eye wants a label. Drop the article that a
sentence would need: `Test_findingsNameSourceLines`, not
`Test_aFindingNamesTheLineItCameFrom`. Three or four words is usually the whole
of it.

Those are the rules `go vet` applies to `ExampleT`, `ExampleT_M` and
`ExampleT_M_suffix`. It does not apply them to tests. Measured against Go 1.26:
`TestStore_NoSuchMethod` and `TestNoSuchTypeAtAll` pass vet and run, while the
example spellings of both are rejected. So the check is written here rather than
delegated.

Vet does catch one thing already, during `go test`'s own build: a name whose
first letter after `Test` is lowercase. `Test_x` is legal, because an underscore
is not a lowercase letter, which is what makes the package-level form usable.

**Enforced by `internal/convention`.** Tests for one subject are kept together
and ordered broader first. The order is the tuple `(X, Y, Z)` with an empty
segment sorting first, which is not the same as sorting the names as strings:
`_` sorts after the uppercase letters, so plain alphabetical puts
`TestStore_Freeze` above `TestStore_emptyBase` and buries the type-level case
under the methods.

```go
func TestStore(t *testing.T)                  {}
func TestStore_emptyBase(t *testing.T)        {}
func TestStore_Freeze(t *testing.T)           {}
func TestStore_Freeze_emptyBase(t *testing.T) {}
func TestStore_Open(t *testing.T)             {}
```

**Judgment.** The convention covers the test function's own name. Subtest names
passed to `t.Run` are prose and describe the row.

**Judgment.** A test passes `t.Context()`, not `context.Background()`. It is
canceled when the test ends, so anything the test started stops with it.

**Judgment.** A test helper that asserts takes the gomega, not the `*testing.T`.
Taking `t` and constructing a gomega inside means every helper makes its own,
and the caller already has one. `THelper` is a field on `WithT` holding
`t.Helper`, so frame skipping still works. A helper that asserts nothing needs
neither.

**Judgment.** Prove each check can fail _individually_. `NewWithT` fails
fatally, so a second assertion in the same subtest never runs once the first has
failed.

**Judgment.** Prove a new gate can fail before trusting a pass from it. Write
the violation, watch the gate go red, then remove it. In `internal/convention`
this is not a habit but a test: every check has a fixture it must flag.

**Judgment.** Write that violation from the rule, not from the implementation. A
gate proved against a violation of its author's choosing tests the author's
reading of the rule.

**Judgment.** When a test cannot fail, test something else. Check the package's
import list instead, so adding to it is a decision somebody makes on purpose.

**Judgment.** Cover the adversarial shapes, not only the happy path. For a
capture: an empty base, a file that is only whitespace, a path with a newline in
it, a round that changes nothing. For a diff: a file with no trailing newline,
one line replaced by four, a rename. Combine them, because the bug lives in the
combination.

**Judgment.** Pin against an independent source, not against another copy of the
same code. A test pinning one function against another passes on a shared
mistake. The icon generator's test compares its output to the committed
stylesheet, which is why it is worth having.

## Formatting outside Go

**Enforced by `make lint`.** Oxfmt formats every committed markdown, CSS and
JSON file at 80 columns with `proseWrap: always`. Go is gofmt's, and Python and
text are left alone. A wrap is the tool's job and never a hand-adjusted line.

**HTML is excluded.** A tearout nests inline spans, and a newline between two of
them renders as a space, so oxfmt moves the `>` to the next line rather than
break between the elements. The markup renders the same and is far harder to
edit by hand, which is what a tearout is for. No width setting avoids it.

**Enforced by `internal/convention`.** No first or second person in committed
markdown. This is the voice rule broken most often by accident, because the
person writing is the one the rule is about and the pronoun arrives without
being chosen.

**Enforced by `internal/convention`.** No em dash in committed markdown. A
comma, a colon or a period says the same thing, and the character arrives
without being typed, from a keyboard substitution or from text pasted in.

Both rules read prose only. A fenced block is code, an inline code span names
something rather than says it, and a blockquote is quoted material, which covers
the annotations backlog writes into a spec. A double-quoted example is exempt
too, which is how `docs/voice.md` states the rule against the first person
without breaking it.

**A reference table is sorted by its key.** The dependency table in
`docs/architecture.md` sorts on the full module path, not the short name, so
`htmx.org` falls after the `github.com/` entries where a reader looking it up
would expect it. A table nobody can predict the order of has to be read start to
finish.

**Specs are formatted like anything else.** backlog derives a todo's id by
hashing its text as wrapped, so moving a line break changes the id and reflowing
a spec churns every todo it rewraps. Editing a todo changes it too. Indentation
alone does not: leading whitespace is stripped before hashing. The answer is
backlog's own: never cache an id, and re-run `backlog spec list` when a command
reports one it cannot find. Nothing outside the spec file holds an id, state
lives in the checkbox, and a review comment anchors to a line in a frozen
capture rather than to text still being edited.

**Run `make fmt` after a backlog command.** The in-progress marker `[/]` is not
a GFM checkbox, which allows only `[ ]` and `[x]`, so oxfmt reads it as ordinary
text and indents the continuation lines two spaces rather than six. A todo left
`[/]` fails `make lint` until it is reformatted. Closing an iteration also
appends its annotation with a blank line oxfmt removes. Both are cosmetic and
neither touches an id.

## Tearouts

Every rule here is **enforced by `internal/convention`**, which reads
`docs/design/` during `make check`. Each describes a fault a browser renders as
something plausible, which is why none of them can be left to a reading.

- `css-declarations`: `app.css` keeps the declarations that have no fallback, so
  a generator deleting more than its own block fails the build rather than the
  page.
- `icons-listed`: `icons.txt` and the generated mask table name the same set. A
  class with no mask leaves a gap that reads as a spacing bug.
- `tearout-agents`: an agent name never appears in prose on a page that also
  displays it. The displayed copy comes from an `.agent` span, and a second copy
  in a sentence is not kept in step with it.
- `tearout-classes`: a class used in markup is defined by a stylesheet. An
  undefined class still renders its text, unstyled.
- `tearout-fragments`: a fragment link points at an id on the page. The tearouts
  expand with `:target`, so a dead link is a control that does nothing at all.
- `tearout-icons`: an icon span carries an `i-<name>` class and holds no text of
  its own. Without the class it draws nothing, and with text it draws that text
  beside the glyph.
- `tearout-nesting`: tags nest. A browser repairs a stray or unclosed tag, so
  the fault surfaces only when a later edit lands inside the wrong element.

The markup is tokenized, never matched with a regular expression. A pattern
expecting `<span class="agent">` written tightly stops matching when a line
break falls inside the tag, and what it reports then is the name it failed to
strip.

## TypeScript

Arrives with the surface. The rules are settled and the toolchain beyond oxfmt
is not installed until there is something to check.

**`strict: true` and more.** `noUncheckedIndexedAccess`,
`exactOptionalPropertyTypes`, `noImplicitOverride`,
`noFallthroughCasesInSwitch`, `noUnusedLocals`, `noUnusedParameters`,
`verbatimModuleSyntax`. `isolatedModules` is mandatory, because esbuild
transforms one file at a time and cannot see across them.

**One job each.** Oxfmt formats, Oxlint lints with its type-aware rules on,
`tsc --noEmit` checks types. esbuild only transforms, so it is not a checker and
cannot replace one.

**Immutability by default.** `readonly` properties and `readonly T[]`,
`as const` for literal tables, pure functions over in-place mutation.

## Commits

Every message is drafted with the `dev-commit-message` skill and passes its
gate. The rules below are the ones specific to this repository. The skill holds
the rest, and the subject format here is the one it defers to.

**Never commit or push without asking.** Approval of one commit is not approval
of the next. A change to a drafted message is not approval either: redraft, then
ask again.

**Leave the work uncommitted until a human has reviewed it.** A commit records a
change a person has already read. Asking is not review: a question answered
before the diff was read approves nothing. Work up to one coherent change, run
the checks, draft the message, and stop with the change still in the working
tree, where `git diff` and `git status` show it whole.

This bounds how large a change gets. A change too large to read in the working
tree is too large to commit as one.

**Subject:** `type(scope): summary`, with a Conventional-Commits type (`feat`,
`fix`, `refactor`, `chore`, `docs`, `test`, `perf`), imperative, under about 72
characters. Scope is the area touched (`store`, `capture`, `diff`, `server`,
`cli`, `design`, `docs`, `deps`, `repo`). Omit it only for a global change.

**Body: usually none.** A subject line is the whole message for anything a
developer reading the history would not stop to ask about. Adding a package, a
document or a screen explains itself.

Write a body only when the why is not obvious from the subject and the diff
together: a constraint that is not visible in the code, a tradeoff that was
weighed, an approach that was tried and abandoned. Then write one paragraph. If
a paragraph would survive being replaced by `git show`, cut it, and if the
reader would not have asked the question it answers, cut it too.

**No trailers.** No `Co-Authored-By`, no `Signed-off-by`.

**No references to specs, plans, tasks or issues,** unless the commit is that
thing. Commits are permanent and those references rot.

**State a correction as a correction.** If a commit reverses an earlier claim,
name the claim and say why it was wrong. A silent reversal leaves two
contradictory statements in the history and no way to tell which one won.

**Ask with a choice, not an open question.** Offer the options: commit it,
commit the subject alone, do not commit, change something. An open question
invites a yes that was meant as a comment.

**Amend only a commit nobody has seen.** Once a commit has been shown, or
reported as done, it is fixed, and a correction is a new commit. An amend
replaces the state that was read and leaves no diff between it and the
correction, so the change cannot be reviewed at all.

One branch per spec, `spec/NNN-name`, cut from the default branch and merged
back with `--no-ff`.

## Specs

Non-trivial work is specced as markdown under `specs/NNN-description.md` and
driven by the `backlog` CLI. Run `backlog doc` for the reference. Loose ideas
that are not specs yet live in [specs/TODO.md](../specs/TODO.md).

**Judgment.** Reference a spec that does not exist yet by name, with its number
in parentheses, as in `capture and diff (004)`. The number is the part that
moves: an unplanned spec takes the next free one and everything after it shifts,
so a bare `004` written before the shift names different work afterward. That
has already happened, with a reference to the surface as `005` left standing
after the surface became `006`. A name survives a renumbering and a number does
not.

A spec that is already written keeps its number, so a bare `001` in prose is
fine. The rule is about forward references, which are the only ones that can
move under a reader.

**Judgment.** A design document describes the target and is corrected when the
target moves. It is speculative by construction, so a departure from it is a
reason to change the document. A plan is the opposite: it records what was
intended, so where the work went differently, the difference belongs with the
plan.

**Judgment.** Carry the design in the spec, with the rejected alternative named.
An iteration that omits the reasoning invites the executor to reinvent a design
its author had already rejected.

**Judgment.** One iteration does one kind of work, and gates on commands
wherever a command can prove it. Reserve an ack for what only a person can
assert.

**Judgment.** A todo is one outcome somebody can verify. The checkbox is the
only part of a spec that survives as state rather than prose, so a box holding
six test cases cannot record that four of them are written. Aim near thirty
words, not two hundred.

**Judgment.** Update todos in the same commits as the work, never batched at the
end.

**Judgment.** Decisions that outlive a spec get promoted into code, into a
package's `doc.go`, or into `docs/` before it closes. The spec is ephemeral. The
code and the documents are the long-term source of truth.

## What is enforced rather than trusted

`go test ./internal/convention` checks this repository's own source, prose and
tearout markup against the rules above that can be checked mechanically. It is
part of `make check`.

The file lists come from `git ls-files --cached --others --exclude-standard`
rather than from walking the tree. A new package is untracked until it is added,
so a list read from the index alone checks nothing in a fresh tree and reports
success. `testdata/` is then dropped from the Go list by hand: two fixtures
break formatting to prove their rule, and a formatter would delete the violation
each exists to show.

Every check has a fixture under `testdata/` that it must flag, so no check is
trusted without having been watched fail. `testdata/` is ignored by the go tool,
so a file that deliberately breaks a rule never has to compile.
