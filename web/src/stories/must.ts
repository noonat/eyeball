/** A value a fixture is known to hold, or a failure naming what is missing. */
export function must<T>(value: T | null | undefined, what: string): T {
  if (value === null || value === undefined) {
    throw new Error(`the fixture has no ${what}`);
  }
  return value;
}

/** The element a play function looks at, or a failure naming the selector. */
export function pick(root: ParentNode, selector: string): HTMLElement {
  const found = root.querySelector(selector);
  if (!(found instanceof HTMLElement)) {
    throw new Error(`nothing matches ${selector}`);
  }
  return found;
}
