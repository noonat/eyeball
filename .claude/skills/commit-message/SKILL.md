---
name: commit-message
description:
  Draft a commit message for this repo, or redraft one the reviewer turned down.
  Use before every commit, including the redraft after a rejection. Runs the
  gate that has to pass before asking for approval.
---

# Commit messages

The rules are in `docs/conventions.md` under Commits, and the prose rules behind
them are in `docs/voice.md`. This file adds the procedure and the checks,
because the same few faults come back draft after draft and a person should not
have to catch them by hand every time.

## The rules that get broken

1. **Plain, simple English.** Short sentences, one idea each. Use the word a
   person would say out loud.
2. **No figures of speech.** Not one, however fresh.
3. **Never state the obvious.** Cut a sentence the reader would assume anyway,
   or that follows from the subject line, or that says a file explains itself.
4. **Usually no body at all.** A subject line is the whole message for anything
   a developer reading the history would not stop to ask about. Write a body
   only for a constraint that is not visible in the code, a tradeoff that was
   weighed, or an approach that was tried and abandoned. Then write one
   paragraph, and add a second only when part of the why goes missing without
   it.
5. **Say why, not what.** The reader has the diff.
6. **No word you invented.** A new name for something the code, the schema or
   the documents already name is a word the reader has to learn first. Use the
   column, the type, or the function's own name.

## Figures of speech to replace

| Instead of                    | Write                               |
| ----------------------------- | ----------------------------------- |
| three fixes ride along        | three fixes are in the same change   |
| in the same breath            | at the same time                     |
| it costs, pays for, buys      | name the tradeoff                    |
| something comes for free      | say what provides it                 |
| settling it on paper          | deciding before the code is written  |
| paid for twice                | it must be fixed in two places       |
| the change lands, carries     | use the plain verb                   |
| what a rule stands in for     | what the rule checks                 |
| wearing an engineering costume| say which kind of question it is     |

This is stricter than the prose in `docs/`, which uses some of these. A design
document is read slowly and more than once. A commit message is read once.

Add a row whenever the reviewer turns one down. The table is a record of what
has actually been caught here, not a list somebody imagined.

## Draft, then gate

Write the message to a file. Run the prompt below on `haiku`. Fix everything it
lists, then run it again. Ask for approval only after PASS.

Do not argue with a flag. If a cheap reader stops at a phrase, the next person
will too.

```text
Check a commit message against fixed rules. Do not judge whether it reads well.

The message is at: <PATH>
The change it describes: <ONE PLAIN SENTENCE>

Step 0. Report how many paragraphs the body has and how many words. Work
  through every paragraph. Skipping one is not allowed.

Step 1. Number every sentence in the body.

Step 2. List every phrase that is any of these, with a plain replacement:
  a. on this list: ride along, in the same breath, pays, costs, buys, for free,
     the end of, settling, on paper, paid for twice, lands, carries, bites,
     earns its keep, reads as, stands in for, at its core, under the hood,
     first-class, the whole point, worth its keep, wearing, costume, clothes
  b. a metaphor, an idiom, or any figure of speech, including one not listed
  c. a word or phrase a person would not use saying this out loud
  A term of art from the thing being changed is not a figure of speech: a round,
  a capture, a base commit, a verdict, a sticky bar, a unix socket, an opaque
  origin. Flag one only where a plainer word is exact.

Step 3. List every sentence that states something obvious. A sentence is obvious
  if the reader would assume it without being told, if it follows from the
  subject line, or if it says that a file or a document explains itself.

Step 4. Look at the body only, never the subject line. List every noun the body
  uses as a label for something the change touches: a column, a table, a type, a
  function, a file, a setting. For each, say whether that is what the code calls
  it. A name the body invented fails, however clear it seems.
  Not names, and not to be listed: ordinary words for things outside the change,
  a name quoted as an example of a bad name, a category word for a group of
  things, and the name of any tool, command or program, whether or not the
  change adds it.

Step 4b. Would a developer reading this in the history have stopped to ask the
  question the body answers? If the body explains something nobody would have
  asked, it fails. A change that adds a file, a package, a document or a screen
  explains itself.

Step 5. The why is the reason the change named in the subject line was made at
  all. For each paragraph after the first, name the part of THAT why which goes
  missing when the paragraph is deleted. Justifying a detail inside the change
  is not part of it: if what goes missing is a detail's reasoning, the paragraph
  fails. If you cannot name anything, it fails.

Step 6. List every paragraph that names two or more separate parts of the change
  and explains each one. Every paragraph you list is a failure. A part the
  subject line or an earlier paragraph already named does not count again.
  Counting parts without naming them is not listing them. List nothing if no
  paragraph does this.

Step 7. Report FAIL if any earlier step found anything. Otherwise report PASS.

Step 8. Only if step 7 said PASS, run this from the repository root and report
  what it printed:
    python3 .claude/hooks/commit-gate.py record <PATH>
  Never run it after a FAIL. It is what lets the commit through, so running it
  on a failing message defeats every step above.

Rules for you:
  - Flag when unsure. A wrong flag costs one rewrite. A missed one ships.
  - Do not say the message is clear, plain, fine or good. Report only failures.
  - Do not rewrite the message. Phrase-level replacements only.
  - Do not count a subject line word against step 2 unless it is a metaphor, and
    never count one against step 4 at all.
```

Step 4 is only as good as the sentence describing the change, since the gate
cannot read the code. Name the real type, function or file in it.

## Check coverage before reading the verdict

A cheap model sometimes reads part of a file and still returns a verdict. Check
step 0's numbers against the file before believing anything else it says:

```sh
awk 'BEGIN{RS=""} {n++} END{print n-1, "body paragraphs"}' <PATH>
wc -w <PATH>
```

Numbers that disagree void the run. Run it again.

## The pass records itself

A `PreToolUse` hook refuses `git commit` for a message with no record. The
record is the sha256 of the message, kept in `.git/eyeball-gate`, so **nothing
is added to the message itself**: what lands in git is byte for byte what was
gated, and the no-trailers rule is not bent to carry a receipt. Editing the
message after recording changes the hash and the commit is refused again, which
is the point.

**The gate writes the record itself, as the last thing it does on a PASS.**
There is no step here for you to run, which is deliberate. When there was one,
it was run without a gate having happened, five minutes after the hook was
written, by the person who had just argued for the hook. A record that can be
written without a gate run is a record that will be.

So: launch the gate, and when it reports PASS the record already exists. Commit
in a separate call, because the hook runs before the command and cannot see a
record written by that same call.

Then show the message in the reply. Tool output does not count. Offer the
choice: commit it, commit the subject alone, do not commit, change something.

## Keeping the gate honest

Fixtures live in `testdata/`, one file per message, with the verdict the gate
must return and the reason recorded in `expected.md`. Run the gate on all of
them after editing the prompt.

A fixture is added when a message is turned down, not invented. A message the
reviewer accepted is still worth keeping as a fixture, because an accepted
message is not the shortest one they would have accepted.

## What to expect from it

- **It pushes a body towards one or two sentences.** Each round of step 3 finds
  another sentence that followed from the one before it. That is the intent.
- **Three rounds is the limit.** The first two cut words. After that the gate
  trades one phrasing for another and the body grows again. On a fourth round,
  keep the last version, show it with whatever is still flagged, and let the
  reviewer decide.
- **A FAIL is evidence and a PASS is weaker.** The same phrase can be flagged on
  one message and missed on another by the same prompt, which is why the gate is
  run again after every fix rather than once at the end.

## Why the prompt is shaped this way

Four faults, each one designed out above rather than left to be rediscovered:

- **A yes-or-no question gets a yes.** Asking whether a body is plain English
  passes drafts a reviewer then rejects. Asking for a list of phrases, each with
  a replacement, catches them.
- **A question whose right answer is "no" gets read backwards.** Every step is
  worded so that finding something is a failure. Step 6 was still phrased as a
  question on the first run here and was read backwards on one fixture, which is
  why it now asks for a list like steps 2 and 3 do.
- **A verdict can cover part of the input.** Hence step 0 and the counts.
- **Strictness needs a limit.** Told to reject anything a changed file also
  says, the gate fails every paragraph, because a well-commented change explains
  itself twice by design. Enumeration is the test that holds.
- **A rule that cannot see the code guesses.** Step 4 asks whether a noun is
  what the code calls something, and the gate has only the one sentence
  describing the change. On the first run here it called `layout`, `gate` and
  `backlog` invented names, and this project uses all three. It now reads the
  body only, and skips category words and tool names.
