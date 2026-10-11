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
  const root = cards[0]?.closest(".scroll");
  if (root === null || root === undefined) {
    return;
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
