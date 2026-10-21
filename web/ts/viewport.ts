// maxTries bounds the resets each return to the foreground makes, so a viewport iOS never gives
// back does not bring a keyboard up on every tap.
const maxTries = 2;

// settle is how long a reset is given before the viewport is judged again. On
// the phone the keyboard a reset brings up took 2.8 seconds to go away, and a
// check in that time would take the next tap for another reset.
const settle = 3000;

// shortBy is how far below the window the visual viewport has to sit, with no
// field focused, to count as the fault. A keyboard is over 250 points tall,
// and the gap that follows a scroll or a toolbar is far under 100.
const shortBy = 100;

// restoreViewport undoes a keyboard-sized viewport that iOS gives a page with
// no keyboard showing. Opening the installed app from a notification while
// another app's keyboard is up loads the page 318 points short with nothing
// focused, so the sticky header and the decision bar sit a keyboard's height
// up the screen, and leaving the page does not clear it. Measured on the phone
// in 2026-10: a focus with no tap changes nothing, and a hidden field focused
// and blurred inside the reader's next tap brings a real keyboard up and down,
// after which the viewport is whole again.
export function restoreViewport(): void {
  const view = window.visualViewport;
  if (!view) {
    return;
  }
  let tries = 0;
  let armed = false;
  let settling = false;
  // A click is a tap, which iOS honors a focus in. A touch that ends a
  // scroll is not, and spent the tries with nothing to show. Whether the touch
  // that starts one is honoured is what touchstart is armed to find out.
  const onTap = (event: Event): void => {
    document.removeEventListener("touchstart", onTap, true);
    document.removeEventListener("click", onTap, true);
    armed = false;
    if (!stuck(view)) {
      report(`${event.type}, not stuck h${Math.round(view.height)}`);
      return;
    }
    tries++;
    report(`reset ${tries} ${event.type} h${Math.round(view.height)}`);
    flick();
    settling = true;
    window.setTimeout(() => {
      settling = false;
      check();
    }, settle);
  };
  let aligns = 0;
  const check = (): void => {
    if (!settling && aligns < maxTries && stuck(view) && view.offsetTop >= 1) {
      aligns++;
      realign(view);
    }
    if (armed || settling || tries >= maxTries || !stuck(view)) {
      return;
    }
    armed = true;
    report(`armed h${Math.round(view.height)}`);
    document.addEventListener("touchstart", onTap, {
      capture: true,
      passive: true,
    });
    document.addEventListener("click", onTap, true);
  };
  view.addEventListener("resize", check);
  // A page left open while another app's keyboard was up comes back short
  // with no resize to say so, so a return to the foreground is a new chance
  // with fresh tries.
  const resumed = (): void => {
    if (document.visibilityState !== "visible") {
      return;
    }
    tries = 0;
    aligns = 0;
    report(`visible h${Math.round(view.height)}`);
    check();
  };
  document.addEventListener("visibilitychange", resumed);
  window.addEventListener("pageshow", resumed);
  check();
}

// realign moves the short visual viewport back to the top of the layout
// viewport, where the sticky header and the decision bar are drawn at the
// screen's edges. Scrolling down slides a short visual viewport down inside
// the layout viewport before the layout viewport moves, so restoring the
// reader's place on load leaves offsetTop at the shortfall and the bar that
// far up the screen. Scrolling past the place by the shortfall and back up to
// it ends on the same place with offsetTop at 0. Whether iOS applies two
// scrolls in one task separately is not known, so a second try puts the
// scroll back a frame later.
function realign(view: VisualViewport): void {
  const before = Math.round(view.offsetTop);
  const place = window.scrollY;
  const gap = window.innerHeight - view.height;
  window.scrollTo(0, place + gap);
  window.scrollTo(0, place);
  window.requestAnimationFrame(() => {
    const after = Math.round(view.offsetTop);
    report(`realign ${before}->${after} y${Math.round(window.scrollY)}`);
    if (after < 1) {
      return;
    }
    window.scrollTo(0, window.scrollY + gap);
    window.requestAnimationFrame(() => {
      window.scrollTo(0, place);
      window.requestAnimationFrame(() => {
        report(
          `realign framed ->${Math.round(view.offsetTop)} y${Math.round(window.scrollY)}`,
        );
      });
    });
  });
}

// report tells the readout what the fix did, while it is measured on the phone.
function report(line: string): void {
  document.dispatchEvent(new CustomEvent("eyeball:viewport", { detail: line }));
}

// stuck reports a visual viewport a keyboard's height short of the window
// while nothing a keyboard types into has focus. A page zoomed in has a short
// visual viewport by design, so only an unzoomed one counts.
function stuck(view: VisualViewport): boolean {
  const active = document.activeElement;
  const typing =
    active instanceof HTMLInputElement ||
    active instanceof HTMLTextAreaElement ||
    (active instanceof HTMLElement && active.isContentEditable);
  const zoomed = Math.abs(view.scale - 1) > 0.01;
  return !typing && !zoomed && window.innerHeight - view.height > shortBy;
}

// flick focuses a hidden text field and blurs it, to have iOS show a keyboard
// and take it away. Its 16px font stops iOS zooming the page on focus.
function flick(): void {
  const field = document.createElement("input");
  field.dataset["flick"] = "";
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
