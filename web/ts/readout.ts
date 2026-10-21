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
  style.top = "0";
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
  // Each change in the visual viewport's height, with the milliseconds since
  // the page began loading and what had focus, so the screenshot shows whether
  // the page loaded short or shrank afterwards.
  const changes: string[] = [];
  let lastHeight = -1;
  const note = (): void => {
    const height = Math.round(window.visualViewport?.height ?? -1);
    if (height === lastHeight) {
      return;
    }
    lastHeight = height;
    const focused = document.activeElement?.tagName.toLowerCase() ?? "none";
    changes.push(`${Math.round(performance.now())}ms h${height} ${focused}`);
    changes.splice(0, changes.length - 10);
  };
  // The fix reports what it does here while it is measured on the phone.
  document.addEventListener("eyeball:viewport", (event) => {
    const detail = event instanceof CustomEvent ? String(event.detail) : "";
    changes.push(`${Math.round(performance.now())}ms ${detail}`);
    changes.splice(0, changes.length - 10);
    draw();
  });
  // Visibility, page loads and the service worker's control, so a screenshot
  // shows what happened around a notification tap.
  const mark = (line: string): void => {
    changes.push(`${Math.round(performance.now())}ms ${line}`);
    changes.splice(0, changes.length - 10);
    draw();
  };
  document.addEventListener("visibilitychange", () => {
    mark(
      `${document.visibilityState} h${Math.round(window.visualViewport?.height ?? -1)}`,
    );
  });
  window.addEventListener("pageshow", (event) => {
    mark(`pageshow${event.persisted ? " cached" : ""}`);
  });
  navigator.serviceWorker?.addEventListener("controllerchange", () => {
    mark(`controller ${controlled()}`);
  });
  const draw = (): void => {
    note();
    const view = window.visualViewport;
    // Pinned to what is on screen rather than to the layout viewport, since
    // the fault moves everything fixed to the layout viewport off screen.
    const top = view ? view.offsetTop : 0;
    style.top = `${Math.round(top) + 70}px`;
    const bar = document.getElementById("bar")?.getBoundingClientRect();
    panel.textContent = [
      `inner ${window.innerHeight}  client ${document.documentElement.clientHeight}`,
      `visual ${view ? Math.round(view.height) : "-"}  offsetTop ${view ? Math.round(view.offsetTop) : "-"}`,
      `scrollY ${Math.round(window.scrollY)}  pageTop ${view ? Math.round(view.pageTop) : "-"}`,
      `bar ${bar ? `${Math.round(bar.top)}..${Math.round(bar.bottom)}` : "none"}  shift ${document.documentElement.style.getPropertyValue("--viewport-shift") || "0"}`,
      `${document.visibilityState}  ${loaded}  ${controlled()}  ${location.pathname}`,
      ...changes,
    ].join("\n");
  };
  window.addEventListener("scroll", draw, { passive: true });
  window.addEventListener("resize", draw);
  window.visualViewport?.addEventListener("resize", draw);
  window.visualViewport?.addEventListener("scroll", draw);
  document.addEventListener("visibilitychange", draw);
  draw();
}

// controlled names whether a service worker controls the page, since the
// worker's navigate only reaches a window it controls.
function controlled(): string {
  return navigator.serviceWorker?.controller ? "sw" : "no sw";
}
