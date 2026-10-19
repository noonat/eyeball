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
    changes.splice(0, changes.length - 6);
  };
  // log adds a line that is not a height change, such as a reset attempt.
  const log = (line: string): void => {
    changes.push(`${Math.round(performance.now())}ms ${line}`);
    changes.splice(0, changes.length - 6);
    draw();
  };
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
      `bar ${bar ? `${Math.round(bar.top)}..${Math.round(bar.bottom)}` : "none"}`,
      `${document.visibilityState}  ${loaded}`,
      ...changes,
    ].join("\n");
  };
  window.addEventListener("scroll", draw, { passive: true });
  window.addEventListener("resize", draw);
  window.visualViewport?.addEventListener("resize", draw);
  window.visualViewport?.addEventListener("scroll", draw);
  document.addEventListener("visibilitychange", draw);
  draw();
  tryResets(log);
}

// stuck reports the fault: the visual viewport a keyboard's height short of the
// window while nothing a keyboard types into has focus.
function stuck(): boolean {
  const view = window.visualViewport;
  if (!view) {
    return false;
  }
  const active = document.activeElement;
  const typing =
    active instanceof HTMLInputElement ||
    active instanceof HTMLTextAreaElement ||
    (active instanceof HTMLElement && active.isContentEditable);
  return !typing && window.innerHeight - view.height > 100;
}

// flick focuses a hidden text field and blurs it at once, to ask iOS for a
// keyboard and take it away again. The field's 16px font stops iOS zooming the
// page on focus.
function flick(): void {
  const field = document.createElement("input");
  field.setAttribute("aria-hidden", "true");
  field.tabIndex = -1;
  const style = field.style;
  style.position = "fixed";
  style.top = "0";
  style.left = "0";
  style.width = "1px";
  style.height = "1px";
  style.opacity = "0";
  style.fontSize = "16px";
  document.body.append(field);
  field.focus({ preventScroll: true });
  window.setTimeout(() => {
    field.blur();
    field.remove();
  }, 50);
}

// viewHeight is the visual viewport's height, rounded.
function viewHeight(): number {
  return Math.round(window.visualViewport?.height ?? -1);
}

// tryResets is the experiment. When the fault is seen, it flicks focus once
// with no tap, and if the viewport is still short, once more inside the next
// tap, where iOS honours a focus as the reader's own. Each attempt is logged
// with the height that followed.
function tryResets(log: (line: string) => void): void {
  let tried = false;
  const check = (): void => {
    if (tried || !stuck()) {
      return;
    }
    tried = true;
    log(`stuck h${viewHeight()}, reset a`);
    flick();
    window.setTimeout(() => {
      log(`after a: h${viewHeight()}`);
      if (!stuck()) {
        return;
      }
      const onTap = (): void => {
        document.removeEventListener("touchend", onTap, true);
        log("reset b on tap");
        flick();
        window.setTimeout(() => {
          log(`after b: h${viewHeight()}`);
        }, 500);
      };
      document.addEventListener("touchend", onTap, true);
    }, 300);
  };
  window.visualViewport?.addEventListener("resize", check);
  window.setTimeout(check, 1000);
}
