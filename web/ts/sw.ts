// The service worker keeps the app's shell for when the daemon cannot answer.
// It answers two kinds of request and nothing else:
//
//   - a navigation to /, from the daemon first, and from the cache when the
//     daemon refuses the connection or has not answered within a second;
//   - a file in the shell's list, from whichever cache holds its exact address.
//
// The live stream, the API, every write, the beacons and every template page go
// to the daemon untouched, so a template page with the daemon down fails as it
// did before the worker kept anything.
//
// The worker also exists for push, because iOS delivers web push only to an
// installed page with a registered worker.

// The file is a module, through the export below, so this declaration
// narrows the worker's global self to the service worker's scope rather than
// colliding with the library's.
declare const self: ServiceWorkerGlobalScope;

// A hash of this script, set by the Go build that bundles it. The daemon logs
// the hash of the sw.js it serves beside each beacon's, so a phone still
// running an older worker shows as two hashes that differ.
declare const workerBuild: string;

// The token of the daemon's build and the files to keep for it, written ahead
// of this script by the daemon as it serves it. They are not part of the
// stamp above, which hashes this code alone. web/ts and the Go side agree on
// the header's name and on the files' addresses.
declare const shellBuild: string;
declare const shellFiles: readonly string[];

// The header that names the build a fill request is for. The daemon answers
// 409 when it is another build, which fails the install.
const buildHeader = "Eyeball-Build";

// How long the daemon has to answer a navigation before the cache does. A
// guess: a refused connection fails in milliseconds, so a stopped daemon falls
// back at once and only a machine that cannot be reached at all waits it out.
const shellWait = 1000;

// How long a window has to answer the address a tap posts to it.
const answerWait = 1000;

const cacheName = `shell-${shellBuild}`;

// What the daemon sends with each request.
export interface Payload {
  readonly title: string;
  readonly body: string;
  readonly url: string;
  readonly badge: number;
}

function payloadOf(data: { json(): unknown } | null): Payload | null {
  if (data === null) {
    return null;
  }
  let v: unknown;
  try {
    v = data.json();
  } catch {
    return null;
  }
  if (
    typeof v !== "object" ||
    v === null ||
    !("title" in v) ||
    typeof v.title !== "string" ||
    !("body" in v) ||
    typeof v.body !== "string" ||
    !("url" in v) ||
    typeof v.url !== "string" ||
    !("badge" in v) ||
    typeof v.badge !== "number"
  ) {
    return null;
  }
  return { title: v.title, body: v.body, url: v.url, badge: v.badge };
}

// setBadge counts what waits on the installed icon, where the platform has a
// badge.
async function setBadge(n: number): Promise<void> {
  const nav: object = self.navigator;
  if ("setAppBadge" in nav && typeof nav.setAppBadge === "function") {
    await Reflect.apply(nav.setAppBadge, nav, [n]);
  }
}

// fillCache fetches each shell file from the daemon into this build's cache. A
// file that fails, including a 409 for another build, fails the install, so the
// worker already in control stays. The half-filled cache is deleted, since a
// cache named for this build would otherwise read as complete.
async function fillCache(): Promise<void> {
  const cache = await caches.open(cacheName);
  try {
    await Promise.all(
      shellFiles.map(async (file) => {
        const res = await fetch(file, {
          headers: { [buildHeader]: shellBuild },
          cache: "reload",
        });
        if (!res.ok) {
          throw new Error(`${file} answered ${res.status}`);
        }
        await cache.put(file, res);
      }),
    );
  } catch (e) {
    await caches.delete(cacheName);
    throw e;
  }
}

// A new version replaces the old at once rather than waiting for every
// window to close, which an installed app's window rarely does, and takes
// control of the windows already open, so a tap can load a round in them. It
// does so only once its cache is full.
self.addEventListener("install", (event) => {
  event.waitUntil(fillCache().then(() => self.skipWaiting()));
});
self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((names) =>
        Promise.all(
          names
            .filter((name) => name !== cacheName)
            .map((name) => caches.delete(name)),
        ),
      )
      .then(() => self.clients.claim()),
  );
});

// shellPage answers a navigation to /: the daemon's page when it answers in
// time, otherwise the cached one. With nothing cached it goes on waiting for
// the daemon, which is what a browser with no worker does.
async function shellPage(request: Request): Promise<Response> {
  const network = fetch(request);
  const late = new Promise<null>((resolve) => {
    self.setTimeout(() => {
      resolve(null);
    }, shellWait);
  });
  try {
    const res = await Promise.race([network, late]);
    if (res !== null) {
      return res;
    }
  } catch {
    // The daemon refused the connection. The cache answers.
  }
  const own = await caches.open(cacheName);
  return (await own.match("/")) ?? network;
}

// A file of the shell is answered from whichever cache holds its exact
// address, so a page draws the files of the build its HTML named, whichever
// worker is in control. An address no cache holds goes to the daemon.
async function shellFile(request: Request): Promise<Response> {
  return (await caches.match(request)) ?? fetch(request);
}

const shellSet = new Set(shellFiles.filter((file) => file !== "/"));

self.addEventListener("fetch", (event) => {
  const { request } = event;
  const url = new URL(request.url);
  if (request.method !== "GET" || url.origin !== self.location.origin) {
    return;
  }
  if (
    request.mode === "navigate" &&
    url.pathname === "/" &&
    url.search === ""
  ) {
    event.respondWith(shellPage(request));
    return;
  }
  if (shellSet.has(url.pathname + url.search)) {
    event.respondWith(shellFile(request));
  }
});

self.addEventListener("push", (event) => {
  const p = payloadOf(event.data);
  if (p === null) {
    return;
  }
  // Without waitUntil the worker can be stopped before the notification
  // shows, and the push is dropped with no error.
  event.waitUntil(
    Promise.all([
      self.registration.showNotification(p.title, {
        body: p.body,
        data: p.url,
        icon: "/static/icons/icon-192.png",
        tag: p.url,
      }),
      setBadge(p.badge),
    ]),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url: unknown = event.notification.data;
  if (typeof url === "string") {
    event.waitUntil(openReview(event, url));
  }
});

// openReview lands a tap on the round. A window already there is focused.
// Otherwise the first window is asked to move itself: the address is posted to
// it with a port, and an answer within a second means the page is loading the
// review, so the window is only focused. Both bundles answer, the React app by
// moving its router where it can. A window that does not answer, such as a page
// older than the worker, is navigated to the round, and if that fails or there
// is no window, one is opened. An installed app on iOS has one window, and
// openWindow only brings it forward without loading the address.
//
// Each step reports to the daemon, which logs it. A tap on a phone shows
// nothing of which branch ran, and the report is how the next fault in one is
// read. A report is sent before the step it names awaits, so a step that never
// settles leaves the earlier lines logged.
async function openReview(event: ExtendableEvent, url: string): Promise<void> {
  const report = reporter(event);
  const windows = await self.clients.matchAll({
    type: "window",
    includeUncontrolled: true,
  });
  const controlled = await self.clients
    .matchAll({ type: "window" })
    .then((list) => list.map(seen), failure);
  report({
    step: "start",
    url,
    windows: windows.map(seen),
    controlled,
  });
  const there = windows.find((w) => new URL(w.url).pathname === url);
  if (there) {
    await traced(report, "focus", () => there.focus());
    return;
  }
  const open = windows[0];
  if (open) {
    report({ step: "post" });
    const asked = Date.now();
    const answered = await askToMove(open, url);
    report({ step: "answer", answered, ms: Date.now() - asked });
    if (answered) {
      await traced(report, "focusAfterAnswer", () => open.focus());
      return;
    }
    const moved = await traced(report, "navigate", () =>
      open.navigate(url),
    ).catch(() => null);
    if (moved) {
      await traced(report, "focusAfterNavigate", () => moved.focus());
      return;
    }
  }
  await traced(report, "openWindow", () => self.clients.openWindow(url));
}

// askToMove posts the address to a window with a port and resolves true when
// the window answers on it, false after answerWait or when the post fails. The
// wait is a guess: a page answered in under a millisecond in headless Chromium
// on 2026-10-29, and a page with no listener gave nothing at all.
function askToMove(w: WindowClient, url: string): Promise<boolean> {
  const { port1, port2 } = new MessageChannel();
  return new Promise<boolean>((resolve) => {
    const timer = self.setTimeout(() => {
      resolve(false);
    }, answerWait);
    port1.addEventListener("message", () => {
      self.clearTimeout(timer);
      resolve(true);
    });
    // A port listened to with addEventListener stays closed to messages until
    // it is started.
    port1.start();
    try {
      w.postMessage({ url }, [port2]);
    } catch {
      self.clearTimeout(timer);
      resolve(false);
    }
  }).finally(() => {
    port1.close();
  });
}

// What a beacon says of one window.
interface Seen {
  readonly url: string;
  readonly visibilityState: string;
  readonly focused: boolean;
}

function seen(w: WindowClient): Seen {
  return {
    url: w.url,
    visibilityState: w.visibilityState,
    focused: w.focused,
  };
}

// An error as a beacon carries it. A DOMException and a TypeError both have a
// name and a message, and the name is what tells a refused navigate from a
// missing window.
interface Failure {
  readonly name: string;
  readonly message: string;
}

function failure(e: unknown): Failure {
  if (e instanceof Error) {
    return { name: e.name, message: e.message };
  }
  return { name: "unknown", message: String(e) };
}

// Report sends one beacon without waiting for it.
type Report = (detail: Record<string, unknown>) => void;

// reporter returns the Report for one tap. Each beacon is its own waitUntil, so
// the worker stays alive until it is sent, and carries the tap's id and its
// place in the tap, since two fetches can arrive in either order.
function reporter(event: ExtendableEvent): Report {
  const tap = Date.now().toString(36);
  let n = 0;
  return (detail) => {
    n += 1;
    event.waitUntil(send({ build: workerBuild, tap, n, ...detail }));
  };
}

// send posts a beacon. One that fails is dropped, because a tap that cannot
// report must still open the review. On the loopback listener every beacon
// fails this way: it needs a token that lives in page storage, which a worker
// cannot read.
async function send(beacon: Record<string, unknown>): Promise<void> {
  try {
    await fetch("/taps", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(beacon),
    });
  } catch {
    // Nothing to do. See above.
  }
}

// traced runs one step of the tap, reporting before it and after it settles.
// The step's own outcome is returned or thrown unchanged.
async function traced<T extends { readonly url: string } | null>(
  report: Report,
  step: string,
  run: () => Promise<T>,
): Promise<T> {
  report({ step, phase: "call" });
  try {
    const result = await run();
    report({
      step,
      phase: "done",
      result: result === null ? null : result.url,
    });
    return result;
  } catch (e) {
    report({ step, phase: "failed", error: failure(e) });
    throw e;
  }
}
