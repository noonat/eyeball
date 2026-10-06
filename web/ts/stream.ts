import { setOffline, unsavedText } from "./state";
import { followBuild } from "./update";

// noticeDelay is how long a stream may be down before the notice shows, so a
// phone waking from sleep does not flash it. Two seconds is a guess.
const noticeDelay = 2000;

// followStream opens the page's live stream. A page that prints a stamp
// reloads when the stream sends a different one, once nothing on it is
// unsaved. Every page holds a stream, since its state is the connection.
export function followStream(): void {
  const page = document.querySelector<HTMLElement>(".page[data-stream]");
  const url = page?.dataset["stream"] ?? "/events";
  const stamp = page?.dataset["stamp"] ?? "";
  let source: EventSource | null = null;
  let timer: number | undefined;
  let stale = false;

  const reload = (): void => {
    if (stale && !unsavedText(true)) {
      window.location.reload();
    }
  };
  const open = (): void => {
    source?.close();
    source = new EventSource(url);
    followBuild(source);
    source.addEventListener("open", () => {
      window.clearTimeout(timer);
      timer = undefined;
      showNotice(false);
    });
    source.addEventListener("error", () => {
      timer ??= window.setTimeout(() => {
        showNotice(true);
      }, noticeDelay);
    });
    source.addEventListener("message", (event) => {
      if (stamp !== "" && event.data !== stamp) {
        stale = true;
        reload();
      }
    });
  };

  // navigator.onLine is trusted only when it says false.
  window.addEventListener("offline", () => {
    showNotice(true);
  });
  document.addEventListener("click", (event) => {
    const retry =
      event.target instanceof Element
        ? event.target.closest("#offline .retry")
        : null;
    if (retry) {
      event.preventDefault();
      open();
    }
  });
  // A reload waiting on unsaved text happens once the text is stored, sent
  // or discarded.
  document.addEventListener("input", reload);
  document.addEventListener("htmx:afterSwap", reload);
  open();
}

// showNotice shows or hides the bar under the top bar. It is shown and hidden
// through its holder, since .offline sets display, which outranks hidden.
function showNotice(down: boolean): void {
  const holder = document.getElementById("offline");
  if (holder) {
    holder.hidden = !down;
  }
  setOffline(down);
}
