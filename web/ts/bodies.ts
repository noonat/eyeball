// loadBodies asks for a deferred card's body once the card is open and within
// a screen of view, by sending the event its hx-trigger waits for. htmx's own
// intersect trigger cannot do either half: it has no margin, so a body would
// start loading only once its header was already on screen, and telling an
// open card from a folded one needs a trigger filter, which allowEval: false
// turns off. A folded card loads when it is unfolded instead.
//
// The root is the .scroll element the cards sit in. With the viewport as root,
// that element's clipping still applies and the margin widens only the
// viewport, so a card below the scroller's visible box never counted as near:
// in headless Chromium only the cards already on screen loaded.
export function loadBodies(): void {
  const cards = [
    ...document.querySelectorAll<HTMLDetailsElement>("details.file[hx-get]"),
  ];
  const root = cards[0]?.closest<HTMLElement>(".scroll");
  if (root === null || root === undefined) {
    return;
  }
  // The skeleton's counts are set here rather than by the server, because the
  // CSP refuses a style attribute and allows the CSSOM.
  if (cards[0] !== undefined) {
    measureRows(cards[0], root);
  }
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
    { root, rootMargin: "100% 0px" },
  );
  for (const card of cards) {
    observer.observe(card);
    card.addEventListener("toggle", () => ask(card));
  }
}

// measureRows renders a hidden code table in a card, where a body's table
// sits, and sets on the scroller the heights a skeleton is built from: a hunk
// header, a line, and what the code block adds around its table, such as a
// scrollbar where the device draws one. They depend on the device and the
// card's width, so a constant would be right on one device only. A line is the
// table's growth over ten more lines, not one row's height: in Chromium the
// first row under a header measured 20.64px and eight averaged 20.27px, since
// border-collapse shares the header's border with it. The table is removed in
// the same task, so it is never painted.
function measureRows(card: Element, root: HTMLElement): void {
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
    const style = root.style;
    style.setProperty("--hunk-h", `${hunk}px`);
    style.setProperty("--row-h", `${line}px`);
    style.setProperty("--code-pad", `${Math.max(pad, 0)}px`);
  }
}
