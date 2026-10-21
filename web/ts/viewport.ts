// maxTries bounds the resets each return to the foreground makes, so a viewport iOS never gives
// back does not bring a keyboard up on every tap.
const maxTries = 2;

// settle is how long a reset is given before the viewport is judged again. On
// the phone the keyboard a reset brings up took 2.8 seconds to go away, and a
// check in that time would take the next tap for another reset.
const settle = 3000;

// quiet is how long a short viewport is not taken for the fault after a field
// loses focus. A real keyboard is still on its way down then: in a test page
// on the phone the viewport stayed 386 short for 41 ms after the field blurred.
const quiet = 1000;

// maxAligns bounds the realigns each return makes, so a viewport that a
// realign does not move cannot scroll the page back and forth for good.
const maxAligns = 4;

// alignFor is how long after a realign the page's own scroll events are not
// taken as a new scroll to realign after. iOS applies a page's scroll later
// than the next frame, so the realign's own scrolls arrive inside it. A read
// 500 ms after the realign showed its result on the phone.
const alignFor = 500;

// shortBy is how far below the window the visual viewport has to sit, with no
// field focused, to count as the fault. A keyboard is over 250 points tall,
// and the gap that follows a scroll or a toolbar is far under 100.
const shortBy = 100;

// restoreViewport undoes a keyboard-sized viewport that iOS gives a page with
// no keyboard showing. Tapping eyeball's notification over an app that has had
// its keyboard up leaves the page 318 points short with nothing focused, so
// the sticky header and the decision bar sit up to a keyboard's height up the
// screen, and leaving the page does not clear it. WebKit applies another app's
// keyboard to the page on purpose and is never told it went away. Measured on
// the phone in 2026-10: a focus with no touch changes nothing, and a hidden
// field focused and blurred inside the reader's first touch has iOS show a
// keyboard and take it away, after which the viewport is whole again.
export function restoreViewport(): void {
  const view = window.visualViewport;
  if (!view) {
    return;
  }
  let tries = 0;
  let armed = false;
  let settling = false;
  // iOS honors a focus in the touch that starts a gesture and in a tap's
  // click. A touch that ends a scroll is not honored, and spent the tries with
  // nothing to show.
  const onTouch = (): void => {
    document.removeEventListener("touchstart", onTouch, true);
    document.removeEventListener("click", onTouch, true);
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
  let aligns = 0;
  let touched = false;
  let quietUntil = 0;
  let aligningUntil = 0;
  const check = (): void => {
    const now = performance.now();
    if (now < quietUntil) {
      return;
    }
    // Before the reader's first touch every scroll is the page's own, such as
    // restoring the reader's place or keeping it while a file's body loads.
    // After it, a realign would fight the reader's finger.
    if (
      !touched &&
      !settling &&
      now >= aligningUntil &&
      aligns < maxAligns &&
      stuck(view) &&
      view.offsetTop >= 1
    ) {
      aligns++;
      aligningUntil = now + alignFor;
      realign(view);
    }
    if (armed || settling || tries >= maxTries || !stuck(view)) {
      return;
    }
    armed = true;
    document.addEventListener("touchstart", onTouch, {
      capture: true,
      passive: true,
    });
    document.addEventListener("click", onTouch, true);
  };
  view.addEventListener("resize", check);
  window.addEventListener("scroll", check, { passive: true });
  document.addEventListener(
    "touchstart",
    () => {
      touched = true;
    },
    { capture: true, passive: true },
  );
  document.addEventListener("focusout", () => {
    quietUntil = performance.now() + quiet;
    window.setTimeout(check, quiet);
  });
  // A page left open while another app's keyboard was up comes back short
  // with no resize to say so, so a return to the foreground is a new chance
  // with fresh tries.
  const resumed = (): void => {
    if (document.visibilityState !== "visible") {
      return;
    }
    tries = 0;
    aligns = 0;
    touched = false;
    check();
  };
  document.addEventListener("visibilitychange", resumed);
  window.addEventListener("pageshow", resumed);
  check();
}

// realign moves the short visual viewport back to the top of the layout
// viewport, where the sticky header and the decision bar are drawn at the
// screen's edges, so the page looks right before the reader's first touch.
// Scrolling down slides a short visual viewport down inside the layout
// viewport before the layout viewport moves, so restoring the reader's place
// on load leaves offsetTop at the shortfall and the bar that far up the
// screen. Scrolling past the place by the shortfall and back up to it ends on
// the same place with offsetTop at 0. It scrolls past and back once in one
// task and again across two frames. Run together on the phone they left
// offsetTop at 0 in the middle of a review, and which of the two did it is not
// known, because iOS applies a page's scroll later than the next frame. At the
// very end of a review they left it at 250 to 278, and the first touch is what
// puts it right there.
function realign(view: VisualViewport): void {
  const place = window.scrollY;
  const gap = window.innerHeight - view.height;
  window.scrollTo(0, place + gap);
  window.scrollTo(0, place);
  window.requestAnimationFrame(() => {
    window.scrollTo(0, place + gap);
    window.requestAnimationFrame(() => {
      window.scrollTo(0, place);
    });
  });
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
