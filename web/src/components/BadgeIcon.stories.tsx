import type { Meta, StoryObj } from "@storybook/react-vite";
import { BadgeIcon } from "./BadgeIcon";

const meta = {
  title: "Foundations/BadgeIcon",
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The installed icon answers "is anything waiting" without opening it. */
export const Waiting: Story = {
  render: () => (
    <div className="iconrow">
      <BadgeIcon waiting={3} />
      <p>
        Three reviews waiting, answered from the home screen without opening
        anything.
      </p>
    </div>
  ),
};
