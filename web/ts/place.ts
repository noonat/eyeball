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
  const scroller = page?.closest<HTMLElement>(".scroll");
  const key = page?.dataset["place"];
  if (
    page === null ||
    page === undefined ||
    scroller === null ||
    scroller === undefined ||
    key === undefined
  ) {
    return;
  }
  const save = (): void => {
    const folded = [
      ...page.querySelectorAll<HTMLDetailsElement>("details.file[data-path]"),
    ]
      .filter((d) => !d.open)
      .map((d) => d.dataset["path"] ?? "");
    remember(`place:${key}`, { scroll: scroller.scrollTop, folded });
  };
  let pending: number | undefined;
  scroller.addEventListener("scroll", () => {
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
    scroller.scrollTop = saved.scroll;
  };
  restore();
  document.addEventListener("htmx:afterSettle", restore);
  window.setTimeout(() => {
    document.removeEventListener("htmx:afterSettle", restore);
  }, 3000);
}
