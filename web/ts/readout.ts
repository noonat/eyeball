// showReadout draws the viewport's measurements over a review page, so the
// decision bar floating partway up the installed app's screen after a reload
// on resume can be read on the phone, where no desktop browser reproduces it.
// It is temporary, and is removed before this branch merges.
export function showReadout(): void {
  if (document.querySelector(".page[data-place]") === null) {
    return;
  }
  const panel = document.createElement("pre");
  const style = panel.style;
  style.position = "fixed";
  style.top = "var(--head-height, 0px)";
  style.left = "0";
  style.zIndex = "100";
  style.margin = "0";
  style.padding = "4px 6px";
  style.font = "11px/1.3 ui-monospace, monospace";
  style.background = "var(--ink)";
  style.color = "var(--bg)";
  style.opacity = "0.94";
  style.pointerEvents = "none";
  document.body.append(panel);

  const navigation = performance.getEntriesByType("navigation")[0];
  const loaded =
    navigation instanceof PerformanceNavigationTiming
      ? navigation.type
      : "unknown";
  const draw = (): void => {
    const view = window.visualViewport;
    const bar = document.getElementById("bar")?.getBoundingClientRect();
    panel.textContent = [
      `inner ${window.innerHeight}  client ${document.documentElement.clientHeight}`,
      `visual ${view ? Math.round(view.height) : "-"}  offsetTop ${view ? Math.round(view.offsetTop) : "-"}`,
      `scrollY ${Math.round(window.scrollY)}`,
      `bar ${bar ? `${Math.round(bar.top)}..${Math.round(bar.bottom)}` : "none"}`,
      `${document.visibilityState}  ${loaded}`,
    ].join("\n");
  };
  window.addEventListener("scroll", draw, { passive: true });
  window.addEventListener("resize", draw);
  window.visualViewport?.addEventListener("resize", draw);
  window.visualViewport?.addEventListener("scroll", draw);
  document.addEventListener("visibilitychange", draw);
  draw();
}
