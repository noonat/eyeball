---
status: active
created: 2026-10-05T00:00:00Z
updated: 2026-10-05T08:25:02.000128591Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
---

# 006 The surface: the queue and a review

> **Autonomous run, 2026-10-04.** Part of the owner's request to build the
> project to completion as product.md and architecture.md describe it, under
> `dev-review autonomous on`. A reviewer subagent stands in for the owner's
> approval of this spec and gives the `reviewed` ack on each iteration. The
> owner keeps tearout changes, questions about behavior, and anything
> outward-facing, which here is joining the tailnet. Departures from this spec
> are recorded in it as they happen.

The reviewer's surface, first part: the listeners it is served on, the queue it
opens on, and the page a review opens on. Reading only. Nothing here writes a
comment or gives a verdict, which is the surface's write path (008), and nothing
here widens past the change or renders a document, which is widening and
documents (007).

The surface (006) in specs/TODO.md held all of the reading surface. It is split
in two here, because it was about twice the size of 005, whose review pass over
the whole branch found sixteen defects living between iterations, and that count
grows with the diff. The queue and the review page are what a reviewer needs
first. Widening, rendered documents and the project view build on the same
templates and come next.

Every claim marked confirmed was run against Go 1.26.8, tailscale.com v1.102.3
and chroma v2.27.0, and re-run by a review pass.

## Design

### Where the surface is served

| Listener | Starts when                                                | Serves                    |
| -------- | ---------------------------------------------------------- | ------------------------- |
| loopback | `eyeball serve --surface-addr` names a loopback address    | plain HTTP, this machine  |
| tailnet  | `state/node` holds a node `eyeball tailnet login` enrolled | HTTPS on `:443` via tsnet |

**Joining the tailnet is a step the owner takes, never one the daemon takes by
itself.** Any agent command starts the daemon, and every daemon test runs one. A
daemon that started tsnet whenever it started would contact Tailscale's control
server and print a login URL from every agent on the machine and every test run.
So the daemon starts tsnet only for a node `eyeball tailnet login` has already
enrolled. Confirmed from tsnet's source: with no state and no auth key it prints
a login URL through `UserLogf` and waits, and with state present it reuses it.

**A first enrollment completes before `state/node` exists.** tsnet creates its
directory and writes state the moment it starts, confirmed, so a first login
interrupted halfway or failing the HTTPS check would otherwise leave a directory
every later daemon reads as enrolled. A node that is not logged in polls
Tailscale and logs a login URL every few seconds for as long as it runs,
confirmed. So a first `eyeball tailnet login` enrolls into `state/node.new`,
waits for the node to be running, checks that MagicDNS and HTTPS certificates
are enabled, closes the node, and only then renames the directory to
`state/node`.

**Logging in again re-authenticates the node in place.** A node whose key
expired needs a new login, and the fix for it is `eyeball stop`, then
`eyeball tailnet login`. With `state/node` present, the login runs tsnet on that
directory rather than a new one. Enrolling a fresh directory would make a second
machine with a new name, which changes the surface's HTTPS origin and with it
the installed page and every push subscription notification (009) creates. A
review pass confirmed that a rename onto the existing directory fails anyway.

**`eyeball tailnet login` takes the daemon's lock for its whole run, and marks
the lock file as a login.** Checking once that no daemon runs leaves the minutes
the owner spends in a browser open to an agent command starting one, and two
tsnet servers on one directory corrupt it. Holding `eyeball.lock` refuses that
daemon. The lock file then also says `login`, which `daemon.ReadHolder` reports.
Without the marker, a command finding the lock held and no socket reads it as a
stuck daemon and prints a fix telling the agent to kill the process, confirmed
by a review pass against `client.stuck`. With it, the command refuses with
`daemon_unavailable` and says a tailnet login is running and to retry once it
finishes.

**The node is started, watched, and only then listened on.** `ListenTLS` calls
`Up` with a background context, and `Up` does not return until the node is
running, confirmed, so a daemon that called it first would hang its surface
goroutine whenever the node is offline or needs a login, and its shutdown would
wait on it. Instead the daemon calls `Start`, opens a watch on the node's state
through `LocalClient().WatchIPNBus` with a context it can cancel, waits for the
running state, reads `Status` for MagicDNS and the certificate domains, and only
then calls `ListenTLS`, whose `Up` then returns at once. A node that reaches or
falls back to needing a login, which is what an expired key looks like, or that
waits for a device approval the tailnet requires, puts the surface in a failed
state and is closed rather than left to poll.

**A failed surface names what the owner does, never a command for an agent.**
The surface's state reaches agents through `/agent/daemon` and `eyeball status`,
and 005 built those to coach an agent through its next step. A fix reading
`eyeball stop` then `eyeball tailnet login` would be followed by an agent, which
would end every other agent's wait and start a login nobody completes, confirmed
by a review pass. So the state says, for example, that the owner needs to run
`eyeball tailnet login` on this machine, or to approve the device in the
tailnet's admin console, and the manual marks the login as the owner's.

**`eyeball tailnet login` requires a terminal on stdin, always.** An agent runs
commands with no terminal, and nothing but a person can complete an interactive
login. An auth key in the environment would let the login enroll the node with
no person involved, and an agent's inherited environment can hold one, so the
terminal is required whether or not `TS_AUTHKEY` is set, and the manual an agent
reads does not mention `TS_AUTHKEY`. A non-interactive enrollment is a question
for the owner, and nothing in product.md asks for one.

**tsnet's log upload is turned off.** tsnet uploads its own logs to Tailscale by
default outside tests, confirmed from its source. Nothing in product.md asks for
it, and sending a machine's logs to a third party is an outward-facing decision
nobody made. `logtail.Disable()` is the switch, process-wide and checked before
any entry is sent, confirmed, and the daemon and `eyeball tailnet login` both
call it before starting the node. tsnet has no field for it.

**The daemon's tsnet is configured only from the state directory.** The daemon
inherits the environment of whichever agent started it, the same problem 005
fixed for agent names, and tsnet reads `TS_AUTHKEY`, `TS_CONTROL_URL` and
`TSNET_FORCE_LOGIN` from the environment. Some of tsnet's knobs are read at
package initialization, before the daemon's own code runs, confirmed, so
removing them inside the daemon is too late. `daemon.Start` removes every `TS_`
and `TSNET_` variable from the environment it gives the daemon it starts. A
daemon a person starts with `eyeball serve` keeps that person's environment, so
no message an agent reads suggests `eyeball serve`: the fix for a daemon that
will not start points at `eyeball.log`, and the manual marks `serve` as the
person's. tsnet ignores an auth key for a node that is already logged in,
confirmed.

**The surface's address comes from tsnet's certificate domains**, not from the
hostname asked for, since the tailnet may give a different name. It is reported
by `/agent/daemon` and `eyeball status` with the surface's state: off, starting,
serving with its URL, or failed with its reason and fix.

**The loopback listener is for development and checks, and is asked for
explicitly.** `eyeball serve --surface-addr 127.0.0.1:8111` serves the surface
on that address, and the manual marks the flag as the person's, for debugging.
An autostarted daemon never serves it, since its flags are the ones the client
passes. The address must be a literal loopback IP: `0.0.0.0` or an empty host
binds every interface, and a name would need resolving. The loopback listener is
bound right after the socket, synchronously, so a bad address or a port in use
is an error `serve` reports at startup, in the foreground where it is read,
before the daemon serves anything.

A loopback listener gives a process running as this user nothing it does not
already have: the same process can open `eyeball.db` and read every review and
comment, released or not. It is not the same for other accounts on the machine.
A loopback port is reachable by every local user, where the state directory is
`0700`. That is the cost of the flag, and the reason it stays the person's, for
debugging, rather than something an autostarted daemon does. architecture.md
records both halves. Once the write path (008) puts a verdict on the surface,
the same reasoning covers deciding a round, and that spec has to say so.

**The tailnet never holds up the socket.** The daemon binds the socket first and
starts the node in a goroutine.

**One goroutine owns the node, and closes it at most once.** It starts the node,
waits on the watch, reports the surface's state, and closes a node that failed.
Shutdown signals and the owning goroutine closes, within the ten-second budget
of step 3 below. Shutdown marks the node stopping under a mutex, which keeps a
start that has not begun from beginning, cancels the watch's context, which ends
the wait for the running state at once, and waits for tsnet's own `Start` call
to return if one is in progress. Only then is the node closed, through a
`sync.Once` shared with the failed-node path, so the two cannot both close it.
Shutdown never waits on the node reaching the running state, which an offline
node never does.

**The node is behind an interface, and nothing in this spec runs it against
Tailscale.** The daemon and `eyeball tailnet login` both reach the node through
one small interface: start, watch its state, read its status, listen with TLS,
close. The real implementation wraps tsnet. The daemon's configuration carries a
constructor for it, so every test injects either one that fails if it is called
or a fake that scripts the node's states and status: running, needing a login,
needing a device approval, missing MagicDNS, missing certificate domains, and a
close that overruns. Every behavior this spec gives the tailnet path is proved
against that fake. No iteration runs `eyeball tailnet login`, runs `serve`
against a real `state/node`, or calls tsnet's start against Tailscale. The
tailnet path runs for real once, in notification's (009) owner iteration. A test
that looked for a `state/node` directory after the fact would fire only after
Tailscale had been contacted, which is why the guard is the injected constructor
rather than that.

**tsnet is pinned at v1.102.3.** v1.104 requires Go 1.27.1, confirmed against
the module proxy. v1.102.3 builds with `CGO_ENABLED=0` against Go 1.26.8 and
raises the `go` directive to 1.26.6, confirmed. A trivial program linking it is
28.5 MB, which the architecture already accepts for esbuild.

**Departures, iteration 3.**

- A `state/node.new` left by a first login that failed its checks is used again
  by the next login, rather than replaced. That node is already a machine on the
  tailnet, and a fresh directory would make a second one, under a new name,
  after the owner fixes the setting the first login reported. It still means
  nothing to a daemon.
- `eyeball tailnet login` reports with three codes the client gives:
  `owner_only` for no terminal, `daemon_running` for a lock held by a daemon or
  another login, and `login_failed` for a login that did not finish or a tailnet
  that cannot serve. Its `--json` answer is `api.LoginResult`. The spec named no
  codes or output for it.
- `golang.org/x/term` decides whether stdin is a terminal. A mode check on stdin
  would pass `/dev/null`, which is a character device too, and `/dev/null` is
  what a started process is given.
- The manual's command check walks subcommands, so `eyeball tailnet login` is
  checked like the rest.

### Shutdown, with the surface

005's shutdown order is extended, and the lock is still released last:

1. Waits are ended and the lock file is marked stopping.
2. The socket server and the surface servers shut down together, under one
   grace.
3. The node is closed within a budget of ten seconds, by the goroutine that owns
   it, in the order the paragraph on that goroutine gives. tsnet's `Close` must
   not run before or during `Start`, and dereferences state `Start` sets,
   confirmed, so a node whose start never began is not closed at all.
4. The store closes.
5. The lock is released.

An earlier draft said tsnet's close has a five-second bound of its own. That was
wrong: the bound covers two of its shutdown goroutines, and the rest of its
close is unbounded, confirmed by a review pass. So the budget is the daemon's. A
close that overruns it makes `Run` return without releasing the lock, and the
process exits, which releases the lock and tsnet's sockets together, so a
successor cannot start a second node on the same directory while the first is
still closing. The lock's file is kept reachable until the process exits, since
an `os.File` the collector finds unreachable has its descriptor closed by a
finalizer, which would release the lock early. A stopping daemon can hold its
lock for `ShutdownGrace` plus the budget, and the client's grace for a stopping
daemon and `stop`'s wait are sized to that sum.

**Departures, iteration 2.**

- `Surface.Stop` closes the node itself, from a goroutine of its own, through
  the `sync.Once` the failed path shares. The design above says the owning
  goroutine closes. A close that overruns has to be given up on, and only the
  caller holding the budget can do that, so the close runs beside the budget.
  The properties the design asks for still hold: no close before or during
  `Start`, and at most one close.
- A node whose watch reports it stopped, or whose watch ends other than by
  shutdown, is a failed surface and is closed. The design named only a login and
  a device approval, and a surface left in starting forever says nothing.
- A failure the node reports for itself, from its start, its watch, its status
  or its listen, has no fix eyeball can name. Its reason carries the error and
  points the owner at the daemon's log, where it is logged in full.

### Assets

**The live read refuses a path through a symlinked directory.** No route in this
spec reads the working copy, but widening (007) does, through
`capture.Reader.Open`, and its working-copy fallback uses `os.Lstat` on the full
path, which follows every component but the last. A path through a link to
outside the project would serve an outside file to the phone. 0a242b1 closed the
same hole in the walk outside git, and the reader shares that check rather than
a copy of it. It is closed here, first, so no file route can exist before it.

**`app.css` is embedded where it is, in `docs/design/`.** architecture.md says
it moves to `web/`. Moving it means editing the stylesheet link in every
tearout, and the owner reserved tearout changes, so it stays, and
architecture.md records the departure. `go:embed` cannot reach a parent
directory, so the embedding package sits at the repository root: `assets.go`,
package `eyeball`, embedding `docs/design/app.css` and `web/templates`,
confirmed to compile. The stylesheet the tearouts link and the one the surface
serves are then one file.

**The convention check that lists packages covers the root package.** It walks
`cmd`, `internal`, `tool` and `web` today, so a package at the root is the one
package the layout block could omit without failing, confirmed.

**A class a template uses must be defined by `app.css` alone.** The existing
tearout check joins `app.css` with `tearout.css`, which defines the sheet's own
chrome, so templates get a check of their own against `app.css`. A class
attribute is read with its template actions removed, and a class a template
chooses at render time comes from a fixed table in `internal/render` that a test
holds to `app.css` the same way. Templates drifting from the tearouts they are
seeded from is the likeliest failure in this spec, and an undefined class
renders its text unstyled, which looks deliberate.

**Light theme only, until the write path (008).** `app.css` defines dark tokens
under `[data-theme="dark"]` only, so following the device needs a script or a
change to the approved stylesheet. The script arrives with the write path (008).

### Rendering a diff

**`internal/render` turns a `diff.Result` into rows a template prints**: a hunk
header, then each line with its number on each side, its sign, and its text as
segments. A segment is a run of the line with a syntax role, a changed-word
mark, both, or neither, and renders as a `w` span around a role span where both
apply. Cutting the line at every boundary of either kind is what keeps the
elements nested.

**Highlighting is chroma, server side, into the five roles `app.css` defines.**
The lexer is chosen by filename, confirmed, and a file with no match is plain.
Token types map by prefix: `Keyword` to `k`, `LiteralString` to `s`, `Comment`
to `c`, `LiteralNumber` to `n`, `NameFunction` to `f`, and everything else is
plain, confirmed on Go source. A line is highlighted alone, and the tokens are
checked to reproduce it exactly. A review pass confirmed that the Makefile lexer
appends a newline and the Go lexer turns `\r` into `\n`, and either would shift
every word mark after it, so a line whose tokens differ from it is plain.
Highlighting a line alone also loses state that spans lines, such as a block
comment, which is in [specs/TODO.md](TODO.md).

**A binary file and a file past the diff limits get no rows**: the file's header
and a line saying which. What a browser could display instead is rendered files
(010).

**Departures, iteration 4.**

- A hunk header is written in git's form, `@@ -41,7 +41,14 @@`, which leaves out
  a count of one, as in `@@ -1 +1 @@`. The review tearout follows it with the
  enclosing function's name, which `diff.Hunk` does not carry, and writes the
  old range without git's minus sign. The function name is in specs/TODO.md.
- The layout's topbar takes a back link and a title, or the wordmark when there
  is no back link. The queue's count pill joins it in iteration 5, with the
  queue.
- Segments are not merged when two neighbors share a role and a mark. Mutation
  testing showed the merge changed nothing a reader sees, so it was removed.
- Correction: the design above credits the Go lexer with turning `\r` into `\n`.
  It is chroma's default `EnsureLF` option, applied to every lexer, found by the
  stand-in's review. Tokenizing with it off keeps a line holding a carriage
  return highlighted, confirmed against the Go and Python lexers.
- Correction: the design above treats the Makefile lexer's newline as a corner
  case that leaves a line plain. 32 of chroma's lexers set `EnsureNL`,
  TypeScript, C, Rust and PHP among them, and their line-comment rule runs to
  the newline, so every line ending in a `//` comment or in spaces would have
  gone plain, found by the stand-in's second review and confirmed for
  TypeScript, Rust and C. The one newline such a lexer appends is trimmed from
  the last token. Tokens that differ from the line in any other way still leave
  it plain.
- Reading a line's tokens is bounded. chroma's Jungle lexer emits empty tokens
  forever for a comment with no newline, which is how every line reaches it,
  found by the stand-in's third review. A lexer that gives back more than four
  tokens per byte of the line, plus sixteen, leaves the line plain.
- This iteration's review ran four rounds where the stop rule allows three. The
  third found the unbounded loop above, which would hang the daemon on one line
  of one kind of file, and a commit needs a review to pass the gate. The fourth
  round was scoped to the bound alone.
- Binary and too-large files get no rows from `render.Diff`, only the flag
  saying which. The page's header and line for them are printed in iteration 6,
  which now lists them.

### The queue

**`GET /{$}` is the queue.** `GET /` is a prefix pattern that matches every
path, confirmed, so it would answer `/agent/daemon` on the surface's side and
fail 005's listener test. The route test's path substitution grows to cover
`{$}`, `{round}` and the wildcards widening and documents (007) adds.

**The sections follow the tearout.** Waiting rows carry the project, agent,
title, round, size and age. Working rows carry no size, because the tearout says
what a size would mean there is not something anybody reads, and say "Changes
requested, round N". Settled rows carry an Approved pill and the round, and
their age is from the decision. Settled keeps the five most recent.

**Ages are written one way**: "now" under a minute, minutes under an hour, hours
under a day, "Yesterday" under two days, days under a week, and the date after
that. The queue tearout's note says "minutes, hours, days, then a weekday" while
its own rows show `Yesterday` and `4d`. A weekday name for something more than a
week old names the wrong week, so this departs from the note there.

**The queue filter waits for a tearout.** product.md says a filter exists. Every
queue row is one link, so a filter link inside a row would split it, confirmed
by a review pass parsing the markup, and no tearout shows a filter control. A
filter is a design change the owner reserves, so it is a `design:` item in
specs/TODO.md rather than something this spec invents.

**With nothing waiting, the queue says so in the tearout's empty state**, with a
sentence counting the agents at work and the list of projects eyeball knows,
each with its root written with `~` and its agent count, or "Idle". Working and
Settled still follow when they have rows. The install steps the empty tearout
also shows are notification (009).

**Departures, iteration 5.**

- A project row in the empty state is a `div`, where the tearout's is a link.
  The project view it would open is widening and documents (007), which makes
  the rows links.
- A working row's age is from the change request, as a settled row's is from the
  approval. The design named only the settled row's.
- `store.OpenWithClock` arrives here rather than in iteration 7, because the
  queue's tests tell a request's age from a decision's only on a clock they
  move. Iteration 7's seed uses the same open.

### A review page

**`GET /reviews/{review}` is the current round, and
`GET /reviews/{review}/rounds/{round}` any round**, in the tearout's order: the
title, project and agent, the state and round, the agent's note in full, the
size, and how long it has waited. The topbar carries the full title, and
`app.css` truncates it with an ellipsis. The tearouts write shorter topbar
titles by hand, which no rule could derive from a title.

**What the change is measured against is one control, `?against=previous` or
`?against=base`**, labeled "Since round N" and "Whole review" as the tearout
labels it. The previous round is the default from round two onward, as
product.md says. A first round has only its base.

**The files a page shows come from the same comparison that measured the
round.** `capture.Freeze` measures a round by comparing, path by path, what
moved since the previous round, and the store sums those counts into the round's
size. That per-path comparison is exported, returning each path's `diff.Result`
with its hunks and its binary and too-large flags, and the measuring sums it,
zeroing a too-large file as it does today. The page renders the same results,
against the previous round or, for `?against=base`, with no previous round,
which compares the round with its base. Recomputing the files separately was the
first draft, and a review pass found four ways it would disagree with the stored
size: a file past the diff limit, a file deleted and then restored, a path
reverted out of the capture, and a file whose scope flag the capture no longer
carries. Scope is `store.Covers` over the review's paths, as the store's own sum
uses. A test holds the page's totals to the stored size for each of those cases.

**Up to ten files and three hundred changed lines, added plus removed, the page
opens expanded. Past either it opens as a list of collapsed files.** Ten files
is product.md's number. Three hundred lines is this spec's reading of its "a few
hundred", and both are product.md's open question. A collapsed file still
carries its body here, because loading a body when its disclosure opens needs
htmx, which arrives with the write path (008).

**A markdown file shows as a diff here.** product.md opens markdown as a
rendered document, which is widening and documents (007). Until then a document
reads as a code change, which is the flip product.md already offers.

**The round's comments are listed read-only.** Each file's comments follow its
diff as the tearout's comment cards, with the path, the lines and the quoted
text, and the review-level ones follow the files. A comment written after the
verdict is marked late. Placing a comment under its own line would leave nowhere
for one whose line falls outside every shown hunk, and the write path (008)
decides placement with the composer.

**A decided round has no decision bar**, as the tearout says. It shows a notice
naming the verdict and when it went out, then the verdict note in the shape of
the agent's note. The comment box the tearout keeps on a decided round is the
write path's (008). An open round shows no decision bar in this spec either: a
bar whose buttons do nothing is the failure product.md calls the worst this
surface can have, so the bar arrives with the handlers behind it.

**An earlier round is reachable from the current one and lists its comments with
the agent's replies**, as the tearout's earlier-round frame shows.

**Links go only to routes that exist.** Nothing links to a project view or a
file route until widening and documents (007) adds them.

**Departures, iteration 6.**

- A review page lists only the files under the review's paths. A file captured
  for context is counted in neither the stored size nor the page's totals, and
  listing it would show more files than the toolbar counts. Widening and
  documents (007) reaches any file in the project.
- A comment on a file the comparison does not show is listed with the
  review-level comments, under its path, rather than dropped.
- A late comment's card says "after the verdict" beside its anchor. No tearout
  marks one, and `app.css` has no class for it, so it is a plain span.
- A line with no newline at the end of its file is not marked. The row model
  carries the flag, and no tearout or class draws it, which is a `design:` entry
  in specs/TODO.md.
- `store.Round` gains `Created`, which the wait on an undecided round is
  measured from, and `render.Ago` writes the long form the decided notice uses.
  Line counts on the queue and the review page are written with thousands
  separators, as the tearouts write them.
- `capture.Compare` takes the two readers, and finds the paths that moved from
  the captures they hold, rather than taking the file lists as well.
- Highlighting a line recovers from a panic in chroma and leaves the line plain,
  found by the stand-in's fourth review of iteration 4.

**Departures, iteration 7.**

- `web/static/shell.css` joins `app.css`, served at `/static/shell.css`. The
  first screenshots showed the browser's default margin as a strip around every
  page, which the tearouts never show because their phone frame provides the
  page around the app. The rules stay out of `app.css` because every tearout
  loads it. This is the one visual change made without the owner's phone, and it
  is in the end-of-run report for the phone check after 009.
- An earlier round shows the tearout's sections first, the closed notice, the
  comments and what was done, and then its notes, its size and its files, which
  the design's list for any round asks for and the tearout's frame leaves out.
- The current round lists the agent's replies under "What was done" too, where
  the tearout keeps them on an earlier round only. An agent can reply as soon as
  a comment is released, and on an approved round, which is the last, the
  current round is the only place a reply could show.
- A decided round keeps the agent's note, which the design's list for any round
  asks for and the decided tearout frame leaves out.
- The queue lists Waiting longest wait first, as 003 decided, where both queue
  tearout frames list the newest first. Two approved documents disagree, and
  choosing between them is the owner's, so it is a `design:` entry in
  specs/TODO.md and in the end-of-run report.
- `render.AgeAgo` writes the verdict pill's form, "2h ago", and the earlier
  rounds' list uses it, as the review tearout writes both.
- The screenshots were taken with headless Chromium at 390 pixels rather than
  the Chrome tools, because headless Chromium writes each one to a file the
  iteration's reviewer can read, and each sits beside a crop of its tearout
  frame. `tool/seed -nothing-waiting` answers every waiting round, for the empty
  queue's screenshot.

### What every surface response carries

- **`Content-Security-Policy: default-src 'self'; img-src 'self' data:; frame-ancestors 'none'`.**
  `app.css` draws every icon as a `data:` mask, and a policy without
  `img-src data:` hides all of them, which a review pass confirmed in Chromium.
  A test checks every `url(` in `app.css` against the policy.
- **`Cache-Control: no-store`**, for the tearout server's reason: iOS Safari
  otherwise shows a stale page.
- **A page for a refusal**, including the 404 and 405 the mux would otherwise
  answer in plain text. The surface's handler asks the mux for the request's
  pattern first, and an empty pattern means the mux's own fallback will answer.
  Only then does it serve through a response writer that catches the fallback's
  404 or 405 and writes the page in its place, keeping the `Allow` header on
  a 405. The pattern alone cannot tell the two apart, and the writer alone would
  replace a handler's own 404 page, so it takes both. The listener test keeps
  checking the inner mux.

### Seeing it

**`tool/seed` writes a populated state directory**: projects with repositories,
reviews in every state, rounds decided and not, comments early and late,
replies. It writes through the store and capture, because nothing on the socket
can decide a round and the write path is 008. The store's clock is unexported,
so the store gains an open that takes a clock, which is how the seed backdates
requests and decisions so ages read like the tearouts. It creates the state
directory through `daemon.Prepare`. A daemon started with `XDG_STATE_HOME`
pointing there and `--surface-addr` on loopback serves it, apart from any daemon
agents are using.

**The stand-in's visual check is the Chrome tools at 390 pixels against that
daemon**, one screenshot per page beside the tearout frame it was seeded from,
saved where the iteration's reviewer can read them. It is the closest available
substitute for the owner's phone until the owner joins the tailnet, which is an
owner-only step taken once, after notification (009), with push.

### Checks

**Pages are tested as HTML, not as strings.** A test parses the response with
`golang.org/x/net/html`, already a module dependency, and asserts on elements
and classes.

## Out of scope

- **Widening, rendered documents, the project view and file routes.** Widening
  and documents (007).
- **Comments, verdicts, htmx, TypeScript, the live stream, the connection
  notice, the theme switch, file bodies loaded on demand, and reopening a review
  where it was left.** The write path (008).
- **Push, the manifest, the service worker, installing, the badge, and the
  owner's tailnet login and phone check.** Notification (009).
- **Images, media, previews, the sandbox, too-large files.** Rendered files
  (010).

## Not converged

Three review passes ran under the stop rule: the whole draft at high effort,
then its fixes, then theirs, because the second and third both found defects
that would have failed at runtime. The third pass's five findings were fixed and
not reviewed again: the owner-only login, the guarded start, the lock's file
kept reachable, the 404 writer, and device approval. The risk left is of the
shape every round found: tsnet's start and shutdown, which no test here can run
against a real tailnet, now proved only against a fake whose script could differ
from what tsnet does. The owner's login after notification (009) is the first
time the tailnet path runs for real.

Decided here rather than by product.md, and reported to the owner at the end of
the run: the loopback listener, which exposes nothing new to this user and
exposes reviews to other accounts on the machine while the flag is set, tsnet's
log upload turned off, `app.css` left in `docs/design/`, the queue filter
deferred for want of a tearout, light theme only until the write path (008), and
the full title in the topbar rather than the tearouts' hand-written short ones,
and ages that turn into a date after a week where the queue tearout's note says
a weekday.

The stand-in's review of the spec added the node interface and the fake that
every tailnet behavior is proved against, the unconditional terminal for the
login, and the single owner of the node's close. Those changes were made after
the three review passes and were checked by the stand-in only.

## Iteration 1: Closing the live-read hole, and the asset checks

- [x] `capture.Reader` refusing a working-copy read through a symlinked
      directory, sharing the walk's check from 0a242b1, with a test reading
      nothing through a link to outside the project
- [x] The packages-listed check covering a package at the repository root, with
      a fixture it must flag
- [x] A template-classes check holding `web/templates` to `app.css` alone, with
      template actions stripped, and a fixture with an undefined class it must
      flag
- [x] `assets.go` at the repository root embedding `docs/design/app.css` and
      `web/templates`, with a first `layout.html`, and an `assets.go` line in
      architecture.md's layout block
- [x] architecture.md recording that `app.css` stays in `docs/design/` and why,
      and its `web/` line no longer claiming the stylesheet

> **Completed** 2026-10-05 08:30 UTC
>
> - acks: reviewed
> - `make check` — 2.0s
> - `go test -race ./...` — 2.0s

## Iteration 2: The listeners

- [x] `eyeball serve --surface-addr` binding a literal loopback address right
      after the socket, and failing startup on any other address or a bind error
- [x] The surface's handler turning the mux's own 404 and 405 into pages, found
      by an empty pattern and a catching writer, keeping `Allow` and a handler's
      own 404, with the CSP and `no-store` on every response
- [x] A test of every `url(` in `app.css` against the CSP
- [x] `GET /healthz` touching the database
- [x] `GET /static/app.css` serving the embedded file
- [x] The route test's substitution covering `{$}` and `{round}`
- [x] `daemon.Start` removing every `TS_` and `TSNET_` variable from the
      environment it gives the daemon
- [x] The node behind an interface with a tsnet implementation, constructed from
      the daemon's config, and every daemon test injecting a constructor that
      fails if called
- [x] A fake node scripting states and status, used by every test of the tailnet
      path, and a unit test of the start decision for no directory, a leftover
      `state/node.new` and an enrolled `state/node`
- [x] The node started with `logtail.Disable()` first, watched until running,
      checked for MagicDNS and certificate domains, then listened on with TLS on
      `:443`, proved against the fake
- [x] A node needing a login or a device approval reported as failed with what
      the owner does, never a command for an agent, and closed, proved against
      the fake
- [x] One goroutine owning the node, closing it at most once, and shutdown
      canceling the watch and waiting only for an in-progress `Start`, proved
      against the fake
- [x] The surface's state and URL from the certificate domains in
      `/agent/daemon`, `eyeball status`, and the manual with `--surface-addr`
- [x] Shutdown in the extended order: watch canceled, node closed within ten
      seconds, and an overrun returning without releasing the lock, with the
      lock's file kept reachable until exit
- [x] The client's grace for a stopping daemon and `stop`'s wait sized to
      `ShutdownGrace` plus the close budget
- [x] tailscale.com v1.102.3 added, its row in architecture.md's table checked,
      and `go install ./cmd/eyeball` still producing a working binary

> **Completed** 2026-10-05 09:34 UTC
>
> - acks: reviewed
> - `make check` — 3.4s
> - `go test -race ./...` — 20.8s

## Iteration 3: Joining the tailnet

- [x] `eyeball tailnet login` refusing with no terminal on stdin, whatever the
      environment holds, before anything else it does
- [x] `eyeball tailnet login` holding the daemon's lock for its whole run, with
      the lock file marked `login` and the holder read back reporting it
- [x] A command finding a login in progress refusing with `daemon_unavailable`
      and saying to retry once it finishes, with a CLI test that it prints no
      `kill` fix
- [x] A first enrollment into `state/node.new` with `logtail.Disable()`, the
      login URL printed, and a wait for the node to run, proved against the fake
- [x] MagicDNS and certificate domains checked, the node closed, and the
      directory renamed to `state/node` only after both pass, proved against the
      fake
- [x] A login with `state/node` present re-authenticating that node in place,
      proved against the fake
- [x] `eyeball tailnet login` in the manual marked as the owner's, with the stop
      it needs first and no mention of `TS_AUTHKEY`, and `serve` and
      `--surface-addr` marked as the person's
- [x] architecture.md stating that the daemon starts tsnet only for an enrolled
      node, what the loopback listener exposes to this user and to other
      accounts, and the fix for a daemon that will not start pointing at
      `eyeball.log`

> **Completed** 2026-10-05 09:59 UTC
>
> - acks: reviewed
> - `make check` — 3.5s
> - `go test -race ./...` — 8.4s

## Iteration 4: Rendering a diff

- [x] Templates parsed from the embedded `web/templates` at startup, the layout
      carrying the topbar, `data-theme="light"` and the stylesheet link
- [x] A row model from `diff.Result`: hunk headers, the old line number on a
      removed line and the new one otherwise, the sign, and the no-newline flag
- [x] chroma v2 added and its row in architecture.md's table corrected to the
      `/v2` module, highlighting a line by its file's lexer into the five role
      classes, plain with no lexer
- [x] A line whose tokens do not reproduce it rendered plain, with tests for the
      Makefile lexer's newline and a carriage return
- [x] Segments nesting a word mark around a role span, with a test of a mark
      crossing a token boundary
- [x] The classes render chooses at runtime in one table, held to `app.css` by a
      test
- [x] Binary and too-large files given no rows, with the flag saying which
- [x] Ages written as now, minutes, hours, Yesterday, days or a date, with a
      table test at each boundary

> **Completed** 2026-10-05 10:32 UTC
>
> - acks: reviewed
> - `make check` — 2.8s
> - `go test -race ./...` — 20.7s

## Iteration 5: The queue

- [x] `ReviewRow` carrying the latest round's decision time, and
      `store.Projects` listing each project's name, root and count of agents
      with open reviews
- [x] `GET /{$}` rendering Waiting, Working and Settled in the tearout's row
      markup, sizes on Waiting only, with the waiting count in the topbar
- [x] The empty state: nothing waiting, the agents at work, and the known
      projects with `~` roots and agent counts or Idle
- [x] Tests parsing the page: section order, each row kind's fields, and the
      empty state
- [x] A `design:` entry in specs/TODO.md for a tearout of the queue filter

> **Completed** 2026-10-05 10:42 UTC
>
> - acks: reviewed
> - `make check` — 3.0s
> - `go test -race ./...` — 20.9s

## Iteration 6: The current round

- [x] The per-path comparison `capture.Freeze` measures with exported, returning
      each path's `diff.Result`, and the measuring summing it unchanged
- [x] `GET /reviews/{review}` rendering the brief, the full note, the size and
      the wait, with the full title in the topbar
- [x] The files that moved from that comparison against the previous round or
      the base, with `?against=` as the tearout's two-link control, absent on a
      first round
- [x] A test holding the page's totals to the stored size for a too-large file,
      a delete then restore, a revert, an out-of-scope file and a binary file
- [x] Files expanded up to ten files and three hundred lines and collapsed past
      either, with a test on each side of each threshold
- [x] Each file's comments after its diff and review-level comments after the
      files, read-only, late ones marked
- [x] A decided round showing the verdict notice and the verdict note, and no
      decision bar on any round
- [x] A review or round that does not exist answered with the 404 page
- [x] Binary and too-large files rendered as their header and one line naming
      which

> **Completed** 2026-10-05 10:57 UTC
>
> - acks: reviewed
> - `make check` — 3.4s
> - `go test -race ./...` — 20.9s

## Iteration 7: Earlier rounds, seeding, and the look

- [x] `GET /reviews/{review}/rounds/{round}` for any round, with the rounds
      listed and reachable from the current one
- [x] An earlier round listing its comments with the agent's replies
- [x] `tool/seed` writing a backdated, populated state directory through
      `store.OpenWithClock` and capture, listed in architecture.md's layout
      block
- [x] Screenshots at 390 pixels of the queue, the empty queue, a review, a
      decided review and an earlier round beside their tearout frames, saved for
      the reviewer
- [x] architecture.md's routes table marking which routes exist and adding
      `/static/app.css`, and specs/TODO.md entries for this spec's departures
