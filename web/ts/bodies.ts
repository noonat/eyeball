import { headBottom } from "./head";

// loadBodies asks for a deferred card's body once the card is open and within
// a screen of view, by sending the event its hx-trigger waits for. htmx's own
// intersect trigger cannot do either half: it has no margin, so a body would
// start loading only once its header was already on screen, and telling an
// open card from a folded one needs a trigger filter, which allowEval: false
// turns off. A folded card loads when it is unfolded instead.
//
// The root is the viewport, since the document is what scrolls. Nothing
// between a card and the viewport clips, so the margin reaches a screen below
// it.
export function loadBodies(): void {
  const cards = [
    ...document.querySelectorAll<HTMLDetailsElement>("details.file[hx-get]"),
  ];
  if (cards[0] === undefined) {
    return;
  }
  // The skeleton's counts are set here rather than by the server, because the
  // CSP refuses a style attribute and allows the CSSOM.
  measureRows(cards[0]);
  for (const body of document.querySelectorAll<HTMLElement>(
    "[data-body][data-lines]",
  )) {
    body.style.setProperty("--hunks", body.dataset["hunks"] ?? "");
    body.style.setProperty("--lines", body.dataset["lines"] ?? "");
  }
  const near = new Set<Element>();
  const ask = (card: Element): void => {
    if (card instanceof HTMLDetailsElement && card.open && near.has(card)) {
      card.dispatchEvent(new Event("eyeball:near"));
    }
  };
  const observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting) {
          near.add(entry.target);
          ask(entry.target);
        } else {
          near.delete(entry.target);
        }
      }
    },
    { rootMargin: "100% 0px" },
  );
  for (const card of cards) {
    observer.observe(card);
    card.addEventListener("toggle", () => ask(card));
  }
}

// holdPlace keeps what is at the top of the screen there when a card above it
// changes height: a body arriving, a document taller than its skeleton, an
// image loading late. It records the card holding the top of the screen, which
// is the bottom of the sticky header, and how far that card sits from it, and after any card resizes it scrolls by
// however far that card moved. A card that grows below the top, or the top
// card's own body, moves nothing above it, so only growth above is corrected.
// This is the browser's scroll anchoring done per card: Safari, where the phone
// reads, has none, and a document's six-line skeleton loading above a jump
// pushed the target off screen there. Chromium's own is turned off, so every
// browser takes this one path and a browser test exercises the code Safari
// runs.
export function holdPlace(): void {
  const cards = [
    ...document.querySelectorAll<HTMLDetailsElement>("details.file"),
  ];
  if (cards.length === 0) {
    return;
  }
  document.documentElement.style.overflowAnchor = "none";
  let anchor: Element | undefined;
  let offset = 0;
  const record = (): void => {
    const top = headBottom();
    anchor = cards.find((c) => c.getBoundingClientRect().bottom > top);
    offset = (anchor?.getBoundingClientRect().top ?? top) - top;
  };
  let pending = 0;
  window.addEventListener(
    "scroll",
    () => {
      if (pending === 0) {
        pending = window.requestAnimationFrame(() => {
          pending = 0;
          record();
        });
      }
    },
    { passive: true },
  );
  const resized = new ResizeObserver(() => {
    if (anchor === undefined) {
      record();
      return;
    }
    const moved = anchor.getBoundingClientRect().top - headBottom() - offset;
    if (Math.abs(moved) >= 1) {
      window.scrollBy(0, moved);
    }
    record();
  });
  for (const card of cards) {
    resized.observe(card);
  }
  record();
}

// measureRows renders a hidden code table in a card, where a body's table
// sits, and sets on the root element the heights a skeleton is built from: a hunk
// header, a line, and what the code block adds around its table, such as a
// scrollbar where the device draws one. They depend on the device and the
// card's width, so a constant would be right on one device only. A line is the
// table's growth over ten more lines, not one row's height: in Chromium the
// first row under a header measured 20.64px and eight averaged 20.27px, since
// border-collapse shares the header's border with it. The table is removed in
// the same task, so it is never painted.
function measureRows(card: Element): void {
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
    const style = document.documentElement.style;
    style.setProperty("--hunk-h", `${hunk}px`);
    style.setProperty("--row-h", `${line}px`);
    style.setProperty("--code-pad", `${Math.max(pad, 0)}px`);
  }
}
