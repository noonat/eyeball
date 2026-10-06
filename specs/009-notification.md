---
status: active
created: 2026-10-06T00:00:00Z
updated: 2026-10-06T17:43:01.860019257Z
required_acks:
  - reviewed
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
---

# 009 Notification

> **Autonomous run, 2026-10-04, as changed on 2026-10-06.** Part of the owner's
> request to build the project to completion as product.md and architecture.md
> describe it. Since 2026-10-06 the agent driving the run implements and reviews
> every iteration itself, with no subagents, and the owner's word stands as
> approval for every ack until the owner says otherwise. The review gate is off.
> Iteration 1 changes a tearout and carries `phone-approved`, which only the
> owner gives after looking on the phone. Iteration 6 is the owner's: joining
> the tailnet, installing the page, granting permission, and checking every
> screen on the phone. Nothing before it sends a push to a real push service.
> Departures from this spec are recorded in it as they happen.

A review asked for while the reviewer is away from the desk has to reach them.
This spec makes the surface installable, subscribes a device to web push, sends
one notification per request deep linked to its round, keeps the icon's badge
counting what waits, and reports a subscription that has lapsed. It is item 1
under Next in specs/TODO.md.

## Design

### Who may write on the tailnet listener

The owner decided on 2026-10-06: any device that reaches the tailnet listener
may comment and decide, with no check of which device or which login. Rejected:
refusing writes from a device whose tailnet address is on one of this machine's
own interfaces, which tsnet's `WhoIs` makes possible; pairing each device with a
one-time link, as the loopback listener does; and requiring the login that
enrolled the node. The cost, stated so nothing downstream assumes otherwise: an
agent on this machine that has tailnet access through the machine's own
Tailscale can approve a review through the tailnet listener with a plain
request. It could already write `eyeball.db` directly. 008 left this open and
pointed here; architecture.md's listeners section says so, and iteration 2
records the answer there.

### Installable

**A manifest and icons make the page installable.** `GET /manifest.webmanifest`
answers the name "eyeball", `start_url` "/", `display` "standalone", the theme
and background colors from app.css's light tokens, and icons at 192 and 512
pixels. The layout links it, and an `apple-touch-icon` at 180 pixels, which iOS
reads instead of the manifest's icons. The policy's `default-src 'self'` covers
the manifest and the worker.

**The icon is the tearout's badge icon**: the letter e on the accent color, as
foundations.html's first-run frame draws it. The PNGs are rendered once from an
HTML page by headless Chromium, through `tool/appicon`, and committed under
`web/static/icons/`, so the build needs no renderer. Drawing them in Go was the
alternative, and needs a font and a rasterizer as dependencies for three files
that change once.

**The service worker exists for push and nothing else**, as architecture.md
says. `web/ts/sw.ts` is a second esbuild entry, served at `/sw.js` so its scope
is the whole surface. It handles `push` by showing the notification and setting
the badge, and `notificationclick` by focusing a window already on the review or
opening one. It intercepts no fetches. A worker's global scope is not a window,
so it compiles under its own `web/tsconfig.sw.json` with the `webworker`
library, and the page's `tsconfig.json` leaves it out; `make lint` checks both.

### Subscriptions

**The VAPID keys are generated on first use and live in the store.** Migration
`0007` adds a one-row `vapid` table, holding the key pair and when it was made,
and a `subscriptions` table: the endpoint, unique; its `p256dh` and `auth` keys;
when it was created and last confirmed; and when it lapsed, `''` while it has
not. The public key reaches the page in a `vapid-key` meta, since a page with no
key cannot subscribe.

**A device posts its subscription on every page load** while permission is
granted. `POST /subscriptions` takes the browser's `PushSubscription.toJSON()`,
stores a new endpoint or confirms a known one, and answers its state, `active`
or `lapsed`. Posting on every load is what lets a device learn that the server
marked its subscription lapsed, since no push reaches a dead subscription to say
so. It is a write, so on the loopback listener it carries the token like any
other, through the same header.

**A subscription the push service calls gone is marked lapsed, not retried.** A
404 or 410 from the endpoint marks it lapsed, as architecture.md says. Any other
failure is logged and the subscription stays active, since a push service that
is briefly down says nothing about the device.

### Sending

**A request sends one notification to every active subscription**, after the
round is stored: a new review and a new round alike. Its payload is
`{"title": the review's title, "body": the project and the round, "url": the round's address, "badge": the number of reviews waiting}`.
Comments and verdicts send none, as product.md says.

**Sending never holds the agent.** The request's answer goes out first, and the
pushes go from a goroutine, each with a ten-second timeout. A push service that
hangs would otherwise hang `eyeball request`. The daemon's shutdown waits for
pushes in flight up to its existing grace.

**The subscriber in the VAPID claim is the surface's address** on the tailnet,
an `https:` URL, which push services accept in place of a `mailto:`. A daemon
with no tailnet address sends nothing, since no installed page can have
subscribed without one.

**The badge counts what waits, and is only as fresh as the last push or page
view.** The worker sets it from each push's payload with `setAppBadge`, and an
open page sets it from the queue's count on load and on every stamp the stream
sends. iOS shows every push, so there is no silent push to correct the badge
when a review is decided on another device; it is corrected the next time a page
opens or a request arrives. A badge cleared on decision would need a
notification the reviewer would see, which product.md rules out.

### First run

**The queue says what is missing, in order, until both steps are done**, above
the reviews, as queue.html's approved "not installed yet" frame draws it: the
warn notice, then the two steps. Iteration 1 draws the two states no frame
showed, an installed page with Allow left and a lapsed subscription.
foundations.html's icon row illustrates the badge and is not placed on the
queue.

- _Installed to the home screen_ is done when the page runs standalone, or when
  the browser offers push in a tab, as every browser but iOS Safari does. On iOS
  Safari in a tab it says to use Share, then Add to Home Screen.
- _Allow notifications_ has the Allow button, which asks for permission and
  subscribes in the same tap, since iOS asks only from a user's gesture.

**A lapsed subscription says so**, with the drawn warning and its Re-subscribe
link, which unsubscribes the dead endpoint and subscribes again. A device whose
permission was granted but has no subscription subscribes again on load, without
asking.

The steps, their notice and the lapsed warning are rendered by the server,
hidden, in holders with no class, and the client shows the ones that apply, as
008 does for its notices.

## Out of scope

- **A chat bot.** product.md's escape hatch, used only if web push proves
  unreliable in practice, which this spec cannot know yet.
- **Choosing which reviews notify**, or quiet hours. Nothing asks for either.
- **Writes on the tailnet limited by device or login.** The owner declined both.

## Not converged

No review pass has run on this spec, under the owner's 2026-10-06 change. What
is least certain: that iOS shows the badge from `setAppBadge` in a worker, and
that it accepts an `https:` subscriber in the VAPID claim. Both are documented
behavior, assumed here and not measured; iteration 6 measures them on the phone.

## Probes

- **webpush-go v1.4.0** generated a VAPID key pair, and sent a push to an
  `httptest` endpoint standing in for a push service: a POST with
  `Content-Encoding: aes128gcm`, a body padded to 4096 bytes, `TTL: 3600` and a
  `vapid` Authorization header. The endpoint's 201 and 410 came back as the
  response's status. It adds `golang-jwt/jwt/v5` to the module.

**Departures, iteration 1.**

- queue.html already held an approved "not installed yet" frame placing both
  steps above the reviews, with a warn notice and no icon row. The new section
  draws the two states it did not: an installed page with Allow left, and a
  lapsed subscription on a populated queue. The todos' icon row, from
  foundations.html's illustration of the badge, is not placed on the queue.
- The app icon is not shown in a frame. The frames draw no icon row, and the PNG
  is iteration 2's; the owner sees it on the home screen in iteration 6.

## Iteration 1: The tearouts for first run

```backlog
required_acks:
  - reviewed
  - phone-approved
```

- [x] queue.html gaining a frame of the queue on a device that has not
      subscribed: the icon row and the two steps at the top, in both themes
- [x] queue.html gaining a frame with the lapsed warning at the top of a
      populated queue
- [x] The app icon at 180 pixels shown in the frame's icon row, rendered from
      the badge icon
- [x] README's queue.html row naming the two new frames

> **Completed** 2026-10-06 18:32 UTC
>
> - acks: reviewed, phone-approved
> - `make check` — 4.8s
> - `go test -race ./...` — 26.8s

## Iteration 2: Installable

- [x] `GET /manifest.webmanifest` with the name, start URL, standalone display,
      colors and icons, linked from the layout with the apple-touch-icon
- [x] `tool/appicon` rendering the 180, 192 and 512 pixel icons from an HTML
      page with headless Chromium, and the icons committed and served
- [x] `web/ts/sw.ts` built as a second entry and served at `/sw.js`, handling
      push and notificationclick only, under `web/tsconfig.sw.json`
- [x] `make lint` running tsc over both configurations
- [x] The page registering the worker, with a browser test that it is registered
      and that the manifest is served with its icons
- [x] architecture.md's listeners section recording the owner's answer on
      tailnet writes, and the routes table gaining the new routes

**Departures, iteration 2.**

- The manifest is a constant in Go rather than a file under `web/`, since it
  holds nothing but the name, the colors and the icons' addresses, and the test
  parses what the route answers.
- The layout also carries `apple-mobile-web-app-capable`, which older iOS reads
  before the manifest's `display`.
- `web/ts/sw.ts` is a module through its exported `Payload` interface, so its
  declaration of `self` narrows the worker's global rather than colliding with
  it. oxlint's type-aware pass did not resolve the worker library's
  `PushMessageData`, so the parser takes the structural `{ json(): unknown }`.
  oxlint runs twice, once per configuration, the page's pass ignoring the
  worker.
- The icon is drawn full bleed, without the badge icon's rounded corners, since
  iOS and Android mask an icon to their own shape.

> **Completed** 2026-10-06 18:38 UTC
>
> - acks: reviewed
> - `make check` — 5.0s
> - `go test -race ./...` — 26.8s

## Iteration 3: Subscriptions

- [ ] Migration `0007`: the `vapid` table and the `subscriptions` table, with a
      test reading an older database forward
- [ ] VAPID keys generated on first use and read thereafter, with the public key
      in the layout's `vapid-key` meta
- [ ] `POST /subscriptions` storing a new endpoint, confirming a known one, and
      answering `active` or `lapsed`, refusing a malformed body with 400
- [ ] The token required on loopback as for any write, with a row

## Iteration 4: Sending

- [ ] A request sending one push to every active subscription, from a goroutine
      after the answer, with the title, body, round URL and waiting count,
      tested against an `httptest` push service
- [ ] A 404 or 410 marking the subscription lapsed, and a 500 leaving it active,
      each with a row
- [ ] No push for a comment or a verdict, with a test
- [ ] A hung push service not delaying the request's answer, and shutdown
      waiting for pushes in flight up to the grace
- [ ] No push at all from a daemon with no tailnet address

## Iteration 5: First run on the page

```backlog
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -tags browser ./internal/server/...
  - cmd: go vet -tags browser ./internal/server/...
  - cmd: go tool staticcheck -tags browser ./internal/server/...
```

- [ ] The queue showing the steps that apply, the icon row with them, and
      neither once the device is subscribed
- [ ] Allow asking for permission and subscribing in one tap, then posting the
      subscription
- [ ] A subscription posted on every load, and the lapsed warning with
      Re-subscribe when the answer is `lapsed`
- [ ] The badge set from the queue's count on load and on each stamp
- [ ] Browser tests with the push manager stubbed through the DevTools protocol:
      Allow posting a subscription, a request then reaching the stubbed
      endpoint, and a lapsed answer showing the warning

## Iteration 6: On the phone

```backlog
required_acks:
  - reviewed
  - phone-checked
```

This iteration is the owner's. The driving agent prepares the commands and the
list of screens; the owner runs `eyeball tailnet login` at a terminal, installs
the page, grants permission, and checks each screen. `phone-checked` is given
only by the owner.

- [ ] The owner joining the tailnet with `eyeball tailnet login`
- [ ] The page installed to the home screen and permission granted from the
      queue's steps
- [ ] A request from an agent arriving as a notification on the locked phone,
      opening its round, with the badge counting what waits
- [ ] Every screen checked on the phone, the page shell 006 added in `shell.css`
      included, and what was found recorded in this spec
