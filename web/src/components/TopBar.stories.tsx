import type { Meta, StoryObj } from "@storybook/react-vite";
import { TopBar } from "./TopBar";

const meta = {
  title: "Foundations/TopBar",
  component: TopBar,
  parameters: { frame: "screen" },
} satisfies Meta<typeof TopBar>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The queue's bar: the wordmark and the eye. */
export const Queue: Story = {
  args: { waiting: 3 },
};

/** A screen below the queue: the way back, its title, and the eye. */
export const Screen: Story = {
  args: {
    waiting: 12,
    back: { label: "Queue", href: "#" },
    title: "Product doc",
  },
};

/** Nothing waits, so the eye is shut. */
export const NothingWaiting: Story = {
  args: { waiting: 0 },
};
