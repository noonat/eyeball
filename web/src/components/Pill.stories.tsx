import type { Meta, StoryObj } from "@storybook/react-vite";
import { Pill } from "./Pill";

const meta = {
  title: "Foundations/Pill",
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The four review states and the plain count. */
export const Tones: Story = {
  render: () => (
    <div className="levels">
      <Pill tone="waiting">Waiting</Pill>
      <Pill tone="working">Working</Pill>
      <Pill tone="approved">Approved</Pill>
      <Pill tone="changes">Changes</Pill>
      <Pill tone="plain">Round 3</Pill>
    </div>
  ),
};
