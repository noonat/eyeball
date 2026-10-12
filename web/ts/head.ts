// trackHead keeps the sticky header's height on the root element as
// --head-height, which scroll-padding-top reads so a jump to a card lands below
// the header rather than under it. The height changes with the safe area and
// with which notice is showing, so it is measured rather than written down.
export function trackHead(): void {
  const head = document.querySelector<HTMLElement>(".head");
  if (head === null) {
    return;
  }
  new ResizeObserver(() => {
    document.documentElement.style.setProperty(
      "--head-height",
      `${head.getBoundingClientRect().height}px`,
    );
  }).observe(head);
}

// headBottom is where the screen's content starts: the bottom of the sticky
// header, or the top of the window on a page without one.
export function headBottom(): number {
  return document.querySelector(".head")?.getBoundingClientRect().bottom ?? 0;
}
