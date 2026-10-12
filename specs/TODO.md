# TODO

The work that is next, in order, and the work that is not scheduled. One or two
lines each. Detail belongs in a spec, and a spec is written when the work is
ready to build.

This file exists because neither a person nor an agent holds a project's
outstanding work in their head. A person forgets it between sessions and an
agent forgets it between contexts.

## Next

Each becomes `specs/NNN-*.md` when it reaches the front. Together they are
everything product.md and architecture.md describe that does not exist yet, so a
requirement in either document that none of them names is a gap in this list.

- [ ] design, 017: a card's file header that stays at the top of the screen
      while its file scrolls under it, so a long diff keeps saying which file it
      is.

Beyond that, as of 015 the reading desk is applied, and every requirement
product.md and architecture.md describe is built as of 010. The next work comes
from Unscheduled below, once real reviews say which of it matters.

## Unscheduled

- [ ] design: markdown rendered as a diff, the owner's words. What form is open:
      today a changed document renders with changed passages marked.
- [ ] design: show that the page is refreshing when the installed app comes back
      to the foreground. Today nothing shows activity, and then new content
      appears all at once.
- [ ] design: TOML front matter between `+++` lines, as Hugo writes it. 014
      renders YAML front matter only, and nothing in this repository uses TOML.
- [ ] design: `...` as front matter's closing line, which YAML allows and Jekyll
      does not. 014 accepts `---` alone.
- [ ] design: a role for YAML keys. The highlighter's five roles leave a key and
      a plain value uncolored, so 014's front matter and a `.yaml` file show
      only numbers, strings and comments in color.
- [ ] design: a byte order mark before a document's first line. goldmark keeps
      it as text, so a file opening with one has no front matter to 014's
      parser, and its first line renders with the mark in it.
- [ ] design: a rendered file in a round whose repository is gone. That page
      shows captured files through a path of its own, which still shows a binary
      file as the binary line, and rendering there needs the raw route, which
      reads the base from that same repository. 010 left it.
- [ ] design: an image not changed since the previous round has no "Not changed
      since round N" note, since a file shown as itself skips the line
      comparison that writes it. 010 left it.
- [ ] design: the project view's live file page still shows a binary file as the
      binary line, where a review's file page names its kind and size.
- [ ] design: a tearout for the comment composer's line-range selection on a
      touch screen. The open question in product.md is whether ranges are worth
      the interaction cost at all.
- [ ] design: settle 011's two numbers against real reviews: a list of files
      past ten files, and a file's body deferred past 500 changed lines. Both
      are guesses written down so they can be corrected.
- [ ] design: keep a reader's place as the card at the top of the screen and its
      offset, not as a scroll position. A document's skeleton is six lines
      whatever its length, so a review reopened with a long document above the
      saved place opens below it. Reasoned in 011, not seen on the phone.
- [ ] product: marking a file reviewed beyond folding its card, such as a count
      of files read or a mark that survives a new round. 011 kept folding alone,
      at the owner's word, and more behavior would be its own spec.
- [ ] design: where an insertion should attach when a run of identical lines
      makes several placements equally minimal. 004 pins one tie-break and GNU
      diff shifts boundaries with heuristics of its own, so the two differ on
      about a fifth of diffs. Which reads better needs real reviews.
- [ ] design: whether word marking should be suppressed where a paired line
      shares almost nothing with the one it replaced. Marking it end to end is
      noise, and the proportion to cut at needs real reviews.
- [ ] product: whether a review's base should follow a branch that moves under
      it. 004 fixes the base at the first round, so a merge landing mid-review
      arrives inside the review as work the agent did not do.
- [ ] product: whether a marker file inside a git repository should get the
      no-git treatment. 004 decides a project is git by whether its root holds a
      `.git` entry, so a monorepo package named by a marker file is captured by
      walking: whole files, no base, and only the paths the review named, inside
      a repository that could have answered better.
- [ ] product: whether a rename should read as a rename. 004 captures it as a
      delete and an add, which is what a path-to-content list holds either way.
- [ ] product: file modes. Nothing records them, so a change that only sets a
      bit shows as a file with no changed lines.
- [ ] product: whether a file too large to send to a phone should be captured at
      all. Today its bytes are stored and the surface refuses to send them,
      which costs disk for something nobody opens.
- [ ] product: whether a review-level comment and a verdict note are one
      concept. They overlap almost completely and differ only in when they are
      written.
- [ ] product: whether the daemon exits when nothing is outstanding, meaning no
      open reviews and no connected agents. architecture.md names it as a
      smaller rule than running until stopped, and does not adopt it.
- [ ] product: what a marker file holds beyond naming a project root, if
      anything ever needs a second field.
- [ ] product: say what an agent declares as its base commit once it has
      committed its own work. A capture is the delta from the base, so a base of
      `HEAD` after committing is an empty review and a base of the branch point
      is not.
- [ ] tooling: rotating or capping `eyeball.log`. It is appended to forever,
      which at a few lines per request is assumed harmless for a long time.
- [ ] tooling: a tsnet node whose `Start` fails late can leave parts of itself
      running until the process exits, and eyeball never closes a failed node,
      so a daemon whose surface failed that way holds them until it stops.
      `Sys()` tells a late failure from an early one, after which `Close`
      dereferences nil in v1.102.3. Whether `Close` is safe after a late one,
      where it would close again what `Start`'s cleanup closed, is not checked,
      and it would not reach an engine whose backend was never built.
- [ ] tooling: content-hashed addresses under `/static/` with a long cache
      lifetime. Every surface response is `no-store`, so htmx's 52 KB and the
      bundle are fetched on every page load. Assumed small on a tailnet, not
      measured. It changes how 006 serves `app.css` and `shell.css` too.
- [ ] tooling: HTTP/2 on the tailnet listener. tsnet's `ListenTLS` offers no
      `NextProtos`, so the phone speaks HTTP/1.1 and each open tab's live stream
      holds one of the browser's few connections to the host. Advertising `h2`
      means eyeball wrapping the listener in a TLS configuration of its own.
- [ ] tooling: answer a collapsed card's body from that file's comparison alone.
      The cards route builds the whole round's page and takes one card, so each
      body opened on a large review compares every file again.
- [ ] tooling: batch the project view's reads of HEAD into one
      `git cat-file --batch`. Each working-copy row runs its own `cat-file blob`
      and its own `check-ignore`. 196 sequential `cat-file blob` calls took 259
      ms where one `--batch` over the same objects took 5 ms, and a page of 200
      rows took about 450 ms in all, both measured.
- [ ] tooling: whether the daemon and CLI need testing on macOS. Nothing in 005
      is Linux-only by intent and nothing is tested elsewhere.
- [ ] design: highlighting a line on its own loses state that spans lines, such
      as a block comment. Highlighting the whole file and slicing it would fix
      it, at the cost of reading both whole files for every hunk. Assumed rare,
      not measured.
- [ ] design: a tearout of the queue filter. product.md says the queue can be
      filtered, every queue row is one link so a filter link inside a row would
      split it, and no tearout shows a filter control.
- [ ] design: a way from a populated queue to a project's view. product.md says
      the view is reachable from the queue, and only the empty queue's project
      rows link there. Every populated row is one link to its review, so a
      project link inside it would nest links, the problem the queue filter has.
- [ ] design: a mark on the last line of a file with no final newline. The row
      model carries the flag, and no tearout or class in `app.css` draws it.
- [ ] design: the queue's Waiting order. 003 lists the longest wait first,
      because that is what holds an agent up, and both queue tearout frames list
      the newest first. The code follows 003 until the owner chooses.
- [ ] design: a hunk header naming the function it is in, as the review tearout
      shows. `diff.Hunk` carries no section heading, and finding one means a
      rule per language or git's own funcname patterns.
- [ ] docs: a deployment document, once there is something to deploy and a
      decision about where it runs.
- [ ] tooling: whether the tearout server survives once the real binary can
      serve `docs/design/` in a dev mode.
- [ ] tooling: whether the spec-reference rule can be checked. A three-digit
      spec number outside parentheses is mechanical to find. Whether a name sits
      beside it is not.
- [ ] design: the tree's back link one level below the root. The topbar there
      reads "‹ Files Files", a back label and a title that say the same word,
      where the approved browse frame in widening.html reads "‹ Review Files".
      The code goes back to the parent directory, as 007's todo asked. The
      options are the frame's "Review", the parent named as "harbor" or "/", or
      a title naming the directory instead of "Files".
- [ ] design: the back link on a file the review changes reads "Review", or
      "Round N" on an earlier round, where the step-two frame in widening.html
      reads "Change". The address decides the label, so it says where the link
      goes. The frame's word says what the reader was looking at.
- [ ] design: the back link on a file opened for context names its directory,
      where the step-three frame in widening.html names the file it was opened
      from, `worker.go`. The server cannot know where a reader came from, since
      an address renders the same however it was reached, so the frame's label
      needs the referring file in the address or is dropped.
- [ ] design: the order of rows. A directory in the tree lists its files by
      name, and the working copy by path, while the browse frame in
      widening.html puts the bold files first and the populated frame in
      project.html puts claimed files first. Either the frames or the code
      moves.
- [ ] design: a markdown file outside the review's paths shows its Document and
      Source control alone at the left of the toolbar, with no counts beside it.
      No frame draws a toolbar without counts. The control could sit at the
      right, where it sits beside counts, or the toolbar could say why there is
      nothing to count.
- [ ] design: an empty file. Its page shows an empty code card with no line
      saying the file is empty, and a new empty markdown file alone in its round
      gets the fresh layout with an empty `.prose`. No frame draws either. An
      "Empty file" line like 006's binary line is one option.
- [ ] design: the placeholder for a block of HTML in a document, "This block is
      HTML, which is not rendered. Read it in Source.", in the hint style. No
      tearout draws it, and its wording has been seen only in headless Chromium
      at 390 pixels, not on a phone.
- [ ] design: the fresh layout on a later round, where it keeps the "Since round
      N" control so the whole review is still reachable. The fresh frame draws
      round 1, which has no control, and this layout has not been seen on a
      phone.
- [ ] design: a link from an identifier in a hunk to its definition, the notice
      in widening.html's step-one frame. Finding a definition means a rule per
      language, or an index nothing builds yet.
- [ ] design: marking the items of a list one at a time in a document. 007 marks
      a whole list as one block, so one changed item marks every item.
- [ ] product: a lost base on the review page. `capture.Compare` fails when the
      commit a review started from is gone, so the review page fails while its
      file pages fall back to the capture. Fixing it changes how a round is
      measured.
- [ ] product: a marker project whose outer checkout's `.git` moved away before
      eyeball first saw the project reads as a project outside git, and its live
      reads follow no ignore rules. `projects.inside_checkout` catches the move
      only after a first sighting, and nothing on disk tells the two apart.
- [ ] tooling: clearing `projects.inside_checkout`. It is set and never cleared,
      so a project flagged by a `.git` above it, such as a dotfiles repository
      in a home directory, loses live browsing for good once that `.git` goes.
      The recovery today is editing the database.
- [ ] tooling: move `capture.Walk` and `Freeze` onto `os.Root`, as every live
      read and listing already is. They read for an agent's own request and
      write to the blob store, so 007 left them on plain paths.
- [ ] tooling: a directory that can be searched but not read, such as mode 0300,
      is listed in its parent, and its own tree page is a 500, because reading
      it fails with a permission error nothing maps to not found. Not tested.
- [ ] tooling: a `.git/HEAD` holding garbage passes the lost check, which looks
      only for the file, and git then refuses the repository, so every route
      under that root would be a 500. Reasoned in 007, not tested. A HEAD naming
      a commit that does not exist is handled on the project view, which says
      the working copy cannot be listed.
- [ ] design: commenting from a file page: the whole file, the tree's file and
      the project's live file. 008 puts the composer on the review page only,
      where the bar and the tally are.
- [ ] design: a decided round gives no sign that its lines take no tap. The
      owner spent a while tapping lines on approved reviews before finding that
      only open rounds open a composer.
- [ ] design: the top bar's eye on a page other than the queue counts what
      waited when the page loaded, and does not follow the stream. 015 left it:
      a reviewer reads the count on arrival, and the queue corrects it.
- [ ] design: a manual theme override. docs/design/README.md says one wins over
      the device in both directions, and no tearout draws a control for it.
- [ ] design: a comment added on a second device shows on the first only after a
      reload. 008's stamps leave comment counts out, so a reviewer's own comment
      does not reload the page that wrote it.

## Open questions

Not repeated here. Product questions live in
[docs/product.md](../docs/product.md) under **Open questions**, and build
questions in [docs/architecture.md](../docs/architecture.md) under **Not decided
here**. Both are where they get answered, and a copy in a third place drifts.
