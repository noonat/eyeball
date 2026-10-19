// maxTries bounds the resets one page makes, so a viewport iOS never gives
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
  const onTap = (): void => {
    document.removeEventListener("touchend", onTap, true);
    armed = false;
    if (!stuck(view)) {
      return;
    }
    tries++;
    flick();
    settling = true;
    window.setTimeout(() => {
      settling = false;
      check();
    }, settle);
  };
  const check = (): void => {
    if (armed || settling || tries >= maxTries || !stuck(view)) {
      return;
    }
    armed = true;
    document.addEventListener("touchend", onTap, true);
  };
  view.addEventListener("resize", check);
  check();
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
