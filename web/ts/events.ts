// htmx's events carry their details untyped, so each field is checked as it is
// read rather than asserted. A detail missing a field reads as absent.

function detail(event: Event): object | null {
  if (!(event instanceof CustomEvent)) {
    return null;
  }
  const d: unknown = event.detail;
  return typeof d === "object" && d !== null ? d : null;
}

// eltOf is the element that made the request.
export function eltOf(event: Event): Element | null {
  const d = detail(event);
  return d !== null && "elt" in d && d.elt instanceof Element ? d.elt : null;
}

// xhrOf is the request's XMLHttpRequest, absent when none was sent.
export function xhrOf(event: Event): XMLHttpRequest | null {
  const d = detail(event);
  return d !== null && "xhr" in d && d.xhr instanceof XMLHttpRequest
    ? d.xhr
    : null;
}

// verbOf is the request's method, from configRequest's verb or afterRequest's
// requestConfig.
export function verbOf(event: Event): string {
  const d = detail(event);
  if (d === null) {
    return "";
  }
  if ("verb" in d && typeof d.verb === "string") {
    return d.verb;
  }
  if (
    "requestConfig" in d &&
    typeof d.requestConfig === "object" &&
    d.requestConfig !== null
  ) {
    const config = d.requestConfig;
    return "verb" in config && typeof config.verb === "string"
      ? config.verb
      : "";
  }
  return "";
}

// succeeded says afterRequest's request got a successful answer.
export function succeeded(event: Event): boolean {
  const d = detail(event);
  return d !== null && "successful" in d && d.successful === true;
}

// setHeader adds a header to configRequest's request.
export function setHeader(event: Event, name: string, value: string): void {
  const d = detail(event);
  if (
    d !== null &&
    "headers" in d &&
    typeof d.headers === "object" &&
    d.headers !== null
  ) {
    Reflect.set(d.headers, name, value);
  }
}

// targetOf is the element a DOM event happened on.
export function targetOf(event: Event): Element | null {
  return event.target instanceof Element ? event.target : null;
}
