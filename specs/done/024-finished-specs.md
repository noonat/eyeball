---
status: done
required_commands:
  - cmd: make check
---

# 024 Finished specs

Finished specs move into `specs/done/`, and `specs/done/AGENTS.md` tells an
agent that opens one that it is a record, not the current design. The 23
finished specs sit beside `TODO.md` and the spec being worked, so a person
scanning `specs/` reads past every finished plan to find the open one. An agent
that opens a finished spec has nothing telling it that its design may since have
changed, and `docs/` is the place that is kept current.

The `dev-workflow` skill now says to move each spec into `specs/done/` once its
last iteration closes, before the branch review, and gives the text of
`specs/done/AGENTS.md`. That change is what the item in `specs/TODO.md` waited
on. Claude Code loads a nested `AGENTS.md` when a file in its directory is read,
which the skill records as measured on 2.1.296. This spec moves the 23 specs
that are already finished and records the rule in `docs/conventions.md`.

The alternative was to leave `specs/` flat and mark a finished spec only by its
`status: done`. It lost because a status line is read only after the file is
opened, and the point is to know before opening it.

Probed on a copy of `specs/` before writing this:

- `punchcard` walks subdirectories. `punchcard list --json` listed 23 specs with
  the same states before and after the move. Their IDs change, since an ID is a
  hash of the path. A finished spec's IDs are not used again.
- Nothing outside `specs/` links to a spec by path. `tool/seed/main.go` and
  `internal/server/review_test.go` name `specs/018-tag-import.md` and others,
  but those are review fixtures and stay as they are.
- Inside the specs, 28 relative links break one level deeper: 18 to `../docs/`
  and 10 to `TODO.md`, in 9 files. Rewriting `](../docs/` to `](../../docs/` and
  `](TODO.md` to `](../TODO.md` fixes all 28.
- `make lint` formats `*.md` through `git ls-files`, which matches files in
  subdirectories, so moved specs stay formatted.

Fixing a link edits a finished spec. That is allowed here because it changes a
path and not what the spec records.

## Review

Spec review pass: declined, because the spec moves files and adds two
paragraphs, and every premise it rests on was probed above.

End-of-branch review pass: declined, because the diff is 23 renames, 28
rewritten links and two short paragraphs.

## Out of scope

- A link check in `make lint`. The resolver below runs once, as a todo. A
  standing check is in `specs/TODO.md`.
- Moving `docs/design/` tearouts or anything outside `specs/`.

## Iteration 1: Move the finished specs

The builder moves the files with `mv`, since it runs no git command that changes
anything. `git add -A` records each as a rename.

- [x] `specs/done/AGENTS.md` holding exactly the text the `dev-workflow` skill
      gives in `reference/execution.md`
- [x] Specs 001 to 023 moved into `specs/done/`, with `TODO.md` and this spec
      left in `specs/`
- [x] `punchcard list --json` listing 24 specs, the 23 moved ones with the
      states they had before the move, shown in the report
- [x] Every relative link in the moved specs rewritten as above, and the
      resolver below exiting 0 over `specs/`, shown in the report
- [x] `docs/conventions.md`'s Specs section saying that a finished spec moves
      into `specs/done/` before the branch review, that `docs/` is cited for the
      current design and a finished spec is not, and that a new spec is numbered
      after the highest in either directory

The resolver, run with `python3 -I` and the directory as its argument:

```python
import re, sys, pathlib
bad = 0
for f in pathlib.Path(sys.argv[1]).rglob('*.md'):
    for m in re.finditer(r'\]\(([^)#\s]+)', f.read_text()):
        t = m.group(1)
        if re.match(r'[a-z]+:', t): continue
        if not (f.parent / t).exists():
            print(f"{f}: {t}"); bad += 1
sys.exit(1 if bad else 0)
```

Once this iteration closes, the orchestrator moves this spec into `specs/done/`
with `git mv` before asking for the branch review, as the skill says.

> **Deviation: line breaks.** `specs/done/AGENTS.md` has the same words as the
> block in the skill's `reference/execution.md`, wrapped at a different width,
> because `make fmt` owns line breaks in `*.md`. The longer link paths also
> pushed some lines past the limit, so the formatter rewrapped the paragraphs
> holding links in 003, 004, 005, 022 and 023. No word of those specs changed
> apart from the 28 link paths.

> **Deviation: the resolver over `specs/`.** The todo asked for the resolver to
> exit 0 over `specs/`. It exits 0 over `specs/done/` and 1 over `specs/`. Every
> hit is in this spec, whose prose quotes `](../docs/`, `](TODO.md` and the
> rewritten forms inside code spans, and the regex reads them as links. The
> probe ran before this spec existed, so it did not see them. The todo is
> satisfied by `specs/done/` exiting 0 and by the hits all being in this file.
> When this spec moves into `specs/done/`, the same four false hits appear
> there. They are not broken links.

> **Completed** 2026-10-25 15:23 UTC
>
> - `make check` — 20.3s
