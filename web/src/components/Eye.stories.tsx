import type { Meta, StoryObj } from "@storybook/react-vite";
import { Eye } from "./Eye";

const meta = {
  title: "Foundations/Eye",
  component: Eye,
} satisfies Meta<typeof Eye>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Open, with the waiting count as its iris. */
export const Open: Story = {
  args: { waiting: 3 },
};

/** Two digits is as many as the iris holds. */
export const TwoDigits: Story = {
  args: { waiting: 12 },
};

/** Shut when nothing waits. */
export const Shut: Story = {
  args: { waiting: 0 },
};

/** The empty state's size, which is decoration and carries no label. */
export const Large: Story = {
  args: { waiting: 0, large: true },
};
