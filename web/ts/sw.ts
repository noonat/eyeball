// The service worker exists for push and nothing else, as architecture.md
// says. iOS delivers web push only to an installed page with a registered
// worker. It intercepts no fetches.

// The file is a module, through the export below, so this declaration
// narrows the worker's global self to the service worker's scope rather than
// colliding with the library's.
declare const self: ServiceWorkerGlobalScope;

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
    event.waitUntil(openReview(url));
  }
});

// openReview lands a tap on the round. A window already there is focused.
// An installed app on iOS has one window, and openWindow only brings it
// forward without loading the address, so an open window is navigated to the
// round instead; with the app open on the queue, a tap left it there.
async function openReview(url: string): Promise<void> {
  const windows = await self.clients.matchAll({
    type: "window",
    includeUncontrolled: true,
  });
  const there = windows.find((w) => new URL(w.url).pathname === url);
  if (there) {
    await there.focus();
    return;
  }
  const open = windows[0];
  if (open) {
    const moved = await open.navigate(url).catch(() => null);
    if (moved) {
      await moved.focus();
      return;
    }
  }
  await self.clients.openWindow(url);
}
