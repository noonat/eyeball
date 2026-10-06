import { classes } from "./classes";
import { eltOf, xhrOf } from "./events";

// The three fixed sentences, for an answer that carries none of its own.
const noAnswer = "Could not reach eyeball. Nothing was sent.";
const anError = "eyeball could not do that.";
const tooSlow =
  "eyeball did not answer in time. Nothing is known to have been sent.";

// reportFailures writes a failed line beside the control whose request failed,
// with the server's own sentence when it sent one. A tap that fails silently
// is the failure product.md ranks worst.
export function reportFailures(): void {
  onFailure("htmx:sendError", noAnswer);
  onFailure("htmx:responseError", anError);
  onFailure("htmx:timeout", tooSlow);
  document.addEventListener("htmx:beforeRequest", (event) => {
    const elt = eltOf(event);
    if (elt) {
      clearFailure(elt);
    }
  });
}

function onFailure(name: string, fallback: string): void {
  document.addEventListener(name, (event) => {
    const elt = eltOf(event);
    if (elt) {
      showFailure(elt, sentence(xhrOf(event), fallback));
    }
  });
}

function sentence(xhr: XMLHttpRequest | null, fallback: string): string {
  const type = xhr?.getResponseHeader("Content-Type") ?? "";
  const text = xhr?.responseText.trim() ?? "";
  return type.startsWith("text/plain") && text !== "" ? text : fallback;
}

// The line goes at the foot of the form, or after the acts row for Edit and
// Delete, which sit in a card rather than a form.
function home(elt: Element): Element {
  return elt.closest("form") ?? elt.closest(".acts") ?? elt;
}

export function showFailure(elt: Element, text: string): void {
  const at = home(elt);
  clearFailure(elt);
  const line = document.createElement("span");
  line.className = classes.failed;
  line.setAttribute("role", "alert");
  line.textContent = text;
  if (at.tagName === "FORM") {
    at.appendChild(line);
  } else {
    at.after(line);
  }
}

function clearFailure(elt: Element): void {
  const at = home(elt);
  const old =
    at.tagName === "FORM"
      ? at.querySelector(`:scope > .${classes.failed}`)
      : at.nextElementSibling?.classList.contains(classes.failed) === true
        ? at.nextElementSibling
        : null;
  old?.remove();
}
