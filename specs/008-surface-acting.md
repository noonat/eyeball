---
status: active
created: 2026-10-05T00:00:00Z
updated: 2026-10-06T11:49:44.475152256Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
---

# 008 The surface: acting

> **Autonomous run, 2026-10-04.** Part of the owner's request to build the
> project to completion as product.md and architecture.md describe it, under
> `dev-review autonomous on`. A reviewer subagent stands in for the owner's
> approval of this spec and gives the `reviewed` ack on each iteration. The
> owner keeps tearout changes, questions about behavior, and anything
> outward-facing. Iteration 1 changes tearouts, so it also carries
> `phone-approved`, an ack only the owner gives after looking at the changed
> frames on the phone, and the run stops there for it. The owner answered this
> draft's three questions on 2026-10-06, and each answer is recorded where it
> applies. The run stops a second time when iteration 5 closes, for the
> whole-branch review that iteration 6 is, as described there. Departures from
> this spec are recorded in it as they happen.
>
> **Changed on 2026-10-06, before execution.** To stay within a usage limit, the
> owner ended subagents for the rest of the run: the agent driving it implements
> and reviews every iteration itself, and the owner's word stands as approval
> for every ack until the owner says otherwise. The owner turned the review gate
> off with `dev-review disable`, so commits carry no per-commit review record.
> Where this spec still says a subagent or the stand-in does something, the
> driving agent does it.

The reviewer's surface, third part: acting on what 006 and 007 made readable. A
comment at any of the four levels, edited or deleted before the verdict, the
decision bar with its note and its refusal, the comment box on a decided round,
the live stream and the notice when it is gone, a failure reported on the
control that failed, a failed save that keeps its text, the theme following the
device, file bodies loaded when their disclosure opens, and a review reopening
where it was left. Under all of it sit htmx, TypeScript compiled by esbuild in
the binary, and the `tsc` and `oxlint` gates.

This is item 1 under Next in specs/TODO.md. It fits thirteen iterations without
a split, one of them a review. They are ordered so the loop from reading to a
released agent works on the loopback listener at the end of iteration 5: a
comment written on the page, a verdict given from the bar, and an agent's
`eyeball wait` returning both. A review of the whole branch since 006 follows at
once, before the owner tries the loop. Passages, the live stream, the failed
save and the remembered place come after it.

Push, the installed page and the owner's phone check of the built surface are
notification (009). Images, media and the preview sandbox are rendered files
(010).

Every claim marked confirmed was run against Go 1.26.8, Chromium 152.0.7977.82,
htmx 2.0.11, TypeScript 7.0.2, oxlint 1.87.0 with oxlint-tsgolint 7.0.2003,
esbuild v0.28.2 and goldmark v1.8.6, in throwaway programs outside the
repository. Probes, below, describes each so a review pass can run it again. No
review pass has run on this draft.

## Design

### Who may act

The owner decided on 2026-10-05:

> The loopback listener may record a verdict or a comment only with a random
> token that `eyeball serve --surface-addr` prints to the terminal. Opening the
> printed URL once sets a cookie; without it the acting routes refuse.

Two options were rejected. A read-only loopback listener would have made acting
testable only on the phone, after `eyeball tailnet login`, so testing this spec
would have waited for notification (009). A loopback listener that can decide
with no token would let any local account, or a casual `curl`, approve a review.
The owner's reason was the one the recommendation gave: the token blocks other
local accounts and a casual `curl`, and keeps acting testable on this machine
before 009.

The owner corrected where the token lives on 2026-10-06, after this spec's first
draft measured the cookie's reach:

> The token lives in `localStorage` and is sent as a request header on every
> write, not a cookie.

The reason is the measurement. Cookies ignore the port: in one Chromium session,
a page served from a second loopback port received the cookie on navigation,
with `Sec-Fetch-Site: same-site`, confirmed. `SameSite=Strict` cannot separate
two ports, which are one site. So another account that got the owner's browser
to open a page from its own loopback server would have held the token, and could
then decide with `curl`. `localStorage` is isolated per origin, port included: a
page on the second port read nothing where the first had stored the token,
confirmed. Rejected: keeping the cookie and recording the risk.

**Each run of `serve` draws one token.** With `--surface-addr`, the daemon draws
it from `crypto/rand.Text()`, 26 base32 characters carrying 128 bits, and prints
one line to its stderr once the listener is bound:
`eyeball: the surface is at http://127.0.0.1:8111/#token=…`. The address comes
from the bound listener, since a test binds port 0. A started daemon never
serves loopback, so the line only ever reaches the terminal of the person who
ran `serve`. The token is never in `/agent/daemon`, `eyeball status`, the
daemon's log lines or the manual.

**The printed address hands the token over in its fragment.** A browser sends
neither the fragment nor any part of it to the server, and a `Referer` never
carries one: the request for the page and for its script arrived with neither,
confirmed. The bundle, which runs first in the head, reads `token=` from the
fragment, stores it in `localStorage` under `eyeball:token`, and replaces the
history entry with the address less the fragment, so it leaves the address bar
and the history. The fragment is read only in that form, so a heading's fragment
on a file page is left alone. A query parameter was the alternative. It would
reach the server's request line and any log of it, which is the leak the
fragment avoids.

**The client sends the token as `X-Eyeball-Token` on every write.** A handler
for `htmx:configRequest` adds it to every request but GET when a token is
stored, which on the tailnet's origin it never is. The loopback listener's
handler, before the mux, compares the header with the run's token using
`crypto/subtle`'s constant-time comparison. Every request but GET and HEAD
without a matching header is refused with 403 and one sentence: "This page
cannot write to this eyeball. Open the address `eyeball serve` printed." The
tailnet listener ignores the header. Reads need no token: the owner's decision
covers acting, and 006's accepted exposure of reading through this listener
stands.

**A write without JavaScript has no header, so the loopback listener refuses
it.** A form submitted natively, with no `HX-Request`, gets the same sentence as
a page. The surface acts only through its script on either listener, as
architecture.md's choice of htmx already makes it.

**The header is also what keeps another origin's page from writing on
loopback.** A page on any other origin, another loopback port included, cannot
send a request carrying a custom header without a CORS preflight, and the
surface answers none with permission. A page on the second port that tried was
refused before its request was sent, confirmed. A cookie would have been
attached to a form post from such a page.

**A new token every run, rather than one kept in the state directory.** A kept
token would survive a restart. It would also be a credential in a file for as
long as the state directory exists. The cost of drawing one per run: after every
restart of `serve`, the owner opens the newly printed address, and a write from
a tab holding the old token is refused with the sentence above. Opening the
plain address later in the same browser, without the fragment, keeps the stored
token, confirmed, so one handover lasts the run.

**What deciding through the loopback listener gives a local process.** TODO item
1 asked this spec to say. Another account gets no write: it cannot read the
token from the owner's terminal or from the browser's storage for an origin it
does not serve. A process running as this user gains nothing it lacked: it can
write `eyeball.db` directly, or read the token from the terminal's scrollback or
the browser's profile. No option the owner was offered stops that, and this spec
does not claim to. The token stops a casual `curl` from any account, which
includes an agent on this machine running one.

**Every surface request passes Go's cross-origin check, on both listeners.**
`http.CrossOriginProtection` lets GET, HEAD and OPTIONS through. It refuses any
other request whose `Sec-Fetch-Site` is neither `same-origin` nor `none`, or,
with no such header, whose `Origin` differs from its `Host`. Each case is
confirmed in Probes. On loopback the header already stops another origin. The
tailnet has no token, and without the check any web page the phone opens could
post a verdict to it as a plain form. A CSRF token in every form was the
alternative. It costs a value in every form and a check in every handler, and
buys nothing the browser's own headers do not already give on Chromium. That
Safari sends `Sec-Fetch-Site` is assumed, not measured. A browser that sends
only `Origin` is still checked through the fallback.

**The tailnet listener has no token, and its writes are 009's to decide.** Any
device on the tailnet reaches it. If this machine runs Tailscale on the same
tailnet, its own processes reach the node's `:443` too, and `curl` passes the
cross-origin check, so an agent here could decide through it. That is the other
half of TODO item 1's question. The owner decided on 2026-10-06 that
notification (009) answers it, with tsnet's `WhoIs` in hand, for example by
refusing writes from this machine's own node. Rejected for now: accepting any
tailnet identity, and pairing each device once. 008 leaves the tailnet listener
without a token, and the `product:` entry in specs/TODO.md points at 009.

architecture.md says "Nothing reachable over the network can open a review, read
one as an agent, or release a batch of comments." Once the verdict is on the
surface, a device on the tailnet can release a batch, by deciding. Iteration 3
corrects the sentence: the agent API never touches the tailnet, and the
reviewer's surface, which now releases batches, is reachable from every device
on it.

**The preview sandbox still sees no cookie, and now there is a stored
credential.** architecture.md's argument against a second port for previews
rests on cookies ignoring the port, and says "There is no cookie today". That
stays true. What changes is that the loopback origin's `localStorage` holds the
token. A frame sandboxed with `allow-scripts` alone, served from the surface's
own origin, got `SecurityError` reading `localStorage` and `document.cookie`,
and its POST went out with `Origin: null` and `Sec-Fetch-Site: cross-site`,
which the cross-origin check refuses, confirmed. `allow-same-origin` would give
the frame the surface's storage, token included. architecture.md already ties
that flag to a separate origin, and iteration 3 adds the token to its reasons.

### The routes

Every route joins the surface's side of the table in
`internal/server/server.go`, so 006's listener test covers it.

| Pattern                                                | Answers                                                | Iteration |
| ------------------------------------------------------ | ------------------------------------------------------ | --------- |
| `GET /static/htmx.js`                                  | the vendored htmx, from the binary                     | 2         |
| `GET /static/app.js`                                   | the TypeScript, bundled once from the binary           | 2         |
| `POST /reviews/{review}/comments`                      | one comment on the round the form names                | 4         |
| `PUT /reviews/{review}/comments/{comment}`             | a comment's new body, before the verdict               | 4         |
| `DELETE /reviews/{review}/comments/{comment}`          | drop one before the verdict                            | 4         |
| `POST /reviews/{review}/decision`                      | the verdict on the round the form names                | 4         |
| `GET /reviews/{review}/composer`                       | a composer anchored where the query says               | 4         |
| `GET /reviews/{review}/comments/{comment}`             | a comment's card, or with `?edit=1` its edit form      | 4         |
| `GET /events`                                          | the live stream, and therefore the connection          | 7         |
| `GET /reviews/{review}/rounds/{round}/cards/{path...}` | a collapsed file card's body, on its disclosure's open | 9         |

architecture.md's table names the comment, delete, decision and stream routes.
The rest are new, and iteration 13 adds them there. Editing is in product.md and
had no route, so it takes `PUT` on the comment's own address. The composer and
the card come from the server because a composer anchored to a line is a piece
of the page the server already knows how to render, and architecture.md's
argument against client-side state covers it. The cards route exists so a body
can load on its own. The file page at `/files/` answers a whole page, and a
second answer at that address would make one address mean two things. All ten
patterns register beside the existing ones with no conflict, confirmed.

**Every write names the round the page showed.** A form carries `round`, the
round's number. The handler resolves it within the review in the path, and a
round of another review, or none, is a 404. A comment is stored on that round
whatever has happened since: decided, abandoned, or followed by a newer round,
it is a late comment and goes out on its own at once, as a late comment does.
Refusing it would discard what somebody typed, which product.md rules out. A
verdict is refused unless its round is the review's latest, undecided, and its
review open, so it is never recorded against a round the reviewer did not see.
The store's own `Decide` takes a round id and cannot know which round was on
screen.

**Quoted text comes from the server.** A comment form carries a path and lines,
never text. The handler reads the round's frozen content through
`capture.State.Reader`, as 007 requires of every page, and quotes the lines from
it. A quote sent by the page would be a second copy of text the server holds,
and nothing would check that the two agree. A removed line, below, is quoted
from the side of the comparison it was removed from.

**A comment write carries a submission key, and a repeated key stores nothing
new.** A retry after a timeout or a lost answer cannot tell whether the first
send was stored. The client draws a key with `crypto.randomUUID()` when a
comment is first sent and sends the same key on every retry. Migration `0006`
adds `comments.submission`, unique where it is not empty, and `AddComment` given
a key it already holds returns that comment. An in-memory table of recent keys
was the alternative, and a retry after a daemon restart would then store the
comment twice. An edit is idempotent already, and a repeated delete answers 200
for a comment already gone.

**What each answer gives the page.** No answer reloads the page while text may
be on it. htmx acts on `HX-Refresh` before it reads the status, and before any
handler of the client's runs except `htmx:beforeOnLoad`, read in its source, so
a 409 carrying it reloaded the page over whatever was typed, confirmed by the
review's re-run. Only a recorded verdict carries it, and a verdict is sent only
once the guard below has found no unsaved text. Every other change the page has
not seen reaches it through the stream's stamp, whose reload waits for unsaved
text.

| Case                                                  | Status            | The page gets                                                     |
| ----------------------------------------------------- | ----------------- | ----------------------------------------------------------------- |
| A comment stored, or a key already stored             | 200               | the anchor's comment list, the tally, composer closed             |
| A comment on a round decided, abandoned or superseded | 200               | the same, the card marked late; the stamp moves and reloads later |
| An edit stored                                        | 200               | the card                                                          |
| A comment deleted, or already gone                    | 200               | the card removed, the tally                                       |
| A change request with no comment and no note          | 422               | the bar holding the refusal notice, the note kept                 |
| A verdict recorded                                    | 200, `HX-Refresh` | the decided page, by reload                                       |
| A verdict on a stale round                            | 409               | a plain sentence beside the bar, the note kept, the reload later  |
| An edit of a released comment                         | 409               | a plain sentence beside the form, the text kept, the reload later |
| A delete of a released comment                        | 409               | a plain sentence beside the card                                  |
| A round or a comment of another review, or none       | 404               | a plain sentence, shown on the control                            |
| A form the page could not have sent                   | 400               | a plain sentence, shown on the control                            |
| No matching token header on the loopback listener     | 403               | the token sentence, shown on the control or as a page             |
| A cross-origin request                                | 403               | Go's own answer                                                   |
| A failure                                             | 500               | a plain sentence, shown on the control, and logged                |

Every row keeps what was typed: a stored comment's text is in its card, a
refused one stays in its form or, failing to send, in an unsent card, below.

**A comment write answers with the tally, not the bar.** The tally is swapped
out of band by its own id. Swapping the whole bar would replace the verdict's
note while somebody was typing it. The 422 refusal is the one answer that
replaces the bar, and it carries the note back as it was sent, so nothing typed
there is lost either.

**An edit or a delete names a comment that belongs to the review in the path.**
`db.CommentReview` already answers which review a comment belongs to. A comment
of another review is a 404, so one review's page cannot edit another's comment.

**A surface write that fails answers in plain text when htmx sent it.** 006's
`failurePage` renders a whole page, which htmx would refuse to swap and the
failure handler could not show. A request carrying `HX-Request: true` gets the
sentence "eyeball could not do that. The daemon's log says why.", and the error
is logged as `failurePage` logs it.

**Iteration 4 builds every fragment, and iteration 5 places them.** The bar, the
tally, a comment list, a card, the edit form and the composer are template
blocks written in iteration 4, where the write handlers answer with them and
tests parse them. Iteration 5 puts them on the page and wires the taps.

### The client: htmx and TypeScript

**htmx 2.0.11, vendored, not 4.0.0.** The npm registry's `latest` tag is 2.0.11
and `next` is 4.0.0, confirmed on 2026-10-05. 4.0.0's build names one error
event, `htmx:error`, where architecture.md's connection layer is written against
2.0's three: `htmx:sendError`, `htmx:responseError` and `htmx:timeout`,
confirmed in both builds. `htmx.min.js` is 52,182 bytes, 16,861 gzipped,
confirmed, and the server sends it uncompressed. architecture.md's "about
fourteen kilobytes" is corrected in iteration 13. The license is 0BSD, which
asks for no notice. The script and its `htmx.esm.d.ts`, from the same package,
live in `web/vendor/` with the version and the script's sha256 written beside
them, and a test holds the file to that hash so an edit is noticed. oxfmt and
oxlint skip `web/vendor/`, since formatting a vendored file makes it differ from
the release it names.

**htmx's configuration is part of the design, and a test holds it.** The layout
carries an `htmx-config` meta, read by htmx at load, with these keys:

- `includeIndicatorStyles: false`. htmx otherwise injects a `<style>` element,
  which the surface's policy refuses, confirmed as a policy violation in
  Chromium.
- `allowEval: false`. Nothing here uses the features that evaluate code, and the
  policy would refuse them.
- `historyEnabled: false`. Nothing pushes an address today, and an htmx history
  entry stores page HTML in `localStorage`, which would be the offline copy
  architecture.md rules out.
- `timeout: 10000`. The default is 0, confirmed in htmx's source, so a request
  that hangs never reports anything. Ten seconds is a guess written down so it
  can be corrected.
- `responseHandling` set to exactly
  `[{"code":"204","swap":false},{"code":"[23]..","swap":true},{"code":"422","swap":true,"error":false},{"code":"[45]..","swap":false,"error":true}]`.
  By default a 422 is not swapped, confirmed, so the refusal notice would never
  reach the bar. The 422 entry must say `error: false`, or htmx would also fire
  `htmx:responseError` and the failure handler would write a line beside a
  refusal it had just shown. Every other 4xx and 5xx must say `error: true`, or
  the failure handler never hears of it.

A test parses the layout's meta as JSON and checks each key, the four entries in
that order, and their `swap` and `error` flags.

**The scripts are sent with `no-store`, like every response, and that is
accepted for now.** 006 put `no-store` on every surface response because iOS
Safari otherwise showed a stale page. So the 52 KB of htmx and the bundle are
fetched on every page load, and the stream's reloads make page loads more
frequent. Content-hashed addresses under `/static/` with a long lifetime would
remove the cost, since a hashed address cannot go stale, and would change how
006 serves `app.css` and `shell.css` too. The cost is assumed small on a
tailnet, not measured, so it is a `tooling:` entry in specs/TODO.md.

**TypeScript sources live in `web/ts/`, bundled at startup by esbuild linked
into the binary.** `assets.go` embeds `web/ts` and `web/vendor/htmx.min.js`. The
root package builds the bundle once, behind a `sync.Once`, with esbuild's Go
API: one entry, `web/ts/main.ts`, with a plugin that resolves imports and loads
files from the embedded tree, as an IIFE targeting ES2022. Two files bundled
that way in 1.7 ms, confirmed. A test builds the bundle and requires no errors
and no warnings, so a source esbuild cannot read fails `make check` rather than
a daemon's first page. A daemon whose bundle failed anyway answers
`/static/app.js` with 500 and logs why.

esbuild added 9.4 MB to a trivial binary, from 2.5 MB to 11.9 MB, confirmed.
eyeball builds to 57.4 MB today. architecture.md already accepts the cost.

**Imports are written without an extension.** tsc 7.0.2 refuses `./theme.ts`
unless `allowImportingTsExtensions` is set, confirmed, and the bundle plugin
appends `.ts` to a path without one. One spelling for both keeps the two
resolvers agreeing.

**`tsc` and type-aware `oxlint` run in `make lint`, through `npx` with pinned
versions.** `web/tsconfig.json` sets the options conventions.md lists, which tsc
7.0.2 accepts, and it caught an unchecked index under them, confirmed. oxlint
1.87.0 with oxlint-tsgolint 7.0.2003 flagged a floating promise under
`--type-aware`, confirmed. Both run as oxfmt already does:

```make
NPX = npm_config_prefer_offline=true npx --yes
OXFMT = $(NPX) oxfmt@0.65.0
TSC = $(NPX) -p typescript@7.0.2 tsc
OXLINT = $(NPX) -p oxlint@1.87.0 -p oxlint-tsgolint@7.0.2003 oxlint
```

With no network, each `npx` call with a cached pinned version took about 70
seconds before running, confirmed under `unshare -rn`. With
`npm_config_prefer_offline=true` it took 0.4 seconds, online or not, confirmed.
A pinned version on npm never changes, so skipping the registry's check costs
nothing. The setting applies to oxfmt as well, which today pays the same wait.

A `package.json` with a lockfile and `node_modules` was the alternative. It
would be a second way to fetch tools beside oxfmt's, and a directory to keep out
of every file list.

**The global `htmx` is typed from the vendored declarations.** A one-line global
declaration gives it the type of `htmx.esm.d.ts`'s default export. `htmx.ajax`
and the event details then type-check, and tsc caught a misuse of an event's
detail, confirmed.

**Classes the TypeScript writes are in one table.** `web/ts/classes.ts` exports
them as a frozen object, and a Go test reads its string values and holds them to
`app.css`, as `render.Classes` is held. `template-classes` cannot see a class
added at runtime. The list is closed by iteration 1: `sel`, `failed`, `why-not`,
`offline` and `retry`, which app.css defines today, and whatever iteration 1
adds for the note. A class the code needs after that is a tearout change.

**The theme follows the device.** `app.css` switches only on `data-theme`, so
the bundle sets it from `prefers-color-scheme` and follows a change. The bundle
is loaded by a plain `<script src>` in the head, without `defer`, and sets the
theme at its top level before anything else. The policy refuses inline script,
and a deferred script would paint light first on a dark device, on every load.
The rest of the bundle waits for `DOMContentLoaded`. A manual override, which
docs/design/README.md mentions, has no control in any tearout, so it is a
`design:` entry in specs/TODO.md.

### Comments

**Text is typed in four places, and the page treats them alike.** The review box
under the note, one anchored composer that a tap opens, an edit form, and the
verdict's note in the bar. Unsent cards, below, hold text too. The verdict
guard, the bar's notices and the deferred reload each ask one question of the
page: does any composer, edit form or unsent card hold text that is not stored.
The note is not a comment, so it does not hold back a verdict, but a reload
waits for it too, and no answer to a comment write replaces it.

**Every field takes 16-pixel text.** The owner decided on 2026-10-06 that
comment boxes use 16-pixel text so iOS Safari does not zoom the page when one
takes focus, and that the change is judged on the phone in iteration 1.
Rejected: keeping `app.css`'s 13 pixels and accepting the zoom. The zoom under
16 pixels is a known iOS behavior, assumed here and not measured. The verdict's
note is a field like the others and takes the same size.

**One anchored composer on a page.** A tap on a line opens it under that line's
file, with the line marked `sel`. A tap on another line moves the same composer
there and keeps what was typed. The chips switch its level among the line, the
file and the review without a request: they set a hidden field and
`aria-pressed`, and the text stays where it is, which is what the tearout's note
asks for. A second anchored composer for each tap would leave half-written
comments scattered through a long review, each one easy to forget.

**The composer opens after the file's diff, as the large-change frame draws it,
and scrolls into view.** The owner decided on 2026-10-06: it stays after the
diff, is scrolled into view when it opens, and the tapped line stays marked
`sel`. Rejected: opening it under the hunk, which is a tearout change and would
put the box inside the table, scrolling sideways with the code.

**Comments and the composer sit under the line they are about.** The owner
decided on 2026-10-06, after trying the loop on loopback, that a line's comments
and the composer a tap opens appear in a row under that line, and a passage's
under that passage. This reverses the owner's earlier decision of the same day,
recorded above, to keep the composer after the file's diff and scroll to it: on
a long diff that put a comment far from its line. The row is a `tr.thread` under
the tapped row, its body pinned to the card's visible width while the code
scrolls sideways beneath it. File-level and review-level comments stay where
they are. Iterations 7 and 8 draw and build it, and the paragraph above stands
as the decision it replaced.

**A composer opened in place is not a card of its own.** Under a line or a
passage it drops the border, shadow and padding a free-standing composer has,
and its field fills the row. The owner found, on the phone on 2026-10-06, that a
margin, the card's padding and then the field's own border cost too much of a
phone's width. Under a passage, a rule in the accent color takes the card's
place in tying the box to what it is about.

**The anchored composer has Cancel**, which closes it and drops the line's
outline, as the owner asked on 2026-10-06. The approved comments frame already
drew Cancel beside Add.

**A card's way out to the file's own page is an icon in its header.** The owner
asked on 2026-10-06 for "Whole file" in the file header rather than the card's
foot, where 007 put it. It is an icon alone, `maximize-2` for a code file and
`file-text` for a document's "Raw diff", labeled for screen readers: the path
already truncates on a phone, and a labeled button would take about 80 pixels
from it where the icon takes 28.

**What a tap anchors to:**

| Tapped                                            | Level   | Range                           |
| ------------------------------------------------- | ------- | ------------------------------- |
| An added or unchanged row                         | line    | its new-side number, one line   |
| A removed row                                     | file    | none, the removed line quoted   |
| A hunk header                                     | nothing | none                            |
| A binary or too-large file's line                 | file    | none                            |
| A rendered block with content lines               | passage | its first and last content line |
| A thematic break, an empty fence, a removed block | nothing | none                            |

**A tap on a removed row opens the file-level box with that line quoted.** The
owner decided this on 2026-10-06. Rejected: leaving removed rows untappable. A
removed line has no number in the round's frozen file, so the comment carries no
range. The form sends the row's old-side number and the comparison, and the
handler quotes the line from the side it was removed from, the previous round or
the base.

The store refuses that today. `checkComment` rejects quoted text on a file-level
comment, and the schema's `(start_line IS NOT NULL) = (quoted IS NOT NULL)`
refuses a quote with no range, both read in the code. Migration `0006` rebuilds
`comments`, since SQLite cannot alter a check, so that a file-level comment may
carry a quote with no range and every other rule stands, and `checkComment`
follows it. Putting the quote into the body was the alternative, and it would
reach the agent as the reviewer's words rather than as quoted text. The manual
says that a file-level comment carrying a quote quotes a line the round removed.
The rebuild is assumed to keep every row and its id, and iteration 4's migration
test reads a comment written before it back after it.

A range of lines is product.md's open question, and the composer takes one line
until a tearout answers it, as the existing `design:` entry says.

**A passage's range comes from the parse the page already shows.** `Document`
walks the shown parse's top-level blocks. Each block's range is the first and
last source line of any content segment beneath it, carried on the block as
`data-lines`. A third parse for ranges would be another pair of passes with
nothing checking that they agree. What goldmark gives, confirmed: a paragraph
spans its wrapped lines, a list spans its nested items, a table spans its rows
and delimiter, and an HTML block spans its lines. A fenced block's range leaves
out the fences, a setext heading's leaves out its underline, and a thematic
break has no content line at all, so it takes no comment. An empty fenced block
has none either. A removed block is not in the round's file. The quote is those
source lines from the frozen content.

**A tap anywhere on a passage selects it, and only the hint shows what it
replaced.** The owner decided this on 2026-10-06. Rejected: keeping a tap on a
changed passage as the way to reveal its replaced text, and giving each passage
a separate control to comment. On a changed block, whose summary toggles the
replaced text, the client cancels the summary's toggle unless the tap landed on
the hint, "Show what it replaced" or "Hide what it replaced". A link inside a
passage still follows: a link that went nowhere would be a control that does
nothing, so this spec reads "anywhere" as everywhere but a link. Without the
script, the whole summary still toggles, as 007 built it.

**The review level is always on the page.** An open round shows the review
composer under the agent's note, where the decided frame shows its box. Reaching
it through a tapped line would leave a review of only binary files with no way
to make the comment product.md calls the most important one.

**Edit and delete act in place.** Edit swaps the card for a composer holding its
body, with Cancel and Save. Delete removes the card and needs no confirmation:
nothing has been released, and the tally changes where the reviewer is looking.
Both update the bar's tally.

**A decided current round keeps the review-level box, as drawn.** It says the
comment sends immediately, and no other level is offered on it. An earlier round
has no box, and an abandoned review has neither box nor bar: its agent withdrew
the question.

### Deciding

**The bar is on an open current round only.** It carries the tally, as the
tearout writes it, "No comments yet" or the count with "Ready to send", a note,
and both verdicts. The verdicts are two submit buttons in one form, each with
`name="verdict"`, and htmx sends the pressed one's value with the note,
confirmed.

**The refused change request is the bar again**, holding the warn notice,
at 422. Both verdicts stay live, as the frame's note says.

**A recorded verdict reloads the page.** The answer carries `HX-Refresh`, which
htmx acts on before it looks at the status, read in its source and confirmed for
a 200. The decided page is a different page, with a notice, a verdict note and
the late box in place of the bar. Swapping that much in place is a second way to
render it.

**A verdict is not sent while any composer, edit form or unsent card holds text
that is not stored.** For typed text the bar shows a danger notice, "A comment
is still being written. Add it, or cancel it, before deciding.", and for unsent
cards the approved one, "1 comment did not save. Retry it, or delete it, before
deciding.", with the count. Both verdicts are marked unavailable while either
shows. A verdict sent then would release the round without that comment, and
product.md says the surface never discards what somebody typed. The guard for
typed text ships in iteration 5, because the owner tries the loop from iteration
6, and the unsent cards and their notice in iteration 11.

**A verdict refused as stale keeps its note.** The 409 carries a sentence saying
the round was decided or withdrawn since the page loaded. The failure handler
writes it beside the bar, the note stays in its field, and the stream's stamp,
which moved with the change, reloads the page once nothing on it is unsaved.

**The socket cannot reach a verdict.** The routes are on the surface's side of
the table, and 006's listener test proves a pattern matches on its own listener
only. Iteration 4 extends it: each write is sent to `AgentHandler` as a real
request, gets 404, and leaves the round undecided and no comment stored.

### When a request fails

**Every htmx request that fails says so beside its control.** One handler for
`htmx:sendError`, `htmx:responseError` and `htmx:timeout` writes a `failed` line
next to the element that made the request: the form's foot for a composer or the
bar, the acts row for Edit or Delete. The text is the server's sentence for a
plain-text answer, and otherwise one of three fixed ones, for no answer, an
error and a timeout. A closed connection fired `htmx:sendError` with status 0
and a 500 fired `htmx:responseError` with no swap, confirmed. This arrives with
the first write controls, in iteration 5, because hands-on testing with taps
that fail silently is the failure product.md ranks worst.

**A failed save becomes an unsent card.** The owner decided on 2026-10-06 that a
failed save follows the approved foundations frame: the comment appears in its
list as the dashed `.comment.unsent` card with Retry and Delete, the box closes,
and several can wait at once. Rejected: keeping the text in the box with Retry
and Discard. Until iteration 11 builds the card, htmx swaps nothing on an error,
so the box still holds what was typed and the failure line says why.

The card holds what the form held: the round, the level, the path, the line, the
text and the submission key. Retry sends the same form with the same key, so a
send that was stored after all is not stored twice. Delete removes the card and
sends nothing, since nothing was stored. An edit whose save fails keeps its form
with the failure line, and its Save sends again.

### The live stream

**`GET /events` is a Server-Sent Events stream, and its state is the
connection.** The handler subscribes to the hub before reading, sends
`retry: 2000`, then the page's stamp, then a comment line every fifteen seconds
so an idle stream is told from a dead one, and flushes after each. It ends when
the request ends or when `BeginShutdown` closes `s.stopping`, as a wait does. A
handler that ignored it held `http.Server.Shutdown` for its whole grace,
confirmed, and every stop with a page open would wait out `ShutdownGrace`. The
fifteen and two seconds are guesses.

**A stream carries a stamp, and a page reloads only when its stamp moves.** The
hub publishes on every write in every project, including an agent's receipt. A
page that reloaded on every signal would reload constantly, and in the middle of
typing. So the stream takes three forms. `/events?review=N` sends a stamp for
that review, `/events?queue=1` sends one for the queue, and plain `/events`,
which every other page opens, sends keepalives only. After each signal the
handler recomputes the stamp and sends it when it changed. A review page and the
queue print the same stamp on their body, and the client compares only when the
body carries one, so a file page never reloads on a stamp meant for another
page.

- A review's stamp is its state, its latest round's number, whether that round
  is decided, and the latest time any of its comments was replied to.
  `store.Reply` replaces an earlier reply and stamps `replied` again, read in
  the code, so a count of replies would not move when a reply changes.
- The queue's stamp is every row it lists, as review id, state, latest round and
  whether that round is decided.

A comment count is in neither. The reviewer's own comment would otherwise reload
the page that wrote it. So a comment added on a second device shows on the first
only after a reload, which is a `design:` entry.

**A reload waits while any composer, edit form or the verdict's note holds
text.** It happens once the text is stored, sent or discarded.

**A reload rather than swapped regions.** architecture.md says the queue and an
open review update without a refresh. Swapping regions in place means choosing
regions that never hold the composer, a file body or the scroll position, and
keeping that choice correct as the page grows. A reload that saves and restores
the place, from iteration 12, keeps one way to render a page. Until iteration
12, a reload starts at the top of the page.

**The notice appears after two seconds without an open stream, or at once on
`offline`, and goes when the stream opens.** It is the `.offline` bar under the
topbar, with Retry reopening the stream. The client inserts and removes it
rather than toggling `hidden`: `.offline` sets `display`, which outranks the
browser's own rule for `hidden`. Two seconds keeps a phone waking from sleep
from flashing it, and is a guess. `navigator.onLine` is read only for its
`false`, as architecture.md says.

**While the notice shows, the verdicts are marked unavailable and say why**, as
the connection frame draws them. `aria-disabled` does not stop a button, so the
client also cancels the verdict form's request in `htmx:beforeRequest`. The
guard asks the page's state, the stream down or text unsaved, rather than the
element: for a form, htmx reports the form as the element, not the button that
was pressed. The composer stays live while the notice shows, as drawn, and
reports its own failure. architecture.md says every write control is marked
unavailable while the stream is down. The approved connection frame keeps the
composer live, so iteration 13 corrects architecture.md, as a correction, rather
than recording it as matching what was built.

**Each tab holds one connection for its stream, on both listeners.** Over
HTTP/1.1 a browser opens a few connections to one host, six in Chromium, which
is assumed, not measured. Enough open tabs would leave the next request waiting
behind their streams. The tailnet listener speaks HTTP/1.1 too: tsnet's
`ListenTLS` builds its `tls.Config` with `GetCertificate` alone and no
`NextProtos`, read in v1.102.3's source, so no client is offered HTTP/2. An
earlier draft assumed HTTP/2 there, and that was wrong. Advertising it would
mean eyeball wrapping the listener in a TLS configuration of its own, which is a
`tooling:` entry. One reviewer with a few tabs is assumed to stay under the cap.

### Loading and remembering

**A collapsed file card's body loads on its first open.** Past the thresholds
006 set, a card renders closed with an empty body carrying `hx-get` to the cards
route, keeping `against`, and `hx-trigger="toggle once"`. One request on the
first open and none on closing and opening again, confirmed in Chromium. A body
that fails to load gets the drawn failure line in the card, with its `retry`
link, and no new class.

**A review reopens where it was.** The page saves its scroll offset and its open
files to `localStorage` on scroll and on `pagehide`, and restores them on load.
The key is the review's id, the round's number and the comparison. The path is
not a key: 006 serves the current round at `/reviews/N` and at
`/reviews/N/rounds/M`, and a place saved under one would never be found under
the other. A notification opens one of those addresses, so a review half-read
and closed reopens there. The history entry was the alternative, and it does not
survive a closed tab.

**Text is saved as it is typed**, which is what the tearout's "Saved as you
type" says. A composer's text, an edit form's, the verdict's note and every
unsent card go to `localStorage` under the review and the round, each with its
anchor, the comment it edits, or its submission key. They come back when the
page is reloaded, and each is cleared once its text is stored, sent or
discarded. A reload the stream triggers, or a 409, then loses nothing. Saving
drafts on the server would be a second kind of comment, written on every
keystroke, and nothing in product.md asks for one on another device.

Both are per device. architecture.md rules out a cache and an offline store, and
these hold neither content nor comments, only where a reader was and what they
had not yet sent. Every read and write of storage is wrapped, so a page where
storage throws still works, and entries older than thirty days are dropped on
load, a guess. The drop skips `eyeball:token`, which is valid for as long as its
run of `serve` lasts.

### The tearouts

Iteration 1 changes what the approved tearouts do not already draw, and nothing
else. The bar, its tally, its refusal, the decided round's box, the connection
notice, the unavailable verdicts, the failure line, the selected row, comment
cards with Edit and Delete, and the level chips are all drawn and approved.

| Tearout            | Frame or section             | Change                                                                                                                                                           |
| ------------------ | ---------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `foundations.html` | `#comments`, both frames     | The composer's field becomes a 16-pixel `<textarea>` and its actions `<button>`s, and one comment shows in place as a composer holding its body, Cancel and Save |
| `foundations.html` | `#actions`, both frames      | A second danger notice beside the drawn one, for text still being written: "A comment is still being written. Add it, or cancel it, before deciding."            |
| `foundations.html` | `#connection`, dark frame    | The composer's field a `<textarea>`, its actions `<button>`s                                                                                                     |
| `review.html`      | `#large`, light frame        | The same, for its composer                                                                                                                                       |
| `review.html`      | `#fresh`, dark frame         | The same, for the decided round's box                                                                                                                            |
| `review.html`      | new `#passage`, both themes  | A changed passage selected, `.blk.sel`, its hint the one control that reveals the replaced text, the composer under it, chips Passage, File and Review           |
| `review.html`      | new `#deciding`, light frame | An open round with the review composer under the note, and the bar's note open with text                                                                         |
| `review.html`      | new `#deciding`, dark frame  | A verdict that could not be sent: the bar keeping its note, with a failure line                                                                                  |
| `app.css`          | shared                       | A textarea field, `.blk.sel`, the bar's note control and field, and `button.btn` matching `a.btn`                                                                |
| `README.md`        | the table                    | review.html's row naming the two new sections                                                                                                                    |

Why each:

- **A real composer is a `<textarea>`**, drawn in the tearouts as a block of
  text with no control behind it.
- **Editing in place** is offered by every card and drawn nowhere.
- **The bar's second notice** covers text still in a composer, which the drawn
  notice for unsent cards does not. The drawn one stays, worded as approved.
- **Sixteen-pixel fields** are the owner's decision, and only the phone shows
  whether the larger text sits well beside 13-pixel code.
- **A selected passage** is one of product.md's four levels, and no frame draws
  one. The owner's decision makes the hint the only reveal, so the frame selects
  a changed passage and its note says a tap elsewhere selects.
- **The review composer on an open round** follows from the review level being
  always on the page.
- **The verdict note** is in product.md, the refusal tells the reviewer to write
  one, and no frame shows where.
- **A failed verdict** is drawn nowhere, and it is the one failure where nothing
  was sent and the reviewer may believe otherwise.

Every class the templates and the TypeScript need is in `app.css` by the end of
iteration 1. A class added after that is a tearout change the owner has not
seen.

### Seeing it

**`tool/seed` grows** an open round with comments at every level, a decided
round with a late comment, a review of only binary files, and a document with a
passage comment.

**The stand-in's visual check is headless Chromium at 390 pixels**, as 006 and
007's last iterations did: one screenshot per state beside a crop of its tearout
frame. The seeded daemon is served with `serve --surface-addr`, and the harness
opens the printed address first, in a persistent profile, so the token is stored
for every page after it.

### Checks

**Pages and fragments are tested as HTML**, with `golang.org/x/net/html`, as 006
and 007 do.

**The client is tested in a browser, behind a build tag.** A test under
`//go:build browser` serves the real surface with one extra script, served
same-origin so the policy allows it. The script drives the page, taps, types and
waits, and writes what it saw into the document. The test runs headless Chromium
with `--dump-dom` and parses what comes back.

It runs in real time. The page also holds an image the harness answers only once
the script posts that it is done, so the page's load, which `--dump-dom` waits
for, ends when the script does. A stream opened, closed by the server, and
reconnected behaved under that harness as it does in a browser, confirmed.
Chromium's `--virtual-time-budget`, which the other browser probes used, is not
used here: under it, streams the server held open reported an error within ten
milliseconds of virtual time, confirmed.

The convention checks parse every Go file whatever its build tags, so the tagged
tests are named and ordered like the rest, as `TestServer_Surface_browser…`. The
go tool's own checks see a tagged file only with the tag, so iterations 5 and 7
to 10 add `go test -tags browser ./internal/server/...`,
`go vet -tags browser ./internal/server/...` and
`go tool staticcheck -tags browser ./internal/server/...` to their required
commands. Without the tag, `make check` needs no browser: a browser in the
default gate would make Chromium a requirement on every machine that runs it,
and behind the tag it is required where an iteration closes, which is this
machine.

**Each iteration ends with a mutation pass**, per the dev workflow. The tests
named here are written to kill these, and an iteration whose pass leaves one
alive has a test that cannot fail:

- dropping the cross-origin check: the same-site row is answered 204
- serving a write on the loopback listener without the header: the token test
  stores a comment
- accepting any header that is present: the row with a wrong token stores a
  comment
- requiring the header on the tailnet listener: the tailnet row is refused
- the bundle leaving the fragment in place: the browser test finds `#token=` in
  the address
- printing the token in the log: the log test finds it
- quoting from a form field instead of the frozen content: a form carrying a
  `quoted` field gets the frozen line, not the field
- deciding the latest round instead of the form's: a form naming an earlier
  round is refused, not recorded
- skipping the comment's review check: another review's comment is a 404
- dropping `HX-Refresh` from the open-form-on-decided-round answer: its row
  finds the header missing
- swapping 422 left out of the response handling: the meta test fails
- `historyEnabled` left at true: the meta test fails
- a comment count in the review's stamp: the row adding a comment finds the
  stamp unchanged
- the stream ignoring `s.stopping`: the daemon with a stream open stops within a
  second
- a fenced block's range including its fences: the fenced row
- a thematic break given a range: its row
- a removed row opening the composer: the browser test taps one and finds no
  composer
- a reload that does not wait for unsaved text: the browser test types, signals,
  and finds the text still in the field
- a client that compares stamps on a page printing none: the file-page browser
  test finds the page reloaded
- a comment write answering with the whole bar: the fragment test finds a bar,
  and the browser test finds a typed note gone

### Probes

Each claim marked confirmed came from one of these, under the scratchpad's
`s008/`.

- **Versions.** `npm view` on 2026-10-05: htmx.org `latest` 2.0.11 and `next`
  4.0.0, typescript 7.0.2, oxlint 1.87.0, oxlint-tsgolint 7.0.2003, and oxfmt
  0.72.0, where the repository pins 0.65.0 and keeps it. `go list -m -versions`
  gave esbuild v0.28.2 as the newest.
- **htmx's events.** Both packages unpacked and searched: 2.0.11's build names
  `htmx:sendError`, `htmx:responseError` and `htmx:timeout`, and 4.0.0's names
  only `htmx:error`. Sizes and the 0BSD license from the 2.0.11 package.
- **tsc.** A scratch project under the tsconfig above. tsc 7.0.2 accepted it,
  reported `xs[0]` as possibly undefined, and refused an import ending in `.ts`.
  With `htmx.esm.d.ts` and the global declaration it typed `htmx.ajax` and
  rejected a property an event's detail lacks.
- **oxlint.** The same project with a floating promise: `--type-aware` reported
  `no-floating-promises` and exited 1.
- **Offline.** Each tool through `npx` under `unshare -rn`, with and without
  `npm_config_prefer_offline=true`, timed.
- **oxfmt on TypeScript.** 0.65.0 under the repository's `.oxfmtrc.json`
  reported and rewrote an unformatted `.ts` file.
- **esbuild.** A scratch module embedding two `.ts` files, bundling them through
  an `OnResolve` and `OnLoad` plugin, timed, and its binary's size against a
  trivial one with an `embed`. The real module was not changed. Iteration 2
  confirms that `go install ./cmd/eyeball` still works and the `go` directive
  stays at 1.26.6.
- **htmx under the policy.** A Go server sending the surface's policy, loading
  `htmx.min.js`, with four buttons posting on load to a 200, a 422, a 500 and a
  handler that closes the connection, run in headless Chromium with and without
  the config meta, the events and policy violations written into the page.
- **htmx's answers.** The same harness with a two-button verdict form, an
  `HX-Refresh` answer counted by page loads, an out-of-band delete, and a
  `<details>` with `hx-trigger="toggle once"` opened, closed and opened again.
  `HX-Refresh`'s place before the status check read in `handleAjaxResponse`.
- **Cookies.** A Go server on two loopback ports. Chromium opened a path that
  set the cookie and redirected to a page holding a frame sandboxed with
  `allow-scripts`, whose script read `document.cookie` and `localStorage` and
  posted. The page then navigated to the second port. Every request's `Cookie`,
  `Origin` and `Sec-Fetch-Site` were logged. This is the measurement behind the
  owner's move from a cookie to a header.
- **The fragment and the header.** A Go server on two loopback ports. Chromium
  opened `/#token=SECRET123` on the first, whose script stored the token,
  replaced the history entry, posted with the header, and navigated to the
  second port. The server logged the page request as `/`, every `Referer`
  without a fragment, the header on the post, and the address afterward with no
  fragment. The second port's page read nothing from `localStorage`, and its
  cross-origin post with the header was never sent. A later visit to the first
  port's plain address sent the stored token.
- **The cross-origin check.** `http.CrossOriginProtection` given eleven requests
  through `httptest`: same-origin, same-site on another port, cross-site, a
  sandboxed frame's `null` origin, `none`, no headers, an `Origin` matching and
  not matching the host without `Sec-Fetch-Site`, a cross-site GET, a cross-site
  DELETE and a same-origin PUT. The five cross-origin writes were refused: the
  same-site, cross-site, `null`-origin and other-port posts and the cross-site
  DELETE. An earlier draft counted four, and the review's re-run counted five.
- **The mux.** Every existing surface pattern and the ten new ones registered on
  one `http.ServeMux`, and nine requests sent, including a POST to a comment's
  address and a GET of the decision, both 405 with their `Allow` lists.
- **The stream and shutdown.** A server with an event-stream handler, one client
  connected, `Shutdown` given two seconds: 1 ms when the handler watched a
  stopping channel that was closed first, and the full two seconds when it did
  not.
- **Passage ranges.** goldmark v1.8.6 with GFM parsing a document of every block
  kind, each top-level node's range taken from its descendants' `Lines()`.
- **A stream in the harness.** A page opening an `EventSource` on a server that
  closed the first stream after 300 ms, sent `retry: 500` and held the second,
  with a two-second timer started on `error`. Under `--virtual-time-budget` the
  held streams reported errors within ten milliseconds and the timer fired. In
  real time, with the page's load held by an image the server released when the
  script posted done, the first stream errored at 302 ms, the second opened at
  804 ms and stayed open, and the timer never fired.

Assumed, not measured: iOS Safari zooming on a field under 16 pixels; Safari
sending `Sec-Fetch-Site`; six connections to one host over HTTP/1.1 in Chromium;
HTTP/2 on the tailnet listener; and the numbers chosen here, ten seconds,
fifteen, two, two and thirty days.

**Departures, iteration 1.**

- foundations.html's `#connection` dark frame drew a failed save kept in the
  composer, with Discard and Retry. The owner's 2026-10-06 decision makes a
  failed save the dashed unsent card, so that frame now shows the card with
  Retry now and Delete, and its note says so. The design's table named only the
  textarea change for that frame.
- review.html's `#large` note said the composer opens under the line. It now
  says the composer opens after the file's diff and scrolls into view, as the
  owner decided and as the frame already drew it.
- The new frames link the brief's project, as 007 made it a link.
- The verdict's note in the bar is the composer's field class, `.decide .field`,
  with no new class. The bar's closed state, a control that opens the note, is
  not drawn: the `#deciding` frames show the note open. Iteration 5 shows the
  field whenever the bar shows, so there is no closed state to draw.
- The selected passage is `.blk.sel`, outlined as `tr.sel` is, with an offset
  that keeps the outline clear of the passage's rule.
- The classes the TypeScript writes stay the five app.css already defined:
  `sel`, `failed`, `why-not`, `offline` and `retry`. The note needed none. The
  templates' new markup uses `field`, `composer`, `levels`, `chip`, `blk`,
  `sel`, `comment`, `unsent`, `failed` and the button classes, all in app.css.
- `.composer .field.empty` is gone. Every field is a `<textarea>`, and its
  placeholder takes the faint color the class gave a drawn empty field.
- The owner's standing approval of 2026-10-06 covers `reviewed`. Whether it
  covers `phone-approved`, which asserts a look at the frames on the phone, was
  asked of the owner at this iteration's close.

**Departures, iteration 2.**

- The bundle is built the first time a page asks for `/static/app.js`, not at
  startup, behind the same `sync.Once`. A daemon started for an agent command
  never serves a page, and building there would spend the time for nothing.
  architecture.md now says so.
- esbuild was already in go.mod as an indirect dependency at v0.19.11, pulled in
  by a tool. It is now a direct one at v0.28.2. The `go` directive stays at
  1.26.6.
- The vendored release's name and hash sit in `web/vendor/VERSION`, and
  `Test_htmxIsTheVendoredRelease` holds the file to the hash.
- oxlint runs with the `correctness` and `suspicious` categories as errors and
  `--deny-warnings`, plus the two promise rules. With no problems it prints
  nothing and exits 0, which was checked by running it directly.
- Each new gate failed once on purpose, in a scratch `web/ts/fail.ts` removed
  afterward. tsc:
  `fail.ts(1,14): error TS2322: Type 'string' is not assignable to type 'number'.`,
  exit 1. oxlint:
  `fail.ts:4:3: error typescript(no-floating-promises): Promises must be awaited`,
  exit 1.
- Headless Chromium follows the desktop's color preference, which on the machine
  that ran this is dark, so the light screenshot needed
  `--blink-settings=preferredColorScheme=1`. Under each preference the queue
  rendered in the matching theme, and no policy violation was logged.
- The attack test in `TestServer_Surface_reviewDocument` asserted no `script`
  element anywhere on the page. It now allows the layout's two, in the head with
  their exact sources, and still fails on any other.
- No reviewer subagent read this iteration, under the owner's 2026-10-06 change.
  Mutation testing was not run either, to stay within the usage limit; the
  gates' deliberate failures above are the evidence they can fail.

**Departures, iteration 3.**

- The bundle's half of the token, reading the fragment and sending the header,
  moved to iteration 5 under round 1's review, as the iteration lists show. This
  iteration built the server's half and the printed address.
- `LoopbackSurface(token)` wraps the same inner handler as `Surface()`, and each
  puts `http.CrossOriginProtection` outermost. A write refused for its token
  still carries the surface's headers.
- The tests that the token never reaches `/agent/daemon` or a log line are in
  `TestRun_surfaceOnLoopback`: the token appears once in everything the daemon
  writes to stderr, the printed line, and not in `/agent/daemon`'s answer.
  `eyeball status` prints that answer, so it is covered by the same check rather
  than by a test of its own.
- Its cross-origin rows run against both handlers, and an origin matching the
  host is let through without `Sec-Fetch-Site`, as the fallback is written.
- architecture.md's preview paragraph now also requires a preview served as a
  page to carry `Content-Security-Policy: sandbox allow-scripts` or come from
  another origin, round 1's finding 11, for 010.
- No mutation run, under the owner's 2026-10-06 change. The token rows include
  the right token passing, which a check that refused every write would fail.

**Departures, iteration 4.**

- A stored comment's answer is the anchor's list and, while the round can still
  be decided, the tally swapped by its id. It does not carry a closed composer.
  The review's composer stays on the page and only empties, and an anchored one
  closes, so the client does both on a 200 in iteration 5. The design's table
  listed "composer closed" as part of the answer.
- A delete answers the tally alone, with an empty main answer, which removes the
  card its `outerHTML` swap targets.
- The edit form's Cancel fetches the card again rather than restoring it on the
  client, so the card shown is always what the store holds.
- The composer's chips are buttons carrying `data-level`. Switching among them
  is iteration 5's. A removed row's form sends `oldline` and the page's
  `against`, and the server reads the line from the previous round's capture, or
  from the base for the whole review.
- `store.Comment` reads one comment with its round's number, so an edit can
  answer with the card. The store had no read of a single comment.
- A comment of a review that does not exist, a malformed id and a round the
  review lacks are each a 404 with a sentence. A form the page could not have
  sent is a 400 with "This page could not have sent that. Reload it and try
  again." A path the round holds no readable copy of, and a line past a file's
  end, are 400s that name the file.
- No mutation run, under the owner's 2026-10-06 change.

**Departures, iteration 5.**

- The browser harness drives headless Chromium over the DevTools protocol, with
  `golang.org/x/net/websocket` from the `x/net` module the repository already
  required, rather than through a same-origin script and an image it releases.
  `Runtime.evaluate` runs outside the page's content policy, so the page carries
  no script of the test's, and a test reads and taps it as a person does.
  Chromium needs `--remote-allow-origins` for the protocol's websocket, and is
  killed as a process group with its profile in its own temporary directory,
  since its helper processes outlive the main one and keep writing there.
- `defaultSettleDelay` is 0 in the htmx configuration, and the config test holds
  it. htmx binds swapped content only once it settles, 20ms by default, and the
  browser test's Save, clicked inside that window, submitted the edit form
  natively as a GET to `/reviews/1?body=…`, losing the edit. A phone tap could
  do the same, and the page uses none of the transitions the delay is for.
- The client writes two classes, `sel` and `failed`. The "still being written"
  notice is rendered by the server inside the bar, in a wrapper with no class
  that the client shows and hides with `hidden`, so no notice classes are
  written at run time. The failure line is a `span.failed` holding the sentence,
  without the drawn icon, whose classes are not in the table.
- An anchored composer arrives wrapped in the drawn `.filecomposer`, carrying
  the slot's id, and its `outerHTML` swap replaces the empty slot. Closing it
  puts an empty slot back.
- On the current round the review-level comments sit under the review box, where
  its answers go, rather than in the comments section at the foot.
- htmx's event details are read through checked accessors in `web/ts/events.ts`
  rather than cast, which oxlint's `no-unsafe-type-assertion` refuses.
- The browser tests do not tap a removed row or a too-large file's line. The
  server's quoting of a removed row is iteration 4's test, and the tap's code
  path is the line tap's with other fields.
- No mutation run, under the owner's 2026-10-06 change.

**Iteration 6, the review of everything since d8b5b6d.** The driving agent read
007 and 008's first five iterations as one change, looking for what spans them,
with the `review:` entry's list as its start.

- Fixed in 40e0f4c: the review page answered 500 once a project's repository was
  gone, its root was missing, or its base commit was no longer readable. 006
  built the page before 007's `capture.Inspect` existed, 007 handled those
  states on the file pages only, and 008 put the bar on the review page, so the
  failure had become one that stopped a review from being decided. The page now
  asks `Inspect` first, and where nothing can be compared it shows the files the
  review changes as the round captured them, without counts, under a warning
  naming the reason. The bar and the composer stay. A probe deleting a review's
  `.git` answered 500 before the fix and 200 after it.
- Fixed in iteration 5, and recorded there: htmx's settle delay let a form
  tapped right after a swap submit natively and lose its text.
- Left, with the reason: the review page still compares through
  `capture.NewReader`. Its readers are read through `Side`, which reads only the
  capture and the base, never the working copy, so the refusals `State.Reader`
  adds for live reads do not apply. `Inspect` now decides whether the comparison
  runs at all.
- Left, with the reason: the server's quote of a comment's lines does not write
  the inside-checkout flag that the file, tree and project routes write. It
  reads only frozen content, and the flag guards live reads, which the next page
  view of the project writes it for.
- Checked, with nothing found: every write resolves its round or comment within
  the review its address names; every address a page builds is numeric or passes
  `escapePath`; a composer's path travels in a hidden field that `html/template`
  escapes; list and slot ids are hashes of the path; a removed row's quote reads
  the side the page's `against` names; banner reasons are still chosen by
  `errors.Is`; the loopback token reaches no log line or request line;
  `http.CrossOriginProtection` is outermost on both listeners; and every
  fragment is rendered from a clone of the template set, as pages are, so no
  execution touches the shared one.

**Departures, iteration 8.**

- A tap's composer for a line arrives as the line's whole row: the comments
  already on that line and the form beneath them. It replaces the row when the
  line has one and is inserted after the line when it does not. A stored line
  comment answers with the row, holding every comment on the line, which
  replaces it. Cancel, a send moved to another level, and a deleted last comment
  each leave no empty row behind.
- A removed row's composer opens in a row under it too, though its comment is
  file-level and goes to the file's list. A binary or too-large file's line has
  no row to open under, so it still fills the file's slot.
- A line comment whose line the diff does not show stays in the file's list.
- htmx fires `afterRequest` on the element that sent the request, and a line
  comment's answer replaces the row that held its form, so the event fires on a
  detached form and never reaches the document. The client clears the line's
  outline after any swap that leaves no composer open, which the browser test
  caught.
- An edit form in a row is a `form.composer` too, so the selector for an
  anchored composer names `hx-post`, and tapping another line never closes an
  edit in progress.
- `.filelinks` is gone from app.css, since no tearout or template uses it after
  iteration 7.
- Fragment tests parse an answer that starts with a row inside a table, since an
  HTML parser drops a `tr` outside one.
- No mutation run, under the owner's 2026-10-06 change.

## Out of scope

- **Push, the manifest, the service worker, installing, the badge and the
  owner's phone check of the built surface.** Notification (009).
- **Images, media, the preview sandbox, `internal/kind`, the per-file view
  override and a file too large to send.** Rendered files (010).
- **A range of lines.** The open question in product.md, and an existing
  `design:` entry.
- **Commenting from a file page**, the whole file, the tree's file or the
  project's live file. The review page is where the bar and the tally are. A
  `design:` entry.
- **A manual theme override.** A `design:` entry.
- **A comment added on another device shown without a reload.** A `design:`
  entry.
- **Writes on the tailnet listener.** Notification (009), with tsnet's `WhoIs`,
  as the owner decided on 2026-10-06, and a `product:` entry pointing there.

## Not converged

One review round ran, at medium effort, the owner's setting for reviews in this
run. It re-ran the claims a defect would hinge on and found three blocking
defects, all applied: a reload answered while text was on the page, which htmx
performs before any handler of the page's runs, so a late comment or a stale
verdict lost what was typed; the verdict's guard against unsaved text arriving
after the owner starts testing; and the bundle's token handling placed in an
iteration with no browser harness to test it. Its twelve minor findings were
applied, and its four questions went to the owner. A second round, scoped to the
fixes, did not run: the owner ended subagent reviews on 2026-10-06. The fixes
carry defects at about the rate of the draft they corrected, so the risk is that
iteration 4's answers table and iteration 11's unsent cards, the two parts most
rewritten, disagree somewhere with the routes section.

What is riskiest, in the shape every earlier review found, two things that must
agree with nothing checking that they do:

- the stamp printed on a page and the stamp the stream sends for it, computed in
  two handlers from one function that both must call
- the composer's anchor in the browser and the quote the server reads, which
  agree only if the row's number and the block's `data-lines` are the frozen
  file's lines
- the tally on the bar and the comments listed, refreshed by different answers
- the header the bundle sends and the one the loopback listener checks, named
  once in Go and once in TypeScript, and the fragment's form the bundle reads
  and the line `serve` prints
- the htmx configuration, the status each refusal is given, and the handler that
  reports failures, which together decide whether a refusal is swapped, shown as
  a failure or lost
- the client's place in `localStorage` and the reload the stream triggers, which
  is correct only once iteration 12 exists
- the class table in `web/ts/classes.ts` and the classes the code actually
  writes, which a test can only check if every class goes through the table

The client's behavior has no test in `make check`. The browser tests behind the
tag are the only check on taps, the stream and storage, and they run where an
iteration closes, not on every change.

Decided here rather than by product.md, architecture.md or the owner, and
reported at the end of the run: the cross-origin check on both listeners; the
fragment as the token's handover and `X-Eyeball-Token` as its header; a token
per run; a write without script refused on loopback; reads open without the
token; the round named by every write; quotes read by the server; one composer
on a page; the composer after the file's diff, as drawn; removed rows quoted at
the file level, the owner's; one line per comment; the review composer always on
an open round; delete with no confirmation; no box on an abandoned review; a
comment on a round decided since stored and shown as late; a verdict withheld
while text is unsaved; a verdict answered by reload; a page reloaded when its
stamp moves, with comment counts left out; the composer and the place kept per
device; htmx 2.0.11 over 4.0.0; TypeScript 7.0.2; `npx` with `prefer_offline`
over a lockfile; the browser tests behind a tag; and every guessed number above.

## Iteration 1: The tearouts for acting

```backlog
required_acks:
  - reviewed
  - phone-approved
```

- [x] app.css rules for a 16-pixel textarea composer field, its placeholder,
      full width and vertical resize, and `button.btn` matching `a.btn`,
      rendered in both themes
- [x] foundations.html's composers holding a textarea and buttons in `#comments`
      and `#connection`, and a comment edited in place in `#comments`
- [x] foundations.html's `#actions` gaining a second danger notice, "A comment
      is still being written. Add it, or cancel it, before deciding.", beside
      the drawn notice for unsent cards
- [x] review.html's large and decided composers holding a textarea and buttons
- [x] review.html's `#passage` section: a changed block selected as `.blk.sel`,
      its hint the only reveal, with the composer under it and Passage, File and
      Review chips, in both themes
- [x] review.html's `#deciding` section: the review composer under the note with
      the bar's note open, and a verdict that could not be sent
- [x] Every class the templates and `web/ts/classes.ts` will use present in
      app.css, listed in this spec
- [x] README's review.html row naming the passage and deciding sections

> **Completed** 2026-10-06 12:06 UTC
>
> - acks: reviewed, phone-approved
> - `make check` — 4.1s
> - `go test -race ./...` — 21.7s

## Iteration 2: The client

- [x] htmx 2.0.11's `htmx.min.js` and `htmx.esm.d.ts` in `web/vendor/` with
      their version and sha256, and a test holding the script to that hash
- [x] `GET /static/htmx.js` serving it, and the layout loading it after an
      `htmx-config` meta
- [x] A test parsing that meta: indicator styles, eval and history off, a 10000
      ms timeout, and the four `responseHandling` entries in order with their
      `swap` and `error` flags
- [x] esbuild v0.28.2 bundling `web/ts` from the binary once into
      `GET /static/app.js`, with a test that it builds with no errors or
      warnings
- [x] `web/tsconfig.json` with conventions.md's settings, and `tsc` and
      type-aware oxlint in `make lint` through `npm_config_prefer_offline`
- [x] oxfmt covering `*.ts` and skipping `web/vendor/`, with the same setting on
      its `npx`
- [x] Each new gate failing once on purpose, a type error for tsc and a floating
      promise for oxlint, recorded in this spec
- [x] The theme set from `prefers-color-scheme` by the head's script and
      following a change, with a screenshot under each preference
- [x] `web/ts/classes.ts` held to app.css by a Go test
- [x] `go install ./cmd/eyeball` serving the bundle, the `go` directive at
      1.26.6, and architecture.md's `assets.go` line and esbuild row checked
- [x] specs/TODO.md's entries on a dark device and on a build step beyond
      esbuild, and architecture.md's matching open question, removed as answered

> **Completed** 2026-10-06 12:13 UTC
>
> - acks: reviewed
> - `make check` — 4.4s
> - `go test -race ./...` — 21.4s

## Iteration 3: Who may act

- [x] `http.CrossOriginProtection` around the surface handler on both listeners,
      with a test row for each case in the probe's table
- [x] `serve --surface-addr` drawing a token once per run and printing the bound
      address with `#token=` once the listener is bound
- [x] Tests that `/agent/daemon`, `eyeball status` and the daemon's log lines
      never hold the token
- [x] Every request but GET and HEAD on the loopback listener refused with 403
      and the token sentence unless `X-Eyeball-Token` matches, compared in
      constant time
- [x] Rows for a missing header, a wrong token and a token from an earlier run,
      and the sentence answered as a page to a write without `HX-Request`
- [x] The tailnet listener ignoring the header, with a row writing there with
      none
- [x] architecture.md's listeners and preview paragraphs: the token stored and
      sent as a header, the cross-origin check, the batch a verdict releases,
      and the rule for 010 that a preview opened as a page carries
      `Content-Security-Policy: sandbox allow-scripts` or comes from another
      origin, since agent HTML on the surface's origin could read the token
- [x] The manual's `serve` entry saying it prints an address to open, naming no
      token

> **Completed** 2026-10-06 12:19 UTC
>
> - acks: reviewed
> - `make check` — 4.3s
> - `go test -race ./...` — 21.1s

## Iteration 4: The write routes

- [x] `POST /reviews/{review}/comments` storing a comment at each level on the
      round the form names, quoted from the round's frozen content
- [x] A line outside the file, a path the round cannot read, and a level with
      the wrong fields each answered 400 with nothing stored
- [x] Migration `0006`: `comments.submission`, unique where not empty, and a
      file-level comment allowed a quote with no range, with a test reading a
      comment written before it back after it
- [x] A repeated submission key storing nothing new and answering the stored
      comment, and a removed row quoted from the side it was removed from
- [x] A comment posted to a round decided, abandoned or superseded since the
      page loaded stored as late and answered 200 with its card marked late,
      with no `HX-Refresh`
- [x] `PUT` and `DELETE` on a comment, refusing another review's comment with
      404 and a released one with 409 and a plain sentence, with no `HX-Refresh`
- [x] `POST /reviews/{review}/decision` deciding the named round with
      `HX-Refresh`, 422 and the bar's notice for an empty change request, and
      409 with a plain sentence and no `HX-Refresh` for a stale round
- [x] `GET /reviews/{review}/composer` and
      `GET /reviews/{review}/comments/{comment}` answering the composer, the
      card and the edit form from template blocks
- [x] Each comment write's fragments, the anchor's list, the tally by its id and
      the composer closed, tested as parsed HTML, with no bar in them
- [x] A failing surface write with `HX-Request` answered 500 with the plain
      sentence and logged
- [x] Each write sent to `AgentHandler` as a real request answered 404, with the
      round undecided and no comment stored
- [x] A daemon test of the whole loop: a request over the socket, a comment and
      a verdict carrying the printed token as the header, and `wait` returning
      both

> **Completed** 2026-10-06 12:31 UTC
>
> - acks: reviewed
> - `make check` — 4.8s
> - `go test -race ./...` — 25.4s

## Iteration 5: Commenting and deciding on the page

```backlog
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -tags browser ./internal/server/...
  - cmd: go vet -tags browser ./internal/server/...
  - cmd: go tool staticcheck -tags browser ./internal/server/...
```

- [x] The open round's page in the tearout's markup: the review composer under
      the note, a composer slot after each file's diff, and cards with Edit and
      Delete
- [x] The bar on an open current round: the tally, the note and both verdicts,
      posting the round shown
- [x] The decided current round's box posting a late comment, and no box on an
      earlier round or an abandoned review
- [x] A tap on an added or unchanged row opening the composer for its line,
      marking it `sel`, scrolling it into view, and carrying typed text to a new
      anchor
- [x] A tap on a removed row opening the file-level composer with that line
      quoted, a binary or too-large file's line opening it at the file level,
      and a hunk header opening nothing
- [x] The chips switching the level without a request and keeping the text
- [x] Edit swapping a card for its form with Save and Cancel, and Delete
      removing it, each updating the tally
- [x] One handler for the three htmx failure events writing a `failed` line
      beside the control, with the server's sentence or a fixed one
- [x] The verdict withheld while any composer or edit form holds typed text,
      with the bar's "still being written" notice and both verdicts unavailable
- [x] The bundle storing a fragment's `token=` under `eyeball:token`, replacing
      the history entry without it, and adding the header to every write
- [x] A browser test harness behind the `browser` tag, driving headless Chromium
      in real time through a same-origin script and an image it releases
- [x] Browser tests: a line comment added, edited and deleted, a review comment,
      a refused change request sent with a note, an approval reloading, and an
      approval withheld while a composer holds text, the store holding no
      decision
- [x] A browser test of the handover: the printed address opened, the fragment
      gone from the address, a write accepted, and no request line holding the
      token

> **Completed** 2026-10-06 12:42 UTC
>
> - acks: reviewed
> - `make check` — 4.7s
> - `go test -race ./...` — 24.7s
> - `go test -tags browser ./internal/server/...` — 9.0s
> - `go vet -tags browser ./internal/server/...` — 72ms
> - `go tool staticcheck -tags browser ./internal/server/...` — 169ms

## Iteration 6: The whole-branch review before the owner tries the loop

```backlog
required_acks:
  - reviewed
  - owner-told
```

The owner decided on 2026-10-06 that the whole-branch review in specs/TODO.md's
`review:` entry runs when iteration 5 lands, before the owner tries the loop on
the loopback listener. Rejected: testing first, with the review left until
before notification (009). The owner tests while the rest of 008 continues, so
nothing here waits on the owner's testing.

Who does what:

- **The driving agent** stops once iteration 5 closes and starts nothing from
  iteration 7. It reviews everything since d8b5b6d, the 006 merge, as one
  change, with the `review:` entry as its brief, looking for what spans
  iterations. The owner's 2026-10-06 change moved this from a fresh subagent to
  the driving agent, which has read every iteration and so brings less distance
  to it than a fresh reader would.
- **Each fix** is its own commit. A finding left unfixed goes to specs/TODO.md
  with its reason.
- **`reviewed`** is given under the owner's standing approval once the findings,
  the fix commits and their record in this spec are in.
- **`owner-told`** is given after telling the owner that the loop is ready to
  try, with the `serve` command and the address to open. It asserts that the
  message was sent and nothing else.

An ack only the owner gives, as `phone-approved` is, was the alternative. It
would hold the run on testing the owner chose not to block on.

- [x] A review of everything since d8b5b6d, run after iteration 5 closes, with
      its findings recorded in this spec
- [x] Each finding fixed in its own reviewed commit or entered in specs/TODO.md
      with its reason
- [x] specs/TODO.md's `review:` entry removed once the review has run
- [x] The owner told that the loop is ready to try on the loopback listener,
      with the command to run and the address to open

> **Completed** 2026-10-06 12:46 UTC
>
> - acks: reviewed, owner-told
> - `make check` — 4.9s
> - `go test -race ./...` — 26.6s

## Iteration 7: The tearouts for comments in place

```backlog
required_acks:
  - reviewed
  - phone-approved
```

- [x] review.html's large-change frame drawing the composer, with Cancel, in a
      `tr.thread` row under the selected line, and an existing comment in a row
      under its line
- [x] review.html's passage frames drawing the composer, with Cancel, under the
      selected passage, with the chips as buttons
- [x] A "Whole file" or "Raw diff" icon in every file card's header in
      review.html and widening.html, and no `.filelinks` footer
- [x] app.css rules for `.fileopen`, `tr.thread` and `.thread-body` pinned to
      the code block's visible width, a composer inside `.prose`, a wrapping
      composer foot, and `button.chip`

> **Completed** 2026-10-06 16:34 UTC
>
> - acks: reviewed, phone-approved
> - `make check` — 4.6s
> - `go test -race ./...` — 25.5s

## Iteration 8: Comments in place

```backlog
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -tags browser ./internal/server/...
  - cmd: go vet -tags browser ./internal/server/...
  - cmd: go tool staticcheck -tags browser ./internal/server/...
```

- [x] A line's comments rendered in a `tr.thread` row under their line on the
      review page, and the composer a tap opens inserted as such a row
- [x] Cancel closing the anchored composer and dropping the line's outline,
      keeping nothing it held
- [x] A comment's answer replacing that line's row, so it appears under its
      line, with file-level comments where they were
- [x] The file card's header carrying the icon link, and the footer gone from
      the template
- [x] Browser tests: a comment added under its line and found there after a
      reload, Cancel closing the composer, and the header link opening the file
      page

## Iteration 9: Passages

```backlog
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -tags browser ./internal/server/...
  - cmd: go vet -tags browser ./internal/server/...
  - cmd: go tool staticcheck -tags browser ./internal/server/...
```

- [ ] Each top-level block's range from the shown parse `Document` walks,
      carried as `data-lines` on kept and changed blocks
- [ ] Range tests: a wrapped paragraph, a nested list, a fenced block without
      its fences, a setext heading without its underline, a table and an HTML
      block
- [ ] A thematic break, an empty fenced block and a removed block carrying no
      range, each with a test
- [ ] A tap anywhere on a block with a range opening the composer at the passage
      level, the summary's toggle canceled unless the tap is on the hint, and a
      link followed
- [ ] A passage comment stored with its range and listed with its quote, on a
      card and in the fresh layout, with a browser test

## Iteration 10: The live stream

```backlog
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -tags browser ./internal/server/...
  - cmd: go vet -tags browser ./internal/server/...
  - cmd: go tool staticcheck -tags browser ./internal/server/...
```

- [ ] `GET /events` subscribing before reading, sending `retry: 2000`, the stamp
      and a keepalive every fifteen seconds, flushed each time
- [ ] The stream ending on `BeginShutdown`, with a daemon test that a stop with
      a stream open finishes within a second
- [ ] The review's and the queue's stamps printed on the page and sent by the
      stream, with tests that a reply or a verdict moves one and a comment does
      not
- [ ] Plain `/events` sending no stamp after a publish, and a browser test that
      a file page stays put when something publishes
- [ ] A page with a stamp reloading when the stream's differs, waiting while any
      composer, edit form or the verdict's note holds text
- [ ] The notice inserted after two seconds without an open stream or at once on
      `offline`, removed on reconnect, with Retry reopening the stream
- [ ] The verdicts marked unavailable with the why-not line while the notice
      shows, and a request from them canceled
- [ ] Browser tests: a verdict recorded elsewhere reloading an open page, and
      the notice after the server stops and gone once it returns

## Iteration 11: A failed save

```backlog
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -tags browser ./internal/server/...
  - cmd: go vet -tags browser ./internal/server/...
  - cmd: go tool staticcheck -tags browser ./internal/server/...
```

- [ ] A comment whose save fails becoming a dashed `.comment.unsent` card in its
      list holding the form's fields and submission key, with Retry and Delete,
      and the composer closing
- [ ] Retry sending the same form and key, and Delete removing the card with no
      request
- [ ] An edit whose save fails keeping its form with the failure line, and a
      failed delete reporting beside its card
- [ ] The bar's approved notice counting unsent cards, with both verdicts
      unavailable while any shows
- [ ] A verdict whose request fails reporting in the bar and keeping its note
- [ ] Browser tests: two saves with the server down becoming two unsent cards,
      one retried once it returns and stored once, one deleted, and a verdict
      withheld while a card remains

## Iteration 12: Loading and remembering

```backlog
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -tags browser ./internal/server/...
  - cmd: go vet -tags browser ./internal/server/...
  - cmd: go tool staticcheck -tags browser ./internal/server/...
```

- [ ] A collapsed card rendered without its body, loading it from the cards
      route on its first open, keeping `against`
- [ ] A card body that fails to load showing the drawn failure line with its
      `retry` link in the card, with no new class
- [ ] The text of the composer, an edit form, the verdict's note and every
      unsent card kept in `localStorage` per review and round as it is typed,
      restored on load, and each cleared once stored, sent or discarded
- [ ] A review page saving its scroll and open files under the review, round and
      comparison on scroll and `pagehide`, restored at either address
- [ ] Every storage call wrapped so a page works when storage throws, and
      entries older than thirty days dropped except `eyeball:token`
- [ ] Browser tests: a large review scrolled with a card open, reloaded, and
      found in place with that body loaded; a draft surviving a reload

## Iteration 13: Seeing it

- [ ] `tool/seed` writing an open round with comments at every level, a decided
      round with a late comment, a review of binary files and a passage comment
- [ ] Screenshots at 390 pixels of each changed frame's state beside its tearout
      frame, opened through the printed address in a persistent profile
- [ ] architecture.md's routes table, its htmx size and its live connection
      section matching what was built
- [ ] specs/TODO.md entries for this spec's departures
