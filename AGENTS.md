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

There is no `CLAUDE.md`. Claude Code reads this file when none exists, and a
`CLAUDE.md` beside it would hide it.

The order work is done in, from tearout to merged spec, is the `dev-workflow`
skill on the owner's machine. The rules below are the ones specific to this
repository, and the documents they point at hold the rest.

## Rules that fit on one line

- Design tearouts are judged on the real phone. See
  [docs/design/](docs/design/README.md).
- Code follows [docs/conventions.md](docs/conventions.md), which says of each
  rule whether it is enforced or left to judgment.
- Never commit or push without asking, and draft the message with the
  `dev-commit-message` skill first. The review gate is enabled here
  (`dev-review status`), so git refuses an agent's commit until the reviewer
  approves the staged change and the message has passed the skill's gate.
- All prose (documentation, comments, specs, commit messages) follows
  [docs/voice.md](docs/voice.md). Read it before writing any.
- The prose voice is shared and has no per-developer override.
- A session's conversational voice is `.claude/voice.md`, injected by a
  `UserPromptSubmit` hook, overridden privately by an uncommitted
  `.claude/voice.local.md`.

This file is a stub. It grows as the project does, one pointer at a time. A
table replaces the list once there is more than one thing to point at.
