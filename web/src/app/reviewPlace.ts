import { recall, remember } from "../../ts/storage";

/** Where a reader was in a review, and the files they folded. */
export interface Place {
  readonly scroll: number;
  readonly folded: readonly string[];
}

/** The key a review's place is stored under, as the template page stored it. */
export function placeKey(place: string): string {
  return `place:${place}`;
}

/**
 * Takes a saved place. An entry from before every card opened holds the open
 * files instead of the folded ones, and on a large review it lists almost
 * none, so applying it would fold nearly every card. Its scroll is kept and
 * its list dropped, and the next save writes the new shape over it.
 */
export function readPlace(value: unknown): Place | null {
  if (
    typeof value !== "object" ||
    value === null ||
    !("scroll" in value) ||
    typeof value.scroll !== "number"
  ) {
    return null;
  }
  const folded =
    "folded" in value && Array.isArray(value.folded)
      ? value.folded.filter((p): p is string => typeof p === "string")
      : [];
  return { scroll: value.scroll, folded };
}

/** The place a review's key holds, if it holds one. */
export function recallPlace(place: string): Place | null {
  return readPlace(recall(placeKey(place)));
}

/** Saves a place under the key the template page used. */
export function storePlace(place: string, value: Place): void {
  remember(placeKey(place), value);
}
