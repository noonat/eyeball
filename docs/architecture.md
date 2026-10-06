# Architecture

How eyeball is built. What it does and why is [product.md](product.md), and this
document is derived from it rather than the other way round. How its prose reads
is [voice.md](voice.md).

> Written as the target. None of it exists yet.

## What runs

One process, on the machine the projects are on.

```
  phone, installed to the home screen
    │   HTTPS over the tailnet, a certificate iOS already trusts
    ▼
  eyeball ── one Go binary
    │  owns: the store, the captured file content, the tailnet node
    │  serves: the surface on :443, via tsnet
    │
    └──◄ unix socket: every agent on this machine, through the CLI
```

**The agent side never touches the tailnet.** An agent talks to the daemon over
a unix socket in the state directory, beside the database. Nothing reachable
over the network can open a review, read one as an agent, or release a batch of
comments. The tailnet carries the reviewer's surface and nothing else.

The socket is not in the user's runtime directory, although that directory is
cleared at logout and would suit it. `XDG_RUNTIME_DIR` is set by a login session
and is missing under cron, under `ssh` without a session manager and in some
harnesses, so a client and a daemon could compute two different places for it.
Every process computes the state directory the same way.

That split is free. The CLI and the phone were never going to share an
interface, and putting the agent API on a socket means file permissions are the
whole access control story for it.

## The binary

**Go, one statically linked binary, `CGO_ENABLED=0`.** Everything is embedded:
the templates, the stylesheet, the TypeScript sources, the icon masks, the SQL
migrations.

```
cmd/eyeball/          the entry point: one function, one error
assets.go             the files the binary embeds: app.css, shell.css, the templates, the TypeScript and htmx
internal/api/         the agent API's wire types, shared by the server and the CLI
internal/blob/        content-addressed store for captured file content
internal/capture/     git shell-out, base resolution, what a round holds
internal/cli/         subcommands, flags, --json, the coaching lines
internal/convention/  tests holding this repo to its own rules
internal/daemon/      single-instance lock, socket, start and stop
internal/diff/        two captures to hunks, word marking
internal/kind/        what a file is, and therefore how it opens
internal/push/        web push, VAPID, subscription lifecycle
internal/render/      a diff as rows, syntax highlighting, ages, and markdown
internal/server/      HTTP: routes, handlers, middleware
internal/store/       SQLite: reviews, rounds, comments, decisions
internal/tailnet/     the tsnet node behind an interface, and the goroutine owning it
tool/icons/           regenerates the icon mask table in app.css
tool/seed/            writes a backdated, populated state directory to look at
tool/tearout/         serves docs/design over the tailnet
web/                  the templates, shell.css, the TypeScript sources, the manifest
```

**A new package is listed here in the change that creates it**, with one line
saying what it is. A list missing entries stops being read as a list of what
exists, and `internal/convention` fails when the two disagree.

`go install` has to produce a working binary. Nothing in the build may require
Node, a bundler on the path, or a committed build artifact. That constraint
decides more of this document than any other.

## The daemon's lifetime

**Started on demand, one at a time, stopped explicitly.** There is no service
unit, no login item, and nothing supervising it.

Any CLI command dials the socket. If nothing answers, it starts the daemon and
retries. `eyeball start` and `eyeball stop` do it deliberately, and
`eyeball status` says whether one is running and since when.

**Single instance is enforced by an exclusive lock on a file, not by the socket
existing.** A crash leaves the socket file behind, and a check that treats a
stale socket as a running daemon leaves the machine unable to start one. Holding
`flock` on `eyeball.lock` for the process's life is what makes "one at a time"
true: on startup the daemon takes the lock, and only then unlinks and rebinds
the socket. Losing the race means another daemon is live, so the loser connects
to it instead.

**A daemon that will not start says why in `eyeball.log`**, in the state
directory, which is where a started daemon's stderr goes. The fix a command
prints for it points there. It never suggests `eyeball serve`: a daemon run that
way keeps the environment of whoever runs it, and an agent's environment can
hold variables tsnet reads.

**It does not exit when idle**, and this is where it stops behaving like a
language server. A language server dies with the editor that spawned it. eyeball
is spawned by an agent and used by a person hours later, from a phone, by
tapping a notification. A daemon that exits when the last agent disconnects is a
daemon that is gone exactly when the reviewer picks up the phone, and the
notification they tapped leads nowhere. So it runs until it is stopped or the
machine restarts.

Exiting when nothing is outstanding, meaning no open reviews and no connected
agents, is a defensible smaller rule and is not adopted yet. It is in
[specs/TODO.md](../specs/TODO.md).

## The project and the agent

**A project is the nearest directory above the caller's working directory that
holds a `.git` entry or a `.eyeball` marker file.** The nearest wins, whichever
it is, so a marker inside a repository names a smaller project that is walked
rather than diffed. The marker is an empty file and nothing is read from it.

**A git worktree is its own project**, because its root holds a `.git` file of
its own. Capture runs git at the project root, and a worktree's changes are
visible only from the worktree. Overlapping scopes are refused within a project,
so two worktrees can claim the same path, which is right: sharing a repository
is no more a reason to share a verdict than sharing a working copy is. A linked
worktree is displayed under its repository's name, taken from
`git rev-parse --git-common-dir`, so every worktree of `harbor` shows as
`harbor`.

**The root is resolved through symlinks for its identity**, so two shells
reaching one repository by two spellings are one project. Review paths are
computed against the root as the shell spelled it, because an agent builds an
absolute path from the directory it sees.

**An agent is named by `--agent`, then `EYEBALL_AGENT`, then its project root's
path with the home directory written as `~`.** The working copy is the identity
product.md asks for, and the root's path is the working copy. The current branch
reads better in a queue and was rejected because it moves: an agent that
switched branches would stop seeing its own reviews.

**git runs without the variables git itself calls repository-local.** The daemon
serves every project and inherits the environment of whichever agent started it,
so a `GIT_DIR` or `GIT_INDEX_FILE` left by a git hook would point every later
capture at one repository. The list comes from `git rev-parse --local-env-vars`.
Everything else in the environment stays, including `GIT_CONFIG_GLOBAL` and
`GIT_SSH_COMMAND`, which belong to the user.

## The two listeners

| Listener     | Carries                | Reachable from                            |
| ------------ | ---------------------- | ----------------------------------------- |
| unix socket  | the agent API          | this machine, by file permission          |
| tsnet `:443` | the reviewer's surface | the tailnet                               |
| loopback     | the reviewer's surface | every account on this machine, when asked |

**The CLI speaks HTTP over the unix socket.** One set of handlers, one
serialization, and `--json` is the response body rather than a second code path.
`eyeball wait` is a long poll against it, so a waiting agent costs a blocked
read rather than a loop.

**tsnet, for a certificate iOS already trusts.** An installed page is what web
push requires, an installed page requires HTTPS with a trusted certificate, and
tsnet provisions one without anything to trust on the device. The listener is
virtual, so `:443` needs no privilege.

**The daemon starts tsnet only for a node the owner enrolled.** Any agent
command starts the daemon, and every test runs one. A daemon that started tsnet
whenever it started would contact Tailscale and print a login URL from every
agent on the machine. So the node exists only once the owner runs
`eyeball tailnet login` at a terminal, which enrolls it into `node.new` and
renames that to `node/` once MagicDNS and HTTPS certificates check out. The
login refuses to run without a terminal, whatever the environment holds, and
holds the daemon's lock while it runs. A daemon that `eyeball start` or an agent
command starts gets an environment with every `TS_` and `TSNET_` variable
removed, so an auth key an agent inherited cannot reach tsnet. tsnet's own log
upload to Tailscale is turned off.

**The loopback listener is for a person debugging, and is asked for by flag.**
`eyeball serve --surface-addr 127.0.0.1:8111` serves the surface on a literal
loopback address. A started daemon never does. It gives a process running as
this user nothing new: that process can already open `eyeball.db`. It does
expose every review to the other accounts on the machine, since a loopback port
is reachable by every local user where the state directory is `0700`. That is
why it is a flag a person passes, and never a default.

**A preview renders in an iframe with `sandbox="allow-scripts"` and nothing
else.** Omitting `allow-same-origin` gives the frame an opaque origin, so
scripts run and the page renders, and it can reach no cookie, no storage and
nothing belonging to the surface.

A second listener on another port was the first answer here and it is the wrong
one. Ports do separate origins for storage, so `localStorage`, IndexedDB and
service workers would be isolated by it. **Cookies ignore the port entirely.** A
page on `:8443` shares the cookie jar of the surface on `:443`, so the isolation
would be real for three mechanisms and absent for the fourth, which is the shape
of a hole nobody finds until something is stored in a cookie. There is no cookie
today, which is what makes the mistake survivable and invisible.

A distinct hostname does isolate cookies, and it costs a second tsnet node. That
is the answer if previews ever need storage of their own, because the reason
they would need it is `allow-same-origin`, and `allow-scripts allow-same-origin`
on content served from the surface's own origin is not a sandbox at all: the
frame can reach straight back out. The separate origin is what makes relaxing
the sandbox safe, so the two decisions move together.

What this costs today: a page that calls a storage API inside the sandbox
throws. Most things a reviewer looks at, a mockup, a chart, a rendered document,
do not. A page that needs storage to render is the signal to buy the second
hostname.

## Storage

Under `$XDG_STATE_HOME/eyeball`, defaulting to `~/.local/state/eyeball`. Nothing
is ever written into a project.

```
eyeball.db            SQLite: reviews, rounds, comments, decisions, subscriptions
blobs/aa/bb/<sha256>  captured file content, content addressed
node/                 the tsnet identity, once eyeball tailnet login enrolls it
node.new/             a first enrollment that has not passed its checks yet
eyeball.sock          the agent API
eyeball.lock          the single-instance lock, also held by eyeball tailnet login
eyeball.log           the daemon's stderr
```

**SQLite through `modernc.org/sqlite`**, which is pure Go, so the binary stays
static and `go install` keeps working. WAL, `foreign_keys`, `busy_timeout` and
`_txlock=immediate`.

**Those settings live in the connection string, and the pool holds one
connection.** A pragma is per-connection, and `database/sql` opens a connection
whenever it wants one, so a pragma set by an `Exec` after opening applies to
that connection and silently not to the next. A misspelled one in the DSN is
accepted and ignored, so each is read back in a test.

`_txlock` is per connection as well, not per transaction, because there is no
way to ask `database/sql` for a deferred one. With one connection nothing
notices. A reader pool would need a second handle with its own DSN, or every
read would take the write lock and the WAL property the pool was bought for
would be gone.

**`synchronous=FULL`, not the usual `NORMAL`.** Losing a comment is the failure
product.md says this must not have, and under WAL with `NORMAL` a commit is
durable against a process crash but not against the machine losing power. The
cost is an fsync per write transaction, on a workload of a few writes per
minute.

**Blobs are written to a temporary name and renamed into place**, so a reader
never sees a partial file. Re-capturing identical content is free: the hash is
the same and the rename is a no-op.

**Migrations are forward-only `*.sql` files** embedded with `//go:embed`,
applied against a `schema_migrations` ledger.

## Capture

**`git` by shell-out, not a Go implementation of git.** The agent's repository
is whatever the agent's git made, including its hooks, its config, its worktrees
and its extensions. A second implementation agrees with that until it does not,
and the disagreement surfaces as a review showing the wrong content.

Capturing a round:

1. Resolve the base. On a first round that is `HEAD` by default, or what the
   agent named. A later round is handed the commit the review already recorded,
   because a review's base is fixed at its first round, and the store refuses a
   round that names another one.
2. Ask git which files differ from it, tracked and untracked alike.
3. Write the content of each into the blob store.
4. Diff what moved since the previous round and record its size.
5. Record the base commit id, the path-to-hash list and that size on the round.

Everything else resolves from git at the base commit when it is asked for, which
is what makes storage proportional to what the agent changed. The three-source
resolution order, and what each is labeled as, is in
[product.md](product.md#a-round-is-frozen).

**Ignored files are not captured.** `ls-files --others --exclude-standard`
already decides what is ignored, and a build directory in a capture is megabytes
nobody will read.

## Diff

**Not shelled out.** A round-to-round diff is between two blob sets eyeball
holds, and git has no view of those. The base-to-round case could shell out and
then there would be two diff paths producing two renderings.

**The line comparison is `go-udiff`, which is x/tools' two-sided Myers.** It is
bidirectional and bounded: where the edit distance grows past what it will
search, it stops and joins a forward and a backward partial subsequence, so a
large refactor still reads as a diff. A one-sided search has to give up instead,
and all it can offer then is the whole file as one removal run and one addition
run, which for two files sharing half their content reads worse than no diff.
That degradation is the whole reason for the dependency; the search itself was
written here first and agreed with the library on every case but one.

**A file past two megabytes or twenty thousand lines on either side is not
diffed at all.** The result carries the line count of each side and no hunks. A
generated file, a minified bundle and a lock file all land there, and a phone
rendering forty thousand marked lines is not reading either. Both numbers are
guesses written down so they can be corrected, and the byte limit is exported so
a caller streaming content can stop reading at the point the answer stops
depending on what follows.

Everything above the comparison is written here: the hunks, their context and
numbering, the size a file is not diffed past, and word marking within a changed
line. The marking runs only where a run of removals pairs one to one with the
additions replacing it. An unequal run is a rewrite, and pairing across one
marks the wrong words with confidence.

**Where several placements are equally minimal, the library picks one.** GNU
diff shifts boundaries with heuristics of its own and lands elsewhere, on about
a fifth of diffs measured over three thousand random pairs. Which reads better
needs real reviews and is in [specs/TODO.md](../specs/TODO.md). A golden file
pins the current answer, so a version bump that changes it is noticed.

## What a file is

`internal/kind` maps a path and its first bytes to one of: text, markdown,
image, audio, video, PDF, or none of those. That decides which view opens first,
per the table in [product.md](product.md). Extension first, content sniffing
second, because an extension is what the agent named it and is right nearly
always.

**SVG is classified as a page, not an image.** It carries script. Shown through
an `<img>` element it cannot execute, and that is how a thumbnail renders it,
but opening it as a document goes through the preview origin like any other
page.

Syntax highlighting is `alecthomas/chroma/v2`, server side, into the five roles
the stylesheet defines. Client-side highlighting would ship a second parser to a
phone to re-derive what the server already knows.

## The surface

**Server-rendered `html/template`, with htmx for the interactions.** The
tearouts under `docs/design/` are HTML and CSS already, and `app.css` ships as
it is. Templates are seeded from the tearout markup rather than written a second
time from a description of it.

**`app.css` stays in `docs/design/`, where every tearout links it.** Moving it
to `web/` would mean editing the stylesheet link in every tearout, and the
tearouts are approved against the real phone, so they are not edited for a
file's location. `go:embed` cannot reach a parent directory, so `assets.go` at
the repository root embeds both `docs/design/app.css` and `web/templates`. The
tearouts and the surface then use one stylesheet, not two copies that drift.

**`web/static/shell.css` holds the page around the app**, which a tearout's
phone frame provides and a real page does not: no margin on the body, and a
height for `.app`'s to resolve against. It stays out of `app.css` because every
tearout loads `app.css` too, and a height on the body would change how the
tearout pages lay out.

Routes, with the spec that builds each:

| Pattern                                                | Answers                                       | Spec |
| ------------------------------------------------------ | --------------------------------------------- | ---- |
| `GET /{$}`                                             | the queue                                     | 006  |
| `GET /reviews/{review}`                                | the current round                             | 006  |
| `GET /reviews/{review}/rounds/{round}`                 | any round, earlier ones as closed             | 006  |
| `GET /reviews/{review}/files/{path...}`                | a file or a directory, at the current round   | 007  |
| `GET /reviews/{review}/rounds/{round}/files/{path...}` | a file or a directory, at that round          | 007  |
| `GET /projects/{project}`                              | the project view and its working copy         | 007  |
| `GET /projects/{project}/files/{path...}`              | one changed file in the working copy, live    | 007  |
| `POST /reviews/{review}/comments`                      | one comment                                   | 008  |
| `DELETE /reviews/{review}/comments/{comment}`          | drop one before the verdict                   | 008  |
| `POST /reviews/{review}/decision`                      | the verdict, releasing the round              | 008  |
| `GET /events`                                          | the live stream, and therefore the connection | 008  |
| `POST /subscriptions`                                  | a device's push subscription                  | 009  |
| `GET /static/app.css`                                  | the stylesheet, from the binary               | 006  |
| `GET /static/shell.css`                                | the page around the app, from the binary      | 006  |
| `GET /healthz`                                         | liveness, after touching the database         | 006  |

`GET /{$}` rather than `GET /`, because `/` is a prefix pattern that matches
every path, and would answer `/agent/daemon` on the surface's listener.

**Paths are spelled out.** `/reviews/` rather than `/r/`, `/files/` rather than
`/f/`. These are read in a browser address bar, quoted in a comment, and typed
into a terminal by somebody debugging, and the four characters saved are worth
less than not having to remember what the letter meant.

`http.ServeMux` with method-and-path patterns. Handlers are methods on a server
holding the store and the blob store, and each declares the narrow interface it
needs rather than depending on a package-wide one.

## The client code

**TypeScript, compiled by esbuild linked into the binary.** The sources are
embedded with `//go:embed` and transformed, the first time a page asks for the
script, into a bundle held in memory. No Node at build time, no Node at run
time, no committed bundle to drift from its source, and `go install` produces
something that works.

With htmx taking the interactions, what is left to write is small: registering
the service worker, asking for notification permission, subscribing to push and
sending the subscription, plus the worker itself. Roughly a hundred lines.

**TypeScript is here for correctness, not for size.** The binary is installed
once on one machine, so esbuild's ten megabytes cost nothing worth counting.
What the types buy is real. A service worker's global scope is not a window, its
events are easy to mishandle, and a push handler that forgets `waitUntil` fails
by dropping notifications rather than by throwing. Strict settings catch the
class of mistake whose only symptom is a notification that never arrived.

The same argument covers the connection layer below, which is the other place a
mistake is silent.

htmx itself is vendored and served as a file. It is not bundled, because it is
already a script and passing it through a bundler achieves nothing.

The costs are real and worth naming. esbuild's Go package adds roughly ten
megabytes to the binary. It strips types rather than checking them, so
**`tsc --noEmit` is a gate rather than a build step**, and Node is a development
dependency for checking and formatting only. `isolatedModules` is mandatory,
because esbuild transforms one file at a time and cannot see across them.

Strict is not the default. `strict: true` plus `noUncheckedIndexedAccess`,
`exactOptionalPropertyTypes`, `noImplicitOverride`,
`noFallthroughCasesInSwitch`, `noUnusedLocals`, `noUnusedParameters`,
`verbatimModuleSyntax`. Oxlint with type-aware rules, oxfmt for formatting, one
job each.

### htmx for the interactions

Every interaction on this surface is the same shape: something is tapped, the
server does it, and a piece of the page is replaced by what the server now says.
Selecting a line asks for a composer anchored to it. Adding a comment returns
the comment list and the tally. A verdict returns the decided page. That is what
htmx does, and doing it by hand means fetch calls, response parsing and DOM
replacement written once per interaction.

The alternative is client-side state, which means the tally, the comment list
and the selection exist twice, in the templates and in TypeScript, and can
disagree. On a tailnet the round trip is a few milliseconds, so there is nothing
to buy by guessing at the answer locally.

What htmx costs: a vendored dependency, about fourteen kilobytes, and handlers
that answer with a fragment as well as a page. Template blocks cover the second,
since the fragment is the block the full page already renders.

**This is the decision offline was blocking.** With comments stored on the
device and drained later, the offline write path had no server to ask, so one
action needed two implementations and the hand-written one carried the data
nobody may lose. With a live connection assumed, that objection is gone.

`<details>` still handles expand and collapse with no JavaScript, and the
compare control, the marked-and-plain switch and the page-and-source switch stay
plain links to different URLs. A file's body loads when its disclosure opens, so
a change of eleven files does not send eleven files.

## Notification

**Web push, VAPID, from the daemon.** Keys are generated on first run and live
in the store. A subscription belongs to a device and is created when the
reviewer grants permission on the installed page.

One request produces one notification, deep linked to the round. Comments and
verdicts produce none: they travel toward an agent that is blocked on a read.

**A subscription that fails permanently is marked lapsed rather than retried.**
The push service returns 404 or 410 for a subscription that is gone, and the
surface reports the lapse, because a queue that looks empty because nothing is
arriving is the failure this exists to prevent.

`SherClockHolmes/webpush-go` for the payload encryption. Hand-rolling aes128gcm
with ECDH key agreement to save a dependency is a way to get it subtly wrong.

## The live connection

The surface assumes one, for the reason in [product.md](product.md): the agent
is running now, on a machine the phone can reach, and the review is a
conversation with it.

**There is no cache and no offline store.** A round's content is immutable and
could be cached forever, and caching it would buy a faster second open in
exchange for a second copy that can be stale after a daemon restart. Nothing
here is large enough to make that trade worth making.

**The service worker exists for push and nothing else.** iOS delivers web push
only to an installed page with a registered worker, so one is required whether
or not it caches. It handles `push` by showing the notification and
`notificationclick` by opening the review's URL. It intercepts no fetches.

**A failed write keeps its text on screen**, marked as not saved, with a retry.
The server is what makes a comment durable, and until it answers, the surface
holds the text and says so.

### Knowing whether the server is there

The failure to prevent is a tap that does nothing, which is what a
server-rendered surface does by default when the server is gone. It is a product
requirement in [product.md](product.md). These are the mechanisms.

**A Server-Sent Events stream is the connection, and its state is the answer.**
The surface opens `GET /events` and holds it. The daemon writes to it when a
review changes, so the queue and an open review update without a refresh, which
is what a surface watching an agent work needs anyway. The stream's `readyState`
is then not an inference about the connection. It is the connection.

`EventSource` reconnects on its own with backoff, and the daemon sends a comment
line as a keepalive, so an idle stream is distinguishable from a dead one.

**`navigator.onLine` is a hint and never the truth.** It reports whether a
network interface is up, so it is true on a wifi network with no route anywhere.
It is worth reading for one thing: a `false` is a fast and reliable negative
that can show the notice before the stream has finished failing. A `true` means
nothing.

**Every htmx request that fails says so, on the control that failed.** htmx
fires `htmx:sendError` when the request reached nothing, `htmx:responseError`
for an unexpected status, and `htmx:timeout` for a slow one. One handler for all
three writes the reason beside the element that triggered it. With none of that,
the swap simply never happens, which looks exactly like a tap that missed.

**While the stream is down, write controls are marked unavailable and say why.**
That is a stronger claim than reporting failures afterward, and it earns it:
being told beforehand that a verdict will not go through beats attempting it and
being told it failed. The read surface stays fully usable, because what is on
screen is already loaded.

**Nothing is rendered while the connection is healthy.** The notice appears on
loss and disappears on recovery.

One case is left over: a write that races a disconnect, where the controls were
live and the stream dropped between the tap and the request. That is what the
per-request handler is for, and it is why both mechanisms exist rather than
either alone.

## Checks

`make check` is the gate: build, lint, test. It has to pass before an iteration
closes.

- `gofmt`, `go vet`, `staticcheck` on the Go.
- `oxfmt --check` on every committed markdown, CSS and JSON file, at 80 columns.
  HTML is excluded, for the reason in docs/conventions.md.
- `tsc --noEmit` and `oxlint --type-aware` on the TypeScript, once there is any.
- `go test ./...`, which includes `internal/convention`, covering the Go
  conventions, the prose rules over every committed markdown file, and the
  tearout markup and stylesheets under `docs/design/`.

**Make every new gate fail on purpose once before trusting a pass from it.**

## Dependencies

| Dependency                              | Why                                                                   |
| --------------------------------------- | --------------------------------------------------------------------- |
| `github.com/alecthomas/chroma/v2`       | syntax highlighting, server side                                      |
| `github.com/aymanbagabas/go-udiff`      | the line comparison, two-sided so a huge change still reads           |
| `github.com/cockroachdb/errors`         | a stack at the point an error was constructed                         |
| `github.com/evanw/esbuild`              | TypeScript without Node, which is what keeps `go install` working     |
| `github.com/onsi/gomega` (test)         | assertions                                                            |
| `github.com/SherClockHolmes/webpush-go` | push payload encryption                                               |
| `github.com/urfave/cli/v3`              | flags in any position, confined to the CLI package                    |
| `github.com/yuin/goldmark`              | markdown with GFM's tables and task lists, which specs are written in |
| `golang.org/x/net/html` (test)          | tokenizing the tearouts, and parsing rendered pages                   |
| `golang.org/x/term`                     | whether stdin is a terminal, which the tailnet login requires         |
| `htmx.org` (vendored)                   | the interactions, as server-rendered fragments                        |
| `modernc.org/sqlite`                    | pure Go SQLite, so the binary stays static                            |
| `tailscale.com/tsnet`                   | the tailnet node is the process, and it brings the certificate        |

Stdlib first everywhere else. `net/http` and `http.ServeMux` over a router,
`database/sql` over an ORM, `encoding/json` over anything.

## Not decided here

- **Where the daemon runs**, and whether a machine that is asleep needs anything
  said about it. The tailnet name does not answer when the daemon is down.
- **Whether previews ever need storage**, which is the one thing that would buy
  a second tsnet hostname.
- **The exact CLI surface.** `eyeball doc` is the contract and it is written
  with the CLI, not ahead of it.
- **Pruning.** How long a decided review is kept, and whether its blobs go with
  it, is an open question in [product.md](product.md).
