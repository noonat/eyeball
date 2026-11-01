import type { Files, NotFound, Project } from "./types";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isCount(value: unknown): boolean {
  return value === null || typeof value === "number";
}

function isTopBar(value: Record<string, unknown>): boolean {
  return (
    typeof value["title"] === "string" &&
    typeof value["back"] === "string" &&
    typeof value["back_label"] === "string" &&
    isCount(value["waiting_count"])
  );
}

/** Whether a decoded body has the shape the generated types promise. */
export function isProject(value: unknown): value is Project {
  return (
    isRecord(value) &&
    isTopBar(value) &&
    typeof value["name"] === "string" &&
    typeof value["root"] === "string" &&
    typeof value["open"] === "number" &&
    (value["copies"] === null || Array.isArray(value["copies"]))
  );
}

/** Whether a decoded body is the view of a tree or a file. */
export function isFiles(value: unknown): value is Files {
  if (!isRecord(value) || !isTopBar(value)) {
    return false;
  }
  if (value["kind"] === "tree") {
    return isRecord(value["tree"]);
  }
  return value["kind"] === "file" && isRecord(value["file"]);
}

/** Whether a decoded body is what a 404 sends. */
export function isNotFound(value: unknown): value is NotFound {
  return (
    isRecord(value) &&
    isCount(value["waiting_count"]) &&
    (value["notice"] === null || isRecord(value["notice"]))
  );
}

/** What an address's view answered: its body, or the 404's. */
export type ViewAnswer<T> =
  | { readonly found: true; readonly body: T }
  | { readonly found: false; readonly body: NotFound };

/** The query key of an address's view. */
export function viewKey(address: string): readonly [string, string] {
  return ["view", address];
}

/**
 * Reads an address's view from /api plus the address as it is, query included,
 * so `?copy=` and `?against=` reach the server unchanged. A body that is not the
 * shape the types say is an error, so a server and an app that disagree fail
 * here and not on a screen. A 404 is an answer, which the app draws.
 */
export async function fetchView<T>(
  address: string,
  isView: (value: unknown) => value is T,
): Promise<ViewAnswer<T>> {
  const res = await fetch(`/api${address}`, {
    headers: { Accept: "application/json" },
  });
  if (res.status !== 200 && res.status !== 404) {
    throw new Error(`GET /api${address} answered ${res.status}`);
  }
  const body: unknown = await res.json();
  if (res.status === 404) {
    if (!isNotFound(body)) {
      throw new Error(`GET /api${address} answered a body that is not a 404's`);
    }
    return { found: false, body };
  }
  if (!isView(body)) {
    throw new Error(`GET /api${address} answered a body of the wrong shape`);
  }
  return { found: true, body };
}
