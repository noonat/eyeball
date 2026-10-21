---
status: ready
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -count=1 -tags browser ./internal/server
---

# 022 Nothing waiting

The queue's empty state shows "Nothing waiting", a sentence counting the agents
at work, and a list of the projects eyeball knows. The sentence counts agents
with an open review, which is the only kind of agent eyeball can see. An agent
working on something it has not yet put up for review has no open review, so the
sentence says no agents are working while agents work. The owner chose on
2026-10-18 to drop the sentence and the list, and leave the heading alone.

The design is in [docs/product.md](../docs/product.md), under The queue and The
project view, and the paragraph there names what the list was for. Two things go
with it. A project with a misconfigured root no longer shows on the queue. The
project view is reached only from a review's header, the owner's choice over
keeping one link to a list of projects, which would have added a page.

Rewording the sentence to count agents with an open review would make it
accurate and add nothing. A review is open while its round waits or after
changes were requested, so with nothing waiting every open review is a row in
the Working section under the heading, and the sentence would count the agents
of rows already on screen. Agents reporting their own status through eyeball
would answer the question the sentence was trying to, and that is in
specs/TODO.md under Unscheduled.

## Review

Spec review pass: declined, because the spec removes a sentence, a list and the
code that builds them, with no change to the schema or to any write.
End-of-branch review pass: declined, for the same reason. The tests below fail
if the list or the sentence comes back.

## Out of scope

- A way from the queue to a project's view. specs/TODO.md holds it, widened from
  the populated queue to the whole queue.
- Noticing a project with a misconfigured root, which the list did. Added to
  specs/TODO.md.
- Agents reporting status and messages through eyeball, already in
  specs/TODO.md.
- A phone check of the built screen. The tearout is judged on the phone, and the
  built state is the existing one with two parts taken away, which the queue
  test checks by the empty state's whole text.

## Iteration 1: The tearout

```punchcard
required_acks:
  - tearout-approved
```

- [ ] queue.html's nothing-waiting frame showing the shut eye and "Nothing
      waiting" alone, with no sentence and no Projects section
- [ ] queue.html's paragraph above the empty frames naming no project list and
      no misconfigured root, as product.md now says
- [ ] foundations.html's two empty-state specimens that print the agents
      sentence showing the heading alone, as the third already does
- [ ] project.html's opening paragraph saying the view is reachable from a
      review's header, with the queue no longer named
- [ ] The tearout judged on the phone by the owner, with any change asked for
      made before the ack

## Iteration 2: The surface

- [ ] web/templates/queue.html printing the empty state as the heading and the
      shut eye alone when nothing waits, with Working and Settled still below it
- [ ] queue.go without `emptyQueue`, `projectRow`, `projectsOnDisk`, `numbers`,
      `number` and `capitalize`, so the empty branch reads nothing from the
      store
- [ ] `store.Projects` returning `[]Project` by name with no agent count and
      `ProjectRow` removed, its comment naming no reader that has gone
- [ ] reads_test.go testing `Projects` for its order and its columns, in place
      of the agent count case
- [ ] A queue test that the empty state's text is "Nothing waiting" and that no
      element on the page links to `/projects/`, failing with the list restored
- [ ] `Test_emptyQueueSentence` and
      `TestServer_Surface_emptyQueueLeavesOutGoneProjects` removed, since what
      they tested is gone
- [ ] tool/seed's `answerAll` comment saying what the state is for without
      "agents still at work"
