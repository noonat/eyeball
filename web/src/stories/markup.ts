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
 * Attributes the template pages carry for the script that drove them and the
 * app has no use for: the hx- ones, the hooks its script found elements by, and the
 * facts a page printed for its own script to read. A component draws none of
 * them, and none changes what a page looks like.
 */
const scriptAttributes = new Set([
  "data-level",
  "data-target",
  "data-cancel",
  "data-retry",
  "data-discard",
  "data-body",
  "data-path",
  "data-stream",
  "data-stamp",
  "data-place",
  "data-review",
  "data-round",
  "data-against",
]);

/** Whether an attribute is one of the script's, which is not compared. */
function isScriptAttribute(node: Element, name: string): boolean {
  return (
    name.startsWith("hx-") ||
    scriptAttributes.has(name) ||
    // The app sets a skeleton's size as properties, where the page's script
    // did through the same properties from its data attributes.
    (name === "style" && node.classList.contains("skeleton"))
  );
}

/**
 * Elements that draw nothing: a hidden one, a template a script clones, and the
 * hidden fields of a form whose values a component keeps as props.
 */
function isInvisible(node: Element): boolean {
  return (
    node.hasAttribute("hidden") ||
    node.tagName === "TEMPLATE" ||
    (node.tagName === "INPUT" && node.getAttribute("type") === "hidden")
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
  if (!(node instanceof Element) || isInvisible(node)) {
    return "";
  }
  const attrs = [...node.attributes]
    .filter((attr) => !isScriptAttribute(node, attr.name))
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

/**
 * A fragment a template route answered with, normalized one top-level element
 * at a time. It is parsed as a template's content, which accepts a table row
 * with no table around it.
 */
export function fragmentParts(html: string): string[] {
  const holder = document.createElement("template");
  holder.innerHTML = html;
  return partsOf(holder.content);
}

/** The top-level elements of a drawn fragment, normalized the same way. */
export function drawnParts(root: Node): string[] {
  const copy = root.cloneNode(true);
  copy.normalize();
  return partsOf(copy);
}

function partsOf(root: Node): string[] {
  return [...root.childNodes].map(normalize).filter((part) => part !== "");
}
