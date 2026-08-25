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
a unix socket in the user's runtime directory. Nothing reachable over the
network can open a review, read one as an agent, or release a batch of comments.
The tailnet carries the reviewer's surface and nothing else.

That split is free. The CLI and the phone were never going to share an
interface, and putting the agent API on a socket means file permissions are the
whole access control story for it.

## The binary

**Go, one statically linked binary, `CGO_ENABLED=0`.** Everything is embedded:
the templates, the stylesheet, the TypeScript sources, the icon masks, the SQL
migrations.

```
cmd/eyeball/          the entry point: one function, one error
internal/blob/        content-addressed store for captured file content
internal/capture/     git shell-out, base resolution, what a round holds
internal/cli/         subcommands, flags, --json, the coaching lines
internal/convention/  tests holding this repo to its own rules
internal/daemon/      single-instance lock, socket, start and stop
internal/diff/        two captures to hunks, word marking
internal/kind/        what a file is, and therefore how it opens
internal/push/        web push, VAPID, subscription lifecycle
internal/render/      markdown, syntax highlighting, templates
internal/server/      HTTP: routes, handlers, middleware
internal/store/       SQLite: reviews, rounds, comments, decisions
tool/icons/           regenerates the icon mask table in app.css
tool/tearout/         serves docs/design over the tailnet
web/                  app.css, the TypeScript sources, the manifest
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
retries. `eyeball start` and `eyeball stop` do it deliberately, and `eyeball
status` says whether one is running and since when.

**Single instance is enforced by an exclusive lock on a file, not by the socket
existing.** A crash leaves the socket file behind, and a check that treats a
stale socket as a running daemon leaves the machine unable to start one. Holding
`flock` on `eyeball.lock` for the process's life is what makes "one at a time"
true: on startup the daemon takes the lock, and only then unlinks and rebinds
the socket. Losing the race means another daemon is live, so the loser connects
to it instead.

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

## The two listeners

| Listener | Carries | Reachable from |
| -------- | ------- | -------------- |
| unix socket | the agent API | this machine, by file permission |
| tsnet `:443` | the reviewer's surface | the tailnet |

**The CLI speaks HTTP over the unix socket.** One set of handlers, one
serialization, and `--json` is the response body rather than a second code path.
`eyeball wait` is a long poll against it, so a waiting agent costs a blocked
read rather than a loop.

**tsnet, for a certificate iOS already trusts.** An installed page is what web
push requires, an installed page requires HTTPS with a trusted certificate, and
tsnet provisions one without anything to trust on the device. The listener is
virtual, so `:443` needs no privilege.

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
node/                 the tsnet identity
eyeball.sock          the agent API
eyeball.lock          the single-instance lock
```

**SQLite through `modernc.org/sqlite`**, which is pure Go, so the binary stays
static and `go install` keeps working. WAL, `foreign_keys`, `busy_timeout`, and
`_txlock=immediate` on write transactions.

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

1. Resolve the base. `HEAD` by default, or what the agent named.
2. Ask git which files differ from it, tracked and untracked alike.
3. Write the content of each into the blob store.
4. Record the base commit id and the path-to-hash list on the round.

Everything else resolves from git at the base commit when it is asked for, which
is what makes storage proportional to what the agent changed. The three-source
resolution order, and what each is labeled as, is in
[product.md](product.md#a-round-is-frozen).

**Ignored files are not captured.** `git status` already decides what is
ignored, and a build directory in a capture is megabytes nobody will read.

## Diff

**Written here, not shelled out.** A round-to-round diff is between two blob
sets eyeball holds, and git has no view of those. The base-to-round case could
shell out and then there would be two diff paths producing two renderings.

Myers over lines, then word marking within a changed line, and the marking runs
only where a run of removals pairs one to one with the additions replacing it.
An unequal run is a rewrite, and pairing across one marks the wrong words with
confidence.

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

Syntax highlighting is `alecthomas/chroma`, server side, into the five roles the
stylesheet defines. Client-side highlighting would ship a second parser to a
phone to re-derive what the server already knows.

## The surface

**Server-rendered `html/template`, with htmx for the interactions.** The
tearouts under `docs/design/` are HTML and CSS already, and `app.css` moves to
`web/` and ships as it is. Templates are seeded from the tearout markup rather
than written a second time from a description of it.

Routes:

| Pattern | Answers |
| ------- | ------- |
| `GET /` | the queue |
| `GET /reviews/{review}` | the current round |
| `GET /reviews/{review}/rounds/{round}` | an earlier round |
| `GET /reviews/{review}/files/{path...}` | a file, at the current round |
| `GET /reviews/{review}/rounds/{round}/files/{path...}` | a file, at that round |
| `GET /projects/{project}` | the project view and its working copy |
| `POST /reviews/{review}/comments` | one comment |
| `DELETE /reviews/{review}/comments/{comment}` | drop one before the verdict |
| `POST /reviews/{review}/decision` | the verdict, releasing the round |
| `POST /subscriptions` | a device's push subscription |
| `GET /events` | the live stream, and therefore the connection |
| `GET /healthz` | liveness, after touching the database |

**Paths are spelled out.** `/reviews/` rather than `/r/`, `/files/` rather than
`/f/`. These are read in a browser address bar, quoted in a comment, and typed
into a terminal by somebody debugging, and the four characters saved are worth
less than not having to remember what the letter meant.

`http.ServeMux` with method-and-path patterns. Handlers are methods on a server
holding the store and the blob store, and each declares the narrow interface it
needs rather than depending on a package-wide one.

## The client code

**TypeScript, compiled by esbuild linked into the binary.** The sources are
embedded with `//go:embed` and transformed at startup into a bundle held in
memory. No Node at build time, no Node at run time, no committed bundle to drift
from its source, and `go install` produces something that works.

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
megabytes to the binary. It strips types rather than checking them, so **`tsc
--noEmit` is a gate rather than a build step**, and Node is a development
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
- `oxfmt --check` on every committed markdown file, at 80 columns.
- `tsc --noEmit` and `oxlint --type-aware` on the TypeScript, once there is any.
- `go test ./...`, which includes `internal/convention`.
- The tearout checker, which today is `docs/design/check.py` and becomes a Go
  test here.

**Make every new gate fail on purpose once before trusting a pass from it.**

## Dependencies

| Dependency | Why |
| ---------- | --- |
| `modernc.org/sqlite` | pure Go SQLite, so the binary stays static |
| `tailscale.com/tsnet` | the tailnet node is the process, and it brings the certificate |
| `evanw/esbuild` | TypeScript without Node, which is what keeps `go install` working |
| htmx (vendored) | the interactions, as server-rendered fragments |
| `alecthomas/chroma` | syntax highlighting, server side |
| `yuin/goldmark` | markdown |
| `SherClockHolmes/webpush-go` | push payload encryption |
| `cockroachdb/errors` | a stack at the point an error was constructed |
| `urfave/cli/v3` | flags in any position, confined to the CLI package |
| `onsi/gomega` (test) | assertions |

Stdlib first everywhere else. `net/http` and `http.ServeMux` over a router,
`database/sql` over an ORM, `encoding/json` over anything.

## Not decided here

- **Where the daemon runs**, and whether a machine that is asleep needs anything
  said about it. The tailnet name does not answer when the daemon is down.
- **Whether previews ever need storage**, which is the one thing that would buy
  a second tsnet hostname.
- **Whether the surface needs a JS build step at all** beyond esbuild's default
  bundling, which is in [specs/TODO.md](../specs/TODO.md).
- **The exact CLI surface.** `eyeball doc` is the contract and it is written
  with the CLI, not ahead of it.
- **Pruning.** How long a decided review is kept, and whether its blobs go with
  it, is an open question in [product.md](product.md).
