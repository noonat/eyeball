import { tokenHeaders } from "./token";

// followPush runs the queue's first-run steps and keeps this device's push
// subscription current. A device posts its subscription on every load, which
// is how it learns the server marked it lapsed, since no push reaches a dead
// subscription to say so.
export function followPush(): void {
  const firstRun = document.getElementById("firstrun");
  if (firstRun === null) {
    return;
  }
  setBadge(document.querySelectorAll(".row.waiting").length);
  document.getElementById("allow")?.addEventListener("click", () => {
    void allow();
  });
  document.getElementById("resubscribe")?.addEventListener("click", () => {
    void resubscribe();
  });
  void refresh();
}

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

async function refresh(): Promise<void> {
  if (pushable() && Notification.permission === "granted") {
    const reg = await navigator.serviceWorker.ready;
    const sub =
      (await reg.pushManager.getSubscription()) ?? (await subscribe(reg));
    show(sub === null ? "steps" : await post(sub));
    return;
  }
  show("steps");
}

async function allow(): Promise<void> {
  if (!pushable()) {
    return;
  }
  // iOS asks for permission only from a tap, so this runs in the click.
  if ((await Notification.requestPermission()) !== "granted") {
    return;
  }
  await refresh();
}

async function resubscribe(): Promise<void> {
  const reg = await navigator.serviceWorker.ready;
  await (await reg.pushManager.getSubscription())?.unsubscribe();
  const sub = await subscribe(reg);
  show(sub === null ? "steps" : await post(sub));
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
async function post(
  sub: PushSubscription,
): Promise<"active" | "lapsed" | "steps"> {
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

// show puts up what applies: the steps until both are done, the lapsed
// warning for a dead subscription, or nothing.
function show(state: "active" | "lapsed" | "steps"): void {
  const firstRun = document.getElementById("firstrun");
  const lapsed = document.getElementById("lapsed");
  const installed = standalone() || pushable();
  const done = document.getElementById("install-done");
  const todo = document.getElementById("install-todo");
  const why = document.getElementById("allow-why");
  if (firstRun) {
    firstRun.hidden = state !== "steps";
  }
  if (lapsed) {
    lapsed.hidden = state !== "lapsed";
  }
  if (done) {
    done.hidden = !installed;
  }
  if (todo) {
    todo.hidden = installed;
  }
  if (why && !installed) {
    why.textContent = "Available once the page is opened from the home screen.";
  }
}

// setBadge counts what waits on the installed icon, where there is a badge.
function setBadge(n: number): void {
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
