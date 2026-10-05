# Voice

How prose in this repository reads. Applies to documentation, code comments,
specs, commit messages and pull request bodies. Everything a person reads that
is not code.

This is shared. Every contributor writes the same way, because a document that
switches voice halfway makes the reader wonder what changed and why. There is no
per-developer override here, unlike the conversational voice below.

## The rules

**Plain, simple English.** Short sentences, one idea each. Cut a clause that
only adds rhythm. A period beats a dash or a semicolon.

**Get to the point.** Never use two words where one will do. Lead with the
answer, then explain it.

**Explain the mechanism.** A claim with no mechanism behind it cannot be checked
or corrected. Say how the thing works, then say what it costs.

**Name the tradeoff.** Every design decision gave something up. A document that
records only the choice invites the next reader to reopen it. Name the rejected
alternative and why it lost.

**State assumptions and uncertainty plainly.** "Measured at 18s" and "assumed,
not measured" are both useful. A guess written as a fact is the expensive kind
of mistake, because nothing downstream knows to doubt it.

**Assume the reader is smart and does not know the problem.** Explain the
problem, not the vocabulary.

**Use the word a developer already knows.** A term of art imported from another
field reads as precise to whoever imported it and as noise to everyone else.
Where two words fit, the one already in the reader's vocabulary wins.

**Avoid jargon, flourish, slogans and rhetorical questions.** Use a technical
term when it is the right term. Do not compress meaning into an idiom the reader
has to work out.

**State the happy path. Leave the rest implied.** A positive followed by its
inverse makes the reader parse both halves and reconcile them before the
sentence means anything, and the second half rarely adds a fact. Name the
alternative only where the contrast is the point, which is where a reader would
otherwise assume the wrong thing.

**Never write in the first or second person, and never name a person.** No "I",
"we", "us", "you", "your", or anyone's name. Write about the code and the
problem. "Measured against X, Y returned in 18s" outlives "we measured", which
dates the text and ties it to whoever was in the room. Naming the audience in
the abstract is fine.

**American spelling.** normalize, not normalise. behavior, color, judgment,
license, center. Quoted text keeps whatever the source wrote.

**Do not become terse, cold or academic.** Brevity is not the goal. Clarity is,
and a sentence cut to the bone often loses the reason with the words.

## Comments say why, not what

Default to no comment and let names carry the what. Write one when the why is
non-obvious: an invariant, a workaround for a specific bug, an external system's
constraint, a performance tradeoff with a reason.

Prefer the incident to the warning. "Both models read the O as a 0" is
checkable. "Be careful here" is not.

Never write "used by X" or "added for the Y flow". Those references rot.

## Commit messages say why, not what

The subject line says what changed. The body says why: the problem, the
constraint, the tradeoff. A reader has the diff already and cannot recover the
reason from it. If a paragraph would survive being replaced by `git show`, cut
it.

Leading with the problem and then pivoting into what was built is still a
what-body.

**State a correction as a correction.** If a commit reverses an earlier claim,
name the claim and say why it was wrong. A silent reversal leaves two
contradictory statements in the history and no way to tell which one won.

## Documents describe the target

A design document describes where the work is going, and is corrected when the
target moves. It is speculative by construction, so a departure from it is a
reason to change the document, not something to annotate inside it.

A plan is the opposite. It records what was intended, so where the work went
differently, the difference belongs with the plan.

## Refining a draft

Run the `avoid-ai-writing` skill over new prose when it is available. It catches
em dashes, bold that carries no weight, and the phrasings that read as machine
output.

The author cannot see that a sentence is scaffolding, because the author knows
what it was holding up. So for anything that matters, hand the draft to a fresh
reader and ask one question: does this give a reason, or does it summarize the
diff?

## The conversational voice is separate

`.claude/voice.md` holds the voice used when talking to a person in a session.
It is deliberately short, because it costs tokens on every single message.

**The hook that injects it is no longer in this repository.** It was a
`UserPromptSubmit` hook in `.claude/settings.json`; the same hook now runs at
machine scope and selects a project's `.claude/voice.md` ahead of its own
default, so this file still governs sessions here. The repository kept the voice
and gave up the delivery.

What that costs: a contributor without that machine configuration gets nothing
injected, and this file becomes documentation rather than an instruction. The
hook is four lines of shell and can be restored here if the repository ever
needs to be self-contained for someone else.

The file is also personal. A contributor who wants a different conversational
voice writes `.claude/voice.local.md`, which is preferred over this one and is
not committed. Nothing about that changes the prose voice in this file, which is
the same for everyone.

The two happen to agree today. They are separate files because they answer
different questions and can drift apart without either being wrong.
