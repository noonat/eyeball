---
status: active
created: 2026-10-05T00:00:00Z
updated: 2026-10-05T20:59:48.227706244Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
---

# 007 Widening and documents

> **Autonomous run, 2026-10-04.** Part of the owner's request to build the
> project to completion as product.md and architecture.md describe it, under
> `dev-review autonomous on`. A reviewer subagent stands in for the owner's
> approval of this spec and gives the `reviewed` ack on each iteration. The
> owner keeps tearout changes, questions about behavior, and anything
> outward-facing. Iteration 1 changes tearouts, so it also carries
> `phone-approved`, an ack only the owner gives after looking at the changed
> frames on the phone, and the run stops there for it. Departures from this spec
> are recorded in it as they happen.

The reviewer's surface, second part: every file route, from a hunk out to the
whole file and on to any file in the project at that round, the tree, markdown
rendered as a document with its changed passages marked, and the project view
with its live working copy. Reading only, like 006: nothing a reviewer does
changes a review. The one write the surface's side makes is the daemon recording
a fact it observed about a project, `projects.inside_checkout` (below).
Comments, verdicts, htmx and the live stream are the write path (008). Push, the
installed page and the owner's phone check are notification (009). Images,
media, the sandbox for HTML and SVG, `internal/kind` and the agent's per-file
view override are rendered files (010).

Every route here joins the surface's handler only. The route table in
`internal/server/server.go` is the one place a route is declared, and 006's
listener test walks it, so a route added there is proved absent from the socket
by the test that already exists.

Every claim marked confirmed was run against Go 1.26.8, goldmark v1.8.6, git
2.55.0 and Chromium 152, in throwaway programs, a copy of this repository and
scratch repositories outside it. Earlier specs cite git 2.43, so a claim about
git here was re-run rather than carried over. Round one of the review re-ran
them. Where it found one wrong, the text says so as a correction.

## Design

### The routes

| Pattern                                                | Answers                                     | Iteration |
| ------------------------------------------------------ | ------------------------------------------- | --------- |
| `GET /reviews/{review}/files/{path...}`                | a file or a directory, at the current round | 5, 6      |
| `GET /reviews/{review}/rounds/{round}/files/{path...}` | a file or a directory, at that round        | 5, 6      |
| `GET /projects/{project}`                              | the project view and its working copy       | 9         |
| `GET /projects/{project}/files/{path...}`              | one changed file in the working copy, live  | 9         |

**One route answers a file and a directory.** The path after `/files/` is a
project path, and the round decides whether it names a file, a directory or
nothing. A separate `/tree/` route was the alternative. It doubles the patterns
for no gain, because the handler has to ask the round what a path is either way,
and it gives one directory two addresses depending on how it was reached.

**`{project}` is the store's project id**, as `{review}` is. Two worktrees of
one repository display under one name, as 005 decided, so a name cannot address
a project.

**`/projects/{project}/files/{path...}` is new, and architecture.md's routes
table gains it in iteration 9.** The project tearout links every working-copy
row, and a row for a file no review has claimed has nowhere else to go. The
alternative was rows that are not links, as 006 shipped the empty queue's
project rows. That leaves the work no agent has submitted, which the project
tearout calls the interesting part, listed and unreadable.

**The current round answers under both addresses**, as 006 decided for the round
page. Links written from a current round use the short form, and links from an
earlier round carry `/rounds/{round}/` so they stay on that round.

**A project path is escaped segment by segment when it becomes an address.**
Written raw, `a#b.md` parses as the path `a` with the fragment `b.md`, `q?.md`
as the path `q` with a query, and `50%.md` does not parse at all, all confirmed
with `net/url`. Every address the surface writes for a project path joins its
segments through `url.PathEscape`, which gives `a%23b.md`, `q%3F.md` and
`50%25.md`, confirmed. The mux hands the handler the unescaped path, so nothing
on the reading side changes.

**Every surface response gains `Referrer-Policy: no-referrer`.** A document can
link to any `https` address, and a browser following it sends the surface's
origin as the `Referer` under its default policy. Over HTTPS that origin is the
tailnet's machine name, which then sits in a third party's logs. The header
covers every link on every page, where a `rel="noreferrer"` on each rendered
link would cover only the ones the renderer remembers. The browser behavior is
assumed from the default policy and not probed.

### Closing the live-read holes first

006 closed one hole in the working-copy fallback before any route could reach
it. More are confirmed, and iterations 2 and 3 close them before iteration 5
adds the first file route.

**The project's state is settled first, once per request.** Before any path is
looked at, `capture.Inspect` answers four questions about the round's project,
in this order, and every rule below reads its answer rather than asking again.
The first two answers short-circuit the rest.

1. Whether the root exists, by `os.Stat`. A missing root answers every file
   route from the round's capture alone, refuses every live read, and the file
   page and the tree carry a `notice warn` naming the missing root.
2. Whether the project has lost its repository, decided from the filesystem with
   no git call and no locale. `FindRoot` only ever records a root that held a
   `.git` entry or a `.eyeball` marker, so a root holding neither has lost its
   repository, whatever the round's base. A round with a base whose root has no
   `.git` entry has lost it too, even with a marker beside it: a root holding
   both a `.git` and a `.eyeball` keeps its marker after the `.git` is moved
   away, so the first test does not fire, and `cat-file -e` on its base then
   exited 128, confirmed, which would make every route a 500. A marker project
   never records a base, so this test cannot misfire on one. It covers only a
   round with a base. The same root on an unborn branch records no base, and
   after `mv .git` held only its marker, with no `.git` at or above it,
   confirmed, so this test misses it and the flag below has to catch it. A root
   whose `.git` is a file naming a `gitdir:` that no longer exists has lost it
   too: the file is read and its target stat'ed. None of these makes a git call.
   A lost repository skips the next two questions.
3. Whether the base commit still exists, by `capture.BaseGone` (below), for a
   round with a base.
4. Which ignore rules apply to a live read (below).

Deciding "lost" first matters. Without it, a linked worktree whose git directory
is gone fails every git call, and every route would be a 500: in one, every git
call exited 128 with "not a git repository: (null)" and did not fall back to an
outer repository, confirmed. And a git project nested in an outer repository
whose `.git` moved away would read as a gone base with live reads allowed, with
git answering for the outer repository.

**A lost repository lists the capture alone and refuses live reads.** With the
`.git` moved away, git either fails or answers for the wrong repository: alone,
`rev-parse --is-inside-work-tree` exited 128, and inside an outer repository
`rev-parse --show-toplevel` named the outer one, both confirmed. Either way the
files the project's repository ignored, its credentials files among them, would
read live with no rule of its own to refuse them. An unborn branch shows why the
test cannot depend on the base: it records no base, and after `mv .git` the walk
found no `.git` above it and `check-ignore` exited 128, confirmed, so the
round-two rules would have read it as a project with no ignore rules and served
its ignored `private.txt`. So under a lost repository the file routes answer
from the capture alone, the tree lists the capture alone, and a path the capture
does not hold is the 404 page. Nothing is compared, as under a gone base. The
banner names the lost repository. Treating it as a project outside git was the
alternative, and it would turn a broken checkout into the widest exposure the
surface has.

**A `.git` segment is refused at any depth.** `safePath` refuses `.git` as the
first segment only. An untracked repository inside the project, such as a
vendored checkout, keeps its own `.git/config` with whatever remote URL and
credentials it holds, and `Reader.Open` served `vendor/x/.git/config` from the
working copy, confirmed. `safePath` becomes `capture.CleanPath`, exported for
the handlers, and refuses a path with a `.git` segment anywhere in it.

**A path holding a NUL byte is refused.** `%00` in an address reaches the
handler as a NUL. `os.Lstat` fails on it with `invalid argument`, and so does
starting git with it as an argument, both confirmed, and neither is
`ErrNotFound`, so the page would be a 500. `CleanPath` refuses it as not found.

**The working-copy fallback refuses a path git ignores.** `Reader.Open` served
an ignored `private.txt` and `build/out` from disk, confirmed. Nothing the
surface writes links to an ignored file, because the tree lists only what the
round's sources hold, but a typed address would reach a credentials file, and
every device on the tailnet can type one. So wherever git can answer, the
fallback asks `git check-ignore -q -- ./<path>` and refuses on exit 0. Exit 1 is
not ignored, and anything else is an error, with one exception below.

The `./` prefix matters. For an untracked, ignored file named `:other.txt`,
`git check-ignore -q -- :other.txt` exits 1 and so calls it not ignored, and the
`--stdin` form does the same. Both read the leading colon as pathspec magic.
`--literal-pathspecs`, which `Reader.loadOIDs` relies on for the same problem,
makes check-ignore exit 128 instead, because the command refuses that magic.
`./:other.txt` exits 0. All confirmed.

A file inside an untracked repository nested in the project, such as
`vendor/y/bar.go`, is not ignored: check-ignore exits 1 for it, confirmed. It is
refused anyway, by the containment check below, because the capture skips a
nested repository as another repository's content and the tree does too, and an
address serving what the tree hides would make the tree a poor guide to what the
surface exposes.

**A path inside a submodule is not found.** `git check-ignore -q -- ./mod/m.go`
exits 128 with "Pathspec './mod/m.go' is in submodule 'mod'", confirmed, which
the rule above would turn into a 500. A submodule's files belong to another
repository, which is why the tree and the capture leave a gitlink out, so an
exit 128 whose message says the path is in a submodule is read as not found. Any
other 128 is still an error. The message is localized: under `LANGUAGE=de` git
wrote it in German, and under `LC_ALL=C LANGUAGE=` in English, both confirmed,
so the call pins both variables. Reading the index for a gitlink above the path
was the alternative, and it costs a second git call on every live read to answer
a case the message already answers.

**A path beyond a symlink never reaches check-ignore.** check-ignore exits 128
with "is beyond a symbolic link" for `./linkdir/doc.go` where `linkdir` is a
link, confirmed. The containment check below refuses a path through a symlinked
directory first, so that 128 cannot arise from a live read.

**A marker project asks git when a repository is around it.** A directory with a
`.eyeball` marker is its own project even inside a repository, as
architecture.md says, and 004 made `isGitProject` false for it on purpose, so it
is captured by walking. Git can still answer for it: in a `notes` directory
holding a marker, `git check-ignore -q -- ./creds.txt` exited 0 for a file its
`.gitignore` names, confirmed. `capture.Inspect` decides by the filesystem and
by one flag the store keeps, rather than by asking git whether it is inside a
work tree:

- It looks for a `.git` entry at the marker root and then above it, starting
  from the resolved root the store keys on, not the logical one. Found, it
  reports that the project sits inside a checkout, and goes on below. None
  found, and the project was never recorded inside a checkout, means no
  repository is around it and no ignore rules apply. None found, and the project
  was recorded inside one, means a repository's `.git` was moved away, and live
  reads are refused.
- With a `.git` found above the root, it asks `git check-ignore -q -- ./` at the
  marker root. A `.git` at the root itself makes the root a git project, and the
  git project's rules apply. Exit 0 means the outer checkout ignores the whole
  project: in an ignored directory holding a marker, `./plan.md` and `./` both
  exited 0, while `./` exited 1 for the unignored `notes`, all confirmed. Every
  live read is refused.
- Exit 1 means per-path check-ignore applies, as in a git project.
- Any other failure is an error, never a reason to skip the check. Asking
  `rev-parse --is-inside-work-tree` was the round-one draft, and any failure of
  it switched the check off: `GIT_CEILING_DIRECTORIES`, which the environment
  scrub keeps because it belongs to the user, made it exit 128 inside `notes`,
  confirmed, and so does a filesystem boundary.

**The owner's decision, 2026-10-05: a marker project the outer repository has
disowned, or whose outer `.git` has gone, gets no live reads.** The spec had
accepted both as exposure. The owner chose to fail closed instead, as the spec
already does for a lost repository: every live read is refused, the tree lists
the capture alone, since a marker project records no base, the file page's live
banner and the project view's working copy say live browsing is unavailable and
why, and captured files still open. Nothing git would ignore can leak, apart
from the two narrow gaps the next paragraph names. The cost is that such a
project's context, beyond what its rounds captured, cannot be browsed from the
surface.

**The moved outer `.git` has no tell on the filesystem, so the store keeps
one.** After `mv .git` on an outer checkout, what was left above the marker
project was its working files, `.gitignore` among them, and check-ignore exited
128, confirmed. A `.gitignore` is no tell: a directory can hold one with no
repository, and a repository can have none. So a marker project cannot tell
"never in a repository" from "its repository's `.git` was moved" from the disk
alone. The store gains a column, `projects.inside_checkout`, in a forward-only
migration, `0005_project_inside_checkout.sql`:
`INTEGER NOT NULL DEFAULT 0 CHECK (inside_checkout IN (0, 1))`, with a comment
saying there is no backfill, as 0002's does, since whether an existing project
sat inside a checkout was never recorded and cannot be recovered. SQLite
accepted that `ADD COLUMN` with its check, gave an existing row 0, refused a 2,
and an `UPDATE ... WHERE inside_checkout = 0` changed one row the first time and
none the second, all confirmed. `store.Project` gains the field, and every read
that fills one reads the column: `store.Project`, `store.ProjectByRoot`, and
`store.EnsureProject`, which returns a project from its own `SELECT`.

It is set, and never cleared, whenever a `.git` entry is found at or above a
root that holds a marker. At the root as well as above it, because a root
holding both a `.git` and a `.eyeball` on an unborn branch records no base, and
after its `.git` moves it looks like a marker project with nothing around it.
Two paths set it. An agent's request sets it when it registers or reuses the
project. `capture.Inspect` cannot, since `capture` holds no store handle: it
takes the stored flag as an input and returns whether it found a `.git` at or
above the marker root, and the handler writes that through a new
`db.MarkInsideCheckout` in `internal/server/writes.go`, which calls
`store.MarkInsideCheckout`. That is a write from a GET, the only one on the
surface, and it is idempotent: the update sets 1 only where the column is 0.
`db.MarkInsideCheckout` does not publish on the hub, as `db.EnsureProject` does
not, because it changes nothing a waiting agent reads. Publishing would wake
every waiting agent on a page view, and once the live stream (008) reloads a
page on a change, a GET that publishes could reload itself in a loop. A project
registered before the migration has the column unset until either path sees its
outer `.git`, so an outer `.git` moved away before eyeball first sees it after
this spec ships is not caught, and that project reads as having no repository
around it. A second, narrower gap is reasoned and not probed: a marker project
inside repository R, itself inside outer repository O. Moving R's `.git` leaves
O's `.git` above the marker, so check-ignore answers with O's rules and the walk
finds a repository around the project. R's `.gitignore` files are working files
and still apply, since git reads them wherever they sit in O's work tree, so
what is lost is only R's `.git/info/exclude`. Both gaps are named in the
exposure below. Keeping the case as accepted exposure was the alternative. The
owner chose the column over it on 2026-10-05, asked with the gap above stated.

"Never cleared" has a cost. If the outer repository is removed on purpose, the
marker project loses live browsing for good. The root is the project's identity,
so the project cannot be re-registered under a new row, and the only recovery is
clearing the column on its row, or the database. Clearing the flag whenever no
`.git` is found would undo it on exactly the request it exists for.

The cost per request is a few `stat` calls and one small read for the lost
check, one `cat-file -e` for a round with a base, and one `check-ignore` at the
root of a marker project with a repository around it. Each live read adds one
`check-ignore`, and live reads are the rarest read there is.

**A failed ignore check refuses only live reads.** When the root check or a
per-path check fails with an error rather than an answer, the request still
answers everything the round's capture and base hold, and every live read is the
404 page with a banner saying the ignore rules could not be read. That fails
closed for the one source the rules protect. Failing the whole page was the
round-two draft, and it made a captured file unreadable because of a question
that does not apply to it.

**A project with no repository around it serves its whole root to live reads.**
Any regular file under the root, outside a `.git` segment, reads live. That is a
project outside git that was never recorded inside a checkout. product.md asks
for this: without git, browsing falls back to the working copy as it is now. It
is new exposure all the same. Before 007 such a project was read only under the
paths a review named, by `capture.Walk`. Browsing reads the whole root, so a
credentials file in one is readable from the tailnet by its address, and the
tree lists it, since the tree lists that project from disk. The exposure is
reported to the owner at the end of the run, with two gaps beside it. A marker
project whose outer `.git` was moved away before eyeball ever recorded it inside
a checkout reads the same way, since nothing on the disk or in the store says a
repository was there. And a marker project inside a repository nested in another
loses only the inner repository's `.git/info/exclude` when the inner `.git` is
moved, as reasoned above.

**Every working-copy read and listing goes through `os.Root`.** The read 006
shipped is check-then-open: `linkedParent` and `os.Lstat` approve a path, then
`os.Open` opens it, and anything swapped in between is followed. Round one's fix
of `O_NOFOLLOW` and a `SameFile` check left gaps a review found: a parent
swapped for a link after the check still passes `SameFile`, `O_NOFOLLOW`'s
`ELOOP` is not `ErrNotExist` and would answer 500, a FIFO swapped in blocks the
open, and the listing has the same window. Go's `os.Root`, opened on the project
root with `os.OpenRoot`, resolves every path inside the root in the kernel and
refuses one that leaves it. Opening `outdir/hostname` through a link to `/etc`,
opening a link to `/etc/hostname`, opening `../x` and
`fs.ReadDir(root.FS(), "outdir")` each failed with "path escapes from parent",
confirmed.

**Five errors mean not found, wherever the read meets them.** `root.Lstat`
returned `ENOTDIR` for `go.mod/x`, `ENAMETOOLONG` for a 300-byte segment, and
`ELOOP` for a path through a link to itself, and none of them is `ErrNotExist`,
all confirmed. So a typed `/reviews/1/files/go.mod/x` would be a 500. In the
parent check, step 3, step 6 of the answer order and the listing, `ErrNotExist`,
`syscall.ENOTDIR`, `syscall.ENAMETOOLONG`, `syscall.ELOOP` and the escape all
map to not found. The escape has no exported sentinel in Go 1.26.8: it arrives
as an `*fs.PathError` wrapping an unexported error whose text is "path escapes
from parent", confirmed, so it is detected by that text, which is Go's own
string and not the operating system's or the locale's. The cost: a real read
failure still answers 500, which is right, since it is not a page that does not
exist. The read, in order:

1. `CleanPath`, then `root.Lstat` on each parent component, refusing a path
   through any symlinked directory, inside the root or out, and a path inside a
   directory holding a `.git` entry, which is another repository. That keeps
   006's rule, matches what the capture and the tree leave out, and runs before
   check-ignore, for the reason above.
2. check-ignore, under the project's rules.
3. `root.Lstat` on the path. A symlink reads as its target text through
   `root.Readlink`, which returns the target without following it, for a target
   outside the root as well, confirmed. That is the text git stores for a link
   and `freezeLink` captures, so a live link compares cleanly against HEAD. A
   regular file is opened with `root.OpenFile` and `O_NONBLOCK`, and its `Stat`
   must be a regular file and `os.SameFile` with the `Lstat`, or it is not
   found. A FIFO opened that way returned at once and its `Stat` was not a
   regular file, confirmed. Anything else is not found.

`os.Root` follows a symlink that stays inside the root: `inlink` and
`indir/a.txt` both opened, confirmed. It also ignored `O_NOFOLLOW` on a link,
confirmed. So step 1 is what keeps the no-symlinked-directory rule, and
`os.Root` is what keeps a swap after step 1 from leaving the root. What remains
is a parent swapped after step 1 for a link to another directory inside the
root, which reads a file there that check-ignore did not see, possibly an
ignored one. The swap would have to come from a local process that can already
read the project, and the risk is recorded.

`capture.ReadLive` and 006's `Reader.Open` share this read, since `Open`'s
fallback is the same read. `capture.Walk` and `Freeze` stay on `linkedParent`:
they read when an agent asks for review in its own working copy and write to the
blob store, not to a page. Moving them is a `tooling:` entry in specs/TODO.md.

**A base commit that is gone falls back to the working copy, as product.md
says.** After a rebase and a garbage collection, `Reader.Open` returned git's
`not a tree object` failure rather than `ErrNotFound`, confirmed, so the
fallback never ran and the page would be a 500. `capture.BaseGone` asks
`git cat-file -e <base>`, which exits 0 for an object that exists, 1 for one
that does not, and 128 for anything else, confirmed. A gone base builds the file
route's reader with no base, so a file the capture does not hold reads live, and
its banner says why. Exit 128 is an error and is reported as one.

**Under a gone base nothing is compared.** `capture.Compare` reads the base side
of every path a capture holds and an earlier one lacks. Given the gone commit it
fails, and given an empty base in its place it would count every captured file
as wholly added, which is a wrong number shown as a right one. So on a file page
or a tree under a gone base there are no counts and no Marked or Plain control,
the file shows plain, and the toolbar says the commit the review started from is
gone, so the change cannot be measured. The tests in iterations 5 and 6 assert
that.

The review page 006 built still fails on a lost base, through the same
`Compare`. Fixing that changes how a round is measured, is out of scope here,
and goes in specs/TODO.md.

### Addressing a file

**The `{path...}` wildcard carries the path, and an empty one is the root.**
`/reviews/1/files/` matches with an empty path, and `/reviews/1/files` is
redirected there by the mux with a 307, both confirmed. Directory links the
surface writes end in a slash, so a directory's address reads as one.

**A path is answered in this order, and whether it is a file or a directory is
decided before anything lists recursively:**

0. `capture.Inspect` settles the project's state, as above.
1. The empty path is the tree's root, answered before any step that reads it.
   `CleanPath` refuses it, since `path.Clean("")` is `.`, and `Reader.Frozen`
   would ask `ls-tree -r ... -- ''`, which exits 128 with "empty string is not a
   valid pathspec", both confirmed. Without this step the root is a 404 or
   a 500.
2. `capture.CleanPath` refuses a climb out of the root, a `.git` segment or a
   NUL, and the refusal is the 404 page. The mux cleans a literal `..` before
   matching and redirects `/reviews/1/files/a/../../x` to `/reviews/1/x`,
   confirmed. Correction: an earlier draft said that answers with a 307. Through
   the surface's own handler it answers with the 404 page, confirmed, because
   006's wrapper finds no pattern for the request, serves it through the writer
   that catches the fallback's status, and writes the 404 page for any status
   but 405. An encoded climb is not cleaned:
   `/reviews/1/files/..%2F..%2Fetc%2Fpasswd` and `%2e%2e/%2e%2e/etc/passwd` both
   reach the handler with the path `../../etc/passwd`, confirmed. So the check
   belongs to the handler and not to the mux.
3. A path the round records as deleted is a deleted file (below).
4. A path the round's capture holds is a file, and a path the capture holds
   files under is a directory. Both are answered from memory.
5. With a base, one `git --literal-pathspecs ls-tree -z <base> -- <path>` with
   no `-r` names the entry at the path and its type, as it named
   `040000 tree ... internal`, confirmed. The flag matters here as in
   `loadOIDs`: without it `ls-tree -- :weird.txt` printed nothing and exited 0
   for a file the base holds, and with it the file was listed, both confirmed,
   so a colon-named base file would have fallen through to a live read. A blob
   is a file, read through `Reader.Frozen`, whose recursive lookup on a file's
   path names that one file. A tree is a directory. Asking `Frozen` first was
   the earlier draft, and its `ls-tree -r` on a directory's path lists
   everything under it: two entries for `internal` in the scratch repository,
   confirmed, and 157 in this one, as round one measured.
6. Where live reads are allowed, check-ignore runs on the path first, and an
   ignored directory is the 404 page, as an ignored file is: check-ignore exited
   0 for the ignored directory `build`, with or without a trailing slash, and 1
   for `internal`, confirmed. Then `root.Lstat`, after the parent check, says
   whether the path is a directory on disk. That decides only that it is a
   directory. Which source lists it is `Reader.List`'s choice, below.
7. Anything else is read live, under the rules above, and a path nothing has is
   the 404 page.

Cleaning that stays inside the root is accepted, so `a/../b.go` reads `b.go`. No
link the surface writes is spelled that way, and refusing it would add a rule
that protects nothing.

**A file the capture does not hold is read at the base through the reader 004
built.** `Reader.Frozen` asks `ls-tree` for the path's object id, with
`--literal-pathspecs` so a leading colon is not read as pathspec magic, and
streams the object with `cat-file blob`, killing git if the page stops reading
early. Nothing about that changes here. What changes is the reader the handler
builds from `capture.Inspect`'s answer: with no base when the base is gone or
the repository is lost, and with live reads refused when the root is missing or
the repository is lost. The same answer chooses the tree's source and the live
banner's reason.

### Three sources, three banners

`capture.Reader.Open` already returns the source a file came from. The page says
which, in the tearout's words where the tearout has them. A file opened for
context looks exactly like a file under review, and the step-three frame's own
note says that confusing the two either approves something unread or comments on
something unchanged.

**A file the review changes carries no banner.** That is a path the round's
capture holds with `InScope` set, the flag 003 records on each round file. It is
the same set the tree shows in bold, so the two cannot disagree.

| Source  | Banner                                                                                                                            |
| ------- | --------------------------------------------------------------------------------------------------------------------------------- |
| capture | `notice`: **Not part of this review.** Changed by the agent outside the review's paths, shown as it was when round N was taken.   |
| base    | `notice`: **Not part of this review.** Unchanged since the commit round 1 was taken from.                                         |
| live    | `notice warn`: **Live, not from this round.** Nothing captured this file, so it is shown as it is on disk now and may have moved. |

A base file inside the review's paths that the review did not change says
**Unchanged by this review** in place of "Not part of this review", because it
is part of it. A live banner in a project outside git, or after the base was
lost, names that as the reason in place of "Nothing captured this file". A
missing root, a lost repository, a marker project the outer checkout ignores,
and a marker project whose repository's `.git` has gone allow no live read, so
their banner says only the captured files can be shown and names which of the
four is why. The base banner names round 1 always, because a review's base is
fixed at its first round.

The tearout writes the capture banner for none of its frames, so its text is
this spec's. The capture is exact and frozen, so it informs rather than warns,
as the tearout's note on the three sources says.

A deleted file takes the capture's banner by scope. One the review deletes
carries no banner. One deleted outside the review's paths says **Not part of
this review.** and that the agent deleted it as of round N. Either page shows
the deletion as the next section describes.

### Widening to the whole file

**The whole file is one hunk.** `diff.Text` groups lines into hunks with three
lines of context. `diff.Whole` runs the same comparison with the context set to
the two sides' line counts added together, which gives a single hunk holding
every line of both sides, and no hunks at all for two identical files, both
confirmed. The bound matters. Correction: an earlier draft said the probe set
the context to the largest int. It set `1<<30`. Round one found that the largest
int, and the largest int less one, both panic on a changed pair, because `hunks`
computes `i + context + 1` and the sum overflows, and that re-ran here,
confirmed. The two line counts added together are never less than the distance
to either end, and cannot overflow for a file the line limit lets through.
Reusing the comparison is what keeps the marks in the whole file identical to
the marks in the hunk the reviewer widened from. Re-deriving them some other way
would be a second diff path, which architecture.md rejects.

**`render.Whole` turns that hunk into rows with no hunk header, and
`render.Plain` turns a file's content into rows with no comparison.** Plain is
needed because an identical pair yields no hunk at all, and an unchanged context
file has nothing to compare. Both highlight each line by the file's lexer, as
006's rows do, so the five role classes are the only ones they add.

**Marked and Plain are two links, `?view=marked` and `?view=plain`**, in the
step-two frame's segmented control. Marked shows the removed lines in place, so
the change reads as it did in the hunk. Plain shows the file as the agent left
it, with no tint and no removed lines. The control appears only for a file that
moved in the comparison. A file the review changes that did not move in the
comparison has nothing to mark, and its toolbar says so in place of the counts.
That happens only against the previous round, on round two or later, where it
reads "Not changed since round N". Against the base it cannot happen, since a
captured file in the review's paths differs from the base by construction. Any
other file that did not move has no toolbar, as the step-three frame has none.

**The comparison is the review page's, and every link carries it.** The file
page takes the same `?against=previous` or `?against=base` as 006's review page,
with the same default, and its counts say what they count: "in this round"
against the previous round or on a first round, "in this review" against the
base, as the step-two frame's toolbar does. Marked and Plain, "Whole file", "Raw
diff", Document and Source, and the back link to the round all keep the
`against` the page was opened with, so a reviewer reading the whole review does
not drop back to the latest change by flipping a view. So do the tree's rows,
its back link, the review page's "All files" row, and an HTML block's link to
Source. The tree takes `against` only to carry it: its own counts are always the
whole review's, for the reason the tree section gives.

**A deleted file shows what was removed.** Marked shows every old line as
removed. Plain says the file was deleted as of this round, in an `.empty` box.
Answering with the 404 page was the alternative, and it would turn the review
page's link to a deleted file into a dead end. A file deleted in an earlier
round and still deleted did not move against the previous round, so under that
comparison it shows the `.empty` box with no control, and under the base it
shows its removed lines.

**A file past `diff.MaxBytes` or the diff's line limit is not shown whole.** The
page shows its header and the line 006 prints for a too-large file. The side is
read through a reader that stops one byte past the limit, as `capture.readSide`
does, so a large file is never held in memory. A binary file gets 006's binary
line. Showing either is rendered files (010). Whole-file rendering up to the
limit is twenty thousand highlighted rows at worst, which is assumed acceptable
and not measured.

**Step one's control moves to the review page's file card.** The step-one frame
draws a page holding one file's hunks, "1 of 6 files", with "Whole file" in its
toolbar. No frame shows how a reviewer reaches that page, and the review page
already shows the same hunks inside each file card. So each card gains the
"Whole file" link instead, and the one-file page is not built. Where in the card
the link sits, and the `app.css` rule that places it, are settled on the phone
in iteration 1, which also redraws the step-one frame to match. Iteration 5
copies that markup.

**The step-one notice that links `backoff` to its definition is out of scope.**
Finding where an identifier is defined needs a symbol index for each language.
The tree is how 007 reaches any file. The notice stays in the tearout as the
target, and specs/TODO.md gains the work.

**The topbar's back link goes somewhere the address alone decides.** A file the
review changes goes back to its round, labeled "Review" or "Round N". Any other
file goes back to its directory in the tree, labeled with the directory's name,
or "Files" at the root. The server cannot know where a reader came from: the
`Referer` header is unreliable, and an address has to render the same however it
was reached, since product.md says every view has one.

Three departures from the widening frames follow from that, and iteration 10
records them in specs/TODO.md as `design:` entries for the owner. The step-two
frame labels its back link "Change", where this spec writes "Review". The
step-three frame labels its back link with the file it was opened from, where
this spec names the directory. The browse frame, showing `internal`, goes back
to "Review", where this spec goes to the parent directory, which for `internal`
is the root, labeled "Files".

### The tree

**One directory level per page.** A directory's entries come from three places,
merged by name:

- the base, through `git --literal-pathspecs ls-tree -z <base> -- <dir>/`, which
  lists that directory's entries one level deep with their full paths, or
  through `ls-tree -z <base>` with no path for the root. With the slash it lists
  the entries, and without it the directory itself. A directory the base lacks
  lists nothing and exits 0. All confirmed.
- the round's capture: each captured path under the directory contributes its
  next segment, as a file or as a directory.
- the capture's deletions. A file the review deletes stays listed, bold, with
  its removed count, and opens on its removed lines, since it is part of what
  the verdict is about. A file deleted outside the review's paths is dropped, as
  it is gone as of the round. A directory whose every base entry the round
  deleted outside the review's paths still lists, because knowing it is empty
  would cost an `ls-tree` per subdirectory on every page, and it opens on the
  `.empty` box saying nothing is there as of round N.

Blobs and trees are listed. A gitlink is not, because its contents belong to
another repository, which is how `Reader.readTree` already treats one. A symlink
the base holds is listed as a file, because the reader serves its target text,
as the capture stores it.

**`Reader.List` chooses the source from `capture.Inspect`'s answer:**

- a git project with a base that exists lists the base and the capture, as above
- a git project with no base, a gone base, a lost repository or a missing root
  lists the capture alone. An unborn branch captured every file against the
  empty tree, so its capture is complete. For the others, listing the disk would
  list every ignored file, or there is no disk, so the tree lists only what the
  agent changed and says so. Under a gone base any other file still opens live
  by its address. Under a lost repository or a missing root nothing does.
- a project outside git with no repository around it lists the disk and the
  capture
- a marker project with a repository around it that does not ignore it lists the
  disk and the capture, filtered by check-ignore
- a marker project whose live reads are refused, under the owner's decision
  above, lists the capture alone, since it records no base

**A listing from disk goes through `os.Root`**, `fs.ReadDir(root.FS(), dir)`,
after the parent check and a `root.Lstat` that refuses a directory whose own
last component is a symlink. It skips `.git`, anything that is neither a regular
file, a directory nor a symlink, and a directory holding a `.git` entry, which
is another repository and which the live read refuses. A symlink is listed as a
file, as the base listing lists one, and opens on its target text, which is what
the live read serves for it. Its notice warns that the listing is live. Without
the checks, `os.ReadDir` through `link` pointing at `/etc` listed `/etc`,
confirmed, and through `os.Root` the same listing fails as escaping, confirmed.

In a marker project with per-path ignore rules, the listed entries go through
one `git check-ignore --stdin -z` call under the pinned locale, each written
with the `./` prefix, and the ignored ones are dropped. Given `./:other.txt` and
`./private.txt` that way, it named both and left two unignored paths out,
confirmed. The batch stops at the first path in a submodule: given
`./private.txt` then `./mod/m.go`, it printed `./private.txt` and exited 128,
confirmed. Skipping directories that hold a `.git` entry keeps a submodule's
checkout out of the batch, and a 128 that still arises is an error, not a reason
to show the entries unchecked.

**Listing one level is what bounds the tree for a large repository.** Nothing
lists recursively. A page costs at most four `ls-tree` calls: one for its
directory, one for all the subdirectories it expands together, step 5's type
lookup when only the base knows the path is a directory, and the batched object
id lookup `ComparePaths` makes for the counts. Each listing call carries
`--literal-pathspecs`, as the type lookup does, since without it a directory
named `:cdir/` listed nothing and exited 0, and with it its file was listed,
both confirmed. Sibling directory pathspecs in one call each list one level,
`docs/` and `internal/` together listed `internal/upload` as a tree entry,
confirmed. A parent given beside its own child loses the parent's tree entries,
`internal/` with `internal/upload/` listed `internal/doc.go` and
`internal/upload/worker.go` and no `internal/upload` entry, confirmed. The
expanded subdirectories are siblings, so they share a call, and the parent takes
its own. One directory can still be large, so a listing stops at a thousand
entries, with a notice counting the rest. A thousand is a guess written down so
it can be corrected.

**Directories first, then files, each in byte order of their names**, as the
browse frame orders them. A subdirectory holding a file the review changes is
expanded one level in place, its entries indented under it with
`treerow indent`, as the frame expands `upload`. Deeper subdirectories inside it
are rows, not expanded, because `app.css` has one indent and the frame shows
one.

**Bold is a file the review changes, and its counts come from the whole
review.** `treerow inscope` marks a path the round's capture holds with
`InScope` set, which is the banner's rule above. The counts come from the
comparison 006's `?against=base` page uses, against the base, and only for the
bold paths the page shows. `capture.Compare` diffs every path that moved, which
on a tree page would diff the whole capture to label a handful of rows, so its
per-path loop is exported as `capture.ComparePaths`, which takes the paths and
loads their object ids in one batch as `Compare` does. `Compare` keeps its shape
and calls it. The whole review is what the tree's notice names: "Files this
review changes are in bold". The latest change is the review page's question,
and a tree counting it would show nothing for a file the review changed in round
1 and left alone since.

**The crumb is the project's name and the directory's path**, in the frame's
`crumb` markup. Going up is the topbar's back link, which goes to the parent
directory, or to the round from the root. No crumb segment is a link, because
the frame draws them as text.

**The review page reaches the tree through one row.** No review frame shows a
way in. Iteration 1 adds a one-row `.tree` reading "All files at round N", in
the markup 006 already uses for earlier rounds, which needs no new class.

### Rendering markdown

**goldmark, configured once in `internal/render`**, with GFM's table,
strikethrough, autolink and task-list extensions. Specs in this repository are
tables and checklists, so a renderer without them would render this project's
own reviews badly. Markdown is recognized by the extensions `.md` and
`.markdown`, by `render.IsMarkdown`. `internal/kind` is rendered files' (010),
and it takes this over.

goldmark v1.8.6 declares `go 1.22`, and the module graph already names v1.4.13
through x/tools' go.mod without linking it. Adding v1.8.6 directly raised that,
and the repository's copy still built and passed its render, diff and server
tests, confirmed.

**Raw HTML stays off.** A document is something an agent wrote and is untrusted.
goldmark's default replaces raw HTML with `<!-- raw HTML omitted -->`, for a
`<script>` block and an inline `<img onerror>` alike, confirmed. The renderer is
never given `html.WithUnsafe`, and a test parses the output of an attack corpus
and finds no `script` element and no `on` attribute.

**Links go through an allowlist, not goldmark's denylist.** goldmark's
`IsDangerousURL` is a case-insensitive prefix check against `javascript:`,
`vbscript:`, `file:` and `data:`, with an exception that lets `data:image/png`,
`gif`, `jpeg` and `webp` through, read from its source. It blanks those to
`href=""`, confirmed, which is a link to the page itself: a tap that appears to
do nothing, the failure product.md calls the worst this surface has. A scheme it
does not list passes through. So links, autolinks and images are rendered by
overrides in `internal/render`, which parse the destination with `net/url` and
accept:

- `http`, `https` and `mailto`, written as given
- a fragment alone
- a relative reference or a root-relative path, resolved into the project
  (below)

Anything else renders as its text, with no `a` element at all. That includes a
destination `net/url` cannot parse and a scheme-relative `//host/x`, which
`net/url` reads as a host with no scheme, confirmed. goldmark turns
`java&#x09;script:` into `java%09script:`, and `net/url` refuses that with
"first path segment in URL cannot contain colon", so it renders as text, both
confirmed. `net/url` lowercases a scheme, so `HTTPS:` is accepted as `https`,
confirmed. A relative link to a file whose name starts with a colon,
`:weird.md`, is refused too, with "missing protocol scheme", confirmed, so it
renders as text unless written `./:weird.md`.

**A relative link resolves to the file page of the same round.** Written as
given, `architecture.md` in `docs/product.md` resolves correctly on
`/reviews/1/files/docs/product.md`, because the route's suffix is the project
path, and wrongly on the review page `/reviews/1`, where the browser makes it
`/reviews/architecture.md`. So the renderer resolves a relative reference
against the document's own directory with `path.Join`, treats a root-relative
one as relative to the project root, runs the result through
`capture.CleanPath`, and writes the file route's address for it through a
function the caller passes, which escapes each segment and writes the
round-scoped form on an earlier round. A path `CleanPath` refuses renders as
text. A link to a path the round does not have still goes to a route that
exists. That route answers it under the order above, so it reads live when the
disk has the path, with the live banner, and is the 404 page when the disk does
not.

**Heading ids are written only on a page holding one document.** goldmark's
automatic ids make two headings named "Title" into `title` and `title-1`,
confirmed, so two documents on one review page could repeat an id. The file page
renders one document and turns them on. The review page leaves them off and
points a fragment link at the document's file page with the fragment kept.

**An image renders as its alt text**, linked to its file page when the source
resolves to a project path, and as plain text otherwise. A relative `src`
written as given asks the file route for an image and gets an HTML page back,
and an external one is blocked by `img-src 'self' data:`. Showing images is
rendered files (010).

**Table alignment is written as an attribute.** goldmark's default writes
`style="text-align:center"` on each cell, confirmed. The surface's policy has no
`style-src`, so `default-src 'self'` governs styles, and Chromium ignored an
inline style attribute under it while honoring `align="right"`, both confirmed.
The table extension's `TableCellAlignAttribute` writes `align`, confirmed.

**A fenced code block is highlighted by its info string** into the five role
classes, through the lexer chroma names for it. goldmark's own output carries a
`language-go` class, confirmed, which `app.css` does not define. An indented
code block, or a fence naming no lexer chroma knows, is plain.

**A task list item renders its checkbox disabled**, as goldmark writes it,
confirmed. The box shows state and cannot be ticked, which is right for a
surface that does not edit.

### Marking what changed in a document

**A document is compared block by block, by rendering.** Each top-level node of
the parsed document is rendered alone, which goldmark supports through
`Renderer().Render` on any node, confirmed. Each block gets a key, and the two
sides' keys go through `diff.Text`, which says which blocks were kept, added and
removed.

**The key comes from a parse and a rendering of its own, never the one shown.**
The key's goldmark instance parses with heading ids off, aligns table cells by
attribute, writes every link and image destination as the source wrote it, and
renders raw HTML, which is safe because the key is hashed and never sent
anywhere. It needs its own parse, not only its own renderer: heading ids are
attributes the parser writes on the node, and a renderer without the option
still printed `id="title"` over an id-writing parse, confirmed. Rendering one
parsed node twice also repeats the default table alignment,
`style="text-align:center;text-align:center"` on the second pass, confirmed,
which a separate parse rules out. A key taken from the rendering shown would
mark blocks that did not change. goldmark numbers a repeated heading's id,
`title` then `title-1`, confirmed, so a heading added above an existing one of
the same name would change the existing one's id and mark it. A link resolved
into the round changes address between the review page and an earlier round's
page, so every block holding a link would mark. And raw HTML shown is
`<!-- raw HTML omitted -->` whatever it held, so `<div>a</div>` and
`<div>b</div>` rendered identically, confirmed, and a change to either would be
invisible.

**Each key is a hash on a line of its own.** The key rendering, with whitespace
collapsed outside `<pre>`, is hashed with SHA-256 and written as its hex digest
on one line, ending in a newline, the last one included. Without the final
newline an appended block changes the last line's ending, and `diff.Text` marks
the block before it: `a\nb` against `a\nb\nc` removed and re-added `b`, where
`a\nb\n` against `a\nb\nc\n` added only `c`, confirmed. A key holding a fenced
block's newlines would span several lines of `diff.Text`'s input, and a change
on one of them would map to the wrong block. The prototype hashed already, which
is why it did not show this. A side with no blocks writes no lines at all, so it
is empty input rather than a lone newline, which `diff.Text` would read as one
empty line removed.

The rendering, not the source, is compared because a reflowed paragraph is the
same paragraph. goldmark writes a soft line break as a newline, confirmed, so
two wrappings of one paragraph render differently until whitespace is collapsed.
With the collapse a reflowed paragraph is unmarked, confirmed in a prototype.
Whitespace inside `<pre>` is kept, because an indentation change in a code block
is a change.

**An HTML block shows a placeholder.** Raw HTML stays off in the rendering
shown, so a block of it would show nothing. It renders as a `.blk` reading that
the block is HTML and is not rendered, linking to the file's Source view with
the page's `against`. An HTML block has no text nodes, so its replaced text is
its old source lines, escaped and with whitespace collapsed, which is what a
changed or removed HTML block opens to. Its source is `Lines()` plus
`ClosureLine` when `HasClosure()` is true. For HTML block types 1 to 5 goldmark
keeps the closing line out of `Lines()`: a comment block's `Lines()` held `<!--`
and `note` and its `ClosureLine` held `-->`, confirmed. A `<div>` block, type 6,
has no closure, and reading `ClosureLine.Value` on it panicked, confirmed, which
is why the guard is there. Raw HTML inline in a paragraph is dropped from the
paragraph shown, and the key still sees it, so a change to it marks the
paragraph.

**The pairing rule is the word-marking rule.** A run of removed blocks followed
by an equal number of added blocks pairs one to one, and each added block
carries the text of the block it replaced. An unequal run is a rewrite, and
pairing across it would claim to know which passage replaced which. So every
block of an unequal run is marked, and the whole run's replaced text goes on its
last new block, read after the new passage. A prototype showed the cost: a run
where a paragraph was rewritten, a list item added and a paragraph removed put
all three old blocks under the new list.

**A removed run with nothing in its place gets a block of its own**, holding
only its replaced text: `<div class="blk changed was">`, the class `app.css`
already gives the amber rule, which iteration 1 keeps beside the new `[open]`
selector. There is nothing to collapse it to, and a removal no rule marks would
be invisible in the rendered view.

**Nothing is marked when the old side has no blocks.** That covers an empty
base, a file new against a real base, and a file new since the previous round. A
prototype without the rule marked every block of a new document, confirmed, so
it is a rule and not a consequence. product.md states it for an empty base, and
a new file is the same reading: marking every passage marks none.

**The replaced text is the old block's text, without markup**, in a `was-text`
span per old block. It comes from the block's text nodes, a code block's lines,
a table's cells, and a task item's box written as `[ ]` or `[x]`. A prototype
that read text nodes alone returned an empty string for a code block, confirmed,
and a box ticked in a checklist would otherwise read as no change at all.
Whitespace is collapsed, so a replaced code block loses its line breaks. That is
the cost of a `span`, and `was-text` is a span in the tearout.

**Blocks are top-level, which is coarse for a list.** Adding one item to a list
of ten marks the list and shows the whole old list as replaced. Marking list
items one at a time needs markup the tearouts do not have, a `.blk` on an `li`,
so it is a `design:` entry in specs/TODO.md. A top-level list is what the
tearout wraps in one `.blk`.

### One tap away, with no JavaScript

**A changed block with replaced text is a `<details>` element.** The block's
rendering is its `summary` and the replaced text is its body:
`<details class="blk changed"><summary>…</summary><span class="was-text">…</span></details>`.
Closed, it shows the teal rule. Open, it shows the replaced text and the amber
rule. foundations.html describes these as two states of one control, which is
what `<details>` is. A block with nothing replaced stays
`<div class="blk changed">`, and a kept block `<div class="blk">`.

In Chromium at 390 pixels, a closed block hid its replaced text, and an open one
showed it with the amber rule from `.blk.changed[open]`, confirmed. The HTML
parser keeps a `<p>` or `<ul>` inside `summary` where it was written, confirmed
with `golang.org/x/net/html`. The content model is not conforming, because
`summary` takes phrasing content and a heading, and a screen reader announces
the passage as a button. Those are the costs. A link inside a summary is
followed rather than toggling the block, which is the HTML specification's
activation rule, confirmed by round one's probe and not re-run here.

The rules it needs live in `app.css`: a block's `summary` displayed as a block
with no marker, and `.blk.changed[open]` taking the amber rule that
`.blk.changed.was` has today. The surface's policy blocks inline styles, so they
cannot go in a template. `app.css` is the owner's, so iteration 1 is the change.

**Two other designs were rejected.** A server-rendered toggle, a `btn small`
"Show what changed" in the toolbar reloading the page with every replaced text
open, needs no stylesheet change. It reveals a whole page at a time rather than
the passage tapped, and each tap is a round trip that loses the scroll position
unless it carries a fragment. Showing every replaced text open, as the tearout's
static frames do, needs nothing at all, and turns a heavily edited document into
a column of struck-through text, which product.md's rule against a wash through
the passage exists to prevent.

**A block that opens carries a cue that it opens.** A closed block with replaced
text showed the same teal rule as an insertion, so nothing told a reviewer which
passages held replaced text without tapping each one. The owner decided on
2026-10-05, after looking at foundations.html's prose specimen, that it needs a
visible cue. Three candidates went in front of the owner on the phone: a
disclosure chevron like the file cards', a quiet text hint, and a dashed rule.
The owner chose the text hint. A closed block that opens ends its summary with
"Show what it replaced" in the small faint hint style, which reads "Hide what it
replaced" when the block is open, two spans switched by
`details.blk.changed[open]` with no script. `render.Document` writes both spans,
`<span class="hint cue-show">` and `<span class="hint cue-hide">`, as the last
children of every `details.blk.changed` summary it writes, since it writes the
whole element and no template reaches inside it. A `div.blk.changed` with
nothing replaced carries neither. The cost is a line of height per passage that
opens. The reasons the other two lost are this spec's analysis, not the owner's
stated ones: the chevron took a strip down the right of every such passage on a
narrow screen, and the dashed rule was a pattern a reader had to learn before it
told them anything.

### Documents on the page

**The review page renders a markdown file as a document**, in its file card as
the round2 frame does: `details.file > .filebody > .prose`. The comparison is
the one the page already chose, so "Since round 1" marks what moved since round
1 and "Whole review" marks what moved since the base. Both sides are read
through the readers that comparison built, stopping one byte past
`diff.MaxBytes`. A binary or too-large markdown file falls back to 006's lines.

**One new document on its own renders on the page**, as the fresh frame shows:
when a round shows exactly one file, it is markdown, and its old side has no
blocks, the prose sits directly in `.page`, which `app.css` already pulls back
by the gutter's width. The toolbar then reads "1 file, new" and carries the
frame's "Raw diff" button. Otherwise every document sits in its card.

**The switch is a link.** Each document card, and the fresh layout's toolbar,
links "Raw diff" to the file page with `?view=source`. The file page for a
markdown file opens on the document with heading ids, and its segmented control
reads "Document" and "Source", as `?view=document` and `?view=source`. Source is
the whole file with the change marked, the step-two view, since the exact edit
is the question when a reviewer flips. A markdown file has no Plain view. The
marking in a document is a rule beside a passage rather than a wash through it,
which is product.md's reason a marked document stays readable, so there is no
tint to switch off.

**A code file's card links "Whole file"** to its file page, carrying the page's
`?against=`.

### The project view

**`GET /projects/{project}` follows the project frame**: the topbar back to the
queue, the brief with the root written with `~` in the `agent` span, then Open
reviews, then the working copy. The project's open reviews come from
`store.ReviewsIn`, filtered to the open state, waiting before working and each
in the queue's order. They use the queue's row markup with the project bit left
out, since the page is the project. A project with no open review has no Open
reviews section, as the outside-git frame shows. The working copy's heading
counts its files, as "Working copy 5 files" in the frame.

The frame's working row reads "Not requested yet", a state the store does not
have: every review opens with its first request. The row reads "Changes
requested, round N", as the queue's working rows do, and iteration 1 corrects
the frame.

**The working copy reuses capture and stores nothing.** `capture.WorkingCopy` is
read by `capture.ReadWorkingCopy`, which:

- asks `capture.Inspect` first, because `ResolveBase` returns an empty base both
  for a project outside git and for an unborn branch, and the two need different
  answers, and because a missing root or a lost repository cannot be listed at
  all. The project view's GET writes `Inspect`'s finding about a `.git` at or
  above a marker root through `db.MarkInsideCheckout`, as the file routes do
- resolves HEAD with `ResolveBase(root, "")`
- lists what differs with `capture.Changed`, which 004 exported for this,
  tracked and untracked, ignored files excluded

It never calls `Freeze`, so nothing reaches the blob store, and a test holds the
blob directory empty after a read. `WorkingCopy.Compare` diffs one path: HEAD's
side through a `capture.Reader` with no capture, whose `Frozen` reads the base,
and the disk's side through `capture.ReadLive`, each stopping one byte past
`diff.MaxBytes`. That is `capture.Compare`'s shape with the disk in place of a
capture.

`capture.ReadLive` is the working-copy read alone, exported. `Reader.Open`
cannot be the disk side, because for a modified tracked file it returns HEAD's
copy, which is the base, before it ever looks at the disk. `ReadLive` keeps the
symlinked-directory and ignore checks, and reads a symlink as its target text,
as `freezeLink` captures one. `Changed` lists a symlink with `Symlink` set, and
the working-copy read refuses a link as not a regular file, both confirmed, so
without that a changed link would show as a file with every line removed.

**A path that vanishes mid-listing is dropped, not fatal.** `Changed` returns
`ErrVanished` when a path git listed is gone by the time it is examined, which
is right for a frozen round, where recording a deletion the agent did not make
would be wrong. A live page shows a working copy an agent may be editing that
second, and failing the page for it would make the view fail when there is
something to see. The listing passes a flag through `keep` that drops a vanished
entry.

**A row claimed by an open review is marked and carries no counts**, as the
frame's note says: its numbers belong to the round that captured it. Claimed
means `store.Covers` over the paths of any open review in the project. The row
reads `pill plain` "In review" and links to that review's file page at its
current round.

**An unclaimed row is bold and carries its counts**, and links to the project
file page. Its counts come from `WorkingCopy.Compare`, the computation the
project file page shows, so the row and the page agree. A test holds them equal.

**The listing stops at two hundred rows**, with a notice counting the rest.
Before any row is compared, the HEAD reader loads the object ids of every
unclaimed row shown in one batched `ls-tree`, as `capture.Compare` does, since
left to itself `Frozen` runs an `ls-tree` per path. Each row then costs one
`git cat-file` process for its HEAD side. 196 sequential `cat-file blob` calls
in this repository took 259 ms, where one `cat-file --batch` over the same
objects took 5 ms, both measured. Two hundred rows is then a quarter of a second
for the HEAD sides, plus the reads from disk and the diffs, which are not
measured. Batching the reads is the better mechanism and is a `tooling:` entry
in specs/TODO.md. Two hundred is a guess.

**Three states with nothing to list.** A clean tree shows the frame's `.empty`
box, "Nothing changed on disk", with the frame's sentence when a review is open
and "Everything is committed." when none is. A project outside git shows the
frame's warning and "Cannot be listed". The frame's warning says there is no
marker file either, which cannot be true of a project outside git, since a
marker file is the only way one is found, so iteration 1 corrects it. A root
that no longer exists, checked first, shows "Cannot be listed" with that as the
reason. A lost repository shows "Cannot be listed" too, saying the repository's
`.git` is gone, rather than the outside-git warning, since what it lost is git
and not a marker. A marker project whose live reads are refused, under the
owner's decision above, shows "Cannot be listed" with a notice saying live
browsing is unavailable and why: the outer checkout ignores it, or the outer
repository's `.git` is gone.

**`GET /projects/{project}/files/{path...}` answers only a path the working copy
lists**, including one past the two-hundred-row cap, since the cap limits the
rows read for counts and not the listing `Changed` returns. It is the step-two
view, live: the file on disk against HEAD, Marked and Plain, counts reading "on
disk", and a `notice warn` saying it is live, in no round, and moving while an
agent works. Its back link goes to the project. A path the listing does not hold
is the 404 page. That keeps the route from becoming a reader of any file under
the root, and an unchanged file has nothing this view adds anyway.

**A markdown file on the project file page gets Marked and Plain, not Document
and Source.** The page answers what moved on disk, which is a question about the
edit, and the review page's reason for opening a document rendered is that it is
being read as prose for a verdict, which nothing here asks for. Rendering it
would also need the block comparison against HEAD for a file that may change
between two requests. The reviewer who wants it rendered reads it from a review.

**The project view is reachable from the review page's brief and from the empty
queue.** The brief's project becomes a link to its view, and the empty state's
project rows become links, as the queue frame already draws them. product.md
says the view is reachable from the queue, and a populated queue gives no way
there: its rows are each one link to their review, so a project link inside one
would nest links, the problem 006 found with a filter. That gap is a departure
from product.md and a `design:` entry in specs/TODO.md. Making the brief's
project a link is a change to the review frame, so it is in iteration 1.

### Links go only to routes that exist

006 set the rule. Each link this spec adds lands in the iteration that adds its
route, or later:

| Link                                        | Route            | Iteration |
| ------------------------------------------- | ---------------- | --------- |
| a code card's "Whole file"                  | file route       | 5         |
| a file page's back link to its round        | round page       | 5         |
| the review page's "All files" row           | file route, tree | 6         |
| a tree row, and a context file's back link  | file route, tree | 6         |
| a document card's "Raw diff"                | file route       | 8         |
| a relative link or image inside a document  | file route       | 8         |
| an HTML block's link to Source              | file route       | 8         |
| the brief's project, the empty queue's rows | project view     | 9         |
| a working-copy row                          | either file page | 9         |

Iteration 5's context files go back to their round, because the tree that would
be their directory arrives in iteration 6. Iteration 7 renders documents in
`internal/render` only, with tests calling it directly, so no page shows a
document link before iteration 8 gives it a route to resolve against.

**Iteration 10 checks the rule rather than trusting it.** A test renders every
seeded page, collects every `href`, and asks the surface's mux for a pattern for
each one starting with `/`. Every such link must match a pattern and answer 200,
and a link to a file must land on a page whose crumb names that file, so an
address cut short by an unescaped `#` or `?` fails even when what it lands on
answers 200. The seed holds a file with `#` in its name for that. A document
link to a path its round lacks answers whatever that path answers, so the seed's
documents link only to files their rounds hold, and a unit test in iteration 8
covers the dangling case. A fragment-only link must name an id on its page, and
an absolute link must be `http`, `https` or `mailto` and sit inside `.prose`.

### Seeing it

**`tool/seed` grows to cover what this spec shows**: a document reviewed over
two rounds with a passage replaced, a passage added, a passage removed and a
reflowed paragraph, a new document on an empty base, a project outside git, a
file captured outside the review's paths, an unclaimed change written to a
seeded repository after its capture, and a project whose tree is clean.

**The stand-in's visual check is headless Chromium at 390 pixels**, as 006's
last iteration did: one screenshot per page beside a crop of the tearout frame
it was seeded from, saved where the iteration's reviewer can read them.

### Probes

Each claim marked confirmed above came from one of these. They are described so
a review pass can run them again.

- **The mux.** A program registering this spec's patterns beside 006's on one
  `http.ServeMux` and sending it fourteen paths through `httptest`. It showed
  the empty wildcard, the 307 from `/files` to `/files/`, the 307 from a literal
  `..` and from a doubled slash, and an encoded `%2F` or `%2e%2e` arriving at
  the handler as `../../etc/passwd`.
- **The live read.** A test in a copy of this repository calling `Reader.Open`
  on a scratch repository holding an ignored `private.txt`, an ignored
  `build/out` and an untracked `vendor/x/.git/config`. All three came back from
  the working copy, and `.git/config` was refused. The same test with a base id
  that was never written got git's `not a tree object` failure.
- **check-ignore and cat-file.** Shell runs in that repository:
  `check-ignore -q` on an ignored and a tracked path, on an untracked ignored
  `:other.txt` given plainly, through `--stdin -z`, under `--literal-pathspecs`
  and as `./:other.txt`, and `cat-file -e` on a written id, an id never written,
  a short id and a directory that is no repository.
- **Nested repositories and links.** In that repository, `check-ignore -q` on a
  file inside a real `git init` nested repository, and a test calling `Changed`
  and `Reader.Open` with an untracked symlink present.
- **ls-tree.** The same repository, committed with plumbing: `ls-tree -z` at the
  root, at `internal/` with and without the slash, through the `<commit>:dir`
  form, and at a directory the commit lacks.
- **goldmark.** A program converting nineteen inputs under GFM with automatic
  heading ids: raw HTML, links under each dangerous scheme and spelling,
  autolinks, a reference definition, images, a task list, tables with each
  alignment under both alignment methods, fenced and indented code, repeated
  headings and relative links. It also rendered each top-level node of a
  document alone.
- **The block comparison.** A prototype in a copy of this repository rendering
  each top-level block, keying it with whitespace collapsed outside `pre`, and
  diffing the keys with `internal/diff`. It was run on a reflowed paragraph, a
  replaced paragraph, an added list item, a removed paragraph, a re-indented
  code block and an empty old side.
- **net/url.** A program parsing eleven link destinations, including
  `java%09script:alert(1)`, `vscode://x`, `//evil.example/x`, `HTTPS://` and
  `:weird.md`.
- **The whole-file hunk.** A test in the copy calling `diff`'s unexported `text`
  on a changed pair and an identical one. Correction: the first run set the
  context to `1<<30`, not the largest int as an earlier draft said. A re-run
  with the largest int and the largest int less one panicked on the changed
  pair, and the two sides' line counts added together gave one hunk.
- **Round one's re-runs.** For the review's findings: check-ignore in a marker
  project inside the scratch repository, `rev-parse --is-inside-work-tree`
  inside and outside a repository, check-ignore on a path in a submodule added
  to it, `ls-tree -r` with an empty path and on a directory's path, `ls-tree`
  given two directory pathspecs, `safePath("")` and `Reader.Open` on a path
  holding a NUL, a literal `..` through `Server.Surface`, goldmark on two raw
  HTML blocks and on repeated headings, `net/url` on unescaped and escaped
  names, and `os.ReadDir` through a symlink to `/etc`.
- **Round two's re-runs.** check-ignore on a file and on `./` in a marker
  project inside an ignored directory, and on `./` in `notes`; `ls-tree` on
  `:weird.txt` with and without `--literal-pathspecs`; check-ignore on a
  submodule path under `LANGUAGE=de` and under `LC_ALL=C LANGUAGE=`, through
  `--stdin` with a submodule path second, and on a path beyond a symlink;
  `rev-parse` under `GIT_CEILING_DIRECTORIES` and with `.git` moved away;
  `ls-tree` with sibling pathspecs and with a parent and its child; goldmark
  rendering an id-writing parse with an id-less renderer, twice; `diff.Text` on
  keys with and without a final newline; and a program opening, listing, reading
  links and opening a FIFO through `os.OpenRoot` on a scratch project holding
  links inside and outside it.
- **Chromium.** Headless Chromium 152 at 390 pixels, screenshotting candidate
  `<details>` markup against `app.css` with the two proposed rules, and a page
  under the surface's policy with one cell aligned by a style attribute and one
  by `align`. `golang.org/x/net/html` parsed the same `<details>` markup.
- **cat-file cost.** 196 sequential `git cat-file blob` calls over this
  repository's tracked files, timed, against one `cat-file --batch` over the
  same ids.
- **The dependency.** goldmark v1.8.6 added to a copy of this repository, which
  then built and passed its render, diff and server tests.

### Checks

**Pages are tested as HTML, not as strings**, with `golang.org/x/net/html`, as
006 does. 006's route test substitutes `{review}`, `{round}` and `{$}`, and
grows `{path...}` and `{project}`, so the listener test covers every new route.

**Classes render chooses join `render.Classes`**, which a test holds to
`app.css`, as 006 set up. That covers a class a template prints through an
action, and every class in a document's HTML, which render writes and no
template check sees: `blk`, `changed`, `was`, `was-text`, `hint`, `cue-show`,
`cue-hide` and the five roles. Literal classes in a template are covered by the
`template-classes` check already. A test renders a document with every block
kind, parses its HTML, and holds every class in it to `render.Classes`, so a
class render starts writing without listing fails.

**Each iteration ends with a mutation pass**, per the dev workflow. The tests
named here are written to kill these mutations, and an iteration whose pass
leaves one alive has a test that cannot fail:

- dropping the `./` before a path given to check-ignore: the `:other.txt` row
- checking `.git` in the first segment only: the nested `.git/config` row
- reading exit 128 from `cat-file -e` as gone: a root that is not a repository
- dropping the empty-old rule: a new document with nothing marked
- dropping the whitespace collapse: the reflowed paragraph
- collapsing whitespace inside `<pre>` too: the re-indented code block
- pairing an unequal run: the rewrite with three old blocks and two new
- leaning on goldmark's denylist: a corpus row with a scheme it does not list,
  such as `vscode:`
- resolving a relative link against the page's address: a document on the review
  page linking `architecture.md`
- the project file route answering any path: an unchanged tracked file gets 404
- the tree's bold read from the comparison instead of the scope flag: a file
  captured outside the review's paths, which the comparison moves, is not bold
- letting a NUL through `CleanPath`: an address holding `%00` gets the 404 page
- reading every check-ignore exit 128 as an error: the submodule row
- skipping check-ignore for a project that is not a git project: iteration 3's
  marker-project row, where `creds.txt` is refused on the live read
- writing a key unhashed: a change on the middle line of a fenced block marks
  that block and no other
- taking the key from the rendering shown, or from the shown parse: a heading
  added above another of the same name leaves the existing heading unmarked,
  with the shown side rendered with ids on so the test can fail
- dropping the final newline from the key lines: an appended block leaves the
  block before it unmarked
- reading every check-ignore exit 128 as not found: a marker project whose check
  fails under `GIT_CEILING_DIRECTORIES` gets `Inspect`'s "rules unreadable"
  answer, and its live banner names that as the reason, where the mutant reports
  the path as merely missing
- treating a marker project as having no rules when its git call fails: the same
  row, which also asserts that a file the project's `.gitignore` names is
  refused on the live read, where the mutant serves it
- dropping `--literal-pathspecs` from the type lookup: a colon-named base file
  reads from the base, not live
- `diff.Whole` keeping three lines of context: a fixture with changes more than
  seven lines apart and more than three unchanged lines at each end
- dropping the parent check in `Reader.List`: a `link/sub/` row
- serving a lost repository live: a formerly ignored file gets 404
- setting `inside_checkout` only for a `.git` above the root: the unborn-branch
  row, with both entries at the root, serves `private.txt`
- the agent request path dropping its write: the request-path rows, first sight
  and reuse, find the store's row unflagged
- the file route dropping its write through `db.MarkInsideCheckout`: the surface
  row, whose project was opened with the flag at 0, finds it still 0
- the project view dropping its write: its row finds the flag still 0
- `db.MarkInsideCheckout` publishing on the hub: the hub-subscriber row receives
  a signal
- writing `inside_checkout` as found or not found instead of set-only: the row
  that runs `Inspect` with the `.git` present and then twice with it moved still
  refuses on the third run, since a flag written both ways would be 0 by then
- treating a `check-ignore -- ./` exit 0 as per-path rules: the ignored marker
  project's tree lists its captured files with the refusal notice, where
  per-path rules would drop every disk entry and list an empty directory
- deciding "lost" from the round's base: an unborn branch whose `.git` moved
  away still refuses `private.txt`
- mapping only the escape and `ErrNotExist` to not found: `go.mod/x`, a 300-byte
  segment and a link loop each get 404
- dropping `--literal-pathspecs` from the tree's listing: a `:cdir/` directory
  lists its file
- reading `ClosureLine` without `HasClosure`: a changed `<div>` block opens
  without a panic
- skipping check-ignore on a directory: an ignored directory gets 404
- failing the whole page on a failed ignore check: a captured file still opens
- dropping the empty path's step: the tree's root answers 200
- escaping nothing in an address: the seeded file with `#` in its name

**Departures, iteration 2.**

- The todo on migration `0005` says no test can catch `store.EnsureProject`
  skipping the column, since a new row always reads 0. That was wrong. A row
  marked and then registered again reads 1 through `EnsureProject`'s own
  `SELECT`, and `TestStore_MarkInsideCheckout` asserts it, so a mutant that
  skips the column fails.
- `capture.Inspect` returns a `capture.State`: one `capture.Live` value of
  seven, two that allow a live read and five that refuse one, plus `BaseGone`,
  `FoundCheckout` and the error behind "rules unreadable", which is for a log.
  `State.Reader` builds the reader from it, with no base for a gone base, a lost
  repository or a missing root, and with live reads refused for every refusing
  value. The design gave the answers and left their shape open, and iteration
  2's capture-level rows need a reader that refuses. `NewReader` still allows a
  live read, so 006's tests and `Freeze` are unchanged, and a reader for a page
  is meant to come from `State.Reader`.
- A `.git` file that holds no `gitdir:` line, and a `.git` symlink whose target
  is gone, read as a lost repository. Against git 2.55.0, `rev-parse` refused
  the first with "invalid gitfile format", and walked past the second to answer
  for the outer repository, both confirmed. A root that is a file, or that lies
  below one, reads as a missing root. The design named only a pointer whose
  target is gone, and a missing root found by `os.Stat`.
- `capture.BaseGone` passes `--end-of-options` before the base. Against git
  2.55.0, `cat-file -e --end-of-options` still exited 1 for an id never written
  and 128 for `-p`, confirmed, so the answer is unchanged and a base can never
  be read as an option.
- The agent request path writes the flag from `capture.CheckoutAround`, the same
  filesystem walk `Inspect` uses, rather than running all of `Inspect`. The
  request needs only the sighting, and the base check and the root's
  `check-ignore` would add two git calls to every agent command, each poll of a
  wait included. Sharing the walk keeps the two writers from disagreeing about
  what was found. A project already flagged is not walked again, since the flag
  is never cleared.
- The walk counts an entry it lacks permission to read as absent and carries on
  upward, as `FindRoot` does, so the two agree on which directories they search.
  That is reasoned and not tested, since a test would need a directory its own
  user cannot search.
- A `.git` in a home directory, such as a dotfiles repository, sits above every
  marker project under that home. Each one is flagged on first sight, and its
  live reads follow that repository's rules, so a dotfiles repository that
  ignores everything refuses them all as ignored. If that `.git` is later moved
  away, every one of them loses live browsing for good, which is the
  never-cleared cost above on a larger scale. It fails closed, and the recovery
  is the same: clearing the column on each row, or the database.
- `store.Projects` does not read the column. `store.ProjectRow` embeds
  `store.Project`, so its `InsideCheckout` reads false there, and its comment
  says so. Nothing that lists projects shows the flag.
- The mutation row for a flag written as found or not found has no code to
  mutate in this iteration. The store has no statement that clears the flag, so
  no write path can write it both ways. The three-run row writes through the
  store between runs, and it kills the three mutants that can be written:
  `Inspect` ignoring the stored flag, the update writing 0, and `store.Project`
  not reading the column. Two mutants survive as equivalent, because the final
  state is the same either way: dropping `AND inside_checkout = 0` from the
  update, and walking a project that is already flagged.

## Out of scope

- **Comments on a line or a passage, and mapping a passage to source lines.**
  The write path (008). The block model is where that mapping will start.
- **A file's body loaded when its disclosure opens, htmx, and the live stream.**
  The write path (008).
- **Images, audio, video, PDF, the sandbox for HTML and SVG, `internal/kind`,
  the agent's per-file view override, and a file too large to send.** Rendered
  files (010). An image in a document renders as its alt text until then.
- **A link from an identifier in a hunk to its definition**, the step-one
  notice. A `design:` entry in specs/TODO.md.
- **The one-file page of the step-one frame.** Its control moves to the review
  page's file card.
- **Marking list items one at a time in a document.** A `design:` entry.
- **A lost base on the review page.** `capture.Compare` fails on it, and fixing
  that changes how a round is measured. A `product:` entry.
- **Batching the working copy's reads of HEAD.** A `tooling:` entry.
- **Moving `capture.Walk` and `Freeze` onto `os.Root`.** They read for an
  agent's own request and write to the blob store. A `tooling:` entry.
- **Push, the manifest, the installed page and the owner's phone check of the
  built surface.** Notification (009).

## Not converged

Round one ran at high effort over the whole draft. It found nine defects that
would fail at runtime or reverse a design decision, and sixteen smaller ones,
and it re-ran the draft's probes. Every finding's probe was re-run before its
fix, and each held. The fixes:

- the live read: ignore rules asked of git for a marker project inside a
  checkout, a path in a submodule read as not found, a NUL refused, the open
  made safe against a swapped symlink, and the root checked first
- the tree: the root answered before any lookup, file or directory decided
  without a recursive listing, a symlinked directory refused outside git,
  deleted files listed, and counts compared for the shown paths only
- `diff.Whole`'s context bounded by the two line counts, with the earlier probe
  claim corrected
- the block keys: a separate key rendering, hashed one to a line, with empty
  input for no blocks, and an HTML block keyed from its source
- addresses escaped segment by segment, `against` carried through every view
  link, and `Referrer-Policy: no-referrer` on every response
- the corrections to the 307 claim, the `:weird.md` message, goldmark's data
  image exception, and the summary link now confirmed
- three tearout departures and the populated queue's missing project link
  recorded, and the project file page's Marked and Plain for markdown named

Round two ran, scoped to those fixes. It found four defects that would fail at
runtime and eleven smaller ones, and every probe behind them held when re-run.
The fixes:

- `capture.Inspect` settling the project's state once per request: a missing
  root, a lost repository, a gone base, and the ignore rules
- a lost repository listing the capture alone and refusing live reads, where the
  round-one draft would have served its formerly ignored files
- a marker project's ignore rules decided by a `.git` entry above it and
  `check-ignore -- ./` at its root, so a project the outer checkout ignores is
  browsable and counted as exposure, and a failing git call is an error rather
  than a reason to skip the check. Correction: the owner's answer of 2026-10-05
  reversed the first half. A project the outer checkout ignores gets no live
  reads
- every live read and listing through `os.Root`, in place of `linkedParent`,
  `O_NOFOLLOW` and a bare `SameFile`, with links read as their target text and
  FIFOs refused
- the submodule message read under a pinned locale, and the parent check placed
  before check-ignore
- `--literal-pathspecs` on the type lookup, step 0 for the project's state, and
  step 6 limited to deciding that a path is a directory
- the key built from its own parse, its lines all ending in a newline, and an
  HTML block's replaced text defined
- the `ls-tree` cost corrected to two calls a page, `against` carried through
  the tree, and the "not changed" line limited to the previous-round comparison
- tests that can fail: the `TestWhole` fixture, a `link/sub/` row, the
  repeated-heading test with ids on, and a check of every class in a rendered
  document
- iteration 2 split in two, since it had grown to sixteen todos, which moved
  every later iteration up by one

Round three ran, scoped to round two's fixes, and it is the last round the stop
rule allows. It found two defects that would fail at runtime and eight smaller
ones, and checked the renumbering against every cross-reference and table with
no defect. Every probe behind its findings held when re-run. **The fixes below
were applied after the last review round and have not been reviewed by anyone.**
The stand-in approves the spec with them unreviewed:

- `capture.Inspect` deciding "lost" first, from the filesystem, so a root
  holding neither `.git` nor a marker is lost whatever its base, including an
  unborn branch, and a worktree whose git directory is gone is lost before any
  git call
- the marker project whose outer `.git` moved away, named as accepted exposure.
  Correction: the owner's answer of 2026-10-05 reversed this. It is refused,
  detected through `projects.inside_checkout`, with only the gap for a `.git`
  moved before eyeball first sees it left as exposure
- five errors from `os.Root`, the escape detected by its text, mapped to not
  found in the parent check, the read and the listing
- the wording that a moved `.git` lets git answer for the wrong repository
- an HTML block's source as `Lines()` plus a guarded `ClosureLine`
- the tree's `ls-tree` count corrected to at most four, and
  `--literal-pathspecs` on its listing calls
- the live read refusing a path inside a nested repository, and the tree listing
  symlinks, so the two agree
- check-ignore on a directory before it is listed
- a failed ignore check refusing only live reads

After round three, the stand-in's approval review escalated one question to the
owner: whether to accept the exposure of a marker project the outer checkout
ignores, and of one whose outer `.git` was moved away. The owner answered on
2026-10-05, verbatim: "Refuse live reads (Recommended): Fail closed, as the spec
already does for a lost repository. Captured and base files still open. The
project view says live browsing is unavailable and why. Nothing git would ignore
can leak." Three fixes followed. **They were applied after the last review round
and have not been reviewed:**

- live reads refused for both marker cases, with the second detected through a
  new `projects.inside_checkout` column set at first sight, since no filesystem
  tell exists, and the gap for a `.git` moved before eyeball first sees it named
- iteration 2's failed-ignore-check todo asserting `ErrNotFound` at the capture
  level, with the 404 page left to iteration 4
- a round with a base whose root holds a marker and no `.git` read as a lost
  repository

The schema change is the largest of them, and it is new work for the store that
no earlier spec planned.

The stand-in's second approval review then blocked on carrying out the owner's
answer: a root holding both a `.git` and a marker on an unborn branch still
served live reads after its `.git` moved, and neither refused marker case had a
listing test anywhere. Those two and six smaller fixes were applied unreviewed,
apart from the stand-in's third look:

- the flag set for a `.git` at or above a root holding a marker, with the
  unborn-branch row, and the claim that the lost check covered that case
  corrected
- the listing assertions moved to iteration 3's `Reader.List` todo, with a row
  for each refused marker case, and both added to `TestServer_Surface_tree`
- a flag test that can fail, and mutation rows for a flag written both ways and
  for an ignored root read as per-path rules
- the seam for the flag, `Inspect` returning what it found and the server
  writing it, the GET write reconciled with "Reading only", and the walk from
  the resolved root
- the column spelled out, its migration and its readers named, and the cost of
  never clearing it
- the two round lists corrected, the banner reasons extended, and the source
  bullet for an unignored marker project with a repository around it restored

The stand-in's third look then blocked on tests that could pass with their fix
removed: the unborn-branch row could seed the flag, neither write path had a
row, and three mutations had nothing to kill them. The design was accepted. The
fixes, applied after that look and not reviewed again: the unborn-branch row
setting the flag by running `Inspect` with the `.git` present, a server-level
row for the agent request path in iteration 2 and for the file route's GET in
iteration 5, mutation rows for the flag set only above the root and for each
write path dropping its write, `db.MarkInsideCheckout` not publishing on the
hub, `store.EnsureProject` reading the column, the "nothing can leak" sentence
pointed at its two known gaps, the nested-repository gap named, and iteration 2
split again, into the project's state and the live read.

Iteration numbers in the round records above are as they stood when each was
written. The two splits moved every iteration from 3 on up by one each time, so
the design and the todos use the current numbers and these records do not.

The stand-in's fourth look found the renumbering consistent and the owner's
decisions carried out, and blocked on five rows that could not fail or could not
be met: the surface row now opens its review through the store, the request-path
row covers a reused project, the marker-project live-read row moved to iteration
3, the `GIT_CEILING_DIRECTORIES` row names `Inspect`'s answer and the banner's
reason, and the hub row subscribes to the hub. The project view's write and its
row were added with them. These fixes were applied without another review. Test
rows that cannot fail are left to each iteration's mutation run, which is the
check built for them.

What remains riskiest, with no review round left to catch it: the unreviewed
fixes above, above all the error mapping, which has to be applied identically in
four places, and the lost check, which every route now reads first;
`capture.Inspect`'s answers agreeing with the banners, the tree's sources and
the live read; the `os.Root` read, whose in-root link following, `SameFile`
check and remaining parent-swap window rest on two probe programs and on
reasoning; and the agreement between what the tree lists and what an address
serves, which round three found broken once. Mutation testing in each iteration
and the stand-in's ack are where a defect left here is caught.

The shape every earlier round found is two things that must agree with nothing
checking that they do. Here the likeliest pairs are:

- the tree's bold, the banner's "a file the review changes" and the back link's
  choice, which all read the scope flag and must not drift to the comparison
- a project row's counts and the project file page's counts
- the review page's document marks and the file page's Source marks, from one
  comparison through two renderings, now with a third rendering for keys
- the link plan's iterations and the todos that write each link
- `CleanPath` in the handlers, in the reader and in the document link resolver
- the tree's sources, chosen by `Inspect`, and the answer order for a path
- the live read's ignore rules and the tree's listing from disk, which must
  refuse the same entries
- `capture.Inspect`'s answer and every place that reads it: the reader, the
  tree, the banners and the project view

Decided here rather than by product.md or architecture.md, and reported to the
owner at the end of the run: the project file route and its limit to listed
paths, refusing ignored files and nested `.git` directories on the live read,
the lost-base fallback, one route for files and directories, the tree listing
one level with a thousand-entry cap, bold and counts from the whole review, step
one's control moved to the file card, back links decided by the address,
`<details>` for the replaced text, block-level marking with the word-marking
pairing rule, nothing marked for any document with no old side, the fresh layout
for a lone new document, Document and Source with no Plain for markdown, the
link allowlist and its resolution into the round, heading ids on the file page
only, images as alt text, the two-hundred-row working copy cap, the two
corrections to the project frame, the new exposure of a whole project root
outside any repository, Marked and Plain for markdown on the project file page,
`Referrer-Policy: no-referrer`, the three back-link departures, a lost
repository refusing live reads, `os.Root` for every live read, links read as
their target text, files in a nested repository refused on the live read, a
failed ignore check refusing only live reads, `projects.inside_checkout` to
detect a moved outer `.git`, and the remaining gap for one moved before eyeball
first sees it. The owner decided one more on 2026-10-05: a marker project the
outer checkout ignores, or whose outer `.git` has gone, gets no live reads.

## Iteration 1: The tearouts for widening and documents

```backlog
required_acks:
  - reviewed
  - phone-approved
```

- [x] foundations.html's prose specimen showing a changed passage closed and
      open as `<details class="blk changed">`, a removed passage, a fenced code
      block, a table and a task list
- [x] app.css rules for a block's `summary`, `.blk.changed[open]`, `pre` and
      `table` owning their overflow inside `.prose`, a file card's link row, and
      the brief's project as a link
- [x] review.html's round2 frame using `<details>` for its replaced passage,
      each file card carrying "Whole file" or "Raw diff", an "All files" row,
      and the project as a link
- [x] widening.html's step-one frame showing "Whole file" on the review page's
      file card, with the definition notice kept as the target
- [x] project.html's working row reading "Changes requested, round 2" and its
      no-git warning no longer saying there is no marker file
- [x] A question to the owner, with the closed-passage frame, about whether a
      block that opens needs a cue beside the teal rule, answered in this spec
- [x] The cue the owner chooses from three candidates on the phone, applied to
      foundations.html's prose specimen and review.html's round2 frames, with
      the other two candidates removed

> **Completed** 2026-10-05 21:47 UTC
>
> - acks: reviewed, phone-approved
> - `make check` — 3.7s
> - `go test -race ./...` — 24.2s

## Iteration 2: The project's state

- [x] `capture.Inspect` answering in order, root, lost, base, rules, with lost
      decided from the filesystem and short-circuiting the rest
- [x] Lost-repository tests: a missing root, a moved `.git` with a base, an
      unborn branch with its `.git` moved, and a worktree whose git directory is
      gone
- [x] A round with a base whose root holds a marker and no `.git` read as a lost
      repository with no git call, with a row
- [x] `capture.BaseGone` reading `cat-file -e` exit 1 as gone and 128 as an
      error, tested against an id never written and a root that is no repository
- [x] A marker project's rules from a `.git` entry above its root and
      `check-ignore -- ./` at the root, with rows asserting what `Inspect`
      returns: per-path rules for `notes`, and "rules unreadable" under
      `GIT_CEILING_DIRECTORIES`
- [x] A marker project in a directory the outer checkout ignores refusing every
      live read with `ErrNotFound`, with a row
- [x] Migration `0005` adding `projects.inside_checkout` with its check and no
      backfill, `store.Project`, `store.ProjectByRoot` and `store.EnsureProject`
      reading it, and `store.MarkInsideCheckout` setting it only where it is 0.
      No test can catch `EnsureProject` skipping the column, since a new row
      always reads 0, and that is harmless: the write after it is idempotent
- [x] `Inspect` taking the stored flag and reporting a `.git` at or above a
      marker root from the resolved root
- [x] A row running `Inspect` with the outer `.git` present and writing what it
      reports, then moving the `.git` and running it twice more, refusing both
      times, with no flag seeded
- [x] A row with `.git` and `.eyeball` at the root on an unborn branch, no flag
      seeded: `Inspect` run with the `.git` present and its finding written,
      then the `.git` moved and `private.txt` refused
- [x] The agent request path writing the flag through `db.MarkInsideCheckout`,
      on first sight and when `Server.project` reuses a known row, with a
      server-level row for each: a new project, and an existing unflagged row,
      each flagged after a request
- [x] `db.MarkInsideCheckout` in `internal/server/writes.go`, not publishing on
      the hub, with a row subscribing to the hub, as `Test_hubSignalsAndForgets`
      does, and receiving nothing after the write
- [x] A lost repository and a missing root refusing every live read, with a row
      for a formerly ignored file answering not found
- [x] A failed ignore check refusing only live reads, with a test at the capture
      level that a captured file still reads and a live read returns
      `ErrNotFound`

## Iteration 3: The live read

- [ ] `capture.CleanPath` exported from `safePath`, refusing a `.git` segment at
      any depth and a NUL, with rows reading `vendor/x/.git/config` and `a%00b`
      refused
- [ ] The live read refusing an ignored path through
      `git check-ignore -q -- ./<path>`, with rows for `private.txt`,
      `build/out` and an untracked ignored `:other.txt`
- [ ] A marker-project row: `notes/creds.txt`, named by the outer checkout's
      ignore rules, refused on the live read, and under
      `GIT_CEILING_DIRECTORIES` refused with the "rules unreadable" reason
- [ ] A submodule's exit 128 read as not found under `LC_ALL=C` and `LANGUAGE=`,
      with a row run under `LANGUAGE=de`, and any other 128 an error
- [ ] The live read through `os.Root`: the parent check, links read by
      `Readlink`, regular files opened `O_NONBLOCK` and checked by `SameFile`,
      and an escape mapped to not found
- [ ] `os.Root` rows: a link to `/etc/hostname`, a path through a link to
      `/etc`, a link inside the root read as its target text, and a FIFO
- [ ] Not found for `ENOTDIR`, `ENAMETOOLONG`, `ELOOP` and the escape by its
      text, with rows for `go.mod/x`, a 300-byte segment and a link loop
- [ ] A path inside a directory holding `.git` refused by the parent check, with
      a `vendor/y/bar.go` row
- [ ] `Reader.Open`'s fallback moved onto the same read, with 006's tests still
      passing
- [ ] `capture/doc.go` recording the project's states, the live read's refusals
      and the move to `os.Root`

## Iteration 4: What the routes read

- [ ] `Reader.Deleted` reporting a path the round records as deleted, with a
      test that a deleted path the working copy has again is still deleted
- [ ] `capture.ReadLive` exporting the live read, with a test that a changed
      link is read as its target text and not as removed
- [ ] `Reader.List` choosing its source from `Inspect`: base and capture, the
      capture alone, the disk and the capture, or the disk filtered by
      check-ignore, with a row for each
- [ ] `Reader.List` listing the capture alone for a marker project the outer
      checkout ignores and for one whose repository's `.git` has gone, with a
      row for each
- [ ] A listing from disk through `os.Root`, refusing a symlinked directory,
      with `link/` and `link/sub/` rows, skipping a directory holding `.git`,
      and listing a symlink as a file
- [ ] A marker project's listing filtered by one `check-ignore --stdin -z` call
      with `./` paths, and its exit 128 an error
- [ ] `capture.ComparePaths` exported from `Compare`'s per-path loop, loading
      the object ids in one batch, with `Compare` calling it unchanged
- [ ] `diff.Whole` with its context set to the two line counts added together,
      and `TestWhole` holding one hunk of every line for a changed pair and none
      for an identical one
- [ ] `TestWhole`'s changed fixture with changes more than seven lines apart and
      more than three unchanged lines at each end, so three lines of context
      fails it

## Iteration 5: A file at a round

- [ ] Both file routes in the route table on the surface side, and the listener
      test substituting `{path...}`
- [ ] The answer order: `Inspect` first, the empty path to the tree, and file or
      directory decided before `Frozen`
- [ ] The type lookup as `git --literal-pathspecs ls-tree` with no `-r`, with a
      colon-named base file read from the base
- [ ] Project paths escaped by segment in every address, and
      `Referrer-Policy: no-referrer` on every surface response, each with a test
- [ ] `render.Whole` rows from `diff.Whole` with no hunk header, and
      `render.Plain` rows from content, both highlighted, with tests of each
- [ ] The `filepage` template: crumb, banner, toolbar counts saying what they
      count, Marked and Plain as `?view=` links keeping `against`, and the whole
      file, with runtime classes in `render.Classes`
- [ ] The banner chosen by source and scope, and by `Inspect`'s answer: the two
      info banners, the live warning's reasons, and the capture-only banner
- [ ] The file route writing `Inspect`'s finding through
      `db.MarkInsideCheckout`, with a server-level row whose project and review
      are opened through the store, as `tool/seed` does, so the request path
      never runs: the flag is 0 before the GET and 1 after it
- [ ] A refused path, an encoded climb and a path in no source answered with the
      404 page, tested through the surface handler with the escaped forms
- [ ] A live read refused by a failed ignore check, a disowned marker project or
      a moved outer `.git` answered with the 404 page and its banner, while a
      captured file opens
- [ ] A deleted file shown as its removed lines in Marked and as deleted in
      Plain, in scope, out of scope and unmoved, under each comparison, and
      binary and too-large files given 006's lines
- [ ] Each code card on the review page linking "Whole file", and every file
      page's back link going to its round, both keeping `against`, with a test
- [ ] `TestServer_Surface_fileBanners` covering each source on a current and an
      earlier round, a gone base and a lost repository with no counts and no
      Marked control

## Iteration 6: The tree

- [ ] A directory path, the empty path and a trailing slash answered with the
      tree, an ignored directory and a path that names nothing with the 404 page
- [ ] Every tree listing call carrying `--literal-pathspecs`, with a `:cdir/`
      row
- [ ] Directories then files by name, expanded subdirectories in one sibling
      `ls-tree` call with `indent`, and the listing capped at a thousand with a
      notice
- [ ] Bold from the round files' scope flag and counts from `ComparePaths`
      against the base for the shown paths, with tests that an out-of-scope
      capture is not bold and a deleted in-scope file is listed
- [ ] A directory left with only out-of-scope deletions listed, and opening on
      the `.empty` box
- [ ] The crumb, the back link to the parent or the round, and the notice saying
      whether the listing is frozen, from disk, or the capture alone
- [ ] The review page's "All files at round N" row, tree rows and the tree's
      back link keeping `against`, and a context file's back link going to its
      directory
- [ ] `TestServer_Surface_tree` covering one level, an earlier round's own
      capture, a project outside git, a gone base listing no counts, and a lost
      repository listing the capture alone
- [ ] `TestServer_Surface_tree` covering a marker project the outer checkout
      ignores and one whose repository's `.git` has gone, each listing the
      capture alone with its notice

## Iteration 7: Rendering a document

- [ ] goldmark v1.8.6 added, its row in architecture.md's table checked,
      `render.IsMarkdown` by extension, and `render.Document` with the GFM
      extensions, cell alignment as attributes, and raw HTML omitted
- [ ] Links, autolinks and images through the allowlist, anything else as text,
      tested against a corpus including `java&#x09;script:`, `vscode:` and a
      reference definition
- [ ] A relative or root-relative reference resolved against the document's path
      through a caller's function, and one `CleanPath` refuses rendered as text
- [ ] An image rendered as its alt text, linked when its source is a project
      path, and a fenced code block highlighted into role classes with no
      `language-` class
- [ ] Blocks keyed by a separate goldmark parse and rendering, ids off,
      destinations as written and raw HTML on, collapsed outside `pre`, hashed
      one to a line, every line ending in a newline
- [ ] Key tests: a reflowed paragraph unmarked, a re-indented code block and a
      fenced block's middle line marking only their block, and an appended block
      leaving the one before it unmarked
- [ ] A repeated heading added above another left unmarked, with the shown side
      rendered with heading ids on
- [ ] An HTML block shown as a placeholder linking to Source, marked when its
      source changes, and opening to its old source lines
- [ ] An HTML block's source read as `Lines()` plus a guarded `ClosureLine`,
      with a comment block row and a `<div>` row
- [ ] Equal runs paired, an unequal run's replaced text on its last new block, a
      removal as its own block, and nothing marked when the old side has no
      blocks
- [ ] Replaced text taken from text nodes, code lines, table cells and task
      boxes as `[ ]` or `[x]`, with a test for each
- [ ] `render.Document` writing the "Show what it replaced" and "Hide what it
      replaced" spans as the last children of every `details.blk.changed`
      summary, and neither in a `div.blk.changed`, with a test for each
- [ ] `blk`, `changed`, `was`, `was-text`, `hint`, `cue-show` and `cue-hide` in
      `render.Classes`, and a test holding every class in a rendered document to
      it
- [ ] Heading ids written only when the caller asks, and `render/doc.go`
      recording the allowlist, the key and the pairing rule

## Iteration 8: Documents on the page

- [ ] The review page rendering a markdown file in its card as `.prose`, each
      block with replaced text as `render.Document`'s `<details>` with its
      hints, and the card linking "Raw diff"
- [ ] A round showing one markdown file with no old blocks rendered on the page,
      with "1 file, new" and "Raw diff" in the toolbar
- [ ] The file page opening a markdown file on the document with heading ids,
      with "Document" and "Source" as `?view=` links
- [ ] "Raw diff", Document and Source, and an HTML block's link to Source each
      keeping the page's `against`, with a test
- [ ] Document links resolving into the round's escaped file routes,
      round-scoped on an earlier round, a link to a path the round lacks read
      live or answered 404, and review fragments pointing at the file page
- [ ] A binary or too-large markdown file falling back to 006's lines on both
      pages
- [ ] `TestServer_Surface_reviewDocument` parsing the page for kept, changed and
      removed blocks, and the attack corpus finding no script and no event
      attribute

## Iteration 9: The project view

- [ ] `capture.ReadWorkingCopy` built from `Inspect`, `ResolveBase` and
      `Changed` with vanished paths dropped, and a test that the blob store
      stays empty
- [ ] The project view writing `Inspect`'s finding through
      `db.MarkInsideCheckout`, with a row whose project is opened through the
      store, the flag 0 before the GET and 1 after it
- [ ] `WorkingCopy.Compare` reading HEAD through a reader with no capture,
      object ids loaded in one batch, and the disk through `capture.ReadLive`,
      with a modified tracked file and a changed symlink as rows
- [ ] `GET /projects/{project}` with the brief and the open reviews in the
      queue's row markup without the project, waiting before working
- [ ] Claimed rows marked "In review" and linking to their review's file page,
      unclaimed rows bold with counts, and the listing capped at two hundred
- [ ] The clean, outside-git, lost-repository and missing-root states, and a
      marker project whose live browsing is refused saying so and why, each with
      its own test
- [ ] `GET /projects/{project}/files/{path...}` answering only a listed path,
      markdown included as Marked and Plain, with tests that its counts equal
      the row's and an unchanged file gets 404
- [ ] The brief's project linking to its view, the empty queue's rows becoming
      links, and the route test substituting `{project}`
- [ ] architecture.md's routes table gaining the project file route

## Iteration 10: Links, seeding, and the look

- [ ] `Test_linksResolve` holding every root-relative `href` on every seeded
      page to a surface pattern, a 200, and a crumb naming the linked file
- [ ] The same test holding fragment-only links to an id on their page, and
      absolute links to `http`, `https` or `mailto` inside `.prose`
- [ ] `tool/seed` adding the documents, the project outside git, the
      out-of-scope capture, the unclaimed change, the clean project and a file
      with `#` in its name
- [ ] Screenshots at 390 pixels of a whole file, a context file under each
      banner, the tree, a document, the fresh layout and the project view,
      beside their tearout frames
- [ ] architecture.md's routes table marking these routes built, and both file
      routes answering "a file or a directory" rather than "a file"
- [ ] specs/TODO.md gaining this spec's deferrals and departures, the back-link
      and queue-link `design:` entries and the `os.Root` move for `Walk` among
      them, and losing item 1 under Next
