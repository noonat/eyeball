import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "./Button";
import { IconGrid } from "../stories/IconGrid";
import { Icon } from "./Icon";
import { Notice } from "./Notice";

const meta = {
  title: "Foundations/Icons",
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The whole subset, each with its name. */
export const Inventory: Story = {
  render: () => <IconGrid />,
};

/** An icon follows the size and color of the text it sits in. */
export const InText: Story = {
  render: () => (
    <>
      <p className="hint">
        Inline with text at three sizes: <Icon name="message-square" /> 11px
        meta, <Icon name="message-square" /> 15px body,{" "}
        <Icon name="message-square" large /> 22px step.
      </p>
      <div className="btnrow">
        <Button tone="primary" icon="check" href="#">
          Approve
        </Button>
        <Button tone="danger" icon="triangle-alert" href="#">
          Request changes
        </Button>
        <Button tone="ghost" icon="x" href="#">
          Cancel
        </Button>
      </div>
      <Notice tone="neutral">Neutral.</Notice>
      <Notice tone="warn">Wants attention.</Notice>
      <Notice tone="danger">Something did not land.</Notice>
    </>
  ),
};
