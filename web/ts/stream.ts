import { openStream } from "./connection";
import { newerBuild, pageBuild } from "./update";

// followStream opens the page's live stream, which says whether the daemon is
// there and which build it runs. The pages that remain print no stamp, so a
// change to the store reloads none of them.
export function followStream(): void {
  const url = "/events";
  const stream = openStream(url, {
    stamp: () => undefined,
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
}

// showNotice shows or hides the bar under the top bar. It is shown and hidden
// through its holder, since .offline sets display, which outranks hidden.
function showNotice(down: boolean): void {
  const holder = document.getElementById("offline");
  if (holder) {
    holder.hidden = !down;
  }
}
