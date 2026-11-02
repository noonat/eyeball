import { headBottom } from "../../ts/head";

/**
 * Keeps what is at the top of the screen there when a card above it changes
 * height: a body arriving, a document taller than its skeleton, an image
 * loading late. It records the card holding the top of the screen, which is the
 * bottom of the sticky header, and how far that card sits from it, and after any
 * card resizes it scrolls by however far that card moved. A card that grows
 * below the top, or the top card's own body, moves nothing above it, so only
 * growth above is corrected.
 *
 * This is the browser's scroll anchoring done per card. Safari, where the phone
 * reads, has none, and a document's six-line skeleton loading above a jump
 * pushed the target off screen there. Chromium's own is turned off, so every
 * browser takes this one path and a browser test exercises the code Safari
 * runs. Turning it off is undone with the rest, since the other screens never
 * had it off.
 */
export function holdPlace(ids: readonly string[]): () => void {
  const cards = ids.flatMap((id) => {
    const card = document.getElementById(id);
    return card instanceof HTMLDetailsElement ? [card] : [];
  });
  if (cards.length === 0) {
    return () => undefined;
  }
  const root = document.documentElement;
  root.style.overflowAnchor = "none";
  let anchor: Element | undefined;
  let offset = 0;
  const record = (): void => {
    const top = headBottom();
    anchor = cards.find((c) => c.getBoundingClientRect().bottom > top);
    offset = (anchor?.getBoundingClientRect().top ?? top) - top;
  };
  let pending = 0;
  const onScroll = (): void => {
    if (pending === 0) {
      pending = window.requestAnimationFrame(() => {
        pending = 0;
        record();
      });
    }
  };
  window.addEventListener("scroll", onScroll, { passive: true });
  const resized = new ResizeObserver(() => {
    if (anchor === undefined) {
      record();
      return;
    }
    const moved = anchor.getBoundingClientRect().top - headBottom() - offset;
    // A page at its top holds nothing: what grows above the first card, as the
    // letter does when its font arrives, is what the reader is looking at, and
    // scrolling past it would take them down the page. Browsers' own anchoring
    // is off at the top for the same reason.
    if (window.scrollY > 0 && Math.abs(moved) >= 1) {
      window.scrollBy(0, moved);
    }
    record();
  });
  for (const card of cards) {
    resized.observe(card);
  }
  // A card folded from its stuck header keeps its top where it was, far above
  // the screen, so the scroll would leave the reader deep inside the next file.
  // Its header goes back to the top of the screen instead. Only a fold the
  // reader made counts, which a tap on the summary tells from a fold the page
  // made, as when a draft opens the fold around its composer.
  let tapped: Element | undefined;
  const onClick = (event: Event): void => {
    const target = event.target instanceof Element ? event.target : null;
    tapped =
      target?.closest("details.file > summary") && !target.closest("a")
        ? (target.closest("details.file") ?? undefined)
        : undefined;
  };
  document.addEventListener("click", onClick);
  const folds = cards.map((card) => {
    const onToggle = (): void => {
      if (card !== tapped || card.open) {
        return;
      }
      tapped = undefined;
      const above = card.getBoundingClientRect().top - headBottom();
      if (above < 0) {
        window.scrollBy(0, above);
      }
      record();
    };
    card.addEventListener("toggle", onToggle);
    return () => {
      card.removeEventListener("toggle", onToggle);
    };
  });
  record();
  return () => {
    resized.disconnect();
    window.cancelAnimationFrame(pending);
    window.removeEventListener("scroll", onScroll);
    document.removeEventListener("click", onClick);
    for (const unfold of folds) {
      unfold();
    }
    root.style.overflowAnchor = "";
  };
}

/**
 * Measures once, on this device, the heights a skeleton is built from, as
 * `measureRows` did for the template page. It renders a hidden code table in a
 * card, where a body's table sits, and sets on the root element a hunk header, a
 * line, and what the code block adds around its table, such as a scrollbar where
 * the device draws one. They depend on the device and the card's width, so a
 * constant would be right on one device only. A line is the table's growth over
 * ten more lines, not one row's height: in Chromium the first row under a header
 * measured 20.64px and eight averaged 20.27px, since border-collapse shares the
 * header's border with it. The table is removed in the same task, so it is never
 * painted.
 */
export function measureRows(): void {
  const root = document.documentElement;
  if (root.style.getPropertyValue("--row-h") !== "") {
    return;
  }
  // An open card, since a folded one has no height to measure.
  const card =
    document.querySelector("details.file[open]") ??
    document.querySelector(".page");
  if (card === null) {
    return;
  }
  const probe = document.createElement("div");
  probe.className = "code narrow";
  probe.setAttribute("aria-hidden", "true");
  probe.style.visibility = "hidden";
  const table = probe.appendChild(document.createElement("table"));
  const head = table.insertRow();
  head.className = "hunkhead";
  const cell = head.insertCell();
  cell.colSpan = 3;
  cell.textContent = "@@ 1,11 +1,11 @@";
  const addLine = (n: number): void => {
    const row = table.insertRow();
    for (const [cls, text] of [
      ["ln", String(n)],
      ["sign", "+"],
      ["src", "x"],
    ]) {
      const c = row.insertCell();
      c.className = cls ?? "";
      c.textContent = text ?? "";
    }
  };
  addLine(1);
  card.appendChild(probe);
  const one = table.getBoundingClientRect().height;
  for (let n = 2; n <= 11; n++) {
    addLine(n);
  }
  const eleven = table.getBoundingClientRect().height;
  const pad = probe.getBoundingClientRect().height - eleven;
  probe.remove();
  const line = (eleven - one) / 10;
  const hunk = one - line;
  if (hunk > 0 && line > 0) {
    root.style.setProperty("--hunk-h", `${hunk}px`);
    root.style.setProperty("--row-h", `${line}px`);
    root.style.setProperty("--code-pad", `${Math.max(pad, 0)}px`);
  }
}
