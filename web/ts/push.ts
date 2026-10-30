import { tokenHeaders } from "./token";

// What a device's push state comes to, which the queue's first-run steps and
// its lapsed notice are drawn from. A device posts its subscription on every
// load, which is how it learns the server marked it lapsed, since no push
// reaches a dead subscription to say so.
export type PushState = "active" | "lapsed" | "steps";

// pushable says whether this browser can subscribe here. iOS Safari offers
// push only to an installed page, and every other browser offers it in a tab,
// so a browser that offers it needs no install step.
function pushable(): boolean {
  return (
    "serviceWorker" in navigator &&
    "PushManager" in window &&
    "Notification" in window
  );
}

function standalone(): boolean {
  return window.matchMedia("(display-mode: standalone)").matches;
}

// pushState keeps this device's subscription current and says what it comes
// to: the steps until both are done, the lapsed warning for a dead
// subscription, or active.
export async function pushState(): Promise<PushState> {
  if (pushable() && Notification.permission === "granted") {
    const reg = await navigator.serviceWorker.ready;
    const sub =
      (await reg.pushManager.getSubscription()) ?? (await subscribe(reg));
    return sub === null ? "steps" : await post(sub);
  }
  return "steps";
}

// allowPush asks for permission and then subscribes. It is null where the
// browser cannot subscribe, or the reader said no.
export async function allowPush(): Promise<PushState | null> {
  if (!pushable()) {
    return null;
  }
  // iOS asks for permission only from a tap, so this runs in the click.
  if ((await Notification.requestPermission()) !== "granted") {
    return null;
  }
  return pushState();
}

// resubscribePush replaces a dead subscription with a new one.
export async function resubscribePush(): Promise<PushState> {
  const reg = await navigator.serviceWorker.ready;
  await (await reg.pushManager.getSubscription())?.unsubscribe();
  const sub = await subscribe(reg);
  return sub === null ? "steps" : await post(sub);
}

async function subscribe(
  reg: ServiceWorkerRegistration,
): Promise<PushSubscription | null> {
  const key = document.querySelector<HTMLMetaElement>(
    "meta[name='vapid-key']",
  )?.content;
  if (key === undefined) {
    return null;
  }
  try {
    return await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: decode(key),
    });
  } catch {
    return null;
  }
}

// post sends a subscription and returns what the server says of it.
async function post(sub: PushSubscription): Promise<PushState> {
  try {
    const res = await fetch("/subscriptions", {
      method: "POST",
      headers: { "Content-Type": "application/json", ...tokenHeaders() },
      body: JSON.stringify(sub.toJSON()),
    });
    const body: unknown = await res.json();
    if (
      typeof body === "object" &&
      body !== null &&
      "state" in body &&
      body.state === "lapsed"
    ) {
      return "lapsed";
    }
    return res.ok ? "active" : "steps";
  } catch {
    return "steps";
  }
}

// installed says whether the install step is done: the page runs from the home
// screen, or the browser offers push in a tab and needs no install.
export function installed(): boolean {
  return standalone() || pushable();
}

// setBadge counts what waits on the installed icon, where there is a badge.
export function setBadge(n: number): void {
  const nav: object = navigator;
  if ("setAppBadge" in nav && typeof nav.setAppBadge === "function") {
    void Promise.resolve(Reflect.apply(nav.setAppBadge, nav, [n])).catch(
      () => undefined,
    );
  }
}

// decode turns the base64url public key into the bytes subscribe takes.
function decode(key: string): Uint8Array<ArrayBuffer> {
  const base64 = key.replace(/-/g, "+").replace(/_/g, "/");
  const raw = window.atob(base64 + "=".repeat((4 - (base64.length % 4)) % 4));
  const out = new Uint8Array(new ArrayBuffer(raw.length));
  for (let i = 0; i < raw.length; i++) {
    out[i] = raw.charCodeAt(i);
  }
  return out;
}
