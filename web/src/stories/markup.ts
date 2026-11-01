// Comparing the markup of a component with the markup the owner approved for the
// same state, which the fixture files hold. Both are reduced to one string by
// the rules below, so a difference that changes how a class selector or a layout
// reads is a difference here, and a difference that does not is not.

/** Whitespace-only text that a browser draws: a space between two inline spans. */
function keepsSpace(text: Node): boolean {
  const parent = text.parentElement;
  const before = text.previousSibling;
  const after = text.nextSibling;
  return (
    parent?.tagName === "SPAN" &&
    before instanceof HTMLElement &&
    before.tagName === "SPAN" &&
    after instanceof HTMLElement &&
    after.tagName === "SPAN"
  );
}

/**
 * One node as a string. Attributes are sorted and so are the tokens of a class
 * list, since neither order is visible. Runs of whitespace in text become one
 * space, which is how the page draws them. Whitespace between block elements,
 * which the template's line breaks leave behind, is dropped unless it sits
 * between two inline spans.
 */
export function normalize(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) {
    const text = (node.textContent ?? "").replace(/\s+/g, " ");
    if (text === " ") {
      return keepsSpace(node) ? " " : "";
    }
    return text;
  }
  if (!(node instanceof Element)) {
    return "";
  }
  const attrs = [...node.attributes]
    // The code table is focusable so a keyboard can scroll it. The template
    // page's table was not, and the approved markup is otherwise unchanged.
    .filter(
      (attr) => !(attr.name === "tabindex" && node.classList.contains("code")),
    )
    .map((attr) => {
      const value =
        attr.name === "class"
          ? attr.value.split(/\s+/).filter(Boolean).toSorted().join(" ")
          : attr.value;
      return `${attr.name}="${value}"`;
    })
    .toSorted()
    .join(" ");
  const inside = [...node.childNodes].map(normalize).join("");
  return `<${node.tagName.toLowerCase()}${attrs === "" ? "" : ` ${attrs}`}>${inside}</${node.tagName.toLowerCase()}>`;
}

/** The top bar and the scrolling column of a screen, normalized. */
export function screenMarkup(root: ParentNode): string[] {
  return [".topbar", ".scroll"].map((selector) => {
    const found = root.querySelector(selector);
    if (found === null) {
      return `missing ${selector}`;
    }
    // React writes "Waiting on you" and " " as two text nodes where the
    // template writes one, so adjacent text is joined before it is compared.
    const copy = found.cloneNode(true);
    copy.normalize();
    return normalize(copy);
  });
}

/**
 * The approved markup of a state: what the template queue page rendered when
 * the owner approved the sheet, frozen in a fixture file when the template was
 * removed.
 */
export function approvedMarkup(html: string): string[] {
  const holder = document.createElement("div");
  holder.innerHTML = html;
  return screenMarkup(holder);
}
