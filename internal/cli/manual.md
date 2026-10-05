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

**The project** is the nearest directory above the working directory that holds
a `.git` entry or a `.eyeball` file. A git worktree is a project of its own. A
directory outside git becomes a project when a person creates an empty
`.eyeball` file at its root. eyeball never writes into a project.

**The agent's name** is `--agent`, then the `EYEBALL_AGENT` environment
variable, then the project root's path with the home directory written as `~`.
An agent acts only on reviews opened under its own name in its own project, so
keep the name the same across a restart. When a command refuses with
`wrong_agent`, its fix names the agent that opened the review.

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

### `eyeball start`, `eyeball stop` and `eyeball serve`

Any command starts the daemon when none is running, so these are rarely needed.
`eyeball start` starts it if needed and prints it. `eyeball stop` stops it and
waits until it has gone, and never starts one. `eyeball serve` runs it in the
foreground, and is the person's, for debugging: an agent does not run it.

`eyeball serve` with `--surface-addr` also serves the reviewer's surface on a
loopback address, such as `127.0.0.1:8111`. It is the person's flag, for
debugging. A loopback port can be reached by every account on the machine.

**The surface** is the page the person reviews on. The daemon serves it on the
tailnet once the owner has set it up, which only the owner does. `status`
reports it under `daemon.surface`: `off`, `starting`, `serving` with its `url`,
or `failed` with a `reason` saying what the owner does. Nothing in that reason
is a command for an agent to run.

When a command warns that the daemon is a different build, run `eyeball stop`
once the person is not mid-review. The next command starts the new build.

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

| Code                   | Meaning                                                        |
| ---------------------- | -------------------------------------------------------------- |
| `already_decided`      | the round already has a verdict                                |
| `awaiting_verdict`     | a next round was asked for while the last one waits            |
| `bad_base`             | `--base` does not name a commit                                |
| `bad_request`          | the daemon could not read the request                          |
| `base_gained`          | the repository gained its first commit after the review opened |
| `base_moved`           | a round was measured from a base other than the review's       |
| `connection_lost`      | the daemon closed the connection before answering              |
| `daemon_stopping`      | the daemon stopped while the command waited                    |
| `daemon_unavailable`   | no daemon could be reached or started                          |
| `empty_change_request` | a change request with no comment and no note                   |
| `internal`             | a failure inside the daemon; its log says more                 |
| `invalid_path`         | a path that is empty or names the project root                 |
| `missing`              | a required value was not given                                 |
| `no_git`               | the project is a git repository and git is not on PATH         |
| `no_project`           | no `.git` or `.eyeball` above the working directory            |
| `no_such_path`         | outside git, a path the project does not have                  |
| `not_found`            | an id that names nothing                                       |
| `not_open`             | the review was approved or abandoned                           |
| `not_released`         | a reply to a comment the agent has not received                |
| `not_top_level`        | the project root is below its repository's top level           |
| `nothing_captured`     | nothing differs from the base                                  |
| `nothing_in_scope`     | nothing under the review's paths differs from the base         |
| `outside_project`      | a path outside the project                                     |
| `path_claimed`         | another open review in the project covers the path             |
| `path_unmatched`       | a path that is neither on disk nor among the captured changes  |
| `round_closed`         | an edit to a comment whose round has a verdict                 |
| `too_many_files`       | outside git, the paths hold more than a thousand files         |
| `usage`                | the command line could not be read                             |
| `vanished`             | a file disappeared while it was being captured; run it again   |
| `wrong_agent`          | the review was opened under another agent name                 |
| `wrong_project`        | the review belongs to another project                          |

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

### start

- `pid`, `started`, `build`, `state_dir`
- `surface.state`, `surface.url`, `surface.reason`

### stop

- `stopped`: false when no daemon was running
- `daemon.surface.state`, `daemon.surface.url`, `daemon.surface.reason`
- `daemon.pid`, `daemon.started`, `daemon.build`, `daemon.state_dir`: `daemon`
  is null when none was running

### refusal

- `error.code`, `error.message`, `error.fix`
