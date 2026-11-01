import { expect, within } from "storybook/test";
import { missingStates, type ScreenState } from "./ScreenSheet";
import type { Entry } from "./Sheet";

/**
 * What every screen sheet has to hold: each state names a story that exists,
 * and has a 390 pixel phone frame and a Full screen button in each theme.
 */
export async function checkSheet(
  canvasElement: HTMLElement,
  all: readonly Entry[],
  states: readonly ScreenState[],
): Promise<void> {
  const canvas = within(canvasElement);
  await expect(missingStates(all, states)).toEqual([]);
  await expect(canvasElement.querySelectorAll(".phone")).toHaveLength(
    states.length * 2,
  );
  await expect(
    canvas.getAllByRole("link", { name: /full screen$/i }),
  ).toHaveLength(states.length * 2);
  for (const phone of canvasElement.querySelectorAll(".phone")) {
    await expect(phone.getAttribute("data-theme")).toMatch(/^(light|dark)$/);
  }
}
