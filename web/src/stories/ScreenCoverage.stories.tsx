import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import * as arrival from "./Arrival.stories";
import * as file from "./File.stories";
import * as project from "./Project.stories";
import * as review from "./Review.stories";
import * as reviewStatesModule from "./ReviewStates.stories";
import { entries } from "./Sheet";
import {
  fileStates,
  largeReviewStates,
  mediaStates,
  projectStates,
  reviewBarStates,
  reviewCommentStates,
  reviewStates,
  treeStates,
} from "./sheetStates";
import * as tree from "./Tree.stories";

const meta = {
  title: "Screen/Coverage",
  parameters: { frame: "screen", sheet: false },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const screens = entries({
  "./Arrival.stories.tsx": arrival,
  "./File.stories.tsx": file,
  "./Project.stories.tsx": project,
  "./Review.stories.tsx": review,
  "./ReviewStates.stories.tsx": reviewStatesModule,
  "./Tree.stories.tsx": tree,
});

const sheeted = [
  ...projectStates,
  ...treeStates,
  ...fileStates,
  ...mediaStates,
  ...reviewStates,
  ...largeReviewStates,
  ...reviewCommentStates,
  ...reviewBarStates,
];

/**
 * The sheets list their states by hand, so a state added to a screen and left
 * off every sheet would never reach the phone. This fails with its id. The
 * queue has a sheet of its own, which checks its states the same way.
 */
export const EveryStateIsOnASheet: Story = {
  render: () => <p>Every screen state is on a sheet.</p>,
  play: async () => {
    const placed = new Set(sheeted.map((state) => state.id));
    const left = screens
      .map((entry) => entry.id)
      .filter((id) => !placed.has(id));
    await expect(left).toEqual([]);
    // A sheet naming a state twice, or one no screen has, is as wrong.
    const listed = sheeted.map((state) => state.id);
    await expect(listed).toHaveLength(placed.size);
    await expect(
      listed.filter((id) => !screens.some((e) => e.id === id)),
    ).toEqual([]);
  },
};
