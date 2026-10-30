import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { Button } from "./Button";

const meta = {
  title: "Foundations/Button",
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Approve is filled and sits on the right. Request changes is outlined. */
export const Verdicts: Story = {
  render: () => (
    <div className="btnrow">
      <Button tone="primary" href="#">
        Approve
      </Button>
      <Button tone="danger" href="#">
        Request changes
      </Button>
      <Button href="#">Open file</Button>
      <Button tone="ghost" icon="x" href="#">
        Cancel
      </Button>
      <Button small href="#">
        Whole file
      </Button>
    </div>
  ),
};

function Allowing() {
  const [allowed, setAllowed] = useState(false);
  return (
    <>
      <Button
        tone="accent"
        small
        onClick={() => {
          setAllowed(true);
        }}
      >
        Allow
      </Button>
      <p className="hint">{allowed ? "Allowed" : "Not asked yet"}</p>
    </>
  );
}

/** A control that acts is a button, and a tap reaches its handler. */
export const Acting: Story = {
  render: () => <Allowing />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Not asked yet")).toBeInTheDocument();
    await userEvent.click(canvas.getByRole("button", { name: "Allow" }));
    await expect(canvas.getByText("Allowed")).toBeInTheDocument();
  },
};
