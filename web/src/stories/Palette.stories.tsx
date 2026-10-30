import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Palette } from "./Palette";

const meta = {
  title: "Foundations/Palette",
  component: Palette,
} satisfies Meta<typeof Palette>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Every token, in the theme the toolbar or the frame names. */
export const Tokens: Story = {};

/** A story can pin the theme, which the toolbar's choice does not override. */
export const Dark: Story = {
  // The sheet stamps each panel itself, so this adds nothing to look at.
  parameters: { theme: "dark", sheet: false },
  play: async () => {
    await expect(document.documentElement.dataset["theme"]).toBe("dark");
  },
};
