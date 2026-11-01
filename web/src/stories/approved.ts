import { expect } from "storybook/test";
import {
  approvedMarkup,
  drawnParts,
  fragmentParts,
  screenMarkup,
} from "./markup";

/**
 * A play function that fails when the story's markup is not the approved
 * markup of its state, which a fixture file holds. Both are normalized the same
 * way, so a class, an attribute or an element changed in a component fails
 * here, and so does text that differs.
 */
export function sameAsApproved(
  html: string,
): (context: { canvasElement: HTMLElement }) => Promise<void> {
  return async ({ canvasElement }) => {
    const drawn = screenMarkup(canvasElement);
    const approved = approvedMarkup(html);
    // A region missing on both sides would make the two equal, so a story with
    // nothing drawn, or a fixture with nothing in it, would pass.
    await expect(drawn.filter((part) => part.startsWith("missing "))).toEqual(
      [],
    );
    await expect(
      approved.filter((part) => part.startsWith("missing ")),
    ).toEqual([]);
    await expect(drawn).toEqual(approved);
  };
}

/**
 * A play function that fails when the fragment a story draws, inside the
 * element marked data-fragment, is not what a template route answered with.
 */
export function sameFragment(
  html: string,
): (context: { canvasElement: HTMLElement }) => Promise<void> {
  return async ({ canvasElement }) => {
    const root = canvasElement.querySelector("[data-fragment]");
    await expect(root).not.toBeNull();
    const approved = fragmentParts(html);
    await expect(approved).not.toEqual([]);
    await expect(drawnParts(root ?? canvasElement)).toEqual(approved);
  };
}
