# eyeball

eyeball puts work in front of a person for review and holds the agent that asked
until the verdict arrives. One daemon serves every project on the machine. The
person reads and comments from a phone or a laptop. The agent learns nothing
until the verdict is given, and then receives the verdict and every comment at
once.

This manual is enough to drive eyeball without any other example.

## The loop

1. Write the work to disk.
2. `eyeball request <paths...> -m "what to look at"` opens a review and prints
   its id.
3. `eyeball wait <id>` blocks until the person decides. Stop all other work on
   the reviewed paths until it returns.
4. On `approve`, the work may continue.
5. On `changes`, make the changes. Optionally answer individual comments with
   `eyeball reply <comment> -m "what was done"`. Then ask for the next round
   with `eyeball request --review <id> -m "what changed"`, and wait again.

Run `wait` in the background, or with `--timeout`, when the harness limits how
long a foreground command may run. Running `wait` again is always safe: it
returns the latest round's verdict and every comment on that round each time,
until a new round is asked for. A comment from an earlier round is handed over
until it is acknowledged, which a plain or `--json` wait does once it has
printed it.

## The project and the agent

**The working copy** is the nearest directory above the working directory that
holds a `.git` entry or a `.eyeball` file. **The project** is the git repository
it belongs to, so a main checkout and its worktrees are one project, and a
review's paths are claimed only in the working copy it was asked for in. A
directory outside git becomes a project of its own when a person creates an
empty `.eyeball` file at its root. eyeball never writes into a project.

**The agent's name** is `--agent`, then the `EYEBALL_AGENT` environment
variable, then the working copy's path with the home directory written as `~`.
An agent acts only on reviews opened under its own name in its own project, so
keep the name the same across a restart. To ask for the next round from another
working copy of the project, such as the main checkout after working in a
worktree, pass the name the review was opened under.

A JSON answer's `project.root` is a working copy: the caller's in `status`, and
the one a review's latest round was captured from in an answer about a review.
When a command refuses with `wrong_agent`, its fix names the agent that opened
the review.

**Paths** are relative to the working directory, as a shell reads them. Absolute
paths work too. A path names a file, or a directory and everything under it. Two
open reviews in one project cannot claim the same file.

## Commands

### `eyeball request`

`eyeball request <paths...> -m <note>` opens a review of the paths.
`eyeball request --review <id> -m <note>` asks for the next round of a review
whose latest round has a verdict. The next round takes no paths and no base,
because a review's scope and base are fixed when it opens.

| Flag              | Meaning                                                      |
| ----------------- | ------------------------------------------------------------ |
| `-m`, `--message` | The note. Repeat it for more paragraphs. Required.           |
| `--title`         | The title the queue shows. Defaults to the note's first line |
| `--base`          | The commit a first round is measured from. Defaults to HEAD  |
| `--review`        | The review to ask the next round of                          |
| `--view`          | `<path>=rendered` or `<path>=source`. Repeatable             |

A file opens in its own default view: a markdown document rendered, an HTML or
SVG file as source, code with the change marked. `--view` picks the first view
for one file in this round, because only the agent knows which question it is
asking. A change to a page's styling wants `rendered`, and a change to its
markup wants `source`. The choice is not carried into the next round.

The note is the person's brief. On a first round, say what to look at. On a
later round, say what changed and what was done about each comment.

After a request, stop and run `eyeball wait <id>`.

### `eyeball wait`

`eyeball wait <id>` blocks until the review has something for the agent, prints
it, and acknowledges the comments it printed.

| Flag        | Meaning                                                |
| ----------- | ------------------------------------------------------ |
| `--timeout` | Give up after this long, such as `9m`, and exit with 2 |

It returns one of three kinds:

| Kind        | When                                                                    | Exit |
| ----------- | ----------------------------------------------------------------------- | ---- |
| `abandoned` | the review was withdrawn                                                | 0    |
| `verdict`   | the latest round has a verdict                                          | 0    |
| `comments`  | the latest round still waits, and comments arrived after an earlier one | 3    |

A `verdict` carries the verdict, the person's note, every comment on that round,
and any earlier comment not yet acknowledged. A `comments` answer means round N
still waits: read them, then run `wait` again. A fourth kind, `still_waiting`,
is what `--timeout` prints when it expires.

Under `--format`, `wait` acknowledges nothing, because it cannot know which
comments the template printed. The same comments are handed over again until a
plain or `--json` wait acknowledges them, and `status` keeps listing an ended
review until one has received its verdict.

### `eyeball status`

`eyeball status` prints the daemon and this agent's reviews in this project: the
open ones, and any ended review whose verdict or comments the agent has not yet
received. Run it first after a restart or a cleared context.

| Flag    | Meaning                                                     |
| ------- | ----------------------------------------------------------- |
| `--all` | List every review in the project, whichever agent opened it |

### `eyeball show`

`eyeball show <id>` prints any review's rounds, verdicts, decision times and the
latest round's captured files with their digests. It reads any project and any
agent, acknowledges nothing, and prints no comments. It is how a script checks
that a person approved a given piece of work.

### `eyeball reply`

`eyeball reply <comment> -m <text>` attaches the agent's answer to one comment
the agent has received. A second reply replaces the first.

| Flag              | Meaning                                            |
| ----------------- | -------------------------------------------------- |
| `-m`, `--message` | The reply. Repeat it for more paragraphs. Required |

### `eyeball abandon`

`eyeball abandon <id>` withdraws a review whose question no longer needs an
answer, whether or not a verdict is pending. A `wait` on it returns `abandoned`.

### `eyeball check`

`eyeball check --base <old> <new>`, run in a repository, exits 0 only when an
approved review in that project, opened by any agent, has a latest round
measured from `<old>` whose capture matches `<new>`'s tree file by file. A git
hook runs it before letting a default branch move from `<old>` to `<new>`. It
compares trees, so a fast-forward and a merge commit are treated alike.

| Flag     | Meaning                                    |
| -------- | ------------------------------------------ |
| `--base` | The commit the branch moves from. Required |

They match when every path `<new>` changes from `<old>` was captured inside the
review's paths with `<new>`'s bytes and mode, so an execute bit or a symlink
counts, or as a deletion where `<new>` lacks it, and every path captured inside
the review's paths is one `<new>` changes. A file captured outside the review's
paths that `<new>` leaves alone is ignored. So a stray file inside a claimed
path that `<new>` does not hold refuses the check: delete it or ignore it before
asking for review.

It prints one line naming the review, or refuses with one line naming the first
path that differs. It reads and records nothing, and acknowledges nothing. It
starts a daemon like any command; with `EYEBALL_NO_START` set, a stopped daemon
is a refusal. A review asked for in a worktree matches a merge made in the main
checkout, since both are one project.

A round captures working-tree bytes and the check reads committed ones, so a
repository whose files pass through a git filter, such as LFS or line-ending
conversion, or that sets `core.fileMode` to false, is refused even where nothing
changed.

### `eyeball cat`

`eyeball cat --base <old> <path>`, run in a repository, prints the bytes
`<path>` had in an approved review in that project, opened by any agent, whose
latest round is measured from `<old>`. A gate uses it to read what was approved,
so it can tell a merged file that differs only by a change it allows from one
that differs by anything else. eyeball's store is a file on this machine, which
a gate has no reason to open itself.

| Flag     | Meaning                                          |
| -------- | ------------------------------------------------ |
| `--base` | The commit the review is measured from. Required |

The path is read as every other path is, relative to the working directory. Only
a file captured inside the review's paths, with content, answers. A deletion
does not. Approved reviews that hold the same bytes are one answer. Two that
hold different bytes are refused as `ambiguous`, and none is refused as
`not_captured`.

The daemon answers in JSON, with the bytes as base64 and their SHA-256. The
command recomputes the SHA-256 and fails, printing nothing else, when it differs
from the one sent, so a damaged blob never reaches a gate as the approved
content. Plainly it prints the bytes as they are, with no added newline.
`--json` and `--format` print the answer instead. It reads and records nothing,
and acknowledges nothing. The daemon holds the whole file in memory to answer,
so it suits a document and not a build artifact.

### `eyeball start`, `eyeball stop` and `eyeball serve`

Any command starts the daemon when none is running, so these are rarely needed.
`eyeball start` starts it if needed and prints it. `eyeball stop` stops it and
waits until it has gone, and never starts one. `eyeball serve` runs it in the
foreground, and is the person's, for debugging: an agent does not run it.

`eyeball serve` with `--surface-addr` also serves the reviewer's surface on a
loopback address, such as `127.0.0.1:8111`. It is the person's flag, for
debugging. A loopback port can be reached by every account on the machine. It
prints an address to open, and only a page opened from that address can comment
or decide there.

**The surface** is the page the person reviews on. The daemon serves it on the
tailnet once the owner has set it up, which only the owner does. `status`
reports it under `daemon.surface`: `off`, `starting`, `serving` with its `url`,
or `failed` with a `reason` saying what the owner does. Nothing in that reason
is a command for an agent to run.

When a command warns that the daemon is a different build, run `eyeball stop`
once the person is not mid-review. The next command starts the new build.

### `eyeball tailnet login`

`eyeball tailnet login` is the owner's, and an agent never runs it. It logs this
machine in to the tailnet so the daemon can serve the surface there, and needs a
person at a terminal to open the login page it prints. Without a terminal it is
refused with `owner_only`. It needs the daemon's lock, so the owner runs
`eyeball stop && eyeball tailnet login` as one line: a waiting agent told the
daemon stopped runs `eyeball wait` again, and that starts a new daemon at once.
While the login runs, a command that needs the daemon is refused with
`daemon_unavailable` and says to try again once the login is done.

### `eyeball doc`

`eyeball doc` prints this manual. It needs no daemon.

## Output

Every command takes these flags, before or after its arguments:

| Flag       | Meaning                                                    |
| ---------- | ---------------------------------------------------------- |
| `--json`   | Print the answer as JSON, in the shape listed below        |
| `--format` | Print the answer through a Go template over that same JSON |
| `--agent`  | The name this agent goes by                                |

A `--format` template addresses fields by the JSON names below, as in
`{{.review.id}}`. Whole numbers compare with `eq` and `gt` against plain
numbers. A template that does not parse is refused before anything is sent.

Without either flag, output is for reading. `request`, `wait`, `status`, `reply`
and `abandon` end with a `next:` line saying what to do now, and their JSON
carries the same text in a `next` field. `start` and `stop` have none.

Flags parse in any position. Put `--` before a path that starts with a dash.

## Exit statuses

| Status | Meaning                                                            |
| ------ | ------------------------------------------------------------------ |
| `0`    | Done                                                               |
| `1`    | Refused or failed: the message says why and `fix:` says what to do |
| `2`    | `wait --timeout` expired with no verdict                           |
| `3`    | `wait` returned late comments while the round still waits          |

## Refusals

A refusal prints `eyeball: <message>` and `fix: <what to do>` on stderr. Under
`--json` it prints this on stdout instead, and still exits with 1:

```json
{ "error": { "code": "path_claimed", "message": "...", "fix": "..." } }
```

| Code                   | Meaning                                                                      |
| ---------------------- | ---------------------------------------------------------------------------- |
| `already_decided`      | the round already has a verdict                                              |
| `ambiguous`            | `cat` found approved reviews holding different bytes for the path            |
| `awaiting_verdict`     | a next round was asked for while the last one waits                          |
| `bad_base`             | `--base` does not name a commit                                              |
| `bad_commit`           | `check` was given a ref that names no commit                                 |
| `bad_request`          | the daemon could not read the request                                        |
| `bad_view`             | `--view` names a file the round lacks, or a view it has not                  |
| `base_gained`          | the repository gained its first commit after the review opened               |
| `base_moved`           | a round was measured from a base other than the review's                     |
| `connection_lost`      | the daemon closed the connection before answering                            |
| `daemon_running`       | a tailnet login found a daemon or another login running                      |
| `daemon_stopping`      | the daemon stopped while the command waited                                  |
| `daemon_unavailable`   | no daemon could be reached or started                                        |
| `empty_change_request` | a change request with no comment and no note                                 |
| `internal`             | a failure inside the daemon; its log says more                               |
| `invalid_path`         | a path that is empty or names the working copy's root                        |
| `login_failed`         | a tailnet login did not finish, or the tailnet cannot serve                  |
| `missing`              | a required value was not given                                               |
| `no_git`               | the project is a git repository and git is not on PATH                       |
| `no_project`           | no `.git` or `.eyeball` above the working directory                          |
| `no_such_path`         | outside git, a path the project does not have                                |
| `not_approved`         | `check` found no approved review in the project measured from `--base`       |
| `not_captured`         | `cat` found no approved review measured from `--base` that captured the path |
| `not_found`            | an id that names nothing                                                     |
| `not_open`             | the review was approved or abandoned                                         |
| `not_released`         | a reply to a comment the agent has not received                              |
| `not_top_level`        | the project root is below its repository's top level                         |
| `nothing_captured`     | nothing differs from the base                                                |
| `nothing_in_scope`     | nothing under the review's paths differs from the base                       |
| `outside_project`      | a path outside the project                                                   |
| `owner_only`           | a command only the owner runs, at a terminal                                 |
| `path_claimed`         | another open review in the project covers the path                           |
| `path_unmatched`       | a path that is neither on disk nor among the captured changes                |
| `round_closed`         | an edit to a comment whose round has a verdict                               |
| `too_many_files`       | outside git, the paths hold more than a thousand files                       |
| `tree_differs`         | `check` found approved reviews from `--base`, and none captured that tree    |
| `usage`                | the command line could not be read                                           |
| `vanished`             | a file disappeared while it was being captured; run it again                 |
| `wrong_agent`          | the review was opened under another agent name                               |
| `wrong_project`        | the review belongs to another project                                        |

## Environment

| Variable           | Meaning                                                                |
| ------------------ | ---------------------------------------------------------------------- |
| `EYEBALL_AGENT`    | The agent's name when `--agent` is not given                           |
| `EYEBALL_NO_START` | When set, a command that finds no daemon fails instead of starting one |
| `XDG_STATE_HOME`   | The parent of eyeball's state directory, `~/.local/state` by default   |

The state directory, `$XDG_STATE_HOME/eyeball`, holds the database, captured
file content, the socket, the lock and the daemon's log, `eyeball.log`.

## JSON fields

Each command's `--json` answer, by dotted path. A path through a list, such as
`comments.id`, names the field of each element.

### request

- `review.id`, the review's id, which every other command takes
- `review.title`, `review.agent`, `review.round`, `review.paths`
- `review.state`: `waiting`, `changes_requested`, `approved` or `abandoned`
- `review.project.name`, `review.project.root`
- `size.files`, `size.added`, `size.removed`: what this round moved since the
  last, counting only paths inside the review
- `next`

### wait

- `kind`: `verdict`, `abandoned`, `comments` or `still_waiting`
- `review.id`, `review.title`, `review.agent`, `review.state`, `review.round`,
  `review.paths`, `review.project.name`, `review.project.root`
- `verdict`: `approve` or `changes`, empty unless `kind` is `verdict`
- `note`: the person's note on the whole round
- `decided_at`: when the verdict was given, null without one
- `comments.id`, the id `eyeball reply` takes
- `comments.round`, the round the comment was written on
- `comments.level`: `line`, `passage`, `file` or `review`
- `comments.path`, empty for a comment on the whole review
- `comments.start_line`, `comments.end_line`: the lines it covers, zero when it
  has no range
- `comments.quoted`: the text those lines hold
- `comments.body`: what the person wrote
- `comments.late`: written after its round's verdict
- `comments.reply`: the agent's reply, empty if none
- `next`

### status

- `daemon.pid`, `daemon.started`, `daemon.build`, `daemon.state_dir`
- `daemon.surface.state`, `daemon.surface.url`, `daemon.surface.reason`: the
  reviewer's surface, as described above
- `project.name`, `project.root`
- `agent`: the name this command ran as
- `reviews.id`, `reviews.title`, `reviews.agent`, `reviews.round`,
  `reviews.state`
- `reviews.size.files`, `reviews.size.added`, `reviews.size.removed`
- `reviews.requested_at`: when the latest round was asked for
- `reviews.undelivered`: comments the agent has not acknowledged
- `others`: the number of reviews in the project under other agent names
- `next`

### show

- `review.id`, `review.title`, `review.agent`, `review.state`, `review.round`,
  `review.paths`, `review.project.name`, `review.project.root`
- `rounds.number`, `rounds.note`, `rounds.base`
- `rounds.verdict`: `approve` or `changes`, empty while the round waits
- `rounds.verdict_note`: the person's note on the round's verdict
- `rounds.decided_at`: null while the round waits
- `rounds.size.files`, `rounds.size.added`, `rounds.size.removed`
- `files.path`, `files.digest`, `files.size`, `files.in_scope`: the latest
  round's captured files, with an empty digest for a deleted file

### reply

- `comment.id`, `comment.round`, `comment.level`, `comment.path`,
  `comment.start_line`, `comment.end_line`, `comment.quoted`, `comment.body`,
  `comment.late`, `comment.reply`
- `next`

### abandon

- `review.id`, `review.title`, `review.agent`, `review.state`, `review.round`,
  `review.paths`, `review.project.name`, `review.project.root`
- `next`

### check

- `review`, `round`, `title`, `agent`

### cat

- `review`, the review's id, and `round`, its latest round's number
- `path`, the file relative to the working copy's root
- `digest`, the SHA-256 of the content, and `size`, its length in bytes
- `content`, the file's bytes as base64

### start

- `pid`, `started`, `build`, `state_dir`
- `surface.state`, `surface.url`, `surface.reason`

### stop

- `stopped`: false when no daemon was running
- `daemon.surface.state`, `daemon.surface.url`, `daemon.surface.reason`
- `daemon.pid`, `daemon.started`, `daemon.build`, `daemon.state_dir`: `daemon`
  is null when none was running

### tailnet login

- `dir`, where the node's state lives
- `url`, the surface's address once a daemon serves it
- `first`: true when the login enrolled the node, false when it logged an
  enrolled node in again

### refusal

- `error.code`, `error.message`, `error.fix`
