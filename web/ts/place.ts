import { recall, remember } from "./storage";

interface Place {
  readonly scroll: number;
  readonly open: readonly string[];
}

function isPlace(v: unknown): v is Place {
  return (
    typeof v === "object" &&
    v !== null &&
    "scroll" in v &&
    typeof v.scroll === "number" &&
    "open" in v &&
    Array.isArray(v.open)
  );
}

// keepPlace saves where a reader is on a review page, its scroll and its open
// files, and puts them back on load, so a review half-read and closed reopens
// there. Opening a card loads its body, and the scroll is set again as bodies
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
    const open = [
      ...page.querySelectorAll<HTMLDetailsElement>("details.file[data-path]"),
    ]
      .filter((d) => d.open)
      .map((d) => d.dataset["path"] ?? "");
    remember(`place:${key}`, { scroll: scroller.scrollTop, open });
  };
  let pending: number | undefined;
  scroller.addEventListener("scroll", () => {
    window.clearTimeout(pending);
    pending = window.setTimeout(save, 200);
  });
  page.addEventListener("toggle", save, true);
  window.addEventListener("pagehide", save);

  const saved = recall(`place:${key}`);
  if (!isPlace(saved)) {
    return;
  }
  for (const card of page.querySelectorAll<HTMLDetailsElement>(
    "details.file[data-path]",
  )) {
    card.open = saved.open.includes(card.dataset["path"] ?? "");
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
