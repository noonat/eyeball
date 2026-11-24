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
over the network can open a review or read one as an agent. The tailnet carries
the reviewer's surface and nothing else, and since the verdict is the surface's,
any device on the tailnet that reaches it can release a batch of comments by
deciding. The owner decided that any device reaching the tailnet listener may
write, with no check of device or login. An agent on this machine with tailnet
access through the machine's own Tailscale could therefore approve with a plain
request, as it could already write `eyeball.db` directly.

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
the app's TypeScript sources and the packages they import, the stylesheets and
their fonts, the icon masks, the SQL migrations.

```
cmd/eyeball/          the entry point: one function, one error
assets.go             the files the binary embeds: the app's sources, web/deps, the stylesheets and fonts
internal/api/         the agent API's wire types, shared by the server and the CLI
internal/appbuild/    esbuild's options for the app's bundle, shared by the vendor step and the binary
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
internal/view/        the surface API's JSON views, which web/src/api/types.ts is generated from
internal/viewdoc/     draws a view as the template pages drew it, for tests that read a screen's content. They prove what the views carry, and the stories compared with the frozen .html files prove what the app draws
tool/apitypes/        writes web/src/api/types.ts from internal/view, and fails when the committed file differs
tool/appicon/         renders the installed page's icons, committed under web/static/icons
tool/fixtures/        writes the views of seeded states, and the files they point at, into web/src/fixtures, and fails when they differ
tool/icons/           regenerates the icon mask table in app.css
tool/seed/            writes a backdated, populated state directory to look at
tool/tearout/         serves the built Storybook over the tailnet
tool/vendor/          copies the files the app's bundle reads from web/node_modules into web/deps
web/src/              the app: routes, components and their stylesheets, the API client, the stream
web/deps/            node_modules/: the files of each pinned package the app's bundle reads
web/ts/               framework-free behaviors the app and the service worker share
web/                  the manifest, the icons, the service worker
```

**A new package is listed here in the change that creates it**, with one line
saying what it is. A list missing entries stops being read as a list of what
exists, and `internal/convention` fails when the two disagree.

`go install` has to produce a working binary. Nothing in the build may require
Node, a bundler on the path, or a committed build artifact. The packages under
`web/deps/node_modules/` are committed sources the build reads, not its output.
That constraint decides more of this document than any other.

**Nothing the binary embeds may sit below a directory named `vendor`.** Go's
module zip, which is what `go install pkg@version` downloads, drops every file
that is more than one level below a `vendor/` directory, since it takes them for
vendored Go packages. A path such as `web/vendor/node_modules/react/index.js` is
missing from that zip, and the `//go:embed` that names it then matches nothing
and fails the build. A test builds the module zip the way the Go command does
and fails when an embedded file is not in it.

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

**A working copy is the nearest directory above the caller's working directory
that holds a `.git` entry or a `.eyeball` marker file.** The nearest wins,
whichever it is, so a marker inside a repository names a smaller project that is
walked rather than diffed. The marker is an empty file and nothing is read from
it.

**A project is the repository a working copy belongs to**, named by
`git rev-parse --path-format=absolute --git-common-dir` with symlinks resolved.
A main checkout and its linked worktrees give the same answer, so they are one
project. The common directory is the only answer git gives correctly in every
layout. Measured on 2026-10-18 with git 2.55: `git worktree list` names the git
directory, not the working copy, as the main entry of a repository made with
`--separate-git-dir` and inside a submodule, and it keeps listing a deleted
worktree until `git worktree prune`. A submodule has a common directory of its
own under the superproject's `.git/modules`, so it stays its own project. A
working copy found by its marker file is a project of its own, keyed by its
root, as before.

This corrects an earlier rule, that a git worktree is its own project. Capture
runs git in the working copy, and a worktree's changes are visible only from the
worktree, but neither needs the worktree to be a project. Treating it as one
left a row on the queue's empty state for every worktree ever removed, and made
`eyeball check` from the main checkout refuse an approval asked for from a
worktree (see Checking a merge).

**A round records the working copy it was captured from**, and everything a
round reads live, the base commit's files and the working copy as it is now,
comes from there. Overlapping scopes are refused within a working copy, so two
worktrees can still claim the same path, which is right: sharing a repository is
no more a reason to share a verdict than sharing a working copy is. A review's
working copy is its latest round's. A round asked for from another working copy
is refused when an open review there already claims one of its paths, the same
refusal a new review gets.

**A project's root is its main checkout** once one has been seen, and until then
the first working copy seen. It is what the project view shows. A main checkout
is the working copy whose git directory and common directory are the same. A
repository with no main checkout, a bare clone with worktrees, keeps the first
working copy seen.

**The root is resolved through symlinks for its identity**, so two shells
reaching one repository by two spellings are one working copy. The common
directory is the same through a symlink, measured on 2026-10-18. Review paths
are computed against the root as the shell spelled it, because an agent builds
an absolute path from the directory it sees.

**An agent is named by `--agent`, then `EYEBALL_AGENT`, then its working copy's
path with the home directory written as `~`.** The working copy is the identity
product.md asks for. An agent asking for a round from another working copy of
the project passes the name it opened the review under. The current branch reads
better in a queue and was rejected because it moves: an agent that switched
branches would stop seeing its own reviews.

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

**Writing through the loopback listener takes the run's token.** `serve` draws
one per run and prints the address with it in the fragment, which a browser
sends to no server and puts in no `Referer`. The page's script stores it in
`localStorage` and sends it as `X-Eyeball-Token` on every write, and the
listener refuses a write without it. `localStorage` is isolated per port and a
cookie is not, so another account's loopback page opened in the same browser
never sees the token. A page on another origin cannot set the header without a
CORS preflight the surface never grants. A process running as this user can
still write `eyeball.db` directly; the token stops other accounts and a casual
`curl`, not that.

**Every surface request passes Go's cross-origin check**, on both listeners.
`http.CrossOriginProtection` refuses a write whose `Sec-Fetch-Site` is neither
`same-origin` nor `none`, or, without that header, whose `Origin` differs from
its `Host`. The tailnet listener has no token, so without the check any page the
phone opened could post a verdict there as a plain form.

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

There is a stored credential, though: the loopback listener's token, in that
origin's `localStorage`. A frame sandboxed with `allow-scripts` alone cannot
read it, and its writes carry `Origin: null`, which the cross-origin check
refuses. `allow-same-origin` would hand the frame the token, which is one more
reason that flag needs a separate origin. The same holds for a preview opened as
a page rather than in a frame: agent HTML or SVG served at the surface's origin
would run with its storage. So a preview served as a page carries
`Content-Security-Policy: sandbox allow-scripts`, or comes from another origin.

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

**A file's bytes come from `GET /reviews/{review}/raw/{round}/{path...}`**,
where round 0 is the base commit, so every side a comparison shows has an
address. The path mirrors the project's, so a page's relative links resolve
through the same route: a mockup's stylesheet beside it loads from the same
round. A round reads its capture and then the base, never the working copy,
because a preview that mixed live bytes into a frozen round would carry no
banner saying so. Every response carries
`Content-Security-Policy: sandbox allow-scripts; frame-ancestors 'self'`, except
a PDF's, since Chrome runs no PDF viewer under the sandbox. That rests on a
browser's PDF viewer keeping a document's script away from the page around it,
which is assumed and not measured. A file kind says is other is never sent.

**The route sends no CORS headers.** The frame's origin is `null`, so a load in
CORS mode is cross-origin and fails: measured in Chromium 152, a module script,
a web font and a `mask-image` from a file do not load, while a stylesheet and a
classic script do. Answering with `Access-Control-Allow-Origin: null` would let
any page the phone opens read a project's files, since any site can frame itself
sandboxed and review ids count up from 1. If a preview ever needs CORS, the
answer is an unguessable path segment per round, answered with CORS only under
it.

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

**A working copy is a row of its own, under its project.** Each project row
written before working copies existed becomes one working copy at the same root,
and every round of its reviews points at it. Which repository such a project
belongs to needs git, which SQL cannot run, so the daemon fills it in at start
for every project whose root is still on disk. Two old projects that turn out to
share a repository are merged into one: their working copies and reviews move to
the project whose root is the main checkout. On 2026-10-18 no two live roots
shared one, so the merge runs only in a test. A project whose root is gone keeps
no repository, stays a project of its own, and is left off the queue's empty
state. Filling it in when a request first arrives was rejected, since a request
from a new worktree cannot find its main checkout's old row: that needs the
repository the row does not have yet. The fill finishes before the agent socket
accepts a request, so no request sees a half-filled store.

**A root made again as something else retires what was there.** A directory
removed and made again at a root eyeball knows, as another repository's working
copy, as a marker directory where git was, or as git where a marker directory
was, finds an old working copy or project at that root. The old working copy and
any project rooted there are renamed to a root that is not on disk,
`<root> (retired <id>)`, so the old project keeps its reviews, which read their
captures, and the queue leaves it out. The directory then registers as what git
says it is.

This corrects an earlier rule, that such a directory merges the old project into
its repository's. A project git was never asked about has no repository, and so
does a marker project, so the store cannot tell them apart, and a merge filed a
marker project's reviews under a repository it never belonged to. A root reused
by another repository was refused with an error, which reached the agent as a
failure inside the daemon on every request. Merging stays where the repository
is known to be the same: the startup fill, and two projects registered for one
repository at the same moment.

**SQLite cannot add the columns as constraints.** Measured on 2026-10-18 with
foreign keys on: `ADD COLUMN` refuses a `REFERENCES` column with a non-null
default, and refuses `UNIQUE`. So `rounds.working_copy_id` is added nullable,
filled by the migration, and a trigger refuses a round written without one.
`projects.repository` is unique through a partial index over the rows that have
one. Rebuilding `rounds` to declare `NOT NULL` was rejected, since every table
holding a round's files, comments and decisions references it.

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

### Marking a rendered document

**A document is compared passage by passage.** Each passage is keyed by a hash
of its rendering, and the two lists of keys are compared as lines, so a passage
rewrapped without a word changed is not a change. A passage is a top-level
block, or one item of a top-level list. goldmark renders a list item on its own
as the same `<li>` it writes inside its list, and keeps an ordered list's first
number. Measured with goldmark 1.8.6 on 2026-10-23. An ordered item's key leaves
out its number, so an item added near the top of a list marks only itself. The
list element stays in the key, so bullets turned to numbers still mark every
item.

**Each list item is written as a list of its own.** A passage is a `.blk`
element carrying its source lines, which the composer finds a tapped passage by,
and its comments are written after it. Neither is valid as a child of a `<ul>`,
so an item is written as a one-item `<ul>`, or `<ol start>` with its own number,
inside its own `.blk`, and the stylesheet spaces consecutive one-item lists as
one list. The alternative, the `.blk` and its source lines on the `<li>` itself,
leaves comments with nowhere valid to go between two items.

**Changed words are found in the rendered HTML, not in the markdown.** Both
passages are rendered, cut into tags and text, and only the text is compared,
with the same word and separator tokens a changed code line uses. The new
passage's tags are copied through where they stand, added text is wrapped in
`<ins>`, and the removed text is written in a `<del>` at the point it was
removed. The markdown is the wrong place to compare: wrapping a word inside a
link's or a code span's syntax in a marker breaks the syntax. The cost is that
removed text keeps none of its formatting, since the old passage contributes its
text and none of its tags.

Four rules hold the marking together. The second and fourth were measured on
2026-10-23, through eyeball's own renderer. The other two are reasoned, not
measured:

- An entity such as `&lt;` is one token. Cut on word characters it would be
  three, and `&lt;` against `&gt;` would mark only the letters, leaving half an
  entity on each side of a marking element.
- A change separated from the next only by whitespace joins it. Kept as a match,
  a lone space cut "gives up" against "records the failure on the job" into two
  pairs of marks.
- Words are marked only where the two passages have the same block tags in the
  same order, such as the same rows and cells of a table. Removed text is
  written where it was removed, and in a table that lost a cell that point is
  between two cells, where text is not valid.
- A task item's box is a token, `[ ]` or `[x]`. It renders as an `<input>` tag
  and no text, so ticking a box changed no word and marked nothing. The new box
  is wrapped in `<ins>`, and the box it replaced is not written, since struck
  through beside the new one it would read as a second box.

**A changed HTML block is shown as a rewrite.** Both sides render the same
placeholder sentence, so comparing their renderings finds nothing, and the
block's old source, which the key compared, is what changed. The old source is
shown struck through above the placeholder.

Everything written is either goldmark's rendering, which has already escaped
text and dropped raw HTML, or old source passed through goldmark's escaping
writer: a struck passage's text, and a changed HTML block's source. Assembling
it adds only fixed elements, and no text but a fold's "N unchanged passages":
`ins` and `del` around changed words, `del class="old"` around struck text and
`ins class="whole"` around a passage tinted whole, a `code` element around
struck HTML source, the list element around a list item, the `.blk` around each
passage with its line range, and a fold's `details` and `summary`.

**A document's own strikethrough is written as `<s>`.** goldmark writes
`~~text~~` as `<del>`, the element a removed word is marked with, so struck text
the author wrote would read as text the round removed.

## Checking a merge

**`eyeball check --base <old> <new>` is `GET /agent/check` on the agent
socket**, like every command, so the daemon that owns the store answers it. A
review gate runs it before a default branch moves from `<old>` to `<new>`, so
the reviewer's eyeball approval is the only one. It passes when an approved
review in the caller's project, opened by any agent from any of its working
copies, has a latest round measured from `<old>` whose capture matches `<new>`'s
tree. The commits are read in the caller's working copy, which shares its
objects with every other working copy of the repository. Reading `eyeball.db`
from the CLI would work with the daemon stopped, and was rejected for adding a
second reader of the schema. A gate that sets `EYEBALL_NO_START` gets a refusal
from a stopped daemon instead of starting one.

**They match when the two disagree on no path that matters.** Every path `<new>`
changes from `<old>` must be captured inside the review's paths with `<new>`'s
bytes, by the SHA-256 the blob store names content by, and its mode: 100644,
100755 or 120000, recorded per round file since 012. A file approved as text and
merged as executable, or as a symlink with the same bytes, is not what was
approved. A round taken before modes were kept is read as regular files, so an
executable or a symlink there fails closed. Or a path is captured as a deletion
where `<new>` lacks it. Every path captured inside the review's paths must be
one `<new>` changes, since a captured file differs from the base by definition.
A file captured outside the review's paths that `<new>` leaves alone is ignored,
because nobody approved it and none of it merges. Comparing trees rather than
history makes a fast-forward and a merge commit the same question.

**Only commits are read**: `rev-parse`, `diff-tree`, `ls-tree` and `cat-file`. A
merge holds `index.lock` while the gate's hook runs, so the index and the
working tree are not the check's to read. Every git command eyeball runs passes
`--no-replace-objects`: with `refs/replace` honored,
`git replace <commit> <approved>` made the check compare an approved tree while
the branch moved to another. A round captures working-tree bytes and the check
reads committed ones, so a repository with a clean or smudge filter, line-ending
conversion or `core.fileMode` off is refused even where nothing changed. That
fails closed, and is left until such a repository needs the gate. A daemon
started from a hook inherits `GIT_DIR`, which git sets to a worktree's git
directory there, and every git command the daemon runs already drops git's
repository-local variables.

**It records nothing.** The project is looked up and never registered, so a
repository eyeball has not seen is refused. A review asked for from a worktree
matches a merge made in the main checkout, since both are one project. Before
working copies, a worktree was a project of its own, and on 2026-10-18 an
approval of review 58, asked for from a worktree, did not match the merge, so
the same tree needed review 59.

## Reading the approved bytes

**`eyeball cat --base <old> <path>` is `GET /agent/cat` on the agent socket.** A
gate that lets a merge differ from the approved tree by a change it can verify
needs the approved bytes to verify against. In the first such gate, a spec file
changes state after its approval, and the gate replays that change on the
approved text and compares. Without a command, the gate would open the blob
store itself, and the gate's trust would rest on a file on disk and on the
store's layout instead of on eyeball's answer. That was rejected. Extending
`show` was rejected too: it reads any review in any project with no project
check, which is right for printing digests and wrong for printing content.

**The answer is the file from the one approved round that holds it.** The
candidates are the approved reviews in the caller's project, opened by any
agent, whose latest round is measured from `<old>` and captured the path inside
the review's paths and with content. A deletion does not answer. Reviews that
hold the same bytes are one answer. Reviews that hold different bytes are
refused as `ambiguous`, because picking one would let a gate approve or refuse
by whichever review the query listed first. None is refused as `not_captured`.
An approval asked for from a worktree is a candidate too, so a spec approved
from a worktree and a branch approved from the main checkout, on one base and
holding different bytes of the spec, are refused as `ambiguous` where before
only the main checkout's counted. That is the same answer two such approvals in
one checkout get today. Like `check`, it reads and records nothing, and a
repository eyeball has not seen is refused.

**The body is JSON, with the content as base64 and its SHA-256, and the command
recomputes the digest.** The client already treats a 200 whose body is not JSON
as a dropped connection, so that a wait cut short never reads as a verdict. A
raw file would lose that protection. It would also let a truncated or damaged
file reach a gate as the approved content. Base64 costs a third more bytes and
the daemon holds the whole file in memory, so the command suits a document and
not a build artifact. Nothing caps the size, because the one caller reads a
spec.

**It rests on one property of `check`**, measured in
`TestServer_checkAcceptsAnUnreferencedCommit`: `check` reads commits and not
refs, so a commit no ref names, whose tree is the merged tree with one blob put
back to the approved bytes, matches the approved review where the merged commit
does not.

## What a file is

`internal/kind` maps a path and its first bytes to one of: text, markdown, page,
image, audio, video, PDF, or other. That decides which view opens first, per the
table in [product.md](product.md). The extension decides first, from a fixed
table, because an extension is what the agent named it and is right nearly
always. A file whose extension is not in the table is text if diff's binary test
passes it and other if not, so the two packages agree on what is text. The same
table gives the content type a file's bytes are served with.
`mime.TypeByExtension` reads the host's `/etc/mime.types`, so the same binary
would answer differently on two machines.

**Page is HTML and SVG**, the two formats with a rendering that can carry
script. HTML renders in the sandboxed frame. SVG renders through an `<img>`
element, where it cannot execute, so it shows before and after like any other
image. An SVG that animates by script stands still there, and one opened
directly is served under the same sandbox as a page.

**A file past 20 MB is not sent to a phone until asked.** At ten megabits a
second it takes sixteen seconds. The number is a guess written down so it can be
corrected. Load anyway opens the file's page with `?load=1`, and the raw route
answers past the limit only with that query. A blob seeks on disk, and a base
file past the limit is copied from `git cat-file` to a temporary file first, so
a range is answered either way and nothing that size is held in memory.

Syntax highlighting is `alecthomas/chroma/v2`, server side, into the five roles
the stylesheet defines. Client-side highlighting would ship a second parser to a
phone to re-derive what the server already knows.

## The surface

**A React app in the browser, over data the Go server builds.** The server
compares, highlights, renders markdown and marks changed words, exactly as it
did for the template pages it replaced. What changes is what it sends: JSON
views of the same models, which the app draws and keeps in a cache. Moving
between screens then loads no page, a change reported by the live stream updates
the screen in place, and the installed app opens from a cached shell when the
daemon cannot answer.

The alternative, server-rendered `html/template` with htmx, is what the surface
was built on first. Every review, round and file was a page load, every change
the stream reported reloaded the page, and every launch fetched HTML from the
daemon. Each of those waits on the daemon before anything on the screen changes.
htmx can swap a page's body without a full load, but the markup still comes from
the daemon on every move, and nothing is drawn until it arrives. What the switch
costs is client state, which the htmx design avoided on purpose. How the two
copies are kept from disagreeing is under "Client state" below.

**Go keeps every rendering decision.** A code row arrives as a list of tokens,
each with its syntax role, so the app maps roles to classes and does no
highlighting of its own. A document passage arrives as HTML the server made,
which the app inserts as it is. That HTML comes only from goldmark with raw HTML
off and from the word marking, the same places `template.HTML` is used today, so
the set of places that produce trusted markup does not grow. Rendering markdown
in the browser was rejected: the word marking in "Marking a rendered document"
works on goldmark's output, and a second renderer would mark a different
document.

**One field type carries that HTML, and one component inserts it.** The view
sends a document passage in a named type that only `render.Document`'s result is
converted to, and a Go test that type-checks the package's source fails on a
conversion anywhere else, and on a constant of the type, which Go converts with
no call. The app inserts it with `dangerouslySetInnerHTML` in one component, and
oxlint's `react/no-danger` fails the prop anywhere else. Measured 2026-10-31:
oxlint 1.87.0 failed a planted use with the rule on, and refused a misspelled
rule name, so a typo cannot turn the check off. Each check covers one side, so
trusted markup reaches the page only along the path that made it.

**Every page address answers the same shell**, and the app's router reads the
address. Links, bookmarks and a notification's deep link keep the addresses they
have now, and a page loaded cold lands on the right screen.

**The shell holds nothing the store changes.** It is the head every page writes,
the bundle and a root element. It prints no stamp: a stamp printed with the page
would be a second read of the store beside the view's, so a change between the
two would be missed. The app fetches the view, takes the stamp the view carries,
opens the live stream itself, and refetches when the stream's stamp differs from
the view's.

**The API path is the page path under `/api`.** `GET /api/reviews/42` is the
view the page at `/reviews/42` draws. One rule instead of a second address
scheme means a page and its data cannot drift apart in name, and a person
debugging can fetch the view of the page in front of them by adding four
characters.

**A missing address answers the shell, and the view answers 404 with a body.**
The shell cannot tell whether a review or a path exists without reading the
store, and reading the store is the view's job, so every address in the app's
list gets the shell. The app fetches the view, and a view for an address with
nothing at it answers 404 with `NotFound`: the waiting count and, where a live
read was refused, the banner saying why. The app draws the 404 screen from that
body. The cost is that a bookmark to a missing address gets a 200, and the 404
shows only once the view has loaded. Nothing relied on the page's 404: the CLI
reads a 404 only from API routes, as "the daemon predates the route", and the
service worker has no 404 branch. The waiting count is read for a view as `fail`
reads it for the eye, by one helper, `waitingCount`, with a bound and the error
logged. A count that cannot be read is `null` in the view, and the screen draws
no eye, since a zero would say nothing waits.

**A review's file address answers a tree or a file, and the view says which.**
The route draws a tree for a directory or an empty path. A view has a `kind`,
which the generator makes a union, and one part per kind, `null` for the other.
A file's `body` says which of its fields holds what the page shows, in the order
the template chose: a rendering, the deleted box, a document, the binary line,
the too-large line, then rows of code. A row of code is a list of tokens, each
with its role and whether its word changed, so the review page can draw its
hunks from the same type.

**A review's round is one view, and each card's body can be fetched on its
own.** `GET /api/reviews/42` and `GET /api/reviews/42/rounds/2` answer the
round: the brief, the verdict, the comparison control, the earlier rounds, the
comments that belong to no card, what the reader can do, and one card per file
with its counts and whether its body is held. A card's body is the rows of code
with the comment threads under their lines, a document's passages, a rendering,
or the line saying why there is none. Up to the sending thresholds under "A long
review is drawn in pieces" the round's view holds every body. Past them each
body is `null`, and the card asks `GET /api/reviews/42/rounds/2/cards/{path}`
for it. One function builds a body from one file and its comments, and both
routes call it, so a body fetched later is the body the round would have sent.
The cards route compares only the file it answers for, using
`capture.ComparePaths`. It still builds the card as the round does: the comments
on the file are placed as the round places them, and whether a document draws as
the page itself depends on how many files the round moved, which
`capture.MovedPaths` reads from the captures' digests with no diff. A path that
did not move, or is outside the review's paths, answers 404. The route takes
`?against=base`, as the page does.

**A document arrives as a list of passages.** In the template page a document
card was one piece of HTML, and the threads under its passages were written into
it through render's `After` hook. In the app a thread is a component, with a
composer, an edit form and a delete button, so it cannot live inside HTML that
is inserted as it is. render instead returns the document as a list. Each entry
is one passage, with its HTML, its line range and whether it changed, or a fold
holding a run of passages. The view sends each passage's HTML as `DocumentHTML`
with its comments beside it, and the app draws a thread as the passage's
sibling. The HTML is what is inside the passage's block element, and the app
draws the element from the range and the changed flag, so a thread sits between
two `.blk` elements as the stylesheet's sibling rules expect, with no wrapper
around either. Whether a run folds still depends on which passages have a
comment, so render is given the commented ranges in place of `After`. The file
page sends the same list with no comments, so one component draws a document
wherever it appears. Rejected: keeping threads in the HTML and mounting
components into it with portals. The components would sit inside markup React
does not own, and a refetch that replaced the HTML would remove them.

**The surface compresses what it sends.** A JSON view, the bundle and the
stylesheets are sent gzipped when the request accepts it, with
`Vary: Accept-Encoding`. A flag on the route table says which routes, so
`/events` and the raw route are never wrapped: gzip would hold the stream's
events in its buffer, and a range request names bytes of the stored file. A code
row's tokens are objects that repeat their key names, and a review's view is
mostly tokens. Measured 2026-11-01 on 11,846 rows of this repository's Go source
in 26 cards, in the code row's shape without its header and final-newline
fields: 4.8 MB as JSON, and 258 KB gzipped at level 6, the level the daemon
uses. The daemon compressed nothing before this, and sent the 383 KB bundle as
it is. A terser encoding, such as a token as an array, was rejected, since gzip
removes the same repetition and the generated types stay readable.

**A write sends JSON and is answered with what it changed.** The write routes
under `/api` take a JSON body whose type is in `internal/view`, so the generator
writes the request's type as it writes a view's, and a field renamed in Go fails
the app's build. A comment names its round, its level, its path, its lines or
the old line of a removed row, the comparison, its text and a submission key.
The server still reads the quote from the round's frozen content, never from the
request, and stores a comment on a round that is decided, withdrawn or followed
by another as late. An added or edited comment is answered with the stored
comment as the view sends it and the review's count of comments, and the app
puts the comment at its anchor in the cached round or card body. A comment on a
passage of a document, added or deleted, also refetches the round: whether a
passage sits in a fold depends on whether it has a comment, which is the
server's rule, and a copy of it in the app would have nothing checking that the
two agree. A delete has no body, since its address names the comment, and is
answered with the count. The count is the comments on the review's latest round,
the number the bar shows. A verdict names its round, the verdict and its note,
and is answered 204 with no body once it is stored. A request the server
refuses, such as an empty comment or a verdict on a round no longer open, is
answered 4xx with `Refusal`, one sentence, which the app shows beside the
control while the text stays where it was typed. A change request with no
comments and no note is answered 422 with the sentence, and the bar draws its
own refused state from the status. A body that is not declared
`application/json`, is larger than a megabyte, holds a field the type does not,
or is followed by anything else is refused. The type check answers 415 with a
`Refusal` instead of a decode failure. It is a second layer behind the
cross-origin check every surface request passes, and has no reason beyond being
explicit. A write that fails in the store is answered 500 with `Refusal`, since
a fetch is answered with plain text, which the app cannot parse. An edit of a
comment already gone answers 404 with `Refusal`, where it answered 500, and a
delete of one answers 200 with the count, so a delete sent twice reports no
failure. The view names the `/api` routes for a comment's edit and delete and
for the verdict in its `href` and `action`, and a write to the loopback listener
carries the run's token, whose refusal under `/api` is a `Refusal` and not the
page. The app builds a comment's request from its anchor in `composerAt.ts`, and
a Vitest test compares it with the hidden fields of each composer answer the
template routes gave, kept as they last came out, each anchor read from the
request that wrote the answer, which `composers.ts` keeps. A request sent as a
level the chips chose carries only the fields that level takes. No answer, a
timeout or a 5xx leaves a comment unsent with a retry that sends the same
submission key, so a first try that reached the store and lost its answer is
stored once. A 4xx leaves the text in its composer with the server's sentence,
since it would be refused again. Form bodies, which the template routes took,
were rejected: the app would build a form from typed data, and nothing generated
would check it.

**Types come from Go.** The API's response structs are the source, and a
generator writes the TypeScript types from them into a committed file.
`make check` regenerates it and fails on any difference, so a renamed field
breaks the build rather than a screen. Hand-written types with a contract test
were the alternative: they catch the same drift, later, and only for the fields
a test names.

**The generator is written here, with the standard library.** It reads
`internal/view`, the package of the API's views, with `go/types` and the
standard library's source importer. A reflection walk over the response structs
also worked, measured 2026-10-29: about 80 lines, and the types it wrote passed
every strict flag under "The client code". Reflection cannot see constants, so
it was passed over. `go/types` can, which lets a named string type with
constants become a union of its values, which a test shows by planting a
constant that changes the union. An external generator was not an option, since
`go run ...@latest` is refused in this environment and the project takes the
standard library first.

Go sends a nil slice or map as `null`, so the generator types each one `| null`,
and a use such as `q.waiting.length` then fails to compile with "possibly
'null'", measured 2026-10-29. A Go kind with no exact TypeScript form is an
error and never a guess: interfaces, integers that can pass 2^53, and maps
without string keys. The 2^53 limit is met two ways. A review's id is an int64,
so the view sends it as a string through json's `,string` option and the type is
`string`. A round number and a count are int32, which a number holds exactly.

Routes, with the spec that built each. "Planned" is a route this document
describes and no spec has built yet.

| Pattern                                                    | Answers                                             | Spec |
| ---------------------------------------------------------- | --------------------------------------------------- | ---- |
| `GET /{$}`                                                 | the shell, drawing the queue                        | 028  |
| `GET /reviews/{review}`                                    | the shell, drawing the current round                | 032  |
| `GET /reviews/{review}/rounds/{round}`                     | the shell, drawing that round                       | 032  |
| `GET /reviews/{review}/files/{path...}`                    | the shell, drawing a file or a directory            | 030  |
| `GET /reviews/{review}/rounds/{round}/files/{path...}`     | the shell, drawing a file or a directory at a round | 030  |
| `GET /projects/{project}`                                  | the shell, drawing the project view                 | 030  |
| `GET /projects/{project}/files/{path...}`                  | the shell, drawing a live file, by `copy`           | 030  |
| `GET /api/{$}`                                             | the queue's view, as JSON, with a stamp of its rows | 028  |
| `GET /api/reviews/{review}/files/{path...}`                | the tree or the file at the latest round, as JSON   | 030  |
| `GET /api/reviews/{review}/rounds/{round}/files/{path...}` | the tree or the file at that round, as JSON         | 030  |
| `GET /api/projects/{project}`                              | the project view, as JSON                           | 030  |
| `GET /api/projects/{project}/files/{path...}`              | a live file, by `copy`, as JSON                     | 030  |
| `GET /api/reviews/{review}`                                | the current round's view, as JSON                   | 031  |
| `GET /api/reviews/{review}/rounds/{round}`                 | that round's view, as JSON                          | 031  |
| `GET /api/reviews/{review}/rounds/{round}/cards/{path...}` | one file card's body, as the card nears view        | 031  |
| `POST /api/reviews/{review}/comments`                      | one comment, answered with the comment              | 032  |
| `PUT /api/reviews/{review}/comments/{comment}`             | a comment's new body, before the verdict            | 032  |
| `DELETE /api/reviews/{review}/comments/{comment}`          | drop one before the verdict                         | 032  |
| `POST /api/reviews/{review}/decision`                      | the verdict, releasing the round                    | 032  |
| `GET /reviews/{review}/raw/{round}/{path...}`              | a file's bytes as of a round, 0 the base            | 010  |
| `GET /events`                                              | the live stream, and therefore the connection       | 008  |
| `GET /manifest.webmanifest`                                | what makes the page installable                     | 009  |
| `GET /sw.js`                                               | the service worker, scoped to the surface           | 009  |
| `GET /static/icons/{name}`                                 | the installed page's icons, from the binary         | 009  |
| `GET /static/app.js`                                       | the app, bundled once from the binary               | 033  |
| `GET /static/app.css`                                      | the components' stylesheets, bundled with the app   | 006  |
| `GET /static/fonts/{path...}`                              | a font the stylesheet names, with an ETag           | 015  |
| `POST /subscriptions`                                      | a device's push subscription                        | 009  |
| `POST /taps`                                               | a beacon from a notification tap, logged            | 026  |
| `GET /healthz`                                             | liveness, after touching the database               | 006  |

The routes that answered with fragments are gone: the composer, a comment's card
and its edit form. The app draws those from data it already holds. So are the
template page's own writes, which took a form and answered a fragment, and the
template route for a card's body, since the app writes and reads under `/api`.
`/static/shell.css` joins the bundled stylesheet once the tearout pages it stays
out of are gone.

**A failure answers the person with the app and anything else with text.** The
surface fails with a 404 or 405 where the mux has no route, a 413 for a raw file
past the send limit, a 500 where a handler fails, and a 403 for a write outside
`/api` without the token. A browser loading the address as a page sends
`Sec-Fetch-Dest: document`, and then the answer is the shell with the failure's
status and a `<meta name="eyeball-error">` tag holding a `view.Failure` as JSON:
the heading, the sentence and the waiting count for the eye. The app reads the
tag before it routes and draws an error screen in place of any route. An image,
a frame, a `fetch` and curl send another value, or none, and get `text/plain`
with the status and the sentence. The tag is needed because a raw file's address
has no route in the app, so the app alone would draw "Nothing here" for a file
that exists and is too large. `Accept` cannot decide this, since a frame sends
the `text/html` a page does. Measured in headless Chromium on 2026-11-22 and in
iOS Safari on 2026-11-23. A browser that sends none gets the text, which is
plain and correct.

Two alternatives lost. A scriptless template page keeps an HTML page for every
client, but the bundle is what sets the theme, so it shows light on a dark
phone, and a frame gets a page inside a card. Answering every failure with the
shell sends a frame and curl an app they cannot use. The worker's cached shell
never carries a tag: it answers a navigation from the network and falls back to
the cache only for `/` when the daemon cannot answer.

`GET /{$}` rather than `GET /`, because `/` is a prefix pattern that matches
every path, and would answer `/agent/daemon` on the surface's listener.

**A worktree's live file is the project's file route with `?copy=<id>`**, the
working copy's row id. Without it the route reads the project's root, so a
project with one working copy keeps the addresses it had. A path segment for the
copy was rejected because every existing project link would change.

**Paths are spelled out.** `/reviews/` rather than `/r/`, `/files/` rather than
`/f/`. These are read in a browser address bar, quoted in a comment, and typed
into a terminal by somebody debugging, and the four characters saved are worth
less than not having to remember what the letter meant.

`http.ServeMux` with method-and-path patterns. Handlers are methods on a server
holding the store and the blob store, and each declares the narrow interface it
needs rather than depending on a package-wide one.

**Screens moved one at a time.** While some were templates, the server answered
a moved screen's address with the shell and every other address with its
template, and a link from one kind to the other was a full page load. Every
screen is now the app's: the queue, a project, a tree or a file and a review's
round answer the shell, which is the only template left. Rebuilding every screen
on one branch was rejected: it is one review too large to read on a phone, and
nothing shipped until all of it did.

**One bundle, at `/static/app.js`.** The shell links it with `/static/app.css`
and `/static/shell.css`, since `app.css` stays one file and the bundle imports
no stylesheet. A link to an address the app does not draw is a plain anchor, so
tapping it is a full page load. React Router intercepts only its own `Link`. The
bundle was `/static/react.js` while a template bundle held this address. The
worker caches by exact address, so a phone with the old shell keeps asking for
`react.js` until its new worker takes over, and no alias for it is kept. That is
reasoned from the worker, not measured.

**One client function says which addresses the app draws**, by matching an
address against the router's routes. The link component draws a router link for
such an address and a plain anchor for any other, and the tap listener moves the
router for one and loads any other. Go builds every href, escaped, as the
templates do, and the app follows it. A view is fetched from `/api` plus the
page's raw path and query, so a query such as `?copy=` reaches the view
unchanged. Measured 2026-10-31 with React Router 7.18.5's memory router: a link
keeps an escaped href, `%20`, `%25`, `%23`, `%3F` and UTF-8 alike, and
`useLocation` keeps the path escaped where the splat parameter decodes it.

## The client code

**React, React Router and TanStack Query, in TypeScript.** The router owns the
address and the screen it draws. The query cache holds every view the app has
fetched, keyed by its API path, and is the only place server data lives in the
app. The app's sources are in `web/src/`.

These are the libraries the other installed apps on this machine are built on,
so how they behave on this phone is already known. Smaller choices, such as
Preact or a cache written here, would save some tens of kilobytes of bundle, and
would give up that record. This paragraph said a few kilobytes cost nothing
measurable on a tailnet, which holds for a bundle and was wrong for a review's
data. Measured 2026-11-01 on the owner's phone, which reached the probe's host
through a Tailscale relay rather than directly: 4.8 MB of rows took 3.5 s to 10
s to load uncompressed, and 0.8 s gzipped. The phone's path to the daemon's own
node was not measured. Compression under "The surface" answers that.

**The binary bundles the app with esbuild, linked into Go, from embedded
sources.** The app's sources and the packages it imports are embedded with
`//go:embed`. The first time a page asks for the bundle, the binary writes the
embedded vendor tree, `web/deps/node_modules/`, to a temporary directory once
per process and bundles the app with esbuild's own resolver over it. The result
is held in memory. The directory also holds the app's sources, the stylesheet
and a `web/package.json` that says only `"type": "module"`. esbuild wraps each
CommonJS package differently when it reads the importer as an ES module, so
without that file the bundle differs from the one built from `web/node_modules`,
and the two have to match byte for byte. No Node at build time and no committed
bundle, so `go install` produces something that works.

The packages have to resolve through their `package.json` `exports` maps, which
esbuild's resolver reads and a hand-written plugin would have to copy. A resolve
plugin that pointed every bare import at a vendored directory was rejected for
that reason. The cost of the replacement is a write to a temporary directory the
first time a page asks for the bundle. Measured 2026-10-30 with the router and
the query library imported, that is about 1.6 MB: 1.4 MB of vendored packages in
68 files, 110 KB of the app's sources and fixtures, and 76 KB of the scripts it
shares with the service worker. The bundle it builds is 313,446 bytes minified
and 98,259 gzipped at level 9, which agrees with the 313 KB and 98 KB first
written.

Measured 2026-10-29, in scratch projects: React 19.3.1, React Router 7.18.5 and
TanStack Query 5.105.0 bundled by esbuild 0.28.2 through its Go API. Copying
only the 68 files esbuild's metafile lists, plus each package's `package.json`,
into a fresh directory and bundling from there gave a bundle identical byte for
byte to the one built from `node_modules`: 353 KB minified, 111 KB gzipped, from
1.4 MB of vendored files out of 21 MB installed. React Router's `exports` reach
only its `dist/development` build, since the production one is not exported, so
that is the build the app bundles.

**Vite is for development only.** Its dev server reloads a component as it is
saved, proxies `/api` and `/events` to a running daemon, and is what Storybook
10 builds on. Measured 2026-10-29: Storybook 10.6.5 with Vite 8.3.5 built a
story, and its full-screen address, `iframe.html?id=<story>&viewMode=story`,
rendered the component alone in headless Chromium. Its Vitest add-on 10.6.5 with
Vitest 4.1.12 ran two stories as tests there, through Playwright's
`executablePath`, and a planted failing story failed. Vite has to pre-bundle
React (`optimizeDeps.include`), or the add-on's setup file fails to import it.
The binary never runs Vite's output. The cost is two bundlers: a fault only
esbuild's bundle has would not show in development. The browser tests under
`internal/server` run against the bundle the binary serves, so that is the
bundle the gate checks.

**`web/deps/node_modules/` holds only the files the app's bundle reads.** It is
not under a directory named `vendor`, for the reason in "The binary".
`make vendor` bundles the app from `web/node_modules` after `npm ci`, and copies
each file esbuild's metafile lists, plus each package's `package.json`, into it.
It is committed because the binary embeds it.

The set follows the app's imports, so it changes when an import does: a new
import of a package already vendored can need a file that is not there yet. A
check against the lockfile misses that, because the lockfile has not changed.
`make check` instead re-runs the vendor step into a temporary directory and
fails when the result differs from the committed tree.

Two alternatives were rejected. Committing the built bundle would put minified
output into every branch's diff, which is read on a phone. Requiring Node for
`go build` would break `go install`.

**TypeScript at its strictest.** `strict: true` plus `noUncheckedIndexedAccess`,
`exactOptionalPropertyTypes`, `noImplicitOverride`,
`noPropertyAccessFromIndexSignature`, `noFallthroughCasesInSwitch`,
`noImplicitReturns`, `noUnusedLocals`, `noUnusedParameters`,
`allowUnreachableCode: false`, `allowUnusedLabels: false`,
`verbatimModuleSyntax` and `isolatedModules`. Each flag turns a mistake that
would only show as a screen drawing the wrong thing, such as an index read past
the end of a list or an optional field set to `undefined`, into a build failure.
`isolatedModules` is mandatory anyway, because esbuild transforms one file at a
time. No `any`, explicit or implied, which oxlint enforces with its type-aware
and React hooks rules. esbuild and Vite both strip types without checking them,
so **`tsc --noEmit` is a gate rather than a build step**, and oxfmt formats.
Measured 2026-10-29: TypeScript 7.0.2 with every flag above and
`skipLibCheck: false` passed over a file using React, React Router, TanStack
Query and React DOM, and failed on a planted `exactOptionalPropertyTypes`
violation.

**Plain CSS, one stylesheet per component, in the end.** A component imports its
own stylesheet, and both bundlers join them into `app.css`. While the tearouts
link it, `app.css` stays one file that they and the components both use, so the
two cannot drift while both exist. It splits into one stylesheet per component
when the tearouts are gone. The class names, the tokens and the reading-desk
design in today's `app.css` carry over as they are, because they were judged on
the phone and chosen against an alternative in 013. Tailwind was considered and
rejected. It would rewrite approved styling into utility classes and put every
screen back in front of the phone for no visible change. The class checks in
[conventions.md](conventions.md) also read plain CSS, and would have to be
rebuilt for utilities.

**The tearouts are Storybook stories of these components.** Every state a screen
has is a story on fixture data, and the fixtures are views the Go server
produced from `tool/seed`, so a story shows what the API really sends. How they
are judged on the phone is in [docs/design/README.md](design/README.md).
Storybook, and the Node it needs, are development dependencies only.

### Client state

**The server is the truth, and the cache is a copy the server corrects.** The
template design rejected client state because the tally, the comment list and
the selection would exist twice and could disagree. Here they still exist twice,
and four rules keep the copy correct.

- **The stream says when to refetch.** Each stamp from `/events` names the view
  it covers, where today one stamp covers a whole page, and the app invalidates
  the views it names. A change no screen shows refetches nothing. Until a view
  has a stamp, it is refetched when its screen opens and when the app comes back
  to the foreground, which TanStack Query does by default. The project view, the
  tree and the file pages have none yet. The review page's views carry the stamp
  `/events?review=N` sends, which covers the review's state, each round's
  verdict and the agent's replies. A review's round and its bodies are fetched
  when the screen opens, on a stamp that differs from the round's own, and after
  the reader's own write where the server decides what is drawn, as a passage's
  folds. Focus and reconnect do not refetch them: a fetch there would arrive
  outside the hold, and the stream sends the stamp again when it returns, which
  finds what was missed. A change refetches the round and the card bodies the
  app asked for. It leaves comments out, since the reader's own comments reach
  the cache from the server's answers. A verdict given elsewhere takes the bar
  and its note away and turns an open composer late, so the change waits for
  what the template's reload waited for: the round is not fetched while text in
  a composer, an edit form, the review's box or the note is held, a comment is
  on its way or one did not save, and a round that was fetched is not shown
  until none is. The reader's own writes are put in what is shown at once, and a
  write's answer cancels a fetch of the round already in flight and fetches
  again, since that fetch may have read the store before the write. A stamp
  whose fetch failed or was lost with the connection is asked for again when the
  stream returns, which sends the same stamp.
- **A refetch never takes typed text away.** The text of a composer, of each
  open edit form, of the review's box and of the verdict's note is the screen's
  state and not the element's, so a refetch that redraws a card keeps it. The
  screen is keyed by the address it draws, so a newer round at the same address
  reaches the screen as a newer view and is taken up through the hold, never by
  drawing the screen again. A key that held the round's number would have
  dropped the text the first time a refetch returned a new round. Browser tests
  type into a composer, move the stamp, bring the app to the foreground, drop
  and restore the connection, and add a comment on a passage after a verdict
  elsewhere and a new round, and read the text back. A composer is keyed by
  where it opened, a line, a passage, a file or the review, and a tap on another
  place moves it there with what was typed, as the template page's did. A second
  composer for each tap would scatter half-written comments through a long
  review.
- **A write updates the cache from the server's answer**, never from a guess. A
  verdict shows as decided only once the server has said so. A comment shows as
  sending until its answer arrives, and as unsent with its text and a retry if
  the answer is a failure, as product.md requires. A verdict sets the blink mark
  when it is sent and clears it if it fails. Once it is stored the router moves
  to the queue, which reads the mark when it mounts, as it does after a page
  load. The round's cached view and bodies and its draft are forgotten first.
  Back to the review then fetches the decided round. The cached open round would
  draw the bar again, and the note restored from the draft would hold the
  fetched round back, so the decided round would never be taken up and a second
  verdict would be refused as stale.
- **What only this device knows stays on this device.** Drafts, the reader's
  place and folded cards stay in `localStorage`, as they are today. The app
  reads them through `web/ts/storage.ts`, which prefixes each key and wraps each
  value with the time it was saved, under the keys and in the shapes the
  template page saved, `drafts:R:N` and `place:R:N:against`, so text typed on
  the template page before the switch is there after it. A draft keeps the
  template's five parts key for key: the verdict's note, the review's box, the
  composer with the query that opened it, each open edit form by its comment's
  id, and each unsent card with its form's action, target and values. A decided
  round keeps none, and the app prunes entries past thirty days as the template
  bundle does when it opens.

**Each screen holds its own stream**, for the build and connection notices and
for the stamps it follows. The queue opens `/events?queue=1`, and a screen with
no stamps opens `/events`. A move between screens closes one and opens the next,
and the notice's two-second delay keeps the gap from showing. One stream for the
whole app was rejected while the queue is the only screen with stamps, since its
address names what it follows.

**Back returns to the place in a list.** A move inside the app is not a page
load, so the browser does not restore the scroll. The router is a data router
with React Router's `ScrollRestoration`, which does. Measured 2026-10-31 in
headless Chromium with React Router 7.18.5 and a cached view: a list scrolled to
4000 px, a row opened, and back returned to 4000 px. iOS is not measured.

**The review screen sets its own scroll, after the router's.**
`ScrollRestoration` scrolls in a layout effect at the root on every move: to the
saved position on back and forward, to the element the hash names if it exists
yet, and otherwise to the top. That is read from React Router 7.18.5's
`useScrollRestoration`, not measured. The root draws the router's component
after the screen, and layout effects run children first, so a layout effect of
the screen would run before the router's. A screen drawn in the same commit as
the move, from a cached view, is where the order shows. A review's view and its
bodies arrive after the move otherwise, so the screen sets the reader's saved
place in an effect, which runs after every layout effect, and again as each body
arrives until the reader's first pointer down, click, wheel, touch move or key,
or three seconds, as the template page did.

Nothing is saved until that ends. The router's scroll to the top, and a place
applied to a page that is still too short, would otherwise be saved over the
place being restored. The scroll saved when the screen goes is the last one its
handler saw, since by then the router may have moved the page for the screen
that replaces it, and a page load saves it as the page is hidden. A review with
no place saved is armed at once.

A jump is an address with a hash that the reader did not come back to: a router
move, or a page load with no place saved. The jump wins over the saved place
there, and a reload or a move back keeps the place, since a reload after a jump
would otherwise return to the card it named. At the move the target is not drawn
when it is in a view fetched after the move, so the router scrolls to the top,
and the screen scrolls to the target once it is drawn, below the page's header
and the card's by the stylesheet's scroll margins. A link to a card or a comment
on the same screen is a router link too, never a plain `#` anchor. A plain one
pushes a history entry with no state, which the router files under the key
`default`, the key of the first page the app loaded, and its scroll then
restores that page's place. Measured 2026-11-01 with React Router 7.18.5 in
headless Chromium: after a router move from a list scrolled to 1500 px, a plain
anchor left the page at 1500 px, and a router link reached its target. The list
of files keeps its `#` address as markup and moves the router on a plain click,
with the query kept, since the comparison is part of the address. A card's fold
is the screen's state, seeded from the saved place, since a card drawn with a
constant `open` opens again whenever a refetch mounts it anew.

**Shared behaviors stay in `web/ts`, and the React app calls them.** Theme, the
loopback token, worker registration, push setup, the verdict's blink mark, the
build check and the keyboard fix in `viewport.ts` are framework-free modules,
and several were settled on a real phone. Where a module mixes its logic with
template DOM, as push setup does with its page's buttons, the logic is a
function both callers use and the template keeps its wiring. The binary embeds
and stages `web/ts` beside `web/src`, so the app imports them. Rewriting them in
React was rejected: two copies would have to agree on storage keys, such as the
blink mark and the token, and nothing would check that they do. The review
page's own modules were the exception, since each was wired to the template's
markup and its events: the composer, the drafts, the unsent cards, the deferred
bodies and the reader's place. The app took their logic into the review screen
and they are gone with the review template, so no two copies are left to agree.

**The iOS behaviors tied to a screen move with that screen**, each re-checked on
the phone when it moves: the deferred card bodies with their skeleton heights
and Safari's missing scroll anchoring, the reader's place, the composer's line
selection and drafts. Each was settled on a real phone, and a component that
only looks the same can reintroduce a fault the original fixed.

**A long review is drawn in pieces.** Past ten files or 300 lines a card arrives
open with an empty body, and the app asks for each body once its card is open
and within a screen of view. Until it comes, the card holds a skeleton as tall
as the body will be, from the hunk and line counts the view sends and row
heights measured once on the device, so a body arriving above the reader moves
nothing. A file that moved more than 500 lines holds its body behind a line
naming its size and a Load control, since a lock file loaded by accident is a
long scroll past it.

A body is a cached view of its own. Its query is keyed by the address its card
names, query included, so `?against=base` stays distinct, and is enabled for a
path the card has asked for, through its observer or a held card's Load. A
disabled query still returns what the cache holds, so a review the reader
returns to, from a file page, draws its bodies at once and reads them again
behind them. The screen merges the bodies into the round to draw and to read it,
and a write updates the shown round and each body's entry, each from its own
latest value, so answers handled in one tick build on one another. A round
fetched again cannot touch a body, and a body arriving does not pass through the
round's writes. A fetch of a body already in flight when a write's answer lands
is cancelled and made again, as the round's is, since it may have read the store
before the write. A comment on a passage of a document reads that card's body
again, since whether a passage sits in a fold is the server's rule. A newer
round, once the screen takes it up, makes every body of that round stale: after
a verdict given elsewhere their comments still offer Edit and Delete. Each body
the screen has asked for is read again, on screen or not, and keeps its old data
meanwhile. That costs a request for each card the reader scrolled past, which
was accepted over dropping the bodies to load again near the reader, since a
body above the reader turning back into a skeleton moves what is read. A body
the cache holds for a card the screen has not asked for is only marked stale. A
body is not read on focus or on reconnect, which would take the decided state
into its comments while the bar is still drawn. A body that fails, or a held
card's Load that does, fails at once with no retry and draws the failed line
with a Retry in its card. The request carries the query's signal and ends when
the screen goes. Measuring a skeleton's row heights happens once per run of the
app, on the first open card, since the device decides them.

A skeleton is only as tall as a guess, and a document's, six lines whatever its
length, is a poor one. Safari has no scroll anchoring, so a body taller than its
skeleton arriving above the reader would push what they were reading down. The
screen keeps the card at the top of the screen where it was whenever a card
above it changes height, as the template page's `holdPlace` did, and turns
Chromium's own anchoring off so a browser test runs the path Safari runs. A page
at its top holds nothing. What grows above the first card there, as the letter
does when its font arrives, is what the reader is looking at, and scrolling past
it would take them down the page. Browsers' own anchoring is off at the top for
the same reason.

Rows are drawn per body, not only near view. Measured 2026-11-01 with a probe
page, not the real one: React and this stylesheet drew 26 cards of this
repository's Go source, 11,846 rows, under the surface's policy, on the owner's
iPhone in Safari. With every body mounted at once, the first draw took 1.5 s.
With each body mounted as its card came within a screen, and the 12 cards over
500 lines held behind Load as the real page holds them, the first draw took 76
ms. The slowest body, 495 rows, took React 17 ms to mount, inside one frame. A
fling still had 10 frames over 50 ms in 340, the worst 63 ms, and a run with the
data uncompressed had 3 in 269. Neither fling looked wrong on the phone: no
stutter, and no skeleton left where code should be. Frames that slow with mounts
that cheap point at the browser's layout and paint of new rows, or at how Safari
times frames while it scrolls, rather than at React, so drawing only the rows
near view is not expected to remove them, and they did not show. That cause is
an inference, not a measurement. Drawing rows only near view was rejected for
its cost: row heights would have to be known before rows are drawn, and the
reader's place and a line's anchor would point at rows that may not exist.
React's `style` prop set the skeleton heights with no violation of the policy,
whose `default-src 'self'` refuses a style attribute, because React writes a
style through the CSSOM rather than as an attribute. The real page, measured
2026-11-22 on the same phone and the same 26 files, had no frame over 50 ms in a
fling of 362 frames, the worst 33 ms. Its first card was drawn 1.3 s after the
view's response ended on a cold load and 0.9 s with the service worker warm,
time not yet divided between parsing the view and laying out the page.

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

**The service worker keeps the app's shell for when the daemon cannot answer.**
The shell is the page every address answers, the bundle, the stylesheets, the
fonts and the icons, kept in a cache named by the build that served them. Data
is never cached. A round's content is immutable and could be, but a second copy
can outlive a daemon restart, and on a tailnet the round trip it would save is a
few milliseconds.

**A launch asks the daemon for the shell first.** The worker answers a
navigation to a shell address from the network, and from its cache only when the
daemon refuses the connection or has not answered within a second. The second is
a guess. A refused connection failed in 4 ms in Chromium on 2026-10-30, so a
stopped daemon falls back at once, and only a machine that cannot be reached at
all waits out the timeout. Serving the shell from the cache first was rejected.
The daemon's build changes with every binary, so the first launch after each
rebuild would run the old shell against the new daemon and show the update
notice, which is the prompt on every build rejected below. Asking first costs
one round trip for a small page on each launch, and the view the screen draws
waits on the daemon anyway.

**The shell's files are addressed by build.** The head writes a short hash of
the build into the address of each script and stylesheet it links, and the
worker answers each one from whichever cache holds that exact address. So a page
draws one build's files, whichever build its HTML came from. Without it, a shell
fetched from a new daemon would draw with the old cache's bundle until the new
worker took over. A probe on 2026-10-30 with one address per file drew mixed
pages: a reload timed to land as a new worker installed drew the old HTML with
the new script and stylesheet in 5 of 27 trials in Chromium and 4 of 37 in
WebKitGTK. The daemon ignores the build in an address and serves what it has.

**Every build changes the worker, and a worker caches only its own build.** The
daemon writes its build and the list of the shell's files into `sw.js` as it
serves it, so a new binary is a new worker to the browser, which installs it on
its next update check. The install fetches each file with a header naming the
worker's build, and the daemon answers 409 when the header names another. A
failed fetch fails the install, and the worker already in control stays.
Measured on 2026-10-30 in Chromium and WebKitGTK with the check missing: a
daemon restarted between the fetch of `sw.js` and the fill of the cache left the
cache named for the old build holding the new build's files.
`Cache-Control: no-store`, which every surface response carries, does not keep
Cache Storage from holding a response, measured the same day in both. When the
new worker activates it deletes every other build's cache.

The worker answers nothing else. The live stream, the API, every write and every
address the app does not draw go to the daemon untouched, so such a page with
the daemon down fails as it does today.

**A page open across a rebuild offers to reload.** The live stream sends the
daemon's build each time it connects, and when it differs from the build that
served the page, the app shows a notice offering to reload. A reload fetches the
shell from the daemon like any launch, so one Refresh reaches the new build. A
reload prompt on every new build was rejected: nearly every build changes
nothing a running screen depends on, and a prompt shown for those teaches the
reader to dismiss it.

**The service worker also exists for push.** iOS delivers web push only to an
installed page with a registered worker, so one would be required even with
nothing to cache. It handles `push` by showing the notification and
`notificationclick` by opening the review's URL.

**A tap moves the open app rather than reloading it.** A window already on the
review is focused. Otherwise the worker posts the review's address to the window
it finds and waits a second for an answer. The app's router moves to an address
it draws with no page load, and a page sets `location` for one it does not, so
both bundles answer. A window that does not answer gets `navigate`, and the
beacons log how long the answer took and whether `navigate` ran. The second is a
guess: in headless Chromium on 2026-10-29 a page answered in under a
millisecond. Only with no window to post to does the worker open one, which
loads the shell. `navigate` on the window, which the worker used while the
server rendered pages, reloads the whole app. It also failed to open the named
review twice during 020, and 026 could not reproduce why, so a path that does
not depend on it is the fix this design offers for that fault.

**The worker reports each tap to the daemon.** A tap on a phone shows nothing of
which of `openReview`'s branches ran, and the worker has no console to read. So
`openReview` posts a beacon to `POST /taps` when it starts and around each step,
and the daemon writes each body as one info line in `eyeball.log`, beside the
stamp of the `sw.js` it serves. The first beacon lists the windows the worker
can see, with and without `includeUncontrolled`. Later ones name each step and
what it resolved to or how it failed. Every beacon carries the worker's own
build stamp, a hash of its script that the Go build sets, so a phone running an
older worker shows as two stamps that differ.

The beacons stay once the fault they were added for is fixed. They cost one
route and a few log lines per tap, and they answer the next question about a tap
without a new instrument. A readout on the page was rejected, since the page
that would show it is the one a failed tap does not replace or replaces too
early. The route is on the tailnet listener, which needs no token. A worker
cannot read the token from page storage, so on the loopback listener every
beacon is refused with 403 and dropped, and the tap goes on regardless.

**A failed write keeps its text on screen**, marked as not saved, with a retry.
The server is what makes a comment durable, and until it answers, the surface
holds the text and says so.

### Knowing whether the server is there

The failure to prevent is a tap that does nothing, which is what a
server-rendered surface does by default when the server is gone. It is a product
requirement in [product.md](product.md). These are the mechanisms.

**A Server-Sent Events stream is the connection, and its state is the answer.**
The surface opens `GET /events` and holds it. The stream sends a stamp of what
each view it covers, recomputed after every change in the store and sent only
when it moved, and the app refetches the views whose stamps moved. So the queue
and an open review follow what agents do, in place, and a reader's own comment
refetches nothing, since the answer to the write already updated the cache. The
stream's `readyState` is then not an inference about the connection. It is the
connection.

`EventSource` reconnects on its own with backoff, and the daemon sends a comment
line as a keepalive, so an idle stream is distinguishable from a dead one.

**`navigator.onLine` is a hint and never the truth.** It reports whether a
network interface is up, so it is true on a wifi network with no route anywhere.
It is worth reading for one thing: a `false` is a fast and reliable negative
that can show the notice before the stream has finished failing. A `true` means
nothing.

**Every write that fails says so, on the control that failed.** A write that
reached nothing, answered with an unexpected status, or took too long, is marked
on the control that sent it, with the reason. With none of that, nothing on
screen changes, which looks exactly like a tap that missed.

**While the stream is down, the verdicts are marked unavailable and say why.**
That is a stronger claim than reporting failures afterward, and it earns it:
being told beforehand that a verdict will not go through beats attempting it and
being told it failed. The read surface stays fully usable, because what is on
screen is already loaded. The composer stays live, as the approved connection
frame draws it: a comment typed while the daemon is gone becomes an unsent card
with Retry rather than being refused. An earlier version of this paragraph said
every write control was marked unavailable, which the frame never showed.

**Nothing is rendered while the connection is healthy.** The notice appears on
loss and disappears on recovery.

One case is left over: a write that races a disconnect, where the controls were
live and the stream dropped between the tap and the request. That is what the
per-write failure is for, and it is why both mechanisms exist rather than either
alone.

## Checks

`make check` is the gate: build, lint, test. It has to pass before an iteration
closes.

- `gofmt`, `go vet`, `staticcheck` on the Go.
- `oxfmt --check` on every committed markdown, CSS and JSON file, at 80 columns.
  HTML is excluded, for the reason in docs/conventions.md.
- `tsc --noEmit` and `oxlint --type-aware` on the TypeScript.
- Vitest over the components, including each story's interaction test and its
  accessibility audit, run in a real browser through Storybook's Vitest add-on.
  A second Vitest project runs checks that have no screen, such as the app's
  comment request against the template's hidden form fields, in the same
  Chromium.
- `web/deps/node_modules/` against a fresh run of the vendor step, and the
  generated API types against the Go structs. Either one stale fails.
- `go test ./...`, which includes `internal/convention`, covering the Go
  conventions, the prose rules over every committed markdown file, the
  stylesheets, and the browser tests against the bundle the binary serves.

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
| `golang.org/x/mod` (test)               | the module zip's rules, to check an embedded file reaches go install  |
| `golang.org/x/net/html` (test)          | tokenizing the tearouts, and parsing rendered pages                   |
| `golang.org/x/term`                     | whether stdin is a terminal, which the tailnet login requires         |
| `modernc.org/sqlite`                    | pure Go SQLite, so the binary stays static                            |
| `tailscale.com/tsnet`                   | the tailnet node is the process, and it brings the certificate        |

The app's packages, vendored under `web/deps/node_modules/` and bundled into the
binary:

| Package                 | Why                                                          |
| ----------------------- | ------------------------------------------------------------ |
| `react`, `react-dom`    | the components                                               |
| `react-router`          | the address and the screen it draws, with no page load       |
| `@tanstack/react-query` | the cache of server views, refetched when the stream says so |

Development only, never reaching the binary: Vite for the dev server, Storybook
for the tearouts, Vitest and Testing Library for the component tests,
TypeScript, oxlint and oxfmt.

Stdlib first everywhere else. `net/http` and `http.ServeMux` over a router,
`database/sql` over an ORM, `encoding/json` over anything.

## Not decided here

- **Where the daemon runs**, and whether a machine that is asleep needs anything
  said about it. The tailnet name does not answer when the daemon is down.
- **Whether previews ever need storage**, which is the one thing that would buy
  a second tsnet hostname.
- **The exact CLI surface.** `eyeball doc` is the contract and it is written
  with the CLI, not ahead of it.
- **Whether a long review needs rows drawn only near view**, which the deferred
  card bodies may make unnecessary. Measured on the phone by the first review
  page spec, not before.
- **Pruning.** How long a decided review is kept, and whether its blobs go with
  it, is an open question in [product.md](product.md).
