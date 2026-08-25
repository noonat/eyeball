# Expected verdicts

One file per message, with the verdict the gate must return and why. Run the
gate on every fixture after editing the prompt.

A rejected fixture is kept, not deleted. A message that was turned down is the
only evidence of what a rule actually means.

| Fixture | Gate must say | Why |
| --- | --- | --- |
| `docs-01-rejected.txt` | FAIL | "wearing an engineering costume" is a figure of speech, and both paragraphs explain what the reader would not have asked |
| `design-01-rejected.txt` | FAIL | three paragraphs, two figures of speech, and a body explaining a change that adds files |
| `specs-01-rejected.txt` | FAIL | "holds it in their head" and "a nagging feeling" are figures of speech, and the second paragraph justifies a different part of the change |
| `foundation-01-rejected.txt` | FAIL | four paragraphs, 167 words, each explaining a detail rather than the why |
| `docs-02.txt` | PASS | subject only |
| `design-02.txt` | PASS | subject only |
| `specs-02.txt` | PASS | subject only |
| `foundation-02.txt` | PASS | subject only |

## The run that shaped the prompt

The first run over all eight matched six. All four rejected messages failed, for
the reasons above. Two accepted messages failed on step 4, and both were false:

- `layout`, `gate` and `the TODO list` were called invented names. This project
  uses all three, and the gate cannot see the repository. Step 4 now reads the
  body only, because a subject line is a summary and summaries need category
  words.
- `backlog` was called an invented term. It is a tool this project drives. Step
  4 now skips tool, command and program names.

One more fault appeared in the reasoning rather than the verdicts. On
`design-01-rejected` the run reported "Step 6 fails: the body does not
enumerate" when step 6 fails on a body that **does** enumerate. It reached the
right verdict from other steps. Step 6 now asks for a list, like steps 2 and 3,
so that finding something is the failure.

After those three fixes, the two disagreements were re-run and both passed. All
eight now match.
