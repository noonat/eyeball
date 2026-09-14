# Product

eyeball is a review surface for work done by agents. Agents ask for review, a
person reads and comments on a phone or a laptop, and the verdict releases the
agent to continue. One instance serves every project on the machine.

> Written as the target, not as a report. None of it exists yet. How it is built
> is [architecture.md](architecture.md), derived from this document and not the
> other way round. How its prose reads is [voice.md](voice.md).

## The problem

Several agents work at once. Each reaches a point where it should stop and get a
human judgment before going further: a spec to agree on, a design to settle, a
change to accept. The person is one, the agents are many, and the work is
asynchronous.

Everything hard about this follows from that ratio. The scarce resource is the
reviewer's attention, not compute and not the agent's time. It arrives in
whatever fragment is available, often on a phone, often away from the desk. A
review that takes ten minutes to open, orient in and act on does not get done.
It gets postponed, and three agents idle behind it.

So eyeball is a queue for one person's attention, plus a surface that makes each
item in it answerable in a couple of minutes. Every decision below is downstream
of that.

Two failures matter more than the rest.

**A lost comment.** A reviewer who writes a paragraph of careful reading and
watches it vanish stops writing careful paragraphs.

**Work that moves while it is being read.** A file an agent is still editing
cannot be reviewed at all. Half the reading is invalidated by the time the rest
is done, and there is no way to tell which half.

## The loop

```
agent            eyeball                      reviewer
  │
  ├─ request ───► review opens, round 1
  │               content frozen  ──────────► notification, one per request
  │                                              │
  │  (waiting)                                   ├─ reads the change
  │                                              ├─ comments, any number
  │                                              └─ approves, or asks for changes
  │                                                        │
  ◄─── every comment and the verdict, at once ─────────────┘
  │
  ├─ makes the changes
  └─ request ───► round 2, frozen against round 1
```

The agent learns nothing until the verdict. That is the point, and it is covered
below.

## A review is an object with a declared scope

A review has a project, an agent, a title, a set of paths, and a state. It is
created deliberately, not inferred from whatever happens to be dirty in the
working copy.

**Several reviews are open at once in one project.** A spec waiting for
agreement and a code change waiting for acceptance are two different questions,
asked by possibly two different agents, and they get two rows in the queue and
two pages. Sharing a working copy is not a reason to share a verdict.

**A review names its paths.** There is no implicit "everything that changed",
because that is exactly what merges two pieces of work into one. Naming the
paths is the agent's job and costs it one argument.

**A path is a file or a directory, which covers everything under it.** Not a
glob. Whether two glob patterns can match the same file has no cheap answer, so
overlap could only be guessed at. A guess merges two pieces of work under one
verdict, or blocks work that never conflicted.

**Overlapping scopes are refused.** A path already claimed by an open review in
the same project cannot be claimed by a second one, because the same change
would then appear in two places with two verdicts. The refusal names the review
holding it.

A review ends when it is approved. Asking for changes closes a round and opens
the next one. A review can also be abandoned, which is the agent saying the
question no longer needs an answer.

## A round is frozen

**Every round captures the content it is asking about, at the moment it asks.**
The reviewer reads that capture. The agent can do whatever it likes to the
working copy afterward without touching what is on screen.

This one decision pays for itself four times.

- **Nothing moves under the reviewer.** The reading stays valid for as long as
  it takes.
- **A comment points at an exact line and never drifts.** The content it refers
  to is immutable, so a line number is a permanent address. No content hashing,
  no anchor rot, no list of detached comments to reconcile.
- **"What changed since the last look" is a real diff**, computed between round
  N-1's capture and round N's, rather than inferred.
- **Any file can be opened as it was**, not as it is now, and not only the files
  the review covers.

**A capture is a base commit and the content of every file that differs from
it.** Nothing else is copied. A file that matches the base matches it byte for
byte, so it resolves out of git at that commit and costs nothing to keep. A file
the agent has touched will move, so eyeball stores its own copy of that one.

Storage is therefore proportional to what the agent changed rather than to the
size of the repository. A six-file change in a repository of five thousand files
stores six files. Copying the tree is the obvious reading of "capture the
project" and the wrong one: it buys nothing, because the untouched remainder is
already immutable in git and addressed by the base commit.

The reviewer still sees exactly what the agent had, whichever file they open.
Clean files come from git, dirty ones from the capture, and clean means
identical, so there is no third case.

**Files the agent changed outside the review's scope are captured too.** They
cost the same as anything else touched, and without them a file opened for
context would show the base rather than what the agent was working with, which
is a difference the reviewer has no way to detect.

**A file opens from the first of three sources that has it**, and says which one
it came from:

1. **The round's capture.** Frozen, exact, and the only source a file under
   review is ever read from.
2. **The base commit.** Exact as well, because a file that matches the base
   matches it byte for byte.
3. **The working copy, as it is now.** Not frozen, and possibly moved since the
   round was taken.

The third exists because the alternative is showing nothing, and a reviewer who
cannot look up a definition goes to a computer, which ends the review. It covers
a base commit lost to a rebase or a garbage collection, a file created after the
base, and a project with no git at all.

**Only the third can mislead, and only ever about context.** A file the review
covers is always in the capture, so a verdict is never given against a live
reading. What can go wrong is looking up a helper, seeing a version the agent
did not have, and drawing a conclusion from it. The label is the whole
mitigation and it is not a complete one. Showing nothing instead trades a small
risk for a certain cost.

The alternative is to read files from disk on every request and compensate for
the movement. That forces a comment anchor to be a hash of the text it attaches
to, because a line number means nothing once the file has shifted. Editing a
paragraph then changes its hash and detaches its comments, so the tool grows a
list of comments it can no longer place, and the reviewer reads bookkeeping.
Freezing removes the problem rather than compensating for it.

**A review's first round needs something to change against.** In a git
repository the base is HEAD by default, and the agent can name a different one.
Elsewhere there is no record of the file before the edit, so the base is empty
and the first round shows whole files. That is the right reading for a document
being written from scratch and a poor one for a change to existing code, which
is the concrete cost of running eyeball outside git.

## Comments release as a batch

**A comment is invisible to the agent until the verdict is submitted.** The
agent is not notified, and a waiting agent does not wake. Nothing about it is
merely deferred, because nothing is delivered.

The reason is that a reviewer forms an opinion while reading. A first comment
written on line 20 may be answered by line 90, or may turn out to be the small
half of a larger point. An agent that receives it immediately starts working on
a reading that is not finished, and the reviewer is then arguing with edits made
underneath an unfinished thought.

Making this the transport's behavior rather than a rule the agent follows is
deliberate. An agent told "wait for the verdict" while being handed each comment
as it lands will eventually not wait. Nothing is delivered, so nothing can be
acted on early.

Both verdicts release the batch. Asking for changes finishes a reading as
completely as approving does.

## Comments are ephemeral, the agent's note is not

Once a new round starts, the previous round's comments are done. What the
reviewer needs on screen is the current state of the work. Earlier rounds stay
reachable, one tap away, and out of the default view.

**Every request carries a note from the agent**, and it is required. On the
first round it says what to look at. On later rounds it says what changed and
what was done about the last round's comments. This is the one piece of history
that stays in front of the reviewer, because a second round opens on what was
done about the first one.

Per-comment replies from the agent are allowed and optional. They live with the
comment they answer, inside the round that is now history.

## The surface

One surface, one layout, at every size. The phone is the hard case and the
common one, so the phone layout is the design and a wider screen gets a wider
column with more of it visible at once. There is no separate desktop
application, and no separate mobile one to keep in step with it.

**Every view has an address.** A notification opens its round directly, a link
to a file opens that file at that round, and going back goes back. A review
half-read and closed reopens where it was.

### The queue

The home screen answers one question: what needs attention.

**Waiting** is the top of the screen and nothing competes with it. One row per
review with a request outstanding, naming the project, the agent, the title, the
round number, the size of the change, and how long it has been waiting.

The size is measured against the previous round from round two onward, which is
the change the round opens on. Measuring against the review's base instead would
put the size of the whole review above a diff of what moved since the last
reading.

**Working** is below it: reviews the agent holds, either acting on a verdict or
not yet ready to ask. Informational, and never above the reviews that are not.

**Settled** is a short collapsed tail of what was approved recently, so a
verdict given an hour ago can be found without a search.

The project and the agent are part of every row rather than a filter to apply,
because several of each feed one surface. A filter exists. Needing it in order
to see what is waiting would be the failure.

**The number waiting is on the installed icon.** A glance at the home screen
answers the question without opening anything, which is the difference between
checking twenty times a day and checking when there is something to check.

An empty queue says so plainly and lists the projects eyeball knows about, which
is also how a project with a misconfigured root gets noticed.

### A review page

The page opens on the current round, with the brief before the work:

1. the title, the project, and the agent
2. the state and the round number
3. the agent's note, in full and never truncated
4. the size of the change, as files touched and lines added and removed
5. how long the request has been waiting

The note is third rather than buried because it is the reviewer's brief. On a
second round it says what was done about the last one, which is the question the
reviewer opens with.

**A small change opens expanded, a large one opens as a list of files.** Under
roughly ten files and a few hundred lines the whole thing is on the page and
scrolling is reading. Past that, a list of files with their line counts comes
first and each file expands on a tap. Both failures are real: scrolling through
four thousand lines to find the interesting one, and tapping through forty
collapsed files to find the same. The threshold is a starting point to be tuned
against real reviews, not a considered number.

### Reading the change

**Unified, never side by side.** Two columns of code on a phone screen leaves
about twenty characters each, which is not reading. One column with removals
above additions works at every width.

**Changed words are marked inside a changed line**, but only where a run of
removals pairs one to one with the additions that replaced it. An unequal run is
a rewrite rather than an edit, and guessing which line replaced which marks the
wrong words with confidence.

**Code scrolls sideways inside its own block.** The page itself never scrolls
horizontally. A page that does is a page where the decision bar drifts off the
screen.

**Additions and removals differ by more than hue.** Red against green is the one
pair some readers cannot separate at all, and a review surface that depends on
it is unreadable rather than inconvenient for them.

**Markdown opens as a rendered document. Code opens as a diff.** A spec is read
as prose because prose is what it is, and a patch of a paragraph is a poor way
to read a paragraph. Code is read as a change because the change is the unit.
Either can be flipped to the other, since sometimes the exact edit to a document
is the question and sometimes the whole file is what a code change needs.

**A changed passage in a rendered document is marked, and can show what it
replaced** without leaving the rendered view. The marking is a rule beside the
passage rather than a color wash through it, so a heavily edited document stays
readable as a document.

**When the base is empty, nothing is marked.** A document written from scratch
is entirely new, and marking every line marks nothing. It reads as a document,
which is the correct reading.

**Code is syntax highlighted**, in a diff and in a whole file alike.

**A file that a browser can display is displayed.** An image is an image, a page
is a page, a recording plays. Reading every file as text would make the surface
useless for whole categories of work: a design mockup is an HTML file, and
reviewing one means looking at the page rather than at the markup. So is a
chart, an icon set, a screenshot attached to a bug, a sound a game will play.

**A file with both a source and a rendering offers both**, one control apart,
the way markdown already does. Which one opens first depends on the kind:

| Kind                                   | Opens as                        |
| -------------------------------------- | ------------------------------- |
| Markdown                               | The rendered document           |
| Image, audio, video, PDF               | The thing itself                |
| HTML, SVG, and every other text format | The source                      |
| Anything else                          | A line naming its type and size |

**The agent can override that per file**, because it knows which question it is
asking. A change to a page's styling wants the page. A change to the same file's
markup wants the source. Nothing else in the request knows the difference.

**A changed image shows before and after, stacked at the same width**, with the
dimensions and byte size of each. Stacked rather than side by side for the same
reason diffs are unified: two images across a phone screen are two thumbnails. A
new image has no before and is shown on its own.

**Rendering a project's own files is the one place this surface runs something
an agent wrote.** An HTML file in a review is not a document, it is a program,
and a page that executes with the surface's privileges can read the queue, post
comments, and approve its own review. The agent that wrote it has every reason
to and no way to be trusted not to.

So anything that can execute renders in a sandbox, from an origin that is not
the surface's, with no access back to it. That covers HTML, and it covers SVG,
which is a document format that can carry script and is routinely mistaken for
an image format. An SVG shown as an image is safe, because an image cannot
execute. The same file opened as a page is not.

**A file too large to send to a phone is not sent.** The surface names it and
stops. The limit is a real one: a review is read over whatever connection the
reviewer has, and a video that would take a minute to arrive has already cost
more than the review was worth.

### Widening

Three steps out, each one control rather than a navigation.

**From a hunk to the whole file**, with the change still marked in place.

**From a file to any other file in the project**, as it was at that round. A
diff often refers to something it does not contain, and walking to a computer to
look it up ends the review. It costs nothing extra: the round already holds
every file the agent touched, and everything else is in git at the base commit.

**What the change is measured against is one control with two settings.** The
previous round is the default from round two onward and answers what moved since
the last reading. The review's base answers what this review has done in total,
which is what a reviewer wants before approving something that took four rounds.

### Making a comment

**A comment attaches at one of four levels.** To a line or a range of lines. To
a passage, which is what a rendered document offers instead of lines. To a file,
for a point about the file rather than a place in it. To the review, for the
point that the approach is wrong, which has nowhere else to go and is the most
important comment anybody writes.

**The reviewer selects in whatever they are reading, and the agent always
receives a path, a line range, and the quoted text.** A passage in a rendered
document maps to lines in the frozen source exactly, because the source cannot
move. So an agent never has to work out what a comment was pointing at, whatever
view produced it.

**A comment is saved the moment it is written.** Storing and releasing are
different things: the comment is durable immediately and invisible to the agent
until the verdict. Holding a round's comments in the page until submission would
be simpler and would lose a review to a closed tab, a dead battery, or a phone
that decided to reclaim some memory.

**A comment is durable once eyeball has acknowledged it, and visible until
then.** A save that fails leaves the text on screen, marked as not sent, with a
retry. The surface never discards what somebody typed, and never claims to have
stored something it has not.

**A live connection is assumed.** This is a conversation with an agent that is
running right now, on a machine reachable from the phone, and the reviewer is
holding one side of it. Reviewing with no connection is not a case worth
building for, any more than a terminal is worth building for a network that is
not there.

That assumption is worth stating rather than leaving implied, because the
alternative was designed and rejected. Storing comments on the device and
draining them later means a second copy of every comment, a reconciliation on
boot, and a write path that has to be correct in a case nobody will hit. What it
buys is a comment typed in a basement, on a phone that could not have loaded the
review in the first place.

**Assuming a connection is not the same as ignoring its absence.** The surface
does not work offline. It does have to say when it is offline, and the
difference between those two is the whole of this section.

A surface that renders from the server and acts through it has one failure worse
than any other: a tap that does nothing. The button looks the same before and
after, so the reviewer taps it again, then holds it, then assumes they missed.
Nothing on screen separates an unreachable server from a mis-aimed thumb. This
has been watched happening on a surface built this way, which is why it is a
requirement here rather than an afterthought.

So:

- **Losing the connection is stated, not implied.** A visible notice, present
  for as long as the condition is, saying what cannot be done and offering to
  try again.
- **Every action that needs the server reports its own failure**, on the control
  that was used, in words. Not a spinner that stops, and never nothing.
- **Nothing is shown while the connection is healthy.** A permanent indicator
  becomes furniture within a day and stops being read, which leaves the reviewer
  with no signal at the moment one matters.
- **Text already typed survives the failure.** A comment whose save did not land
  keeps its text and offers a retry.

**This round's comments are listed, editable, and deletable until the verdict.**
Nothing has been released yet, so a point that turned out to be wrong by the end
of the reading can be removed rather than explained away.

### Deciding

**The decision bar is pinned to the bottom of the review page.** Approving a
long document must not require scrolling to the end of it, and the bar is where
the reviewer already looks.

**Two verdicts: approve, or request changes.** Both release the round.

**A verdict carries an optional note**, which is the place for the judgment that
is about the whole thing rather than any line of it.

**Requesting changes with no comment and no note is refused.** The agent would
receive an instruction to change something with no statement of what, and would
either guess or come back to ask. The refusal names the fix.

**The bar says what is about to be sent**, as a count of comments. Submitting is
the moment a reading becomes irreversible, and the reviewer should not have to
remember how many points they made.

**After a verdict the round is closed**, the page shows the decision, and the
review leaves the waiting list. A comment written on a closed round is allowed
and goes out on its own immediately. Batching exists to protect a reading in
progress, and that reading is over, so holding the comment would hold it until a
round that may never be requested.

### The project view

Reachable from any review's header and from the queue, one per project.

**It lists the project's open reviews**, which is the same information the queue
carries, arranged by project instead of by urgency.

**It shows the working copy**, meaning everything currently changed on disk,
including work no agent has submitted for review. This is the step back: what is
going on in this repository right now, across every review and outside all of
them. It is read-only context rather than a reviewable object, and it is live
rather than frozen, because showing the present is its whole job.

### First run

**The surface is installed to the home screen once, and granted notification
permission once.** Until that happens, notifications do not arrive, which makes
this the one setup step that has to be impossible to miss rather than something
found later in a settings screen.

So an uninstalled surface says what is missing and offers the two steps in
order, and a surface whose push subscription has lapsed says that too. A queue
that looks empty because nothing is arriving is the failure mode this prevents.

## Notification

**One notification per request, deep linked to the review.** Opening it lands on
the round, not on the home screen.

Comments and verdicts generate none: they travel toward the agent, which is
waiting on a command rather than watching a phone.

The channel has to reach a phone that is locked, with the surface not open, and
it has to work while away from home.

**Web push carries them**, from the surface itself. The page is installed to the
home screen and granted permission once, and after that a request wakes the
phone with no other software involved. Nothing to run, no bot to register, no
token to hold, and no third party between an agent and the person reviewing its
work.

The cost is a real setup step and a platform constraint. iOS delivers web push
only to a page installed to the home screen, served over HTTPS with a
certificate the phone already trusts, so the surface has to be installable and
properly served before a single notification arrives. A subscription can also
lapse silently, which means the surface has to notice a dead subscription rather
than assume delivery.

**A chat bot is the escape hatch**, wired the same way and used if web push
proves unreliable in practice. Telegram is the obvious candidate, because a bot
there is the shortest path to a phone that already exists. The two things such a
bot is good at do not help here. A bot is bidirectional, and the reply path is
the surface rather than the message. A bot carries pictures and files, and the
payload is a link. What is left is a credential to hold and a service to depend
on, weighed against a delivery mechanism the surface owns outright.

## The agent's side

An agent drives eyeball through a command-line tool. The target is that an agent
gets it right with no worked example in front of it, which is a stricter bar
than a person needs and settles most of the interface.

- **Flags parse in any position.** Argument order is the thing a model gets
  wrong once per session forever.
- **`--json` on everything**, with a documented, stable schema, and a `--format`
  template over the same shape.
- **`eyeball doc` prints the manual**, complete enough that an agent can drive
  the tool from it alone.
- **Every mutation prints what to do next**, including, at the one place it
  matters, an instruction to stop and wait rather than to continue.
- **A refusal explains and names the fix**, rather than reporting an error.

The verbs an agent needs:

```text
eyeball request <paths...> -m "what to look at"   opens a review, returns its id
eyeball wait <id>                                 blocks until the verdict, prints the batch
eyeball status                                    this agent's reviews, and their states
eyeball reply <comment-id> -m "what was done"     optional, per comment
eyeball abandon <id>                              withdraw the question
```

**Waiting is a blocking command, not a poll.** An agent that polls burns context
on nothing happening. `wait` returns once, with every comment and the verdict
together, which is also exactly what a monitor in a harness needs.

**State survives a cleared context and a restarted session.** Everything lives
in eyeball, not in the conversation. An agent that has lost its memory runs
`eyeball status` in its working copy and gets back the reviews it owns, their
states, and what it is waiting on. Nothing has to be remembered across a restart
except the working directory, which the shell already knows.

**An agent sees its own reviews.** The reviewer's surface combines every project
and every agent. An agent's view is its own slice, so a spec review in another
project is not something it has to read past or, worse, act on.

Identity has to survive a restart, so an agent is identified by the working copy
it runs in, with an explicit name available for the case of two agents in one
directory. A session id would be the obvious choice and is the wrong one: it
dies with the session, which is the exact event this has to survive.

## One instance, many projects

One eyeball runs. Every project on the machine uses it.

**A project is discovered by walking up from the working directory**, so an
agent does not have to be told which project it is in, and does not have to get
it right after a restart. The first `.git` above the working directory is the
project. A directory that is not a git repository is named by a marker file a
person writes there once, which is the only setup eyeball ever asks for and is
asked for only where the free answer is unavailable.

**eyeball does not modify the project.** No state written into the repository,
no files added to a git project at all, no changes to its build or tooling. It
reads. Its own state lives in its own store, keyed by the project it belongs to.

That is a hard constraint rather than a preference. A control surface each
project has to adopt is one that gets adopted where somebody remembered, which
is not everywhere, and the projects it misses are the ones where an agent is
working unattended.

**Git earns its place three times and is not otherwise required.** It answers
where the project root is. It answers what has changed on disk, which is what
the working copy view shows and what a first round diffs against. And it holds
every file the agent did not touch, which is what makes opening one for context
free rather than a reason to copy a repository.

None of those answers is available cheaply anywhere else. Everything after that
is eyeball's own: a round is a diff between two captures it holds, so rounds two
and later never consult git for the change itself.

Without git there is no base commit, so the middle source is gone and browsing
falls back to the working copy as it is now. Reviews work, and a file opened for
context is labeled as live rather than as of the round. That is the second cost
of running outside git, after a first round that shows whole files.

**The tool is harness independent.** It is a command-line program with JSON
output. Anything that can run a process can drive it. Convenience glue for one
harness, such as an installable skill pointing an agent at `eyeball doc`, is
additive and never required.

## An approval is a fact, not a claim

A gate on a piece of work is either a **proof**, which is a command that can be
run and can fail, or a **claim**, which is a label a person asserts and nothing
verifies. Proofs are worth more, so the useful move is to convert claims into
proofs wherever one can be built. Human review is the standing example of a
claim that resists it: no command can establish that a person read something and
agreed to it.

eyeball turns that one into a proof. An approval is a record of which review,
which round, what content, and when, held outside the session that asked for it.
Anything that gates on human review can read it instead of trusting a label.

The line eyeball does not cross: an approval records that a person approved
something, and nothing else. It never advances a spec, checks a box, or writes
to another system. That state has an owner already, and an unauthenticated page
on a private network is not it.

## Out of scope for now

Named, because each is a plausible next step and none is in the first version.

- **Commenting on a file no agent has submitted**, from the working copy view,
  as a way to open a review the agent did not ask for. Useful, and reachable
  once that view exists. Today the same thing happens by telling the agent.
- **Threaded discussion.** A comment and an optional reply is the whole model. A
  conversation on a phone about a line of code is a worse version of talking to
  the agent.
- **More than one reviewer.** One person, one queue. Multiple reviewers change
  the meaning of a verdict and most of the design with it.
- **Authentication.** The surface is reachable on a private network only, which
  is the same protection the machine itself has.
- **Editing from the surface.** An edit made there is a change nobody reviewed,
  and the surface is a worse editor than the one the agent already has.
- **Anything that runs the project's build.** eyeball reads and displays. Proofs
  belong to whatever already owns them.
- **Downloading a file the surface cannot render.** A binary saved to a phone is
  not a thing anybody reviews. Naming the file, its type and its size answers
  what the reviewer actually asked.
- **Side by side diffs.** Unified reads at every width, and a second column that
  only works on a desktop is a second layout to maintain for the case that
  matters least.
- **Search across a project.** A review names its paths and a diff links to what
  it references, which covers finding things by following them. Finding things
  by guessing at a word is a different tool.
- **A native application.** An installed page delivers notifications, an icon
  and a badge. What is left needs a store account and a build per platform.
- **Reviewing with no connection.** The work being reviewed is on a machine the
  phone has to reach anyway, and the agent waiting for the verdict is running on
  it. A surface that works without the network is a surface that cannot do
  anything useful when it gets one.

## Open questions

- **What a scope is made of.** Paths are the obvious answer and cover the cases
  named here. Whether a review ever needs to name something other than a path is
  not yet known.
- **Whether web push holds up on a locked iPhone** across days of real use, and
  what the surface does about a subscription that has silently lapsed. The
  escape hatch exists because this is unproven here, not because it is expected
  to fail.
- **What a marker file contains** beyond naming a project root. Nothing yet
  requires a second field, and adding one before something does is how a
  zero-setup tool acquires configuration.
- **How a range of lines is selected on a touch screen.** A single line is a tap
  and a range is not obviously anything. The fallback is that a comment on one
  line of a hunk is usually enough, which would make ranges a desktop
  convenience rather than a feature.
- **Where the expand-or-list threshold sits.** Ten files and a few hundred lines
  is a guess written down so it can be corrected, and the correction needs real
  reviews rather than reasoning.
- **How long a decided review is kept**, and whether captures are pruned with
  it. Comments are ephemeral by design. The record of what was approved may not
  want to be.
