---
status: done
created: 2026-08-26T00:00:00Z
updated: 2026-08-27T00:08:44.161126909Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
  - cmd: go test -race ./internal/capture ./internal/diff
---

# 004 Capture and diff

What a round is asking about, frozen at the moment it asks, and what moved
between two of those. Capture shells out to git for the base commit and the set
of files that differ from it, writes their content into the blob store, and
hands 003 a file list. Diff turns two of those captures into hunks with
word-level marking. Nothing here is reachable from a command line and nothing
renders.

Two facts shape the design. A capture is the delta from a base commit, so
everything depends on resolving that base and on agreeing with the agent's git
about what differs from it. And a round is frozen, so a comparison between two
rounds reads content this process already holds, which is why the diff is
written here rather than shelled out.

Every claim below marked as confirmed was run against git 2.43.

## Design

### Resolving the base

**Resolving a base and taking a capture are separate calls.** `ResolveBase`
turns a ref into a commit id. `Freeze` takes that commit id and captures against
it. Their callers differ: a first round resolves `HEAD` or whatever the agent
named, and a later round is handed the commit already recorded on the review.

An empty base means there is nothing to measure against, which is the case
outside git and in a repository with no commits yet. It is not a request for a
default. Folding the two meanings into one empty string is how a later round
silently re-bases itself onto whatever `HEAD` had become.

**A `.git` entry at the root, file or directory, is what makes this a git
project, and the filesystem answers it rather than git.** Asking `git rev-parse`
first is circular: with git missing from `PATH` the call cannot run, so its
failure cannot separate a repository whose tool is absent from a directory that
was never one. An `os.Lstat` answers before any subprocess exists, which is what
makes the refusal possible: a `.git` entry with no git on `PATH` refuses and
names the fix. Falling back to the walk would show whole files with no base and
report none of it as unusual.

A directory inside a repository but with no `.git` of its own is walked, not
refused. That is the marker-file project someone put inside a checkout, and
walking gives it the documented behavior outside git. Refusing would be the
stricter rule and buys nothing, because only the enumerations break below a top
level and the walk does not use them.

**`ResolveBase` resolves through `--verify <ref>^{commit}` or refuses.**
`git rev-parse` alone is not a check. Confirmed: `git rev-parse README.md` exits
zero and prints `README.md`, and a tree id resolves to itself. Either would be
stored as a base commit and handed back to `git diff`, which reads a path as a
pathspec and compares the index to the working tree instead.

The ref comes from an agent, so it is passed after `--end-of-options`. That
separator does not change what this call decides, and the earlier claim that it
does was wrong: the appended peel stops a ref from matching an option exactly,
and every flag-shaped spelling tried exits non-zero either way. What it stops is
git reading an agent's string as an option at all. Confirmed:
`--path-format=relative^{commit}` is parsed as the option without it and as a
revision with it. The argv is built in one function so the separator cannot be
dropped by an edit elsewhere, and that is what the test asserts, because a test
through `ResolveBase` would pass with the separator removed.

**A review's base is fixed at its first round.** Round two and later capture
against the same commit round one did, and the agent cannot name a different
one. The alternative is a base that follows `HEAD`, and it breaks two things at
once. It makes the widening control's "the review's base" setting name a
different commit on every round, so the total a reviewer sees before approving
is not the total. And an agent that commits its own work mid-review then has a
base equal to `HEAD`, which captures nothing and shows an empty round.

The cost is a base that goes stale. A branch that has main merged into it
mid-review is still measured from the old commit, so the merge arrives inside
the review as work the agent did not do. The fix available today is to abandon
the review and open a new one. Whether the base should follow a moving branch is
in [specs/TODO.md](TODO.md).

**A base that no longer resolves is a refusal, not a fallback.** A commit lost
to a rebase or a garbage collection cannot be enumerated against, so there is no
capture to take. The refusal names the commit and says to open a new review. The
three-source read in [product.md](../docs/product.md) does fall back for a lost
base, and that is about displaying a file, which is a different question from
whether a round can be taken at all.

### Asking git

**git is a subprocess, per [architecture.md](../docs/architecture.md).** Every
call is `git -C <root> --no-optional-locks <verb>`, run through
`exec.CommandContext` so the caller's context cancels it. `--no-optional-locks`
stops an enumeration from taking `index.lock`, which an agent's own git may be
holding.

The agent's configuration is left alone. No `GIT_CONFIG_NOSYSTEM`, no
`-c core.something`. The repository is whatever the agent's git made, and a
capture that reads it through different settings shows content the agent does
not have.

**Enumeration output is buffered, file content is streamed.** A raw diff of ten
thousand files is about a megabyte, cheaper to read with `cmd.Output` than to
stream. File content has no bound, so `cat-file` writes into a pipe the caller
reads, with stderr going to a bounded buffer so a failing call still has a
message and cannot fill memory.

### What differs from the base

**The enumeration is `git diff --raw` plus `git ls-files --others`.** The first
covers tracked files, staged and unstaged alike, because `git diff <commit>`
compares that commit to the working tree. The second covers untracked files that
are not ignored, and `--exclude-standard` is what keeps a build directory out.
Both run with `-z`, so a path containing a newline arrives intact rather than as
two paths that are each wrong.

**The two calls disagree about what a path is relative to and how much of the
repository they cover.** Confirmed, with the repository root at `/r` and the
command's directory at `/r/sub`: `diff --raw` prints `sub/deep/a.txt` and
`other/b.txt`, repo-root relative and covering everything. `ls-files --others`
prints `deep/u.txt`, directory relative and covering only the subtree.
`--full-name` fixes the base and leaves the scope.

That is why a root holding a `.git` entry has to be the repository's top level,
confirmed with `--show-toplevel` rather than assumed.
`rev-parse --is-inside-work-tree` answers true from a subdirectory, so it cannot
be the check. A root below the top level would capture paths that do not exist
under it, which the vanished-path refusal would report on every capture, plus
files from outside the project. The refusal names the top level it found.

**A path can arrive from both calls, and the untracked answer wins.** After
`git rm --cached foo.go`, the ordinary way a tracked file becomes untracked,
`diff --raw` reports it deleted and `ls-files --others` reports it present,
confirmed. Two rows for one path is a duplicate the store refuses outright, so
every capture in that working tree would fail until the agent finished what it
started. Untracked wins because the file is on disk and its content is what the
agent has. Merging happens by path after both calls return, so the rule holds
whichever order git reports things in.

**`--raw` rather than `--name-status`, for one mode.** A raw record carries the
source and destination file modes, and mode 160000 is a submodule, which nothing
else in the output distinguishes from a file. A changed submodule is a modified
path whose worktree entry is a directory, so reading it as a file fails.
Submodules are skipped and their contents are not part of any review.

Either mode being 160000 skips the record, not just the destination: a removed
submodule has a source mode of 160000 and a destination mode of zero, so a check
on the destination alone records it as an ordinary deleted file. Confirmed: a
submodule whose commit moved reports as `:160000 160000 <oid> 0000000 M`.

**Both object ids in the record are thrown away.** The source id would let a
first round fetch its base content without asking git a second time, at the cost
of a second way to resolve a base blob alongside the one the reader uses for
every other round. One resolution path is worth more than one saved call.

The destination id is not the shortcut it looks like. It is all zeros only where
the working copy differs from the index, and a staged path carries a real blob
id: confirmed, a staged addition reports `:000000 100644 0000000 3e75765 A` and
only an unstaged change reports zeros. Reading from git where the id is present
would capture the staged copy rather than the file on disk, and those differ the
moment an agent stages a change and keeps editing. Content always comes from the
working copy.

**Every path is `lstat`ed before it is read, whichever call reported it.** A raw
record's mode 120000 marks a symlink and is not enough, because an untracked
symlink comes from `ls-files --others`, which reports no mode. Confirmed: a
symlink to a file and a symlink to a directory both list as a bare name, without
the trailing slash that marks an embedded repository.

So `os.ReadFile` on an untracked symlink to a directory fails with `EISDIR` and
takes the whole capture with it, and on one pointing at a file it silently
captures the target's bytes as if they were the link. `os.Lstat` answers once
for both enumerations and for the walk outside git, and a symlink's content is
its target from `os.Readlink`.

**`--no-renames`, and a rename is a delete and an add.** Rename detection is on
by default, confirmed, and emits one record with two paths. A capture is a
path-to-content list, so a rename has to become two entries regardless.
Detecting it would only buy a nicer rendering, which nothing here renders.

The parser still handles a two-path record, and it turns out to be the parser
rather than the flag that makes this correct. Removing `--no-renames` changes
nothing the enumeration returns, because the record git then emits is read as a
delete and an add, which is exactly what the flag makes git emit as two records.
So the flag buys git not doing the detection work, on a diff that can be the
whole of a large change, and it is asserted on the argument list because no
assertion on the output can see it.

The flag does beat the configuration: confirmed,
`git -c diff.renames=copies diff --raw --no-renames` emits a delete and an add
and never an `R`. So no repository produces a two-path record, and the parser's
handling of one is tested by handing it the record. It is kept because the
failure it prevents is silent and total: a parser that assumes the flag worked
reads the second path as the next record's status and produces garbage for the
rest of the round.

**An embedded repository arrives as a directory.**
`git ls-files --others --exclude-standard -z` reports an untracked directory
containing its own `.git` as a single entry with a trailing slash rather than as
the files inside it, confirmed. An entry ending in `/` is skipped as the listing
is parsed.

The `lstat` every path gets would also keep it out, since the entry is a
directory. Skipping it earlier is what stops a repository deleted between the
listing and the stat from coming back as a vanished path and refusing the whole
round, over something that was never review material.

**No commits yet means diffing against the empty tree.** `git rev-parse HEAD`
fails on an unborn branch, so `ResolveBase` returns an empty base, enumeration
runs against the empty tree object, and every staged file reports as added.
Untracked files come from `ls-files` as always. One code path covers the first
commit of a repository, and the round records an empty base commit because there
is no commit to record.

That id is asked for rather than written down.
`4b825dc642cb6eb9a060e54bf8d69288fbee4904` is the SHA-1 value, and a repository
created with `--object-format=sha256` rejects it with
`fatal: ambiguous argument`, confirmed. `git hash-object -t tree /dev/null`
returns the right one for either format, at one call on a path that is already
the unusual one.

**A repository that gains its first commit ends the review.** An empty base
resolves to the empty tree on every round, and what that is compared against
grows as the repository does, so a review opened on an unborn branch with a
commit landing before round two would capture every tracked file rather than the
agent's work. Nothing else catches it, because an empty base always resolves and
the missing-base refusal never fires. So a later round whose review has an empty
base refuses once the repository has a commit, and says to open a new review.
Re-resolving the base at round two is the rejected alternative: that is the
moving base, arriving through the one door left open.

**Without git, capture walks the review's paths.** There is no way to ask what
changed, so the capture is every file under the paths the review named and
nothing outside them. Files the agent touched elsewhere are not captured, which
is the third cost of running outside git, after a first round that shows whole
files and a browse that falls back to the working copy.

The walk takes regular files and symlinks and skips everything else, so a socket
or a device node is never opened. It does not descend through a symlink, and it
skips a nested `.git`, because a project with no marker of its own may still
contain a repository. It refuses past a thousand files, naming the path that
exceeded it, because nothing outside git says what is ignored and a review that
names a directory with `node_modules` under it would otherwise capture all of
it. A thousand is a guess written down so it can be corrected, and the refusal
says to name narrower paths.

**A review path the project does not have is a refusal.** Under git a named path
that matches nothing costs nothing, because the enumeration is driven by what
changed rather than by the paths. The walk is driven by the paths, so a typo in
one of two silently captures the other and reports nothing. The refusal names
the path.

**A first round that captures nothing is refused**, naming the base. It means
the agent named the wrong paths, forgot to write a file, or resolved a base that
already contains its work.

**A later round that captures nothing means the work was reverted, and is
allowed.** A round is measured from the review's fixed base, so an agent
answering a verdict with a note and no edit still captures every file that
differs from that base. An empty capture means the working tree matches the base
again, which is a `git checkout` or a stray stash. It is allowed because that is
a real thing to ask about and the round shows it: every path the previous round
held reports as removed, and the note says why. Refusing would discard the note,
which is the one place the reason would have been written down.

### Writing the content

**Hash, ask, then write.** Capture streams each file, computes its sha256, calls
`blob.Has`, and calls `blob.Put` only on a miss. Round N+1 recaptures every file
that still differs from the base, and most are byte for byte what round N
captured. This is the saving `blob.Has` exists for, described in 003.

What that saving is, exactly, is narrower than it first reads. `Put` already
refuses to duplicate content, so skipping the question would still leave one
blob. What asking first avoids is a second read of the file and the temporary
`Put` writes and then removes, once per unchanged file per round. The count of
blobs is therefore the wrong thing to assert: the test counts reads.

**The recorded digest is the one describing the bytes in the store, which on a
miss is `Put`'s.** That path reads the file twice, and `Put` returns the digest
of what it actually read, so a file rewritten between the two passes makes them
differ and recording the first would name content the store does not hold. On a
hit nothing is written, so the hashing pass supplies both the digest and the
byte count. The refusal below covers a file that disappears; this covers one
replaced in place, which leaves no error to notice.

**A deleted path is a row with an empty digest**, which is the shape 003
defined. Leaving it out would make a deletion indistinguishable from a file that
never changed.

**A path that vanishes between the enumeration and the read is a refusal.** The
gap is small and not zero. Recording it as deleted would put a deletion the
agent did not make into a frozen round, so the refusal names the path and says
the working copy moved during capture, which the agent can retry.

Capture does not try to be atomic across the tree. Its guarantee is narrower and
is the part that matters: every digest recorded is the digest of bytes in the
blob store, so a round never names content that cannot be read.

**A file whose content matches the base is still captured if git says it
differs.** That happens on a mode change and under a checkout filter, and both
show as a file with no changed lines. Dropping them would need the base content
of every file for comparison, and would hide a mode change completely, which is
worse than showing an empty diff for one.

**Base content is the committed bytes, not the filtered ones.** A repository
with `text=auto` and a CRLF working copy has a worktree file that differs from
its blob, and a diff between them marks every line. `git cat-file --filters`
would produce the checked-out form and remove the noise. It is rejected because
it runs the project's configured smudge filters, and a git-lfs smudge fetches
over the network inside a capture that has to finish. Reading raw bytes on both
sides is also what makes the digest mean what it says.

### Reading a file back

**A frozen read and a browse read are different calls.** Both follow the
three-source order from [product.md](../docs/product.md), and they end
differently:

- `Frozen` tries the round's capture, then the base commit, and reports that the
  file does not exist. It is what the diff uses.
- `Open` tries the same two, then the working copy as it is now, labeled as
  live. It is what browsing uses.

The diff must not reach the working copy. A path in neither the capture nor the
base does not exist as of that round, and reading the live file instead would
put content the agent never had into a frozen comparison. One function for both
is how that happens, since the third source is correct for browsing and wrong
for diffing.

**An empty base has no second source, and the reader skips it.** There is no
commit for `ls-tree`, and outside git no repository to ask. Both reach the
reader, since a review can be opened on an unborn branch or on a project with no
git, so a reader assuming a base would fail every read on a source that does not
exist. `Frozen` then answers from the capture or says the file is not there, and
`Open` falls through to the working copy.

**Base content is fetched by object id, not by `<commit>:<path>`.** A capture
asks `git ls-tree -r -z --full-tree` for the paths it needs, which returns the
mode, type and object id of each and silently omits the ones the base does not
have. Content then comes from `git cat-file blob <oid>`.

The rejected alternative is feeding `<commit>:<path>` keys to
`git cat-file --batch`. Its response for a missing object is `<key> missing` on
one line, and the key is the path, so a path containing a newline produces a
response that cannot be parsed, confirmed. Keying on object ids removes paths
from the protocol entirely.

`git --literal-pathspecs` wraps the `ls-tree` call, and it is a top-level flag
rather than an `ls-tree` one. What it stops is a leading colon being read as
pathspec magic: confirmed, a file named `:weird.txt` matches nothing without it
and reads as absent from the base, which is the silent kind of wrong. The
earlier claim here, that a file named `a[1].txt` would otherwise be a character
class, was wrong. Both spellings find the file either way; the magic prefix is
the case that does not.

**An `ls-tree` entry that is not a blob is treated as absent.** A gitlink
answers with type `commit`, and `git cat-file blob` on a commit id fails.
Skipping mode 160000 keeps a submodule out of a capture; this keeps one out of a
read.

**One `cat-file` process per read.** A round of fifty files spawns fifty
processes at a few milliseconds each. A persistent `git cat-file --batch` is the
answer once that shows up in a measurement, and nothing here has one. The batch
is also what makes the protocol ambiguous again, so the `ls-tree` step stays
either way.

### The diff itself

**`internal/diff` takes bytes and returns hunks.** No git, no store, no
filesystem. Two byte slices and a result, so every adversarial shape is a table
row rather than a fixture repository.

**The line comparison is `go-udiff`, and only the comparison.** It is x/tools'
two-sided Myers: bidirectional, and bounded so that a change too large to search
exactly still comes back as a diff. Where the edit distance grows past what it
will search it stops and joins a forward and a backward partial subsequence.

That degradation is the whole reason for the dependency. The search was written
here first, one-sided with a cap of 1500 and the fronts kept as `int32` for a
nine-megabyte bound, and it agreed with the library on every test case but one.
What it could not do is give up gracefully: past the cap all it could offer was
the trimmed middle as one removal run and one addition run, and for two files
sharing half their content that reads worse than no diff at all. A large
refactor is not a rare shape in a review tool.

The rejected alternative is Myers' linear-space refinement, written here. It
removes the memory bound at the cost of a recursive divide and conquer, and it
would still be a one-sided search that has to give up somewhere.

Everything above the comparison stays here: the hunks, their context and
numbering, the size a file is not diffed past, and the marking. What the
dependency replaced is about a hundred and twenty lines.

**A line carries its own newline into the comparison.** That is how a last line
ending in one is told from a last line that does not. Comparing the text alone
reports a file that lost its final newline as unchanged, while the capture that
recorded it holds a different digest, so the round would show a file that moved
no lines. GNU diff makes the same distinction, printing the pair with its
no-newline note.

**Where several placements are equally minimal, the library picks one.** GNU
diff shifts boundaries with heuristics of its own and lands elsewhere, on about
a fifth of diffs measured over three thousand random pairs. Both are minimal and
both replay correctly, so this is a question about reading rather than
correctness, and it needs real reviews. It is in [specs/TODO.md](TODO.md), and a
golden file pins the current answer so a version bump that changes it is
noticed.

**There is no flag for a wholesale replacement.** The earlier design had one,
because a one-sided search that gave up fabricated a removal run and an addition
run of equal length, and the pairing rule below would have marked them line by
line against lines they had nothing to do with. A real subsequence does not
fabricate that: an equal-length run means a genuine one-to-one replacement,
which is exactly what the rule says to mark. The concept went with the bail-out.

**A file past a size limit is not diffed at all.** Two megabytes or twenty
thousand lines on either side, and the result says so, carrying the line count
of each side and no hunks. A round's size drops those counts rather than storing
them: the line limit's own answer is the length of each side, which as a stored
size reads as a one-line edit moving every line in the file. The byte limit says
the same thing by never reading the file at all, and two refusals reporting
opposite numbers is the shape of a bug nobody notices. A generated file, a
minified bundle and a lock file all land here, and a phone rendering forty
thousand marked lines is not reading either. The numbers are guesses written
down so they can be corrected.

**Binary is a NUL byte in the first eight kilobytes.** The same test git uses,
and enough to decide whether a line diff means anything. `internal/kind` decides
how a file opens, which is a different question, and it arrives with rendered
files (008). Waiting for it would block this spec on one predicate.

**A missing trailing newline is a flag on the line, not a marker line.** The
last line of a file that does not end in a newline is a real line with a
property, and rendering it as an extra line of diff output puts a line in the
file that is not in the file. The renderer decides what to show for it.

**CRLF is part of the line.** No normalization. A file whose line endings
changed shows every line as changed, which is what happened. Normalizing would
hide a change that breaks builds.

**Word marking runs only where a run of removals pairs one to one with the
additions that replaced it.** Equal-length runs are paired in order and marked.
An unequal run is a rewrite, and pairing across one marks the wrong words with
confidence, which is worse than marking nothing.

Tokens are runs of word characters and runs of everything else, whitespace
included, so a diff of `foo(a, b)` against `foo(a, c)` marks one token. Marking
is capped by token count for the same reason lines are, and a pair over the cap
is left unmarked.

**No similarity floor.** Two paired lines with almost nothing in common get
marked nearly end to end, which is noise. Whether to suppress marking below some
proportion of shared tokens needs real reviews to tune against, and it is in
[specs/TODO.md](TODO.md) rather than guessed at here.

### The size of a round

**A round's size is measured against the previous round, and stored.** The queue
row and the review page's brief both sit directly above the change they
describe, and from round two onward the change shown by default is what moved
since the last reading. A header saying six files above a diff of one file is a
header describing something else.

This corrects 003, which stored the count of a round's in-scope captured files
and called that the size. That number is what the review has done in total,
which is the other setting of the widening control. Storing the round-to-round
number is also what keeps the queue cheap: the alternative is diffing every
waiting review's latest round on every queue render, and the queue re-renders on
every live update.

**Three columns on `rounds`, not per-file rows.** A path can move between two
rounds without being in the later capture: a file changed in round one and
reverted in round two differs from round one and matches the base, so round two
records nothing for it. Per-file counts on `round_files` would have nowhere to
put it, and the round's total would be wrong by exactly the files somebody
undid. The per-file counts a file list shows are computed when the list is
rendered, which is the surface's spec (006), already diffing those files to show
them.

**The store sums the counts, because the store owns what is in scope.** Capture
hands over a `Changes` list of path, lines added and lines removed, and the
store tests each path against the review's paths and stores the totals for those
inside it. 003 refused to let a caller assert whether a file is in scope,
because a capture could then disagree with the review it belongs to, and that
reason has not changed.

`Changes` and `Files` are different sets and both are needed. `Files` is what
the round holds. `Changes` is what moved since the last one.

A `Change` path is normalized and refused as a duplicate the way a `File` path
already is, before the scope test. Both failures are the same silent
disagreement: `pathsOverlap` compares `cmd` against `./cmd/x.go` as unequal, so
an unnormalized path scores out of scope and the size comes out short, and two
rows for one path count it twice, which is the shape the merge rule above shows
git producing.

**A path whose diff moved no lines still gets a `Change` row.** A mode change, a
checkout filter and a binary file all produce one. Leaving them out would make
`files_changed` on a first round smaller than the count of in-scope captured
files, and those two are supposed to be the same number there.

**Capture computes the diff, so the queue never has to.** A round costs one diff
per file that moved, on top of the hash it was already paying. A file whose
digest is unchanged since the previous round is skipped without being read,
which is most files on most rounds.

**A round whose base differs from its review's is refused by the store.** The
rule that a base is fixed at round one deserves an enforced check rather than a
convention, because the caller that would break it is the one place the value
gets passed along.

**`ReviewRow.Files` changes meaning**, from the round's in-scope captured files
to the count of in-scope paths that moved since the previous round. On a first
round the two agree, which is what the zero-line rule buys: every captured path
moved by git's reckoning, even where no line did.

### Tests

**Tests build real repositories.** A fake git returning canned output tests the
parser against its author's belief about what git prints, and the whole reason
for shelling out is that a second implementation agrees until it does not. A
helper builds a repository in `t.TempDir`, and each shape the parser handles has
one: a submodule, an embedded repository, a symlink, a type change, a rename, an
unborn branch, a path with a newline in it, and a file with no trailing newline.

**Every diff limit is a parameter, not a global**: the two size limits, and the
token cap that arrives with marking. An unexported entry point takes the set, so
a test reaches every bail-out with a handful of lines instead of a
twenty-thousand-line fixture. Mutating a package variable from a test is the
alternative, and it leaves the limits writable in the binary.

**A whole diff is a golden file, not a string literal in a table.** The
expectations are whole diffs, and a diff written as a Go literal is unreadable
in exactly the way the thing it describes is readable. As files under
`testdata/`, a change to one is reviewed as a diff of a diff. The risk is that
regenerating them makes a wrong answer the new expectation, so what has to hold
is that planting a fault turns a test red rather than rewriting a fixture.

## Out of scope

Named because each is close enough to this work to be assumed part of it.

- **The CLI, the daemon and the socket.** `eyeball request` resolving a base
  from a flag, and everything that turns a capture into a command, is the daemon
  and the CLI (005). Nothing here reads a flag or outlives a test.
- **Rendering.** Hunks to HTML, syntax highlighting, markdown rendered with its
  changed passages marked, and the expand-or-list threshold are the surface
  (006). This spec produces values.
- **The working copy view.** The project view's live listing of everything
  changed on disk is the surface (006). The enumeration it needs is here and is
  exported, since it is the same call with no blob writes.
- **What a file is, and how it opens.** `internal/kind`, the per-file view
  override, images shown before and after, and the sandbox are rendered files
  (008). The diff's binary test is a NUL scan and is not that classification.
- **Rename detection.** A rename is a delete and an add. Reading it as a rename
  is a rendering, and it is in [specs/TODO.md](TODO.md).
- **File modes.** A mode-only change shows as a file with no changed lines. 003
  declined to add a column nothing writes, and nothing writes one yet.
- **A size cap on what is captured.** Every file git reports is written to the
  blob store whatever its size. The surface decides what is too large to send,
  using the size already stored on the round. Capping capture instead needs a
  column to record that a file was seen and skipped, which is the guess 003
  refused.
- **Pruning.** Blobs are never removed, and retention is an open question in
  [product.md](../docs/product.md).
- **Delivery, comments and verdicts.** 003 owns them, and what an agent has
  already been handed arrives with the daemon and the CLI (005).
- **Text encoding.** Content is bytes, and lines are split on `\n`. A UTF-16
  file reads as binary because of its NUL bytes, which is the correct answer for
  a line diff and the wrong one for a display, and display is elsewhere.
- **Running anything the project configures.** No textconv, no smudge filters,
  no diff drivers. eyeball reads and displays, per
  [product.md](../docs/product.md).

## Iteration 1: Running git, and resolving a base

- [x] A runner for one git call: `-C <root>`, `--no-optional-locks`,
      `exec.CommandContext`, stdout buffered, stderr in the error it returns
- [x] A `.git` entry at the root, file or directory, read with `os.Lstat` before
      any subprocess, deciding whether this is a git project
- [x] git absent from `PATH` with a `.git` entry present refusing and naming the
      fix, and a root with no `.git` entry walked rather than refused
- [x] `git rev-parse --show-toplevel` confirming the root is the top level, with
      anything else refused and the top level named
- [x] `ResolveBase` resolving through `--verify <ref>^{commit}`, mapping an
      empty ref to `HEAD` and an unborn branch or a non-git root to an empty
      base
- [x] The agent's ref passed after `--end-of-options`, and a ref that names a
      path or a tree refused along with one that does not resolve at all
- [x] A test helper building a repository under `t.TempDir` from a list of
      files, with commits made by plumbing so no hook or template interferes
- [x] Tests over a normal repository, an unborn branch, a root that is not a
      repository, a root below the top level, a root inside a repository with no
      `.git` of its own, and a ref naming a path
- [x] `internal/capture/doc.go` states that a base is fixed at a review's first
      round and that an empty base means no base rather than a default

> **Completed** 2026-08-26 08:21 UTC
>
> - acks: reviewed
> - `make check` — 1.7s
> - `go test -race ./internal/capture ./internal/diff` — 95ms

## Iteration 2: What differs from the base

- [x] `Changed` running `git diff --raw -z --no-renames` against the base and
      parsing the mode, status and path out of each record
- [x] `git ls-files --others --exclude-standard -z` for untracked files, with an
      entry ending in `/` skipped as an embedded repository
- [x] Either mode being 160000 skipping the record, so a removed submodule is
      not read as a deleted file
- [x] `os.Lstat` on every path from either call deciding whether it is a
      symlink, since an untracked one arrives with no mode and a link to a
      directory then fails the whole capture
- [x] A two-path record read as a delete of the first path and an add of the
      second, tested by handing the parser the record, since `--no-renames`
      means no repository produces one
- [x] The two calls merged by path with the untracked answer winning, since
      `git rm --cached` puts one path in both and the store refuses a duplicate
- [x] An empty base enumerated against the empty tree id from
      `git hash-object -t tree /dev/null`, not the SHA-1 constant
- [x] The no-git walk over the review's paths: regular files and symlinks only,
      no descent through a symlink, `.git` skipped
- [x] The walk refusing past a thousand files and naming the path that exceeded
      it
- [x] Tests for each shape against real repositories: submodule changed and
      removed, embedded repository, tracked and untracked symlinks including one
      pointing at a directory, type change, rename, deletion, a path reported by
      both calls, a path with a newline

> **Completed** 2026-08-26 08:39 UTC
>
> - acks: reviewed
> - `make check` — 1.8s
> - `go test -race ./internal/capture ./internal/diff` — 139ms

## Iteration 3: Freezing content into blobs

- [x] `Freeze` reading each enumerated path, hashing it, calling `blob.Has`, and
      calling `blob.Put` only on a miss
- [x] The recorded digest and size coming from `Put` on a miss and from the
      hashing pass on a hit, so a file rewritten between the two reads cannot
      leave a round naming content the store does not hold
- [x] A deleted path recorded with an empty digest and a size of zero, and a
      symlink recorded with its target as the content
- [x] A path that no longer exists when it is read refuses, naming it and saying
      the working copy moved during capture
- [x] `Options.Round`, counting from one, as what tells `Freeze` which round it
      is taking, since a later round may legally capture nothing and an empty
      file list therefore says nothing about which round this is
- [x] A first round that captures nothing refuses and names the base, and a
      later round that captures nothing is allowed
- [x] A later round on a review with an empty base refused once the repository
      has a commit, saying to open a new review against a real base
- [x] `Freeze` returning a `store.Request` carrying the base commit and the file
      list, with the note left for its caller to fill
- [x] Tests: recapturing identical content writes no new blob, a deleted file
      round trips, and every digest returned is readable from the store

> **Completed** 2026-08-26 09:06 UTC
>
> - acks: reviewed
> - `make check` — 1.8s
> - `go test -race ./internal/capture ./internal/diff` — 157ms

## Iteration 4: The line diff

- [x] `diff.Text` over two byte slices, splitting on `\n` and recording whether
      the last line ended in one
- [x] `go-udiff` for the line comparison, with each line carrying its own
      newline so a last line that ends in one is told from a last line that does
      not
- [x] Its line-indexed replacements converted into the script the hunks are
      built from, with the context between them filled in
- [x] Every limit passed in through an unexported entry point, so a test reaches
      each bail-out without a large fixture
- [x] Hunks with three lines of context, adjacent hunks merged where their
      context would overlap, each line carrying its old and new numbers
- [x] A file past two megabytes or twenty thousand lines on either side
      returning no hunks, the line count of each side, and a flag saying why
- [x] Binary detected as a NUL byte in the first eight kilobytes, returning no
      hunks and no counts
- [x] Lines added and removed counted on the result, which is the number a
      round's size is summed from
- [x] A property test that replaying the script over the old side yields the new
      side, which says the conversion is right without a second diff to compare
      to
- [x] Tests as a table over the adversarial shapes, each expectation a golden
      file: an empty side, no trailing newline, one line replaced by four, CRLF,
      a file with one line changed at each end
- [x] The dependency entered in `docs/architecture.md`, with the Diff section
      saying what is the library's and what is written here

> **Completed** 2026-08-26 09:33 UTC
>
> - acks: reviewed
> - `make check` — 1.8s
> - `go test -race ./internal/capture ./internal/diff` — 168ms

## Iteration 5: Word marking

- [x] Tokenizing a line into runs of word characters and runs of everything
      else, whitespace included, with byte offsets kept
- [x] Pairing a run of removals with the additions that follow it only where the
      two runs are the same length, in order
- [x] Marking each pair by diffing its tokens, recorded as byte spans into the
      line's text rather than as a second copy of it
- [x] A pair past the token cap left unmarked, and the cap passed in rather than
      read from a package variable
- [x] The rendered form in the tests showing where the marks fall, so the golden
      files cover the marking rather than only the lines
- [x] Tests: an equal run marked, an unequal run left alone, a pair over the
      cap, and a line whose whole content changed
- [x] `internal/diff/doc.go` states the pairing rule, the caps and why the
      package takes bytes rather than paths

> **Completed** 2026-08-26 09:39 UTC
>
> - acks: reviewed
> - `make check` — 1.8s
> - `go test -race ./internal/capture ./internal/diff` — 162ms

## Iteration 6: Reading a file back at a round

- [x] `NewReader` over a root, a base commit, a blob store and a round's file
      list, holding the path-to-digest map the round recorded
- [x] `Frozen` resolving the round's capture, then the base commit, and
      reporting that the file does not exist, with the source it came from
- [x] `Open` adding the working copy as a third source, labeled live, which is
      what browsing uses and what a diff must not reach
- [x] An empty base skipping the second source entirely, since a round taken on
      an unborn branch or outside git has no commit to ask about
- [x] Base object ids from `git --literal-pathspecs ls-tree -r -z --full-tree`,
      batched so the argument list stays bounded, content from `cat-file blob`
- [x] A path the round recorded with an empty digest reported as not existing,
      rather than falling through to the base
- [x] An `ls-tree` entry whose type is not `blob` treated as absent, so a
      gitlink does not fail the read
- [x] Tests: a path resolving from each of the three sources, a path in none of
      them, a deleted path, a gitlink, a reader with an empty base, and a path
      with a newline in it

> **Completed** 2026-08-26 09:47 UTC
>
> - acks: reviewed
> - `make check` — 1.8s
> - `go test -race ./internal/capture ./internal/diff` — 189ms

## Iteration 7: The round's size, in the store

```backlog
required_commands:
  - cmd: make check
  - cmd: go test -race ./internal/capture ./internal/diff ./internal/store
```

- [x] A forward migration adding `files_changed`, `lines_added` and
      `lines_removed` to `rounds`, each `NOT NULL DEFAULT 0`
- [x] `store.Change` and `Request.Changes`, holding one path and its added and
      removed line counts
- [x] The store normalizing each `Change` path and refusing a duplicate, the way
      it already does for a `File` path, before anything tests it against the
      review's paths
- [x] The store summing `Changes` over the paths the review's own paths cover,
      and writing the three totals in the same transaction as the round
- [x] `OpenRound` refusing a request whose base commit differs from the review's
      first round, naming both
- [x] `rowSelect` and `Round` reading the stored columns, replacing the
      correlated subquery that counts `round_files`
- [x] `ReviewRow.Files` and its doc comment saying it counts what moved since
      the previous round, which is the same number on a first round
- [x] Tests: a reverted file counted in the round it was reverted in, an
      out-of-scope file counted in neither, a rejected base change, a duplicate
      `Change` path refused, and one that only matches the scope once normalized

> **Completed** 2026-08-26 15:45 UTC
>
> - acks: reviewed
> - `make check` — 1.9s
> - `go test -race ./internal/capture ./internal/diff ./internal/store` — 194ms

## Iteration 8: Measuring a round as it is captured

- [x] `Freeze` building a reader over `Options.Previous`, which iteration 3
      already carries, resolving everything absent from it out of the base
- [x] A file whose digest is unchanged since the previous round skipped without
      being read, which is most files on most rounds
- [x] Every path in either capture diffed through `Frozen` on both sides, so a
      reverted file and a file created then deleted both appear
- [x] The `Changes` list filled from those diffs, with a binary, undiffed or
      zero-line file counted as a file that moved with no lines
- [x] A later round that captured nothing reporting every path the previous
      round held as removed, which is what an empty capture means
- [x] Tests: a round that changes one file of six, a round that changes nothing,
      a reverted file, a mode-only change counted as a file on a first round,
      and a first round against an empty base
- [x] `docs/architecture.md` under Capture: a base is fixed at round one, so the
      step saying `HEAD` by default describes a first round only, and the
      ignored-files line names `ls-files --exclude-standard` rather than
      `git status`
- [x] `docs/architecture.md` under Diff: the caps and what exceeding one
      produces
- [x] One line in `docs/product.md` where the queue is described, saying a
      round's size is measured against the previous round from round two onward

> **Completed** 2026-08-27 00:08 UTC
>
> - acks: reviewed
> - `make check` — 1.9s
> - `go test -race ./internal/capture ./internal/diff` — 243ms
