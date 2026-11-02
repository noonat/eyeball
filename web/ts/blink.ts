import { eltOf } from "./events";
import { setDeciding } from "./state";

// A verdict answers with HX-Redirect to the queue, so the eye blinks on the
// page that loads next. The queue draws the eye with the count the review
// page's eye showed, which is the count from before the verdict, and changes to
// the new count while the lid is shut. A mark naming the review and that count
// is set when the verdict request starts and cleared when it fails, so
// nothing depends on an event htmx raises after it reads HX-Redirect. htmx 2
// sets location.href and returns from the response there, before it raises
// afterRequest or any swap event, so the failure events are the only ones that
// can clear the mark. It lives in sessionStorage, which belongs to this tab
// alone: a verdict in one tab does not blink another.

const markKey = "eyeball:blink";

// lidShutAfter is how long after the blink's animationstart the lid is shut, in
// milliseconds. app.css's eye-blink runs 420ms and holds scaleY(0.08) from 45%
// to 60% of it, which is 189ms to 252ms in. This is the middle, so a timer that
// runs a few tens of milliseconds late still lands with the lid shut.
export const lidShutAfter = 220;

// blinkGiveUp is how long after the queue draws the old count it shows the new
// one whatever happened, for a browser whose animation never started. The
// blink lasts 620ms from the class, so this is well after it.
export const blinkGiveUp = 1500;

// reducedMotion says whether the device asks for no motion, where app.css draws
// .blink as no animation and there is no lid to change the count behind.
export function reducedMotion(): boolean {
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

// BlinkMark is what a verdict leaves for the queue.
export interface BlinkMark {
  // review is the id of the review the verdict was given to.
  readonly review: string;
  // count is how many reviews waited on the page the verdict was sent from, or
  // null when it did not say.
  readonly count: number | null;
}

// eyeCount is what the page's eye shows now: its iris, or none when it is shut.
function eyeCount(): number | null {
  const eye = document.querySelector(".topbar .eye");
  if (eye === null) {
    return null;
  }
  const iris = eye.querySelector(".iris");
  if (iris === null) {
    return 0;
  }
  const n = Number(iris.textContent);
  return Number.isInteger(n) ? n : null;
}

// setBlinkMark leaves the mark a verdict returns to the queue with. The app
// calls it as the verdict is sent and clears it with clearBlinkMark if it
// fails, so this module is the one place that writes the key.
export function setBlinkMark(mark: BlinkMark): void {
  write(mark);
}

// clearBlinkMark drops the mark, for a verdict that was not stored.
export function clearBlinkMark(): void {
  write(null);
}

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
      setBlinkMark({ review, count: eyeCount() });
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
    clearBlinkMark();
    setDeciding(false);
  }
}

// takeBlinkMark reads the mark once and clears it, so it blinks one load and no
// other. It is the review a verdict was just sent for, or null.
export function takeBlinkMark(): BlinkMark | null {
  const marked = read();
  if (marked !== null) {
    clearBlinkMark();
  }
  return marked;
}

// blinks says whether the queue's eye blinks for a marked review: when the
// review has a row and that row is not under Waiting, which is where a verdict
// puts it. Approved goes to Settled and changes requested to Working. A mark
// left by a request whose answer never arrived names a review that is still
// under Waiting, or has no row, and does not blink.
export function blinks(
  marked: string,
  rows: readonly { readonly id: string; readonly waiting: boolean }[],
): boolean {
  const row = rows.find((candidate) => candidate.id === marked);
  return row !== undefined && !row.waiting;
}

// blinkOnLoad clears a mark left on a page that is not the queue, so it cannot
// blink a later load of the queue. The queue is the React app, which takes the
// mark itself.
export function blinkOnLoad(): void {
  takeBlinkMark();
}

function read(): BlinkMark | null {
  let raw: string | null = null;
  try {
    raw = window.sessionStorage.getItem(markKey);
  } catch {
    return null;
  }
  if (raw === null) {
    return null;
  }
  // The review's id, then the count after a colon. A mark with no count blinks
  // with the new count.
  const [review = "", count] = raw.split(":");
  const n = count === undefined ? Number.NaN : Number(count);
  return { review, count: Number.isInteger(n) && n >= 0 ? n : null };
}

function write(mark: BlinkMark | null): void {
  try {
    if (mark === null) {
      window.sessionStorage.removeItem(markKey);
    } else {
      const count = mark.count === null ? "" : `:${mark.count}`;
      window.sessionStorage.setItem(markKey, mark.review + count);
    }
  } catch {
    // Storage that refuses the mark costs the blink, and nothing else.
  }
}
