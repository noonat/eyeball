import { classes } from "./classes";
import { eltOf } from "./events";
import { setDeciding } from "./state";

// A verdict answers with HX-Redirect to the queue, so the eye blinks on the
// page that loads next, which already shows the new count. A mark naming the
// review is set when the verdict request starts and cleared when it fails, so
// nothing depends on an event htmx raises after it reads HX-Redirect. htmx 2
// sets location.href and returns from the response there, before it raises
// afterRequest or any swap event, so the failure events are the only ones that
// can clear the mark. It lives in sessionStorage, which belongs to this tab
// alone: a verdict in one tab does not blink another.

const markKey = "eyeball:blink";

let verdict = "";

// keepBlink records a verdict as it is sent, for the queue it returns to.
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
    // Both verdicts lower the waiting count, so both blink.
    if ((verdict === "approve" || verdict === "changes") && review) {
      write(review);
      setDeciding(true);
    }
  });
  // Only the events htmx raises for a failure clear the mark. Its
  // afterRequest for an answer carrying HX-Redirect does not count the request
  // successful, because htmx navigates and returns before it says so, and
  // reading that as a failure cleared the mark before the queue could read it.
  // A refused change request is a 422, which app's htmx config swaps in as an
  // answer rather than an error, so it raises no failure event. beforeSwap is
  // raised for it, while a redirect returns before any swap event.
  document.addEventListener("htmx:beforeSwap", clear);
  document.addEventListener("htmx:responseError", clear);
  document.addEventListener("htmx:sendError", clear);
  document.addEventListener("htmx:timeout", clear);
}

// clear drops the mark and the flag when the bar's request was answered with
// something other than the redirect.
function clear(event: Event): void {
  if (eltOf(event)?.id === "bar") {
    write(null);
    setDeciding(false);
  }
}

// blinkOnLoad reads the mark once and clears it. On the queue it blinks the eye
// when the marked review has a row and that row is not under Waiting, which is
// where a verdict puts it: Approved goes to Settled and changes requested to
// Working. A mark left by a request whose answer never arrived names a review
// that is still under Waiting, or has no row, and does not blink. Under reduced
// motion app.css draws .blink as no motion at all.
export function blinkOnLoad(): void {
  const marked = read();
  if (marked === null) {
    return;
  }
  write(null);
  if (location.pathname !== "/") {
    return;
  }
  const row = document.querySelector(`a.row[href="/reviews/${marked}"]`);
  if (row !== null && !row.classList.contains("waiting")) {
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
