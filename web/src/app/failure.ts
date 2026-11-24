import type { Failure } from "../api/types";
import { isFailure } from "../api/views";

/**
 * The failure the server put in the shell's head, or null for a shell that
 * carries none. The tag is how the app learns that the address failed, since a
 * raw file's address has no route here, and the router has no catch-all route,
 * so it would show its default error page for a file that exists and is too
 * large. A tag that does not parse is read as none, so the router answers the
 * address as if no failure were carried.
 */
export function readFailure(doc: Document): Failure | null {
  const tag = doc.querySelector<HTMLMetaElement>("meta[name='eyeball-error']");
  if (tag === null) {
    return null;
  }
  try {
    const value: unknown = JSON.parse(tag.content);
    return isFailure(value) ? value : null;
  } catch {
    return null;
  }
}
