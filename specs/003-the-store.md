---
status: done
created: 2026-08-26T00:00:00Z
updated: 2026-08-26T05:32:35.555035454Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
  - cmd: go test -race ./internal/blob ./internal/store
---

# 003 The store

Where a review lives: SQLite for the objects and a content-addressed directory
for the bytes a round captured. What it produces is a pair of packages that
capture and diff (004) can write into and the daemon and CLI (005) can serve
from, holding the product's invariants rather than trusting whatever calls it.
Nothing here is reachable from a command line and nothing renders.

Two failures shape the design. A lost comment is the one
[product.md](../docs/product.md) says this must not have, so a write is fsynced
before it is acknowledged. A round that names content the disk does not hold is
a review that cannot be displayed at all, so content is made durable before the
row referencing it.

## Design

**The store is given its paths and never finds them.** `store.Open` takes a
database path and `blob.Open` takes a directory, each creating what is missing.
Resolving `$XDG_STATE_HOME` belongs to the daemon and CLI (005), which is the
one place that knows the process it is running as. A test then needs a temporary
directory and no environment.

**Content is durable before the row that names it.** `Put` writes to a temporary
name, fsyncs the file, renames it into place, and fsyncs the containing
directory. The path-to-hash list reaches SQLite only after that returns. A crash
between the two leaves an unreferenced blob, which is garbage nobody reads. The
other order leaves a round naming content that is not there.

The directory fsync is what makes a rename survive power loss rather than only a
process crash. It is implemented on that reasoning and not verified by a test,
because proving it needs fault injection.

**Content already present is not written again.** Round N+1 captures every file
that differs from the base, and most of those are byte for byte what round N
captured. `Has` answers whether a digest is already in the store. Capture hashes
the file it is about to write, asks, and calls `Put` only on a miss, so an
unchanged file costs a read and a stat.

The saving has to happen in the caller. A digest is not known until the whole
stream has been read, so by the time `Put` can check the destination the
temporary file is already written. What `Put` skips on a hit is the rename and
both fsyncs, and it removes the temporary instead.

**Temporaries live in a directory that cannot be a fan-out directory.** A digest
fans out as `<aa>/<bb>/<digest>`, and those first four characters are hex, so a
directory named `tmp` can never collide with one. Renaming within the store also
keeps the rename on one filesystem, which is what makes it atomic.

**A digest is validated before it becomes a path.** Sixty-four lowercase hex
characters and nothing else. The digest arrives from the database now and from a
URL once the surface exists (006), and a path join over an unchecked string
reads whatever the caller asked for.

**No verification on read, and no garbage collection.** Re-hashing a file on
every page view costs time proportional to its size, to catch corruption nothing
here has seen. Collecting unreferenced blobs waits on the retention question
[product.md](../docs/product.md) leaves open, because what is kept decides what
is collected.

**Pragmas go in the connection string, not in an `Exec` after opening.**
`journal_mode`, `synchronous`, `foreign_keys` and `busy_timeout` are
per-connection. `database/sql` opens a connection whenever it wants one, so a
pragma set once applies to one connection and silently not to the next. The
driver takes `_pragma=` in the DSN and applies it to every connection it opens,
which a probe confirmed across four of them.

The same probe found the reason this needs a test: a misspelled pragma name in
the DSN is accepted and ignored, and the first query still succeeds. A
durability setting that did not apply looks exactly like one that did, so each
one is read back on a connection the pool opened.

**One connection.** `SetMaxOpenConns(1)`. Under WAL a reader does not block the
writer, so the usual arrangement is one writer connection beside a pool of
readers. This is a few requests a minute against a database of a few thousand
rows, and one connection means the daemon never contends with itself.
`busy_timeout` covers a second process holding the write lock, and a write that
still fails after it returns an error for the caller to report rather than being
retried here. The signal to buy the reader pool is a read slow enough to stall a
write, which nothing here has.

**`_txlock=immediate` is what keeps a check-then-insert correct.** The overlap
refusal reads the paths of open reviews and then inserts. A deferred transaction
takes the write lock at its first write, so two of those can both read before
either writes and both can pass a check that only one should. An immediate
transaction takes the lock at `BEGIN`. A probe confirmed it: with the pool
opened to four connections, a second transaction blocked at `BEGIN` until the
first committed, and then read the committed value rather than the stale one.

Today the single connection already serializes writers, so this is the setting
that keeps the invariant true if the pool grows or a second process opens the
file. A process-wide mutex is the rejected alternative for the same reason: it
protects one process, and a database file is openable by a `sqlite3` shell and
by whatever repair tool this grows.

**`_txlock` is set per connection, not per transaction.** The DSN applies it to
every `BEGIN` the driver issues on that handle, and `database/sql` has no way to
ask for a deferred one. That costs nothing while the store is the only thing
opening transactions. A reader pool would need a second handle with its own DSN:
sharing this one would make every read take the write lock, which is the WAL
property the pool would have been bought for.

**Migrations are forward-only, checksummed, and refuse a downgrade.** Each
`*.sql` file is embedded, applied in lexical order in its own transaction, and
recorded in `schema_migrations` with its name, the sha256 of its bytes and when
it ran. A recorded file whose bytes have changed fails the open. So does a
recorded migration this binary does not carry, because that is old code about to
run against a newer schema.

The cost is that correcting a migration means deleting the local database. That
is the right price while nothing has shipped. Without the checksum an edited
migration leaves two databases with the same ledger and different schemas, and
nothing reports the difference.

**Timestamps are RFC3339 with three fractional digits, always UTC, always `Z`.**
`2006-01-02T15:04:05.000Z`. Fixed width is what makes lexical order agree with
chronological order, and `time.RFC3339Nano` trims trailing zeros, which breaks
it on exactly the values a test is most likely to produce. Integer milliseconds
were the alternative: smaller, and impossible to format wrongly, and unreadable
in a `sqlite3` shell, which is a real activity for a tool with one database.

The clock is a `func() time.Time` field on the store, defaulting to one that
returns `time.Now().UTC()`. A captured `time.Time` would stamp every row with
the moment the store opened, and nothing would report it until the queue's
ordering came out arbitrary.

**IDs are integers.** They are typed by an agent into `eyeball wait` and read in
an address bar. An opaque token buys unguessability, which is worth nothing on a
surface that has no authentication and sits on a private network. A round is
addressed by its number within its review, which is the form the routes in
[architecture.md](../docs/architecture.md) already use.

**A round is created together with its file list, in one transaction.** Capture
writes and fsyncs every blob first, then hands the whole list to the call that
creates the round. A round that names no files because a second call never
happened cannot exist.

**A deleted file is a row with no digest.** A path the agent removed differs
from the base and has no content to store, so the file list carries it with an
empty digest. Leaving it out instead makes a deletion indistinguishable from a
file that never changed, and the diff (004) would miss it.

**A review's paths are files and directories, never globs.** A directory covers
everything under it. Two entries overlap when they are equal, or when one is a
prefix of the other at a segment boundary, so `docs` claims `docs/product.md`
and does not claim `docsite`.

Globs lose on the refusal. Deciding whether two glob patterns can match the same
file is not cheap, so the test would have to approximate, and an approximate
refusal is worse than none. It either merges two pieces of work under one
verdict or blocks work that never conflicted.

Paths are cleaned, relative and forward-slashed, with no leading separator and
no `..`, and anything else is refused by name. Without that, `docs` and `./docs`
compare unequal and the refusal has a hole in it.

**The overlap check runs in Go.** The candidate set is the paths of the open
reviews in one project, which is tens of rows. In SQL the same rule is three
`LIKE` clauses, one of them with a variable prefix no index helps. The
segment-boundary case is what reads better as code than as a pattern.

**Review state is a column.** It holds `open`, `approved` or `abandoned`.
Abandonment has no round-level record, so something has to hold it. Approval is
written into the same column, in the same transaction as the decision that
caused it.

Approval is therefore stored twice, which is a denormalization. It pays because
the queue and the overlap check both ask for a project's open reviews, and
neither should join through rounds and decisions to find out.

Waiting and working are not stored at all. An open review is waiting when its
latest round has no decision and working when it does, and an approved or
abandoned review is in neither. That is one fact read two ways rather than two
facts that can disagree.

**A decision is a row, not three columns on the round.** A round has at most
one, which `UNIQUE(round_id)` states exactly. Undecided is then a missing row
rather than three columns that have to be NULL together. The cost is a join on
every read of a round.

It is also the record [product.md](../docs/product.md) asks for: an approval
names the review, the round, the content and the time, in one row that outlives
the session that asked for it.

**A comment is editable and deletable only while its round is open.** A round is
open until it has a decision, and abandoning a review closes its current round
as well. Nothing has been released while a round is open, so nothing downstream
has seen the comment.

Abandonment has to close the round for the same reason it has its own state. It
writes no decision, so a rule keyed on the decision alone would leave a comment
on an abandoned review mutable and unreleased for good. The comment written
after a verdict needs no special case: its round is already closed, so it is
frozen the moment it exists.

Deletion removes the row rather than leaving a tombstone. A tombstone would have
to be filtered out of every read and every release. What it protects is a
comment the reviewer removed on purpose, which is not the loss this store exists
to prevent.

**Release is derived. Delivery is not modeled.** A comment is released when its
round is closed, which needs no column. What is absent is any record of what an
agent has already been handed. Nothing in this spec or in capture and diff (004)
can say what delivery means, because `eyeball wait` defines it. It arrives with
the daemon and CLI (005) as one forward migration instead of as a guess here.

**Whether a captured file is in the review's scope is stored, not recomputed.**
It follows from the review's paths, which never change, so it is derivable. It
is stored because the match is Go code, and a round is frozen: what was in scope
when the capture was taken should not change when that code does.

**A column nothing writes is not added.** Line counts, file modes and the
per-file view override all belong to a later spec. Adding one now is a guess
that later code has to honor or migrate away from, which is the same migration
the guess was meant to avoid. A forward migration is one file.

**The store never opens a blob.** It holds paths and digests as strings. A
digest whose file is missing is a display failure for the server to report, not
an error the store can find. The alternative is a store that has to be handed a
filesystem before it can answer a question about a row.

**Indexes are the unique constraints and nothing more.** Every table here is
bounded by what one person reviews. An index added against a query nobody has
timed is a guess with a write cost.

**Tests run against a file, not `:memory:`.** The pragmas are most of what
iteration 2 delivers, and an in-memory database refuses WAL and reports its
journal mode as `memory`. The fastest option is the one that cannot check the
thing under test.

## Out of scope

Named because each one is close enough to this work to be assumed part of it.

- **Capture.** Resolving a base commit, asking git which files differ, and
  writing their content is capture and diff (004). The store accepts a file list
  and does not produce one.
- **Diff.** Round to round and base to round, with word marking, is the same
  spec (004). Nothing here compares two captures.
- **The daemon, the socket and the CLI.** That spec (005) owns the lock file,
  the listeners, `--json` and `eyeball doc`. No process here outlives a test.
- **The surface.** Templates, htmx, routes and the live stream are the surface
  spec (006). The read methods here return values, not HTML.
- **Push subscriptions and the VAPID key pair.** The notification spec (007).
  The columns follow from the shape the push library wants, and that spec is the
  first to have it in front of it. The package comment on `internal/store`
  already names them, because it describes the target rather than what exists.
- **What a file is, and how it opens.** `internal/kind` and the per-file view
  override are the rendered-files spec (008).
- **Retention and pruning.** How long a decided review is kept, and whether its
  blobs go with it, is an open question in [product.md](../docs/product.md).
  Blobs are therefore never removed, and an abandoned review keeps everything it
  captured.
- **Line counts on a round.** The queue shows a change's size as files touched
  and lines added and removed. Files touched is a count of the round's in-scope
  rows here. The line counts need the diff, so they arrive with capture and diff
  (004).
- **Per-comment replies from the agent.** product.md allows one optional reply
  per comment, written through `eyeball reply`. That command arrives with the
  daemon and CLI (005), and nothing before it writes a reply, so the column
  arrives with the command that fills it.
- **Delivery tracking.** Which comments an agent has already received, for the
  comment written after a verdict to reach it. The daemon and CLI spec (005),
  with `eyeball wait`.
- **Project discovery.** Walking up from a working directory to the first `.git`
  and reading a marker file is the daemon and CLI spec (005). The store is told
  a root path and stores it.
- **Authentication and more than one reviewer.** Out of scope for the product,
  not just for this spec.

## Iteration 1: The blob store

- [x] `blob.Open` creating the directory tree, and `Put` streaming to a
      temporary name while hashing, returning the digest and the byte count
- [x] `Put` fsyncs the file, renames it to `<aa>/<bb>/<digest>` and fsyncs the
      directory, removing the temporary instead when the destination exists
- [x] `Has` answering whether a digest is in the store, so a caller that hashed
      the file first skips `Put` on content already held
- [x] Every call that turns a digest into a path refuses one that is not 64
      lowercase hex characters, `Has` and `Stat` included, not only `Open`
- [x] `Open` returning an `io.ReadSeekCloser` and `Stat` returning the size
- [x] Temporaries written under a `tmp` directory, which cannot collide with a
      fan-out directory because a fan-out name is hex
- [x] Tests: content round trips at its digest, the same content twice leaves
      the first file untouched, and an abandoned temporary is not readable
- [x] Tests: for each of `Open`, `Stat` and `Has`, a digest carrying a path
      separator, an uppercase digit or the wrong length is refused before any
      filesystem call
- [x] `internal/blob/doc.go` states the write ordering, why the saving on
      re-capture needs `Has`, and that the directory fsync rests on reasoning

> **Completed** 2026-08-26 04:22 UTC
>
> - acks: reviewed
> - `make check` — 1.6s
> - `go test -race ./internal/blob ./internal/store` — 1.4s

## Iteration 2: The database and its migrations

- [x] `store.Open` with the pragmas, `_txlock=immediate` and the busy timeout in
      the DSN, `SetMaxOpenConns(1)`, and a `Close` that releases the pool
- [x] The migration runner: `*.sql` embedded with `//go:embed`, applied in
      lexical order, each in its own transaction
- [x] `schema_migrations` recording each file's name, the sha256 of its bytes
      and when it ran, with a changed file failing the open and naming itself
- [x] A recorded migration this binary does not carry fails the open, because
      that is old code about to run against a newer schema
- [x] A test reading `journal_mode`, `synchronous`, `foreign_keys` and
      `busy_timeout` back, since a misspelled pragma is accepted and ignored
- [x] The store's clock as a field defaulting to `time.Now().UTC()`, with the
      timestamp layout and its fixed width in one place
- [x] Two lines in `docs/architecture.md` under Storage: the single connection
      with the DSN as where the pragmas live, and `_txlock` as per connection
      rather than the per-transaction setting recorded there today

> **Completed** 2026-08-26 04:38 UTC
>
> - acks: reviewed
> - `make check` — 1.7s
> - `go test -race ./internal/blob ./internal/store` — 69ms

## Iteration 3: The schema

- [x] `projects`, keyed by a cleaned absolute root path that is unique, with a
      display name that is not
- [x] `reviews` holding the project, the agent, the title, the state and its
      timestamps, and `review_paths` holding one row per claimed path, unique on
      the review and the path
- [x] `rounds` holding the review, a number unique within it, the agent's note
      and the base commit, which is empty where there is no git
- [x] `round_files` holding the path, the digest, the size and whether the path
      is in the review's scope, unique on the round and the path, with the
      digest empty where the file was deleted
- [x] `comments` holding the round, the level, the path, the line range, the
      quoted text and the body, with CHECKs tying each level to its columns
- [x] `decisions`, at most one row per round, holding the verdict, the optional
      note and when it was given
- [x] Every foreign key cascading on delete, so the pruning nothing does yet
      stays a single statement when it arrives
- [x] A test per CHECK and per unique constraint, inserting the row it must
      reject, because a constraint nobody has watched fail is not a constraint

> **Completed** 2026-08-26 04:45 UTC
>
> - acks: reviewed
> - `make check` — 1.7s
> - `go test -race ./internal/blob ./internal/store` — 74ms

## Iteration 4: Projects, reviews and rounds

- [x] `EnsureProject` inserting or returning the project for a cleaned absolute
      root path, refusing a path that is relative
- [x] Path normalization for a review's paths and a round's files: relative,
      cleaned, forward-slashed, no leading separator and no `..`
- [x] `OpenReview` creating the review, its paths, round 1 and that round's file
      list in one transaction, refusing an empty title, agent or note
- [x] The overlap refusal, comparing at segment boundaries against the open
      reviews of the same project, naming the review that holds the path
- [x] `OpenRound` creating round N+1 with its file list, refused unless the
      review is open and its latest round was decided as changes
- [x] `Abandon` moving an open review to abandoned from any round state, and
      refused on a review that is already approved or abandoned
- [x] A test running concurrent `OpenReview` calls that claim the same path,
      under `-race`, where exactly one succeeds and the rest name the winner
- [x] That test opens a handle whose pool is wider than one, so `_txlock` is
      what serializes the calls and dropping it fails the test
- [x] One line in `docs/product.md` where a review's scope is described, saying
      the paths are files and directories rather than globs

> **Completed** 2026-08-26 05:00 UTC
>
> - acks: reviewed
> - `make check` — 1.7s
> - `go test -race ./internal/blob ./internal/store` — 3.2s

## Iteration 5: Comments and decisions

- [x] `AddComment` at each of the four levels, refusing an empty body and an
      anchor whose columns do not match its level
- [x] `EditComment` and `DeleteComment`, both refused once the comment's round
      has a decision, and the delete removing the row
- [x] `Decide` writing the decision and, on an approval, moving the review to
      approved in the same transaction, refusing a round that is already decided
- [x] `Decide` refusing a change request carrying neither a comment nor a note,
      with the refusal naming what is missing
- [x] A comment allowed on a decided round and on an abandoned review, since
      refusing it discards what somebody wrote to protect nothing
- [x] The cost of `synchronous=FULL` measured against NORMAL by a benchmark, so
      the number is rerunnable rather than written where it goes stale

> **Completed** 2026-08-26 05:20 UTC
>
> - acks: reviewed
> - `make check` — 1.7s
> - `go test -race ./internal/blob ./internal/store` — 3.8s

## Iteration 6: The reads the queue and an agent need

- [x] `Queue` returning waiting, working and settled, each row carrying the
      project, agent, title, round number, in-scope file count and requested-at
- [x] Waiting ordered longest wait first, settled newest first and limited, and
      abandoned reviews in none of the three
- [x] `ReviewsFor` returning one agent's reviews in one project with their
      states, which is what `eyeball status` prints
- [x] `Review` returning one review with its state and its latest round number,
      which is what capture and diff (004) needs before it can ask for a round
- [x] `Round` and `RoundFiles` reading a round back by review and number, which
      is how capture and diff (004) reads the previous capture to diff against
- [x] `Comments` for a round, ordered by path then line then id, which is the
      order the review was read in
- [x] `internal/store/doc.go` states the timestamp layout, the id scheme and the
      review state machine, since those outlive this spec

> **Completed** 2026-08-26 05:32 UTC
>
> - acks: reviewed
> - `make check` — 1.6s
> - `go test -race ./internal/blob ./internal/store` — 4.0s
