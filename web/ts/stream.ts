import { openStream } from "./connection";
import { isDeciding, setOffline, unsavedText } from "./state";
import { newerBuild, pageBuild } from "./update";

// followStream opens the page's live stream. A page that prints a stamp
// reloads when the stream sends a different one, once nothing on it is
// unsaved. Every page holds a stream, since its state is the connection.
export function followStream(): void {
  const page = document.querySelector<HTMLElement>(".page[data-stream]");
  const url = page?.dataset["stream"] ?? "/events";
  const stamp = page?.dataset["stamp"] ?? "";
  let stale = false;

  const reload = (): void => {
    if (stale && !isDeciding() && !unsavedText(true)) {
      window.location.reload();
    }
  };
  const stream = openStream(url, {
    stamp: (next) => {
      if (stamp !== "" && next !== stamp) {
        stale = true;
        reload();
      }
    },
    build: (newest) => {
      const holder = document.getElementById("update");
      if (holder) {
        holder.hidden = !newerBuild(pageBuild(), newest);
      }
    },
    connection: showNotice,
  });

  document.addEventListener("click", (event) => {
    const retry =
      event.target instanceof Element
        ? event.target.closest("#offline .retry")
        : null;
    if (retry) {
      event.preventDefault();
      stream.reopen();
    }
  });
  // A reload waiting on unsaved text happens once the text is stored, sent
  // or discarded.
  document.addEventListener("input", reload);
  document.addEventListener("htmx:afterSwap", reload);
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
