# Expected verdicts

One file per message, with the verdict the gate must return and why. Run the
gate on every fixture after editing the prompt.

A rejected fixture is kept, not deleted. A message that was turned down is the
only evidence of what a rule actually means.

| Fixture                      | Gate must say | Why                                                                                                                                       |
| ---------------------------- | ------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `docs-01-rejected.txt`       | FAIL          | "wearing an engineering costume" is a figure of speech, and both paragraphs explain what the reader would not have asked                  |
| `design-01-rejected.txt`     | FAIL          | three paragraphs, two figures of speech, and a body explaining a change that adds files                                                   |
| `specs-01-rejected.txt`      | FAIL          | "holds it in their head" and "a nagging feeling" are figures of speech, and the second paragraph justifies a different part of the change |
| `foundation-01-rejected.txt` | FAIL          | four paragraphs, 167 words, each explaining a detail rather than the why                                                                  |
| `docs-02.txt`                | PASS          | subject only                                                                                                                              |
| `design-02.txt`              | PASS          | subject only                                                                                                                              |
| `specs-02.txt`               | PASS          | subject only                                                                                                                              |
| `foundation-02.txt`          | PASS          | subject only                                                                                                                              |
| `convention-01.txt`          | PASS          | subject only                                                                                                                              |
| `convention-02.txt`          | PASS          | subject only                                                                                                                              |
| `repo-01.txt`                | PASS          | subject only                                                                                                                              |
| `conventions-01.txt`         | PASS          | subject only                                                                                                                              |
| `skill-01-rejected.txt`      | FAIL          | "a reader asked whether a body was wanted will supply one" is malformed, which step 2c catches as a phrase nobody would say out loud      |
| `skill-02.txt`               | PASS          | the same message with the clause rewritten                                                                                                |
| `design-03-rejected.txt`     | FAIL          | "fails the wrong way" and "introduced itself" are figures of speech, and "form" labels a pattern the code names `agent_span`              |
| `design-04.txt`              | PASS          | subject only, after the body moved into the code comment                                                                                  |

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

## The run that fixed the empty body

Five subject-only messages were run through the prompt at once. Two passed and
three failed, all on step 4b, with the same reasoning each time: the body is
empty, so a developer would have unanswered questions, so it fails. The messages
were structurally identical, which makes the split a fault in the prompt rather
than a judgement about the messages.

Step 4b was still phrased as a question, the fault step 6 had. Asked whether a
developer would have wanted a body, a reader supplies one. It now asks for a
list of paragraphs, so an empty body can only produce an empty list.

Step 0 also short-circuits now. With no body there is nothing for steps 1 to 6
to examine, and leaving that undefined is what let the reader invent a rule that
every change needs a body. Rule 4 says the opposite, and the four oldest passing
fixtures are all subject-only.

The three rejected messages are kept as `convention-01`, `convention-02` and
`repo-01`.

All twelve fixtures were re-run after the fix and all twelve verdicts match. The
four rejected ones still fail, which is the half of the check that matters: a
prompt edited until the author's own messages pass is worth less than the broken
one it replaced.

Two of those four failed for reasons this table does not record.
`specs-01-rejected` failed on step 4b rather than on the figures of speech named
above, and `design-01-rejected` found one of its two. The verdicts are right and
the coverage moved, which is the same instability that made a subject-only
message pass twice and fail three times. A verdict is worth more than the
reasoning under it.

`skill-01-rejected` is the first fixture that fails on grammar rather than on a
figure of speech or an over-explaining body. Step 2c already covered it, as a
phrase nobody would say out loud, and nothing else in this directory exercises
that reading of the step.

`design-03-rejected` is the only fixture that fails step 4. The body called the
compiled pattern a form, where the code names it `agent_span`, and a reader
looking for that word in the file finds nothing.

Its accepted version has no body at all. Three rounds each cut a real fault and
the body still failed, which was the signal that it should not have existed: the
reason was already in a comment beside the pattern, where somebody reading the
code finds it without going through the history.
