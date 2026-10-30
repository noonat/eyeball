import type { Meta, StoryObj } from "@storybook/react-vite";
import { TypeScale } from "./TypeScale";

const meta = {
  title: "Foundations/Type",
  component: TypeScale,
} satisfies Meta<typeof TypeScale>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Atkinson Hyperlegible for the chrome, Literata for the agent's words. */
export const Scale: Story = {};
