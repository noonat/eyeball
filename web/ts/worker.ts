// registerWorker registers the service worker that push needs. A browser
// without workers, or a page not in a secure context, has no push either, and
// the page works the same without it.
export function registerWorker(): void {
  if (!("serviceWorker" in navigator)) {
    return;
  }
  navigator.serviceWorker.register("/sw.js").catch(() => {
    // Push then cannot arrive; the first-run steps say what is missing.
  });
}
