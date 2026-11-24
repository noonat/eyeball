// The page's half of a notification tap. The service worker posts the review's
// address to the window it found, with a port, and waits a second for an answer
// on it. An answer tells the worker that this page is moving itself, so the
// worker only focuses the window. Without one, the worker navigates the window,
// which reloads the whole app and once failed to open the named review.

// address reads the address out of the worker's message. Only one on this
// origin is taken, so a message can move the page nowhere else.
function address(data: unknown): string | null {
  if (typeof data !== "object" || data === null || !("url" in data)) {
    return null;
  }
  if (typeof data.url !== "string") {
    return null;
  }
  const parsed = new URL(data.url, location.origin);
  if (parsed.origin !== location.origin) {
    return null;
  }
  return parsed.pathname + parsed.search + parsed.hash;
}

// answerTaps answers each tap the worker posts, then hands the address to move.
// The answer goes first, since a move that loads another page ends this one
// before it could send it. It returns what removes the listener.
export function answerTaps(move: (to: string) => void): () => void {
  if (!("serviceWorker" in navigator)) {
    return () => undefined;
  }
  const listen = (event: MessageEvent): void => {
    const to = address(event.data);
    const port = event.ports[0];
    if (to === null || port === undefined) {
      return;
    }
    port.postMessage("moving");
    move(to);
  };
  navigator.serviceWorker.addEventListener("message", listen);
  return () => {
    navigator.serviceWorker.removeEventListener("message", listen);
  };
}

// load moves a page to an address by loading it, for a page with no router.
export function load(to: string): void {
  location.assign(to);
}
