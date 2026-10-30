import type { Meta, StoryObj } from "@storybook/react-vite";
import { LevelChips } from "./LevelChips";

const meta = {
  title: "Foundations/LevelChips",
  component: LevelChips,
} satisfies Meta<typeof LevelChips>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Where a comment will land, with the review level chosen. */
export const Levels: Story = {
  args: {
    levels: [
      { label: "This review", pressed: true },
      { label: "This file" },
      { label: "Lines 90–97" },
    ],
  },
};
