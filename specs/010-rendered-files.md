---
status: active
created: 2026-10-06T00:00:00Z
updated: 2026-10-06T19:50:10.90650348Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
---

# 010 Rendered files

> **Autonomous run, 2026-10-04, as changed on 2026-10-06.** Part of the owner's
> request to build the project to completion as product.md and architecture.md
> describe it. Since 2026-10-06 the agent driving the run implements and reviews
> every iteration itself, with no subagents, and the owner's word stands as
> approval for every ack until the owner says otherwise. The review gate is off.
> Iteration 4 changes the tearouts and carries `phone-approved`, which only the
> owner gives after looking on the phone. Iteration 8 is the owner's phone
> check. Departures from this spec are recorded in it as they happen.

A file a browser can display is displayed. Today the surface shows every file as
text, and a file that is not text as the line "Binary file, not shown as a
diff." This spec classifies each file, serves a file's bytes so the page can
show them, renders images before and after, plays audio and video, shows PDFs,
renders HTML in a sandbox, lets the agent pick a file's first view in its
request, and refuses to send a file too large for a phone. It is item 1 under
Next in specs/TODO.md and the last item there.

## Design

### What a file is

**`internal/kind` maps a path and its first bytes to one of seven kinds:** text,
markdown, page, image, audio, video, PDF, and other. architecture.md lists six
and then calls SVG "a page" without naming page as a kind. Iteration 1 corrects
it. Page is HTML (`.html`, `.htm`) and SVG.

The extension decides first, from a fixed table. A file with no extension in the
table is text if diff's binary test passes it and other if not, so the two
packages cannot disagree about what is text. `diff.IsBinary` is exported for
that. `http.DetectContentType` was the other candidate for sniffing, and it
names a handful of formats by magic bytes the extension table already covers, so
it adds a second answer without adding a case.

Each kind carries what the rest of the code asks of it:

- the views it has, and which opens first, per product.md's table;
- the content type its bytes are served with, from the same fixed table.
  `mime.TypeByExtension` reads the host's `/etc/mime.types`, so the same binary
  would answer differently on two machines;
- a name for the line that says what a file is, such as "PNG image" or
  "QuickTime video", and "Binary file" for other.

`render.IsMarkdown` and the markdown tests in `fileAt` move into kind, so one
table decides how every file opens.

### Views

| Kind     | Views              | Opens as  |
| -------- | ------------------ | --------- |
| Markdown | Document, Source   | Document  |
| Page     | Page, Source       | Source    |
| Image    | Image              | Image     |
| Audio    | Player             | Player    |
| Video    | Player             | Player    |
| PDF      | Document           | Document  |
| Text     | Marked, Plain      | Marked    |
| Other    | the line naming it | that line |

**SVG's rendered view is an image element, not a frame.** An image element
cannot run script, which product.md already names as the safe way to show one,
and it lets SVG reuse the image's before and after. The cost: an SVG that
animates with script stands still. SMIL animation still plays. A frame would run
its script, and the reviewer of an icon change is asking how it looks, not how
it behaves. So for SVG the toolbar reads Image and Source, and for HTML, Page
and Source. architecture.md says SVG opened as a document goes through the
sandbox, and iteration 1 corrects it.

### The bytes

**`GET /reviews/{id}/raw/{round}/{path...}` serves one file's bytes as it was at
a round**, and round 0 is the base commit. The previous round's side of a
comparison is `raw/{n-1}`, and the base side of a whole-review comparison is
`raw/0`, so every side the file page compares has an address. A round resolves a
path as the file page does, from its capture and then the base commit, and never
from the working copy. A file the round does not hold is a 404. The working copy
holds only what the capture skipped, which is ignored files, and a preview that
silently mixed live bytes into a frozen round would carry no banner to say so.

The path in the address mirrors the project's path, so a page's relative links
resolve through the same route. A mockup's `app.css` beside it loads from the
same round, which is how this repository's own tearouts load.

**Every response carries
`Content-Security-Policy: sandbox allow-scripts; frame-ancestors 'self'`**,
whatever its type. A page opened directly runs as it does in a frame, with an
opaque origin, and the policy has no `default-src`, so a page may load what it
likes. The policy on the surface's own pages keeps `frame-ancestors 'none'`.
`nosniff` stays on, which is why the content types come from the table: a
stylesheet served as `text/plain` is refused.

**Other is a 404.** A download onto a phone is out of scope in product.md, and a
route that served it would be that download.

**The route refuses past the send limit**, so a direct address cannot send what
the page declines to show. The limit is 20 MB, exported as `kind.MaxSend`. It is
a guess, written down so it can be corrected, as diff's 2 MB was. At 10 Mbit/s,
20 MB takes 16 seconds. A file under the limit is read into memory and served
with `http.ServeContent`, which answers Range requests. iOS Safari plays no
video from a server that does not. A blob is seekable on disk, but a base-side
file comes out of `git cat-file` as a stream, and reading both into memory keeps
one code path. The cost is up to 20 MB held per request in flight, and one
reviewer makes few requests at once.

**No CORS headers.** A probe in Chromium 152 framed a page with
`sandbox="allow-scripts"` and served it with the policy above:

| From the framed page        | Result                            |
| --------------------------- | --------------------------------- |
| `localStorage`              | throws `SecurityError`            |
| `fetch` POST to the surface | refused by the cross-origin check |
| `fetch` GET to the surface  | fails, as CORS mode               |
| A linked stylesheet         | loads, as `no-cors`               |
| A classic script            | runs, as `no-cors`                |
| A module script             | does not run, as CORS mode        |
| A web font                  | does not load, as CORS mode       |
| `mask-image` from a file    | does not load, as CORS mode       |

The frame's origin is `null`, so its CORS-mode loads are cross-origin. Answering
them with `Access-Control-Allow-Origin: null` would let any page the phone opens
read a project's files, since any site can frame itself sandboxed and review ids
count up from 1. The cost is the last three rows. This repository's tearouts use
system fonts, data-URL masks and no scripts, and render. If a preview ever needs
CORS, the answer is an unguessable path segment per round, answered with CORS
only under it. That is recorded, not built.

### Showing it

**Image.** Before over after, at the same width, each captioned with its
dimensions and byte size, as widening.html's frame draws it. A new image has
only After and a deleted one only Before. Dimensions come from the standard
library's `image.DecodeConfig` for PNG, JPEG and GIF, read from the bytes the
page already has a size for. Any other format is captioned with its size alone.
Reading `naturalWidth` in the page covers every format the browser decodes, and
was rejected because the caption would fill in late, by script, after the image
arrived.

**Audio and video.** A player for each side, stacked the same way, with
`preload="metadata"` so a page of them fetches only headers until one is played.
Video carries `playsinline`, so iOS does not take it full screen on play.

**PDF.** A frame with no `sandbox` attribute, served without the CSP sandbox,
and an Open link beside it. Chrome refuses to run its PDF viewer in a sandboxed
frame or a document under the CSP sandbox, and iOS has shown only the first page
of an inline PDF, which is not reading one. Which of frame and link survives is
decided on the phone in iteration 8. Serving a PDF unsandboxed at the surface's
origin is safe on the assumption that a browser's PDF viewer gives a document's
script no access to the page around it. PDFium, PDF.js and PDFKit all document
that, and it is assumed, not measured. The 404 for other and the fixed content
type keep an HTML file from being served as a PDF.

**Page.** A frame with `sandbox="allow-scripts"`, the width of the card and a
fixed height, captioned Sandboxed, with an Open link to the same address. The
round's current side only: the Source view is where the change is marked.

**Too large.** The line naming the kind and size, and "Too large to send over a
phone connection." Size comes from the capture for a round's file and from
`git cat-file -s` for the base, so nothing is read to say it.

**Other.** The line naming its kind and size, which replaces "Binary file, not
shown as a diff."

All of these appear on the file page and in the review page's cards, and the
deferred card bodies render them too. A rendered card takes a comment on the
file, as a binary file's line does today. Images and frames load lazily.

### The agent's view

**`eyeball request --view <path>=rendered|source`, repeatable**, picks a file's
first view in that round. `rendered` is Document for markdown and Page or Image
for a page, and `source` is Source. The body carries it as
`views: {"path": "rendered"}`, with paths written as the review's paths are,
relative to the caller's directory. A view applies to that round's card and to
its file page's default. It is not carried into the next round, because each
request says what that round is asking, and a choice that carried over would
outlive its question.

**A view is refused before anything is stored**, with the code `bad_view`, when
its path is not in the round's capture or its kind has no such view: `source`
for a PNG, or `rendered` for Go. A typo surfaces at once instead of doing
nothing. The check runs after the capture, since only the capture knows the
round's files, and before the round is written.

A view is stored on `round_files` in a `view` column, empty for the default, by
migration 0008.

## Out of scope

- **Thumbnails** of an image in a file list. Nothing draws one today.
- **A per-round preview origin**, the second tsnet hostname architecture.md
  names. Nothing needs storage yet.
- **Downloading a file the surface cannot render**, per product.md.
- **Comments on a region of an image or a time in a recording.** A rendered file
  takes a comment on the file.

## Not converged

No review pass has run on this spec, under the owner's 2026-10-06 change. What
is least certain: how iOS shows a PDF in a frame, and whether a frame's fixed
height reads as a page on the phone. Both are measured in iteration 8. That a
browser's PDF viewer keeps a document's script away from the page is assumed.

## Probes

- **The sandbox, in Chromium 152.** A Go server framed a page with
  `sandbox="allow-scripts"`, every response under
  `sandbox allow-scripts; frame-ancestors 'self'`, behind
  `http.NewCrossOriginProtection`. The results are the table above. The POST
  never reached the handler.
- **PDF in headless Chromium** drew a blank frame with the sandbox and without
  it, so headless shows nothing either way, and the PDF view is measured on the
  phone.

## Iteration 1: What a file is

- [x] `internal/kind`: the kinds, the extension table with content types and
      names, the binary fallback through `diff.IsBinary`, and each kind's views
      and default
- [x] `render.IsMarkdown` and `fileAt`'s markdown tests replaced by kind
- [x] architecture.md corrected: the page kind, SVG's rendered view as an image
      element, and the send limit
- [x] Tests over the table: every extension, a file with none, a text and a
      binary fallback, and each kind's views

> **Completed** 2026-10-06 19:54 UTC
>
> - acks: reviewed
> - `make check` — 5.1s
> - `go test -race ./...` — 30.2s

## Iteration 2: The bytes

- [x] `GET /reviews/{id}/raw/{round}/{path...}` resolving round 0 as the base
      and a round as the file page does, without the working copy
- [x] The sandbox policy and content type on every response, 404 for other and
      for a path the round does not hold
- [x] The send limit, with `Reader.Size` from the capture and from
      `git cat-file -s`
- [x] Range requests through `http.ServeContent`
- [x] A browser test: a framed preview's script cannot read the loopback token
      in `localStorage`, and its POST is refused
- [x] architecture.md's route table and sandbox section naming the route and the
      CORS cost

> **Completed** 2026-10-06 19:59 UTC
>
> - acks: reviewed
> - `make check` — 5.0s
> - `go test -race ./...` — 30.8s

## Iteration 3: The agent's view

- [x] `views` on `RequestBody` and `--view` on `eyeball request`
- [x] Migration 0008 adding `view` to `round_files`, stored per round
- [x] `bad_view` for a path not in the capture or a view the kind lacks, checked
      before the round is written
- [x] product.md's command list naming `--view`
- [x] Tests: each refusal, a stored view, and a next round not inheriting it

> **Completed** 2026-10-06 20:05 UTC
>
> - acks: reviewed
> - `make check` — 5.1s
> - `go test -race ./...` — 2.7s

## Iteration 4: The tearouts for rendered files

```backlog
required_acks:
  - reviewed
  - phone-approved
```

- [x] widening.html's "an image, shown as an image" frame showing the image
- [x] New frames: a new image, a deleted image, SVG in its Image view, audio,
      video, a PDF, and a page's frame with its Open link
- [x] review.html's cards for a changed image, a page in its Page view, a media
      file, too large, and other
- [x] README's rows naming the new frames

The owner looked at the tearouts on the phone, served by `tool/tearout` on the
tailnet, and approved them on 2026-10-06 through dispatch with no changes. The
players had no file to play, and a PDF in a frame is still to be judged on a
real one in iteration 8.

> **Completed** 2026-10-06 22:56 UTC
>
> - acks: reviewed, phone-approved
> - `make check` — 5.4s
> - `go test -race ./...` — 31.0s

## Iteration 5: Rendered on the file page

- [x] The image before and after with dimensions and bytes, the players, the PDF
      frame and link, the page frame, and SVG as an image
- [x] Page and Source, Image and Source in the toolbar, and the round's stored
      view as the default
- [x] The too-large and other lines naming kind and size
- [x] Tests for each kind's page, and a browser test that a page's frame loads
      its stylesheet from the round

**Departures, iteration 5.**

- A page or SVG in its Source view has Page or Image and Source in its toolbar,
  as markdown has Document and Source, and no Marked and Plain. Source is the
  whole file with the change marked, so there is no Plain view of HTML or SVG.
- A file the table names other, such as a ZIP archive, is named by its kind even
  when its bytes pass diff's binary test, which a short archive can. A file
  whose name says text or markdown and whose bytes are binary is named "Binary
  file", since its name is wrong about it.
- A file read live from the working copy has no raw address, since the route
  reads frozen rounds only. One the browser would show is named, with "Read from
  the working copy, which is not sent."
- A file shown as itself skips the line comparison, so an image not changed
  since the previous round has no "Not changed since round N" note. Reading both
  sides to diff them was the cost of keeping it.

> **Completed** 2026-10-06 23:04 UTC
>
> - acks: reviewed
> - `make check` — 5.1s
> - `go test -race ./...` — 32.6s

## Iteration 6: Rendered in the review's cards

- [x] The same views in the review page's cards and deferred card bodies,
      following the round's stored view
- [x] A rendered card taking a comment on the file
- [x] Tests over the cards, deferred and inline

**Departures, iteration 6.**

- A card for a file shown as itself, or named by the facts line, has no line
  counts, since it moved by no lines. A page in its frame keeps the counts of
  its source.
- A tap on an image or a caption in a card, or on the facts line, opens a
  comment on the file. A player keeps its own taps, and the Open link follows.
- A round whose repository is gone shows its captured files without a
  comparison, through a separate path, and that path still shows a binary file
  as the binary line. Rendering there needs the raw route, which reads the base
  from that same repository.

> **Completed** 2026-10-06 23:10 UTC
>
> - acks: reviewed
> - `make check` — 5.2s
> - `go test -race ./...` — 33.7s

## Iteration 7: Seeded

- [x] `tool/seed` gaining a review with an image changed, added and deleted, an
      SVG, audio, video, a PDF, an HTML page with a stylesheet beside it, a file
      past the send limit, and a binary file
- [x] A view override on the HTML page in the seeded round

**Departures, iteration 7.**

- The seeded review is in a fifth project, `status-page`. Its images, the WAV
  and the PDF are made by the seed in Go. The MP4 is three seconds of ffmpeg's
  test pattern, committed at 17 kB under `tool/seed/testdata`, since a real
  video is the only way to see one play and the build must not need ffmpeg.
- Seen at 390 pixels in Chromium, a 48 pixel icon is stretched to the card's
  width and reads as pixelated. Whether a small image should show at its own
  size instead is put to the owner in iteration 8.

> **Completed** 2026-10-06 23:16 UTC
>
> - acks: reviewed
> - `make check` — 5.1s
> - `go test -race ./...` — 32.3s

## Iteration 8: On the phone

```backlog
required_acks:
  - reviewed
  - phone-checked
```

This iteration is the owner's. The driving agent serves the seeded review on the
tailnet and lists what to check. `phone-checked` is given only by the owner.

- [/] Each kind checked on the phone, in the file page and the card
- [ ] The PDF view decided: frame, link, or both, and the spec and code
      following
- [ ] The page frame's height judged, and what was found recorded in this spec
