import { useEffect } from "react";
import type { ComposerAt } from "../components/composerAt";

const none: ReadonlySet<string> = new Set();

/**
 * The anchor a tap opened a composer at, or nothing where it opened none. A tap
 * on a line or a removed row of code opens a composer on it, a tap on a passage
 * of a document opens one on the passage, and a tap on a file shown as itself,
 * the line saying it has no body or an image, opens one on the file. A link
 * inside any of them still follows.
 *
 * It reads the page, as the template's script did, since what a tap lands on is
 * what the page drew and the click's target is all it has. A decided round
 * takes a tap too: a comment there is sent at once as a late one. `held` names
 * the cards whose body has not been loaded, which take none.
 */
export function tappedAt(
  target: Element | null,
  held: ReadonlySet<string> = none,
): ComposerAt | undefined {
  if (target === null) {
    return undefined;
  }
  // A passage names its file by the nearest element that holds the path, which
  // is its card, or the document itself where the round shows one document as
  // the page.
  const block = target.closest<HTMLElement>(".prose .blk[data-lines]");
  if (block !== null) {
    // The hint is the one control that shows what a changed passage replaced,
    // as the owner decided, and a link inside it still follows, so a tap on
    // either is not a tap on the passage.
    if (target.closest("a") !== null || target.closest(".hint") !== null) {
      return undefined;
    }
    const path = block.closest<HTMLElement>("[data-path]")?.dataset["path"];
    const [start, end] = (block.dataset["lines"] ?? "").split("-");
    const first = Number(start);
    const last = Number(end);
    return path !== undefined &&
      Number.isInteger(first) &&
      Number.isInteger(last) &&
      first > 0
      ? { level: "passage", path, first, last }
      : undefined;
  }
  const file = target.closest<HTMLElement>("details.file[data-path]");
  if (file === null) {
    return undefined;
  }
  const path = file.dataset["path"] ?? "";
  const row = target.closest<HTMLElement>("tr[data-line], tr[data-old]");
  if (row !== null) {
    const line = row.dataset["line"];
    if (line !== undefined) {
      return { level: "line", path, line: Number(line) };
    }
    return { level: "file", path, oldLine: Number(row.dataset["old"]) };
  }
  // A card held back has no body, so no slot under it: a composer moved here
  // would be open and never drawn, and would hold the text out of sight.
  if (held.has(path)) {
    return undefined;
  }
  // A file shown as itself takes a comment on the file, as the line naming a
  // file with no rows does. A player and the Open link keep their taps.
  if (
    target.closest(".toobig, .filefacts, .rendered .shot, .rendered .cap") !==
      null &&
    target.closest("a") === null
  ) {
    return { level: "file", path };
  }
  return undefined;
}

/**
 * Opens a composer where a tap lands, for a round that takes comments. The
 * listener is on the document and reads the click's target, since iOS Safari
 * sends the click only from an element it takes to be clickable, which
 * app.css marks `.page[data-acting]` for. A desktop browser has no such rule,
 * so a browser test cannot catch it going missing.
 */
export function useTaps(
  enabled: boolean,
  open: (at: ComposerAt) => void,
  held: ReadonlySet<string>,
): void {
  useEffect(() => {
    if (!enabled) {
      return undefined;
    }
    const listen = (event: MouseEvent): void => {
      const target = event.target instanceof Element ? event.target : null;
      const at = tappedAt(target, held);
      if (at !== undefined) {
        open(at);
      }
    };
    document.addEventListener("click", listen);
    return () => {
      document.removeEventListener("click", listen);
    };
  }, [enabled, open, held]);
}
