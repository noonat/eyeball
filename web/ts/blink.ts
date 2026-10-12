import { classes } from "./classes";
import { eltOf } from "./events";

// Approving answers with HX-Refresh, so the eye blinks on the page that loads
// next, which already shows the new count. A mark naming the review is set
// when the Approve request starts and cleared when it fails, so nothing
// depends on whether htmx raises an event between reading HX-Refresh and
// reloading. It lives in sessionStorage, which belongs to this tab alone: an
// approval in one tab does not blink another.

const markKey = "eyeball:blink";

let verdict = "";

// keepBlink records an Approve as it is sent, and blinks the eye once on the
// page an approval reloads to.
export function keepBlink(): void {
  document.addEventListener(
    "submit",
    (event) => {
      if (
        event.target instanceof HTMLFormElement &&
        event.target.id === "bar"
      ) {
        const by = event.submitter;
        verdict = by instanceof HTMLButtonElement ? by.value : "";
      }
    },
    true,
  );
  document.addEventListener("htmx:beforeRequest", (event) => {
    // A guard that holds the verdict back cancels this event first, and then
    // no request is sent and nothing is marked.
    if (eltOf(event)?.id !== "bar" || event.defaultPrevented) {
      return;
    }
    const review = document.querySelector(".page")?.getAttribute("data-review");
    if (verdict === "approve" && review) {
      write(review);
    }
  });
  // Only the events htmx raises for a failure clear the mark. Its
  // afterRequest for an answer carrying HX-Refresh does not count the request
  // successful, because htmx reloads and returns before it says so, and
  // reading that as a failure cleared the mark before the reload could read it.
  document.addEventListener("htmx:responseError", clear);
  document.addEventListener("htmx:sendError", clear);
  document.addEventListener("htmx:timeout", clear);
}

// clear drops the mark when the bar's request failed.
function clear(event: Event): void {
  if (eltOf(event)?.id === "bar") {
    write(null);
  }
}

// blinkOnLoad reads the mark once and clears it, and blinks the eye if the mark
// names the review on screen and that review is approved. Under reduced motion
// app.css draws .blink as no motion at all.
export function blinkOnLoad(): void {
  const marked = read();
  if (marked === null) {
    return;
  }
  write(null);
  // The current round has two addresses, the review's own and its round's,
  // and an approval reloads whichever it was made on.
  const here = /^\/reviews\/(\d+)(?:\/rounds\/\d+)?$/.exec(
    location.pathname,
  )?.[1];
  const approved = document.querySelector(".brief .pill.approved") !== null;
  if (here === marked && approved) {
    document.querySelector(".topbar .eye")?.classList.add(classes.blink);
  }
}

function read(): string | null {
  try {
    return window.sessionStorage.getItem(markKey);
  } catch {
    return null;
  }
}

function write(review: string | null): void {
  try {
    if (review === null) {
      window.sessionStorage.removeItem(markKey);
    } else {
      window.sessionStorage.setItem(markKey, review);
    }
  } catch {
    // Storage that refuses the mark costs the blink, and nothing else.
  }
}
