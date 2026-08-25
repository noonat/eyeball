# eyeball

A review surface for work done by agents. Agents ask for review, a person reads
and comments from a phone, and the verdict releases the agent to continue. One
instance serves every project on the machine.

[docs/product.md](docs/product.md) is what it does and why.
[docs/architecture.md](docs/architecture.md) is how it is built, and is derived
from it. [docs/conventions.md](docs/conventions.md) is how the code is written.
[specs/TODO.md](specs/TODO.md) is what is next.

**This file is an index. Keep it that way.** Nothing belongs here that is not a
pointer or a rule short enough to fit on one line. A context file that grows
into a manual stops being read, and the things in it stop being followed.

`CLAUDE.md` is a symlink to this file, so both names find it.

## Rules that fit on one line

- Work with a user-visible surface opens with a design tearout, settled on the
  real phone, before any plumbing. See [docs/design/](docs/design/README.md).
- Work that is not being done right now goes in [specs/TODO.md](specs/TODO.md),
  one or two lines. Detail goes in a spec, not there.
- A change to an existing surface starts by changing the tearout it was approved
  against.
- Code follows [docs/conventions.md](docs/conventions.md), which says of each
  rule whether it is enforced or left to judgment.
- Work waits uncommitted until a human has reviewed it. Amend only a commit
  nobody has seen; a correction to one already shown is a new commit.
- Never commit or push without asking, and draft the message with the
  `commit-message` skill first. A `PreToolUse` hook refuses a commit whose
  message the skill's gate has not passed. The gate records its own pass, so
  there is nothing to run by hand.
- All prose — documentation, comments, specs, commit messages — follows
  [docs/voice.md](docs/voice.md). Read it before writing any.
- The prose voice is shared and has no per-developer override.
- A session's conversational voice is `.claude/voice.md`, injected by a
  `UserPromptSubmit` hook, overridden privately by an uncommitted
  `.claude/voice.local.md`.

This file is a stub. It grows as the project does, one pointer at a time. A
table replaces the list once there is more than one thing to point at.
