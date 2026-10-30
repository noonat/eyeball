// The service worker exists for push and nothing else, as architecture.md
// says. iOS delivers web push only to an installed page with a registered
// worker. It intercepts no fetches.

// The file is a module, through the export below, so this declaration
// narrows the worker's global self to the service worker's scope rather than
// colliding with the library's.
declare const self: ServiceWorkerGlobalScope;

// A hash of this script, set by the Go build that bundles it. The daemon logs
// the hash of the sw.js it serves beside each beacon's, so a phone still
// running an older worker shows as two hashes that differ.
declare const workerBuild: string;

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

// A new version replaces the old at once rather than waiting for every
// window to close, which an installed app's window rarely does, and takes
// control of the windows already open, so a tap can load a round in them.
self.addEventListener("install", () => {
  void self.skipWaiting();
});
self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
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
// An installed app on iOS has one window, and openWindow only brings it
// forward without loading the address, so an open window is navigated to the
// round instead; with the app open on the queue, a tap left it there.
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
