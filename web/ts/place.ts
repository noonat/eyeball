import { recall, remember } from "./storage";

interface Place {
  readonly scroll: number;
  readonly folded: readonly string[];
}

// readPlace takes a saved place. An entry from before every card opened holds
// the open files instead of the folded ones, and on a large review it lists
// almost none, so applying it would fold nearly every card. Its scroll is kept
// and its list dropped, and the next save writes the new shape over it.
function readPlace(v: unknown): Place | null {
  if (
    typeof v !== "object" ||
    v === null ||
    !("scroll" in v) ||
    typeof v.scroll !== "number"
  ) {
    return null;
  }
  const folded =
    "folded" in v && Array.isArray(v.folded)
      ? v.folded.filter((p): p is string => typeof p === "string")
      : [];
  return { scroll: v.scroll, folded };
}

// keepPlace saves where a reader is on a review page, its scroll and the files
// they folded, and puts them back on load, so a review half-read and closed
// reopens there. Every card opens by default, so the folds are what is kept: a
// file new to the round opens with the rest. The scroll is set again as bodies
// arrive, since each one moves what lies below it.
export function keepPlace(): void {
  const page = document.querySelector<HTMLElement>(".page[data-place]");
  const key = page?.dataset["place"];
  if (page === null || key === undefined) {
    return;
  }
  const save = (): void => {
    const folded = [
      ...page.querySelectorAll<HTMLDetailsElement>("details.file[data-path]"),
    ]
      .filter((d) => !d.open)
      .map((d) => d.dataset["path"] ?? "");
    remember(`place:${key}`, { scroll: window.scrollY, folded });
  };
  let pending: number | undefined;
  window.addEventListener("scroll", () => {
    window.clearTimeout(pending);
    pending = window.setTimeout(save, 200);
  });
  page.addEventListener("toggle", save, true);
  window.addEventListener("pagehide", save);

  const saved = readPlace(recall(`place:${key}`));
  if (saved === null) {
    return;
  }
  for (const card of page.querySelectorAll<HTMLDetailsElement>(
    "details.file[data-path]",
  )) {
    card.open = !saved.folded.includes(card.dataset["path"] ?? "");
  }
  const restore = (): void => {
    window.scrollTo(0, saved.scroll);
  };
  restore();
  // Setting the place again as bodies arrive stops at the reader's first tap,
  // scroll or key. Until then it would undo what they did: a tap in the list
  // of files during those seconds was pulled back to the saved place.
  const events = ["pointerdown", "click", "wheel", "touchmove", "keydown"];
  const stop = (): void => {
    document.removeEventListener("htmx:afterSettle", restore);
    for (const e of events) {
      document.removeEventListener(e, stop, true);
    }
  };
  document.addEventListener("htmx:afterSettle", restore);
  for (const e of events) {
    document.addEventListener(e, stop, true);
  }
  window.setTimeout(stop, 3000);
}
