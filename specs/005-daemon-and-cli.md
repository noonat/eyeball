---
status: done
created: 2026-10-04T00:00:00Z
updated: 2026-10-05T06:55:07.268014622Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
---

# 005 The daemon and the CLI

> **Autonomous run, 2026-10-04.** The owner asked for the project to be built to
> completion as product.md and architecture.md describe it, following the
> dev-workflow skill, with `dev-review autonomous on`. A reviewer subagent
> stands in for the owner's approval of this spec and gives the `reviewed` ack
> on each iteration. The owner keeps every tearout change, every question about
> how something should behave, and anything outward-facing. Where the work
> departs from this spec, the departure is recorded here as it happens.

One process owns the store, and every agent on the machine talks to it through a
command-line client over a unix socket. This spec builds that process, the
socket API an agent drives, and the CLI. After it, an agent can open a review,
ask for the next round, wait for the verdict, reply to a comment, abandon a
review and read its own state back after a restart. Nobody can give a verdict
yet: the reviewer's surface is the surface (006) and its write path (007).

Three facts shape the design. The daemon is started by an agent and used by a
person hours later, so it has to start on demand, survive its starter, and never
be two. `wait` is how an agent learns the verdict, so its answer has to survive
a killed process and a cleared context without losing a comment. And an agent
gets this interface right with no worked example in front of it, which settles
most of the CLI.

Every claim below marked as confirmed was run on Linux 7.2 with Go 1.26, and
re-run by a review pass.

## Design

### Where things live

**Everything is under the state directory**, `$XDG_STATE_HOME/eyeball`,
defaulting to `~/.local/state/eyeball`: the database, the blobs, the socket, the
lock file and the daemon's log. A relative `XDG_STATE_HOME` is ignored, as the
XDG specification says, because `store.Open` refuses a relative path and a
directory that depends on the caller's working directory is not one place.

architecture.md says the state directory under Storage and "the user's runtime
directory" for the socket under What runs. The two disagree, and the state
directory wins. The runtime directory is the better home for a socket in
principle, because it is cleared at logout. It loses on one property:
`XDG_RUNTIME_DIR` is set by a login session and is missing under cron, `ssh`
without a session manager, and some harnesses. A client with the variable and a
daemon without it would look for the socket in two places, and the lock file
would still need a home both agree on. architecture.md is corrected in the
iteration that builds the daemon.

**The state directory is created `0700` and checked.** Connecting to a unix
socket needs write permission on it, and a socket is created with the process
umask: confirmed, a listener under umask `022` produced `srwxr-xr-x`, which
already refuses other users. The directory mode is what makes that a decision
rather than a umask accident, since file permissions are the whole of the agent
API's access control. A directory that exists with group or other bits set is
refused with its mode named, rather than silently tightened, because a directory
somebody opened up on purpose is not this process's to close.

**A socket path is at most 107 bytes.** Linux stores it in a 108-byte field with
a terminating NUL. Confirmed: a 107-byte path binds and a 108-byte one fails
with `bind: invalid argument`, which names nothing.

**The client runs both checks before it starts a daemon.** A started daemon's
stderr is the log file, so a refusal from it reaches nobody, and the agent would
see only a timeout. The checks are one function in `internal/daemon` that the
daemon and the client both call.

### One daemon, started on demand

**Single instance is an exclusive `flock` on `eyeball.lock`, held for the
process's life.** The daemon opens the lock file with `O_RDWR|O_CREATE` and
takes `LOCK_EX|LOCK_NB`. Confirmed: a second process asking while the first
holds it gets `EWOULDBLOCK` at once. The file is opened without `O_TRUNC`,
because a loser that truncates on open wipes the winner's contents before
finding out it lost, which a review pass confirmed. Only after taking the lock
does the daemon truncate the file, write its pid and start time, unlink the
socket path and bind it. That order is what makes a stale socket left by a crash
harmless.

**A daemon that loses the race exits with status zero** and writes the holder's
pid and start time to stderr. Another daemon is live, and whoever started this
one connects to that one instead. `eyeball serve` run by hand against a running
daemon behaves the same way.

**Any command that needs the daemon dials the socket, and starts one if nothing
answers.** A dial that fails with `ENOENT` or `ECONNREFUSED` starts
`eyeball serve` from `os.Executable()` and keeps dialing for up to five seconds.
If the child it started exits before the socket answers, it starts another, once
per exit, which settles a start that raced a stop: the first child loses the
lock to the dying daemon and exits, and the next one wins. On giving up, the
client prints the last lines of `eyeball.log`. Five seconds is a guess, measured
against nothing yet: opening the database and applying migrations is the whole
of startup until the surface (006) adds a network node.

**The started daemon is detached from the command that started it.** It runs in
its own session (`Setsid`), with stdin and stdout on `/dev/null`, stderr
appended to `eyeball.log`, and its working directory set to the state directory.
A child that inherits the CLI's stdout keeps the write end of the pipe an
agent's harness is reading, and the harness waits for end-of-file on that pipe.
Confirmed: with the pipe inherited, `parent | cat` took three seconds, the
child's whole life, and with the child detached it took two milliseconds. An
agent's first `eyeball request` would otherwise never return. The working
directory is set because the daemon would otherwise hold open whatever directory
its first caller ran in, which may be a worktree that is deleted an hour later.

**git runs without the variables git itself calls repository-local.** The daemon
serves every project and inherits the environment of whichever agent started it,
for its whole life. A review pass confirmed what that does: with `GIT_DIR`
pointing at one repository, `git -C <another>` still reported the other
repository's top level, so 004's top-level check passed, and `diff` listed the
first repository's files. `GIT_INDEX_FILE` does the same for the index. An agent
whose first `eyeball request` ran inside a git hook would corrupt every later
capture of every project.

The list comes from `git rev-parse --local-env-vars`, asked once per process and
cached. It is the list git uses itself when it recurses into a submodule, so it
names exactly the variables that point at one repository: `GIT_DIR`,
`GIT_WORK_TREE`, `GIT_INDEX_FILE`, `GIT_CONFIG_PARAMETERS` and eleven more on
git 2.55. Removing every `GIT_*` was the first answer and is wrong: it also
removes `GIT_CONFIG_GLOBAL`, `GIT_EXEC_PATH` and `GIT_SSH_COMMAND`, which belong
to the user rather than to a repository. The scrub is in capture's one git
runner, so a daemon started by hand gets it too.

004 left the agent's configuration alone, and this does not reverse that.
Repository, global and system configuration still apply, including a global file
relocated with `GIT_CONFIG_GLOBAL`. What is removed is the state of one shell
that happened to be inside one repository.

**`eyeball start`, `eyeball stop` and `eyeball serve` do it deliberately.**
`start` dials and starts if needed, then reports the daemon either way. `serve`
runs a daemon in the foreground, which is what `start` executes and what a
person debugging runs. `stop` asks the daemon to exit through the socket and
then waits until a dial fails, so the next command starts a fresh daemon rather
than reaching the dying one. `stop` never starts a daemon in order to stop it.

**Shutdown happens in one order, and the lock is released last.**

1. The stop handler answers, then signals the main goroutine. Calling `Shutdown`
   inside the handler waits for that handler to finish, which a review pass
   confirmed runs out the deadline before the client hears back.
2. The server's base context is canceled. `http.Server.Shutdown` waits for
   active requests, and a long poll does not finish. Confirmed: with one `wait`
   in flight, `Shutdown` ran out its two-second deadline, and with the base
   context canceled first it returned at once.
3. `Shutdown` closes the listener, which unlinks the socket.
4. The store closes.
5. The lock is released.

The lock goes last because a closing Go `UnixListener` unlinks whatever is at
its path. A review pass confirmed it: a daemon that released the lock and then
closed its listener deleted the socket file its successor had just bound,
leaving a live daemon nobody could reach.

**A canceled `wait` answers `503`, never an empty `200`.** Confirmed: a handler
that returned when its context ended produced a `200` with an empty body,
because nothing had been written. A client that read that as a response would
print an empty batch, which reads as a verdict with no comments. The handler
writes a `daemon_stopping` refusal, and the client also treats a body that does
not decode as a failure.

**The server sets no `WriteTimeout`**, because it would cut every long poll at
that duration. `ReadHeaderTimeout` is set, since it bounds only a client that
never finishes sending a request.

**A daemon running a different binary from its client is reported.**
`go install` replaces the binary on disk and leaves a running daemon on the old
code. Each process computes a build identifier once, at startup: the VCS
revision and modified flag from `debug.ReadBuildInfo`, plus the size and
modification time of `os.Executable()`. The second half is what changes on a
rebuild of a dirty tree, where a review pass confirmed the VCS fields alone stay
the same. Once at startup matters: a review pass confirmed that a running binary
stating `os.Executable()` after the file was replaced reports the new file, so a
daemon computing it per request would always match the new client. A client
whose identifier differs writes one line to stderr naming `eyeball stop`. It
does not restart the daemon itself: a reviewer may be mid-review on the surface
the old daemon serves. Two installed copies of the binary at different paths
also differ and also warn, which is accurate: they are different builds. The
identifier is a variable the tests set, because a test binary carries no VCS
information.

**It does not exit when idle**, per architecture.md. That rule and the smaller
alternative are already in [specs/TODO.md](TODO.md).

**Bare `eyeball` prints help.** The comment in `cmd/eyeball` says that with no
subcommand the binary is the daemon. That was written ahead of the CLI and is
wrong for the user it serves: an agent running `eyeball` to see what it does
would block forever in a foreground daemon. The comment is corrected in the same
change.

### The project and the agent

**A project is found by walking up from the working directory to the nearest
directory holding a `.git` entry or a `.eyeball` marker file.** The nearest
wins, whichever it is. That is the rule product.md states for git, extended to
the marker, and it agrees with 004, which decides whether a root is a git
project by whether the root itself holds `.git`. A marker inside a repository
therefore names a project that is walked rather than diffed, which is the case
already in [specs/TODO.md](TODO.md). Reaching `/` with neither is a refusal that
says to run from inside a git repository or to create `.eyeball` at the project
root.

**The root's identity and the home directory are resolved through symlinks, and
review paths are computed against the root as it was reached.** `os.Getwd`
returns the shell's `$PWD`, which a review pass confirmed keeps a symlinked
path. The root is the project's identity in the store, so two shells reaching
one repository by two spellings would otherwise be two projects, and an agent
restarted through the other spelling would find its reviews gone. So the walk
returns two roots: the logical one it found by walking up `$PWD`, and the same
directory resolved, which is the identity.

Review paths are made relative to the logical root, after joining a relative
path to the logical working directory. An agent builds an absolute path from the
`$PWD` it sees, and a path computed against the resolved root would come out as
leaving it, for relative and absolute paths alike. Nothing in the path is
resolved, because that would follow a symlink inside the project, which is
content to review rather than a spelling. The home directory is resolved so the
`~` in the default agent name still applies when `/home` is itself a link.

The marker's name is a decision this spec makes and product.md did not:
`.eyeball`, an empty file, with nothing read from it. What a marker could hold
is an open question in product.md and stays one.

**A git worktree is its own project.** Its root holds a `.git` file, so the walk
stops there. This is the reading the walk produces, and it is the right one for
three reasons. Capture runs git at the project root, and a worktree's changes
are only visible from the worktree. Overlapping scopes are refused within a
project, and two worktrees are two working copies: claiming `internal/store` in
one must not block the same path in another, because sharing a repository is no
more a reason to share a verdict than sharing a working copy is. And the root is
the project's identity in the store, so two worktrees at two paths are two
projects whatever is done.

The rejected alternative is a project per repository, with each worktree an
agent inside it. It matches how the tearouts label rows, with a repository name
as the project and a branch as the agent, and it needs a capture root per
review, an overlap rule that knows about working copies, and a second identity
beside the root. The tearout labels are kept by naming rather than by structure,
below.

**A linked worktree's display name comes from its repository.** When
`git rev-parse --path-format=absolute --git-dir` and `--git-common-dir` differ,
the root is a linked worktree, and its name comes from the common directory: the
base name of the directory holding it when it is called `.git`, and otherwise
its own base name with a `.git` suffix stripped, or the base name of its parent
when it is called `.bare`. Every worktree of `harbor` then displays as `harbor`.
Everywhere else, a main checkout, a submodule, and a directory outside git, the
name is the root's own base name. A review pass confirmed the cases this avoids:
a submodule's common directory is `super/.git/modules/sub`, which would name it
`modules`, and a main checkout prints a relative `.git` without the absolute
flag. The name is read only when a project is first seen, as
`store.EnsureProject` already does.

**An agent is named by `--agent`, then `EYEBALL_AGENT`, then the working copy's
path with the home directory written as `~`.** product.md identifies an agent by
the working copy it runs in, with an explicit name for two agents in one
directory. The working copy is the project root here, so the default name is the
root's path, which is how the tearouts show an agent in a main checkout
(`~/src/harbor`). An agent that wants its branch shown, as the tearouts show for
worktrees, sets `EYEBALL_AGENT` once in its environment.

The rejected default is the current branch name. It reads well in a queue and it
moves: an agent that switches branches would stop seeing its own reviews, which
is the one thing identity has to survive.

**An agent acts on its own reviews only.** `request --review`, `wait`,
`receipt`, `abandon` and `reply` refuse a review in another project with
`wrong_project`, naming that project, and one in this project opened under
another name with `wrong_agent`, naming the agent and saying to pass
`--agent <name>`. Both checks run before anything is captured or written.
Without the project check, an agent in one project running
`request --review <id>` with another project's id would freeze its own working
copy into the other project's review, and the store's base check catches that
only when the two bases differ. The agent check is product.md's: an agent's view
is its own slice. The lost-context case that a mismatch usually is gets its fix
in the refusal rather than a hole in the rule.

**`status` lists this agent's reviews and counts the rest.** It reads
`store.ReviewsFor`, which already says it is the slice an agent sees, and adds
one line when the project holds reviews under other agent names, giving the
count and saying `eyeball status --all` lists them with their agents, which it
reads through `store.ReviewsIn`. An agent that forgot its `--agent` name finds
it that way.

**`eyeball show <id>` reads any review, and changes nothing.** It is the one
command that skips the project and agent checks and posts no receipt. product.md
says an approval is a fact that anything gating on human review can read: which
review, which round, what content and when. A gate such as a git hook is not the
agent and does not run from its root, so it cannot use `wait`, and if it could,
the receipt `wait` posts would stamp the agent's late comments delivered on the
agent's behalf. `show --json` returns the review, each round's number, note,
verdict and decision time, and the latest round's captured files with their
digests. It returns no comments, which are the agent's to receive.

**Review paths are relative to the working directory**, as a shell would read
them, and absolute paths are accepted. The daemon joins a relative one to the
caller's logical directory, makes either relative to the logical root and hands
it to the store, which already normalizes and refuses a path that leaves the
root.

### The agent API

**HTTP over the unix socket, JSON both ways**, per architecture.md. One set of
handlers, and `--json` prints the response body as it arrived rather than a
second serialization.

| Route                                  | Answers                                       |
| -------------------------------------- | --------------------------------------------- |
| `GET /agent/daemon`                    | pid, start time, build identifier             |
| `POST /agent/stop`                     | stop after answering                          |
| `POST /agent/requests`                 | open a review, or the next round of one       |
| `GET /agent/reviews/{review}/wait`     | long poll for the verdict and its comments    |
| `POST /agent/reviews/{review}/receipt` | which released comments the agent has printed |
| `POST /agent/reviews/{review}/abandon` | withdraw the question                         |
| `POST /agent/comments/{comment}/reply` | the agent's reply to one released comment     |
| `GET /agent/status`                    | this agent's reviews and what each waits on   |
| `GET /agent/reviews/{review}`          | any review, read only, for `eyeball show`     |

Every request carries the caller's working directory and agent name, in the body
or as query parameters on a `GET`.

**The socket serves the agent API and nothing else, and the surface's listener
will serve nothing of it.** Every route is declared once, in one table that
names the listener it belongs to, and the server builds one mux per listener
from that table. A test walks the table and asks each listener's mux, through
`http.ServeMux.Handler`, which pattern a request for the route matches: its own
side must match the route's pattern and the other side must match none. A status
code would not do, because a route that exists answers 404 for a review that
does not. In this spec the surface's side has no routes yet, and the test can
still fail: a server that built one mux and served it on both listeners fails it
on the first agent route. The surface (006) and its write path (007) add their
routes to the same table and inherit the test. Without it, nothing stops a later
route from putting a verdict on the socket, where an agent could approve its own
review, or the agent API on the tailnet.

**The wire types live in a new package, `internal/api`**, imported by both the
server and the CLI. A response type is the stable schema product.md promises,
and keeping it in one place is what lets a test hold `eyeball doc` to every
field in it. The rejected alternative is the CLI importing `internal/server` for
its types, which ties the client to handler code and spreads the schema across a
package nobody reads for it.

**A refusal is `{"error": {"code", "message", "fix"}}`** with a 4xx status, and
a failure inside the daemon is the same shape with a 500 and a message that does
not leak the raw error, per docs/conventions.md. `code` is a stable lowercase
identifier, `message` says what happened, and `fix` is the command or action
that resolves it. product.md asks that a refusal explain and name the fix, and a
separate field is what stops the fix from being dropped by a handler that writes
only a message.

**Every refusal the store or capture can return is a sentinel or a typed error,
mapped to a code.** The store today returns plain errors for an empty note,
title, agent or path, which would surface as a 500 with no fix. Those become
`store.ErrMissing`, wrapped with the field's name. An overlap becomes
`store.ClaimedError`, carrying the path and the holding review's id and agent
and matching `ErrPathClaimed` through `errors.Is`, so the handler can build the
fix without parsing a message. A test parses the source of `internal/store` and
`internal/capture` with `go/parser`, collects every exported `Err` variable, and
fails on one the handler's mapping does not name. Go cannot enumerate a
package's variables by reflection, and a hand-written list in the test would
pass when a sentinel is added and forgotten.

### Requesting a review

**`eyeball request <paths...> -m <note>` opens a review.
`eyeball request --review <id> -m <note>` asks for its next round.** Paths and
`--review` are mutually exclusive, because a review's scope is fixed when it
opens. The rejected alternative is inferring the next round from paths that
match an existing review. It saves one flag and turns a typo in a path into a
second review over a near-identical scope, or the reverse.

**`-m` may be given more than once, and the values join with a blank line**, as
`git commit` does. A review pass confirmed that a plain string flag keeps the
last value and drops the rest without an error, and an agent writing
`-m "summary" -m "detail"` from git habit would lose its summary.

**The overlap refusal names the holding review and the right next step for it.**
The likeliest reason to run `request` on a claimed path is an agent asking for
round two the way it asked for round one. If the holding review is waiting, the
fix is `eyeball wait <id>`. If its latest round is decided, the fix is
`eyeball request --review <id> -m "..."`. If it was opened under another agent
name, both commands would be refused, so the fix names that agent and says to
pass `--agent <name>` or to choose other paths. The handler reads the holding
review from the id the `ClaimedError` carries.

**`--title` defaults to the first line of the note that is not blank**, cut at
72 characters on a word boundary. A review needs a title for the queue,
product.md's verb list does not include one, and the note's opening line is what
an agent writes as a summary anyway. The CLI and the handler both refuse a note
that is blank throughout. The length is a guess at what reads well in a row and
is tunable once there is a queue to look at.

**`--base <ref>` names the base on a first round**, resolved through
`capture.ResolveBase`, and is refused with `--review`, because a base is fixed
at the first round and the store already refuses another one.

The handler finds the project, ensures it in the store, checks a `--review`
against the caller's project and agent, resolves the base on a first round or
reads the review's recorded base on a later one, freezes the capture with
`capture.Freeze`, and writes the review or the round. A later round is checked
for an outstanding request before anything is captured, so an agent asking twice
is refused without a capture's worth of git calls. The overlap check runs after
capture, inside the store's transaction, because only that transaction can make
it race-free. A refused review therefore leaves its blobs behind, which costs
disk until pruning exists, the same answer 004 gave for content nobody
references.

**A first round with nothing inside its scope is refused.** 004 refuses a
capture that is empty, and a capture holding only files outside the scope is
not: it would open a review whose own paths show no change, with the agent's
other work visible only as context. That means the paths are wrong or the base
already contains the work. `store.OpenReview` refuses it with
`store.ErrNothingInScope`, because the scope match is the store's and is decided
inside its transaction, and the handler's fix names both narrower or different
paths and `--base`.

**A first-round path that matches nothing is refused by name.** A path that is
neither on disk nor covered by a captured file, a deletion included, is almost
always a typo. 004 refuses this only outside git, where the walk is driven by
the paths. The handler checks it after capture for both, through
`store.NormalizePath` and `store.Covers`, which export the store's own
normalization and segment-boundary match rather than a second copy of them.

### Waiting

**`wait` answers with one of three kinds, and each has its own exit status.**
The first that applies wins.

| Kind        | When                                                                     | Exit |
| ----------- | ------------------------------------------------------------------------ | ---- |
| `abandoned` | the review was withdrawn                                                 | 0    |
| `verdict`   | the latest round has a decision                                          | 0    |
| `comments`  | the latest round is waiting and a late comment has not been acknowledged | 3    |

A late comment is one written after its round was decided, or on an abandoned
review's undecided last round. Otherwise `wait` blocks. A `comments` response
holds only the unacknowledged late comments, since those are what triggered it,
and the round's still-waiting state.

The `comments` kind gets a status of its own because a harness running
`eyeball wait 5 && continue` must not continue while the round is still waiting,
and that is what an exit of zero would tell it. A `--timeout` that expires exits
with 2.

**Only late comments trigger the `comments` kind, and every unacknowledged
comment goes into a verdict batch.** The two rules are kept apart on purpose.
Counting an ordinary comment as a trigger would make `wait` return at once on
every later round for any agent that never acknowledged its last batch, which is
every agent that read it through `--format`. Leaving an ordinary comment out of
later batches would lose it: once the next round exists, `wait` shows that
round, so an agent that read round one's verdict without printing its comments
and asked for round two would never be shown them again.

**A `verdict` batch is the latest round's verdict, note and every comment on it,
plus every released comment on an earlier round the agent has not
acknowledged.** Running `wait` twice returns the same batch twice, which is what
lets an agent that lost its context, or whose `wait` was killed by a harness,
run it again and get everything.

**Comments on an undecided round of an open review are never in any response.**
product.md requires that nothing is delivered before the verdict, and this is
where that is enforced.

**A comment is released when its round is decided or its review has ended.**
product.md allows a comment on a closed round and says it goes out on its own
immediately, and the store already allows a comment on an abandoned review on
the same reasoning. So a comment written after a verdict, or on an abandoned
review's last round, is released at once and wakes a waiting agent. An
`abandoned` batch carries every released comment the agent has not acknowledged.

**Receipt is acknowledged by the client after it has printed, and delivery is at
least once.** The CLI prints the batch, then posts the ids of the comments it
printed to `/receipt`, and the store stamps them delivered. Delivery state
decides two things: whether a comment is still owed to the agent, which is what
the `comments` kind reports, and whether `status` says comments are waiting to
be read. The rejected alternative is stamping comments delivered when the
response is written. A long poll killed between the daemon's write and the agent
reading the output would then lose those comments, which is product.md's first
named failure. Printing a comment twice costs a repeated line.

**`wait --format` posts no receipt.** The client cannot know which comments a
template printed, and a template showing only the verdict would stamp every
comment delivered without the agent having seen one. `--json` prints every
comment, so it does post one. The cost falls on late comments: an agent that
only ever waits with `--format` keeps being handed the same late comments, with
exit 3, until a plain or `--json` `wait` acknowledges them. Repetition is the
failure this design prefers to loss. The manual says so.

**The waiter reads, then blocks, with its subscription taken before the read.**
The handler subscribes to change signals, reads the review, and blocks on the
subscription. Read first and a verdict landing between the read and the
subscription is never seen, and the agent waits forever on a review that was
answered. A test hook runs immediately after the read and gives a verdict there,
which is the only place the race lives.

**Change signals are a coalescing broadcast with no payload.** Each subscriber
holds a channel of capacity one, and a publish does a non-blocking send to every
subscriber. A subscriber that has not drained its last signal loses nothing,
because a signal says only that something changed, and the waiter re-reads. A
payload would need a buffer per subscriber and a policy for a full one, and
every policy either drops an event or blocks the writer.

**Every write publishes, because the handlers never see the bare store.** The
server wraps the store in a type whose mutating methods call the store and then
publish on success. A handler written later that wrote directly would leave a
waiter that never wakes, so the server holds only the wrapper. The surface's
verdict handler (007) uses the same wrapper.

**`--timeout` is enforced by the client**, as a deadline on the request context.
The server needs no notion of it, and a client killed by its harness looks to
the server exactly like one that timed out. The `still_waiting` body the client
prints under `--json` is therefore written by the client, not received, and its
type is in `internal/api` with the rest so the manual covers it.

**A dropped connection exits with status 1 and names the command to re-run.**
The alternative is reconnecting, which dials, finds no daemon, and starts one,
undoing an `eyeball stop` somebody ran on purpose.

### Status, reply, abandon

**`eyeball status` reports the daemon and this agent's reviews.** It starts a
daemon if none answers, like every command but `stop` and `doc`, and says so
when it did. A daemon that is not running is not a state worth reporting
separately, because the next command starts it.

It lists the agent's open reviews, plus any ended review that still holds
comments the agent has not acknowledged, with `--all` listing every review in
the project and its agent. Each row carries the review's id, title, agent,
round, state, and the count of undelivered comments. A review is `waiting` with
no decision on its latest round, `changes_requested` when that decision asks for
changes, and `approved` or `abandoned` once it has ended. That vocabulary is the
store's, with waiting and working split the way product.md's queue splits them.

**`eyeball reply <comment> -m <text>` replies to one released comment.** A reply
to a comment that is not released is refused, since the agent cannot have seen
it. A second reply replaces the first: product.md allows one optional reply per
comment, and the latest is the one that says what was done.

**`eyeball abandon <id>` withdraws a review**, waiting or not, through
`store.Abandon`, and wakes any `wait` on it, which returns `abandoned`.

### The command line

**`urfave/cli/v3`, per architecture.md, confirmed to parse flags in any
position.** Against v3.14.0: `request a.go -m "hi there" b.go --json` yielded
the note, `--json` and both paths, `--json request -m x a.go` yielded the same,
and a root flag after the subcommand was read as the root flag. `--` ends flag
parsing, so a path that starts with a dash is reachable.

**Its own exit and output handling is replaced, on every command.** A review
pass confirmed that an unknown subcommand made v3 call `os.Exit(3)` from inside
`Run`, through its default exit-error handler, which would kill the test binary
of any in-process CLI test, and that a usage error prints help on stdout, which
corrupts `--json` output. The root sets a no-op `ExitErrHandler` and takes
`Writer` and `ErrWriter` from the run function's arguments. `OnUsageError` and
`DisableSliceFlagSeparator` are set on the root and on every subcommand, because
v3 applies each per command: a review pass confirmed that `OnUsageError` on the
root alone still printed help for a bad flag on `request`, and that a slice flag
splits `-m "detail, with a comma"` into two values unless the subcommand
disables the separator.

**There is no root `Action`.** With none, bare `eyeball` prints help and returns
nil. With one, an unknown subcommand reaches it as an argument, and a review
pass confirmed that `eyeball wiat 5` then printed help and exited zero, which an
agent reads as having waited. Without one, an unknown subcommand returns an
error with nothing printed, and the run function turns it into a refusal naming
`eyeball --help`.

**Exits 2 and 3 travel in a private error type, and `main` honors nothing
else.** v3 builds an `ExitCoder` with code 3 for an unknown subcommand, which
collides with `wait`'s 3. So the run function returns the CLI's own status type
for 2 and 3, `main` checks for that type alone, and every other error,
`ExitCoder` or not, exits 1.

**`--json` prints the response body, and a refusal under `--json` prints its
error body on stdout.** An agent that asked for JSON parses stdout, and a
refusal it cannot parse is a refusal it cannot act on. The exit status is still
1, and human mode prints the message and fix on stderr.

**`--format <template>` runs a `text/template` over the same body decoded as
JSON**, so a template addresses `{{.review.id}}` by the names `--json` shows
rather than by Go field names an agent cannot see. The body is decoded with
`UseNumber`, and every number is then converted to an `int64` when it is
integral and a `float64` otherwise. Decoding straight into `float64` prints a
size of twenty million as `2e+07`, and leaving `json.Number` in place breaks
`{{if gt .review.undelivered 0}}` with `incompatible types for comparison`, both
confirmed by a review pass. The decoded value is `map[string]any`, the one place
the convention against `any` gives way, because the shape is the JSON document's
and the template is its only reader.

**Human output ends with a `next:` line on every mutation**, and the JSON
carries the same text in a `next` field so an agent reading JSON gets it too.
After `request` it says to stop and run `eyeball wait <id>` and not to continue
until it returns, which is the one place product.md asks the tool to say stop.
After a `changes` verdict it says to make the changes, optionally reply, and run
`eyeball request --review <id> -m "..."`. After an approval it says the work may
continue.

**Exit status is 0 on success, 1 on a refusal or a failure, 2 when
`wait --timeout` expires, and 3 when `wait` returns comments while the round is
still waiting.**

**`eyeball doc` prints the manual from the binary**, with no daemon. It is a
markdown file embedded in `internal/cli`, so the prose checks in
`internal/convention` cover it. It has a reference section listing every JSON
field of every response type by its dotted path, as in `review.id`, in a code
span. A test fails when a command, a flag, or a dotted path is missing from it.
A substring test would pass with an entry deleted, because short field names
like `id` and `next` appear all over any manual. Without the test the manual
drifts from the CLI the first time either changes, and product.md's bar is an
agent driving the tool from the manual alone.

**Departures, iteration 5.**

- The client calls `Check` before its first dial, not only before starting a
  daemon. A socket path past 107 bytes fails the dial itself with "invalid
  argument", before any start. A version of this iteration called `Check` only
  through `daemon.Start`, on the claim that a separate call could not change any
  outcome. That claim was wrong: mutation testing showed only that no test
  covered the difference, and a review pass found the case.
- Two codes the daemon never sends were added for the client to report:
  `connection_lost`, for a connection closed before an answer on any command,
  and `daemon_unavailable`, for a daemon that cannot be reached or started.
- `StopResult` gained `stopped`, so `stop --json` prints one shape whether or
  not a daemon was running.
- A receipt that fails after the comments were printed is a warning on stderr,
  and the exit status stays the one the wait's kind gives.
- `eyeball doc` is wired in this iteration with a placeholder, and the comment
  in `cmd/eyeball` is corrected here rather than in iteration 6, because `main`
  changed in this iteration and the old comment described the new code wrongly.
- `EYEBALL_NO_START`, which turns autostart off, is how the CLI tests run
  without a binary to start. Iteration 6's manual documents it, with the other
  environment variables.

**Departures, from the review pass over the whole branch.**

- A command starts at most three daemons, and only while the lock is free or is
  released in time: within a second, or within the shutdown grace for a daemon
  that has marked its lock file stopping. The marker exists because a stopping
  daemon removes its socket first and may then finish a capture, which a
  one-second wait would have called stuck. The spec said to start another on
  every early exit, for five seconds, with no bound. A daemon that cannot start
  exits at once, so that rule started about 1,800 processes in five seconds, and
  it never helped against a live daemon whose socket was removed, since each new
  one lost the lock.
- A daemon that holds the lock and does not answer is reported with its pid and
  a `kill` fix, by every command and by `stop`. `stop` had answered "no daemon
  is running" while one held the lock.
- `stop` waits until the lock is free or held by another pid, so a new daemon
  started by another agent's command in the meantime does not read as a daemon
  that failed to stop.
- Shutdown ends waits through a channel the server closes, and no longer cancels
  every request. "Stopping cancels every waiting agent first" was right about
  waits and wrong in mechanism: canceling the base context also aborted a
  capture in progress, which answered `internal`. A capture now finishes within
  the five-second grace.
- A plain-text 404 or 405 from the daemon is reported as an older build, naming
  `eyeball stop`, rather than as a lost connection.
- A failed write of human output fails the command, so `wait` posts no receipt
  for comments that never reached the screen.

- The caller sends its home directory, and an agent's default name is written
  relative to it. The daemon inherits whichever agent started it, so a name
  taken from the daemon's home changed when a different agent started the next
  daemon, and an agent lost its own reviews.
- A verdict has a receipt of its own, in migration `0004`, stamped when a wait
  prints it. `status` lists an ended review until both its last verdict and
  every comment are received. The spec's rule listed an ended review only while
  it held unread comments, so an approval with none dropped out of `status` the
  moment it was given, reachable after a killed wait only by an id the agent no
  longer had. An abandoned review owes no verdict, since a wait on it returns
  abandoned and receives none, and `0004` marks verdicts already in a database
  as received. `show` also returns the reviewer's note on each round.
- Outside git, a review path that runs through a symlinked directory is refused.
  `os.Lstat` follows every component but the last, so `link/s.txt` with `link`
  pointing outside the project read an outside file into the blob store, which
  the walk's own comment says it prevents.
- The project's name is read once, when the project is first seen, as the spec
  said. A version ran `git rev-parse` on every request. No test can tell the
  difference, since the name is kept from the first registration either way.
- `--base` is refused outside git, where it was silently ignored.
- `decided_at` and `stop`'s `daemon` are always present, as null when empty, so
  a `--format` template naming one does not fail on an answer without it. The
  `still_waiting` body the client writes fills the review's fields from `show`.
- Not changed: a comment written in the same millisecond as its round's verdict
  counts as on time, not late. Counting it late was tried, and it labeled a
  comment written before the verdict as written after it. On time costs only
  that it arrives with the next verdict rather than at once.
- Not changed: `wait`'s reads are separate queries, so a write landing among
  them can mix two states into one answer. The write publishes, so a wait reads
  again at once, and an answer already sent is corrected by the next wait. A
  retry loop was tried and dropped, because no test could show it changing an
  outcome.

**Departures, iteration 6.**

- `eyeball stop` waits until the lock is free as well as until the socket stops
  answering, through a new `daemon.Held`. The daemon closes its socket before it
  releases its lock, so a stop that returned on the socket alone let the next
  command's daemon lose the lock to one still exiting. The end-to-end test found
  it.

### The store

Each addition is written by something this spec builds.

- **`comments.reply`, `comments.replied` and `comments.delivered`**, in
  migration `0003`, as text that is empty when absent, like every optional text
  column in 0001. 003 deferred the reply column and delivery tracking to this
  spec, so the command that fills each arrives with it.
- **`store.Comment` gains `Created`, `Reply` and `Delivered`.**
- **New reads and writes:** `Decision` for a round, `Released` for a review's
  released comments with whether each is delivered, `MarkDelivered`, `Reply`,
  and `ReviewsIn` for every review in a project. `ReviewRow` gains the latest
  round's `Verdict` and a count of undelivered released comments, which is what
  `status` needs per row and what the queue will show in the surface (006).
- **Departure, iteration 1:** `ErrNotReleased` was added for a reply to a
  comment the agent cannot have seen. The spec named the refusal and no sentinel
  for it, and the handler needs one to map it to a code.
- **Departure, iteration 3:** `Project` and `CommentReview` were added as reads.
  The handlers need a project's root to name it in a `wrong_project` refusal and
  in every response, and a comment's review to check ownership before a reply.
  Neither was listed.
- **New refusals:** `ErrMissing` for an empty required field, `ClaimedError` for
  an overlap, `ErrNothingInScope` for a first round with nothing in scope, and
  the exported `NormalizePath` and `Covers`.

### Tests

**The server is tested in process, over a real unix socket**, under a short
temporary directory, because `t.TempDir` nests the test's name in the path and a
long name passes the 107-byte limit. A CLI test calls the CLI's run function
with arguments, a working directory, an environment and two writers, against
that server, with autostart turned off by the environment the test passes.

**A verdict in a test goes through the server's wrapped store.** Nothing on the
socket can give a verdict, by design. In-package tests decide through the
wrapper, which publishes, exactly as the surface's handler will.

**A stale socket is made deliberately.** A Go listener closed normally removes
its socket file, so a test that starts and stops a daemon leaves nothing stale
to test against. The test binds a listener with `SetUnlinkOnClose(false)`,
closes it, and starts a daemon on the same state directory.

**The shutdown test asserts what a waiter receives**: a `503` carrying
`daemon_stopping`. Asserting only that the request ended within a second would
pass for a server that cut the connection after a short deadline.

**One end-to-end test builds the binary** and runs it as an agent would, with
`XDG_STATE_HOME` in a short temporary directory: `request` in a fresh repository
starts a daemon, `status` sees the review, `abandon` wakes a `wait` running in
the background, and `stop` releases the lock. Piping each command through a
reader that waits for end-of-file is what proves the started daemon is detached.

## Out of scope

Named because each is close enough to this work to be assumed part of it.

- **The reviewer's surface and its listener.** tsnet, the HTTPS listener,
  `/healthz`, the templates and every reading route are the surface (006). The
  daemon here serves the socket and nothing else.
- **Giving a verdict, writing a comment, the live stream.** The surface's write
  path (007). Nothing reachable from the socket decides a round.
- **Push.** Notification (008) hangs a push on a request being opened. The
  wrapper that publishes change signals is where it attaches, and nothing here
  sends anything.
- **The per-file view override.** product.md lets an agent say a file should
  open as its page or its source. That is a flag on `request`, and it arrives
  with rendered files (009), the first spec with a view to override.
- **Exiting when idle.** In [specs/TODO.md](TODO.md) already.
- **Log rotation.** `eyeball.log` grows without bound. At a few lines per
  request that is assumed harmless for a long time, and it is added to
  [specs/TODO.md](TODO.md).
- **What a marker file contains.** An open question in product.md. `.eyeball` is
  read for its existence only.
- **Pruning.** Refused reviews leave blobs behind, like everything else
  captured. Retention is an open question in product.md.
- **macOS.** `flock`, `Setsid` and unix sockets exist there and nothing here is
  Linux-only by intent, but nothing is tested on it. It is added to
  [specs/TODO.md](TODO.md).

## Not converged

Three review passes ran, under the stop rule: the whole draft at high effort,
then the fixes, then the fixes to those, because the second pass found seven
defects that would have failed at runtime. The third found four more, all in
text the second round's fixes had written, and their fixes above were not
reviewed again. The risk that leaves is of the same shape: a paragraph and a
todo, or two paragraphs, that disagree about a detail of `wait`, the exit
statuses, or symlinked paths. Mutation testing in each iteration and the
stand-in's ack are where that is caught.

Decided here rather than by product.md, and reported to the owner at the end of
the run because each is a behavior a person could reasonably want otherwise: a
git worktree is its own project, the marker file is `.eyeball`, an agent's
default name is its root's path, `status` shows only the agent's own reviews
while `show` reads any, and `wait --format` never acknowledges a comment.

## Iteration 1: Replies, receipts and the store's refusals

- [x] Migration `0003` adding `reply`, `replied` and `delivered` to `comments`,
      each `TEXT NOT NULL DEFAULT ''`, with a comment saying why empty means
      absent
- [x] `store.Comment` carrying `Created`, `Reply` and `Delivered`, read by
      `Comments` and every new read
- [x] `Decision(ctx, roundID)` returning the verdict, the note and when it was
      given, and `ErrNotFound` for an undecided round
- [x] `Released(ctx, reviewID)` returning comments on decided rounds and on any
      round of an ended review, each marked late or not, in round order
- [x] `MarkDelivered(ctx, reviewID, ids)` stamping only released comments of
      that review, ignoring an id that is not, and leaving a stamped one alone
- [x] `Reply(ctx, commentID, body)` refused on an unreleased comment and on an
      empty body, replacing an earlier reply and stamping `replied`
- [x] `ReviewsIn(ctx, projectID)` listing every review in a project, newest
      request first, through the shared row query
- [x] `ReviewRow` carrying the latest round's `Verdict`, empty when undecided,
      and `Undelivered`, the count of released comments not yet stamped
- [x] `ErrMissing` wrapped with the field's name for an empty note, title, agent
      or path, replacing the plain errors `OpenReview` returns today
- [x] `ClaimedError` carrying the path and the holding review's id and agent,
      matching `ErrPathClaimed` through `errors.Is`
- [x] `OpenReview` refusing a first round with no captured file in scope with
      `ErrNothingInScope`, and `NormalizePath` and `Covers` exported
- [x] The store's test helper `newReview` and its callers given an in-scope
      file, so `ErrNothingInScope` does not fail tests written before it
- [x] Tests: an open round's comment absent from `Released` and refused a reply,
      a comment after the verdict and one on an abandoned review's undecided
      round both released as late, another review's id ignored by
      `MarkDelivered`

> **Completed** 2026-10-05 05:30 UTC
>
> - acks: reviewed
> - `make check` — 1.6s
> - `go test -race ./...` — 1.8s

## Iteration 2: Finding the project and naming the agent

- [x] `capture.FindRoot(dir)` walking up to the nearest directory holding `.git`
      or `.eyeball`, returning the logical root and the resolved one, and
      refusing at `/` with the fix named
- [x] `capture.ProjectName(ctx, root)` naming a linked worktree from its common
      directory, stripping `.git` and `.bare`, and using the root's base name
      everywhere else
- [x] capture's git runner removing the variables
      `git rev-parse --local-env-vars` lists, asked once and cached, with a test
      that sets `GIT_DIR` to another repository and captures correctly
- [x] An agent name resolved from `--agent`, then `EYEBALL_AGENT`, then the
      root's path with the resolved home directory written as `~`
- [x] Review paths, relative ones joined to the logical working directory, made
      relative to the logical root and refused when they leave it
- [x] Tests against real repositories: a subdirectory, a relative path from a
      symlinked directory, an absolute path through one, a linked worktree, a
      worktree of a bare repository, a submodule, a marker inside a repository,
      and no project at all

> **Completed** 2026-10-05 05:39 UTC
>
> - acks: reviewed
> - `make check` — 1.6s
> - `go test -race ./...` — 7.0s

## Iteration 3: The agent API

- [x] `internal/api` holding every request and response type, the error shape
      and the client's `still_waiting` body, listed in architecture.md's layout
- [x] A coalescing signal: subscribers with a channel of capacity one, a
      non-blocking publish, and an unsubscribe that a test proves releases the
      subscriber
- [x] The server holding the store only through a wrapper whose mutating methods
      publish after success
- [x] One route table naming each route's listener, one mux per listener built
      from it, and a test that `mux.Handler` matches each route on its own side
      only
- [x] `POST /agent/requests` opening a review or its next round, refusing paths
      with `--review`, `--base` with `--review`, and a round asked for twice
      before capture
- [x] The project and agent checks on `--review`, `wait`, `receipt`, `abandon`
      and `reply`, run before any capture or write, refusing with
      `wrong_project` or `wrong_agent` and the fix
- [x] The overlap refusal's fix chosen by the holder's state and agent: `wait`,
      `request --review`, or `--agent <name>`, and an unmatched first-round path
      refused by name
- [x] `wait` answering `abandoned`, `verdict` or `comments` in that precedence,
      from a subscription taken before the read, with only late comments
      triggering `comments`
- [x] A canceled `wait` answering `503` with `daemon_stopping`, and a hook
      immediately after the read proving a verdict given there is seen
- [x] `/receipt` stamping the comments it names through `MarkDelivered`
- [x] `/abandon` withdrawing the review and waking its waiters, and `/reply`
      storing a reply through `Reply`
- [x] `/status` reading `ReviewsFor` and counting other agents' reviews, or
      `ReviewsIn` with the agent column under `--all`
- [x] `GET /agent/reviews/{review}` answering any review with its rounds,
      verdicts, decision times and latest captured files, with no checks, no
      comments and no receipt
- [x] A test parsing `internal/store` and `internal/capture` with `go/parser`
      that fails on an exported `Err` variable with no mapping, and one that a
      500 carries no raw error text

> **Completed** 2026-10-05 06:05 UTC
>
> - acks: reviewed
> - `make check` — 1.6s
> - `go test -race ./...` — 3.9s

## Iteration 4: The daemon

- [x] The state directory from an absolute `XDG_STATE_HOME` or `~/.local/state`,
      created `0700`, with an existing one carrying group or other bits refused
      by mode
- [x] One check function for the directory mode and the 107-byte socket path,
      called by the daemon before binding and exported for the client
- [x] The lock file opened without truncation, locked, then truncated and
      written with the pid and start time, before the socket is unlinked and
      bound
- [x] A losing daemon exiting zero and naming the holder on stderr
- [x] Shutdown in the spec's order, the stop handler answering before it starts,
      the lock released last, and SIGINT and SIGTERM taking the same path
- [x] The socket server configured with no `WriteTimeout` and with a
      `ReadHeaderTimeout`
- [x] A build identifier computed once at startup from the VCS fields and the
      executable's size and mtime
- [x] Starting a daemon detached: `Setsid`, stdin and stdout on `/dev/null`,
      stderr appended to `eyeball.log`, working directory the state directory
- [x] A test that starts a child that way through a pipe and sees the pipe reach
      end-of-file while the child is still running
- [x] Tests: two daemons on one directory with one winning, a stale socket from
      `SetUnlinkOnClose(false)`, a waiter receiving `503 daemon_stopping`, and
      the mode refusal
- [x] architecture.md's What runs section corrected to put the socket in the
      state directory

> **Completed** 2026-10-05 06:22 UTC
>
> - acks: reviewed
> - `make check` — 1.6s
> - `go test -race ./...` — 5.3s

## Iteration 5: The command line

- [x] `urfave/cli/v3` added to `go.mod` and imported only by `internal/cli`
- [x] A no-op `ExitErrHandler`, the writers from the run function, no root
      `Action`, and `OnUsageError` and `DisableSliceFlagSeparator` on the root
      and every subcommand
- [x] Exits 2 and 3 carried in the CLI's own status type, `main` exiting 1 on
      every other error, and a test that `eyeball wiat 5` exits 1
- [x] A dialer that runs the daemon's checks, starts a daemon on `ENOENT` or
      `ECONNREFUSED`, restarts once per early child exit, and prints the log's
      tail on giving up
- [x] `request` and `wait`, wired to their routes
- [x] `show`, `status`, `reply` and `abandon`, wired to their routes
- [x] `start`, `stop` and `serve`, with `stop` never starting a daemon
- [x] `-m` repeatable, joined with blank lines, a comma kept inside a value, and
      a note blank throughout refused before any request is sent
- [x] `--json` printing the body as received, a refusal's body on stdout under
      `--json`, and `--format` over the body with numbers converted to `int64`
      or `float64`
- [x] Human output for each command, with a test per command against a golden
      file, and every mutation ending with the same `next:` text the JSON
      carries
- [x] `wait` posting a receipt for exactly the comments it printed, after
      printing, and none under `--format`
- [x] Exit status 1 on a refusal, 2 on `wait --timeout` with the client's
      `still_waiting` body, 3 on `comments`, and 1 naming the command on a
      dropped connection
- [x] A differing build identifier writing one line to stderr naming
      `eyeball stop`, tested with an injected identifier
- [x] Tests against an in-process server: flags after paths, a root flag after
      the subcommand, `--` before a dashed path, a bad subcommand flag under
      `--json`, and `--format` comparing a number

> **Completed** 2026-10-05 06:43 UTC
>
> - acks: reviewed
> - `make check` — 1.8s
> - `go test -race ./...` — 8.2s

## Iteration 6: The manual and the whole loop

- [x] `eyeball doc` printing an embedded manual, without dialing the daemon,
      covering every command, flag, exit status and refusal code, with a
      reference section of every response field by dotted path
- [x] The manual listing every environment variable the CLI reads:
      `EYEBALL_AGENT`, `EYEBALL_NO_START` and `XDG_STATE_HOME`
- [x] A test failing when a command, a flag or a dotted path is absent from the
      manual, proved by deleting one of each
- [x] An end-to-end test building the binary with a short `XDG_STATE_HOME`:
      `request` starting a detached daemon, `status`, `abandon` waking a
      background `wait`, and `stop`
- [x] The comment in `cmd/eyeball` corrected to say bare `eyeball` prints help
- [x] `docs/architecture.md` naming the marker file `.eyeball`, the worktree
      rule, the agent's default name and the git environment scrub, in the
      sections those belong to
- [x] `internal/daemon/doc.go`, `internal/cli/doc.go` and `internal/api/doc.go`
      stating their packages' invariants: detachment and shutdown order, receipt
      after printing, and the schema being the manual's

> **Completed** 2026-10-05 06:55 UTC
>
> - acks: reviewed
> - `make check` — 1.9s
> - `go test -race ./...` — 1.9s
