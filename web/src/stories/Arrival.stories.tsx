import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { NotFound } from "../api/types";
import { NotFoundScreen } from "../components/NotFoundScreen";
import { ScreenLoading } from "../components/ScreenLoading";

const meta = {
  title: "Screen/Arrival",
  parameters: { frame: "screen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** What the API sends with a 404 for an address that has nothing at it. */
const plain: NotFound = { waiting_count: 4, notice: null };

/** The same where a live read was refused, which the banner explains. */
const refused: NotFound = {
  waiting_count: 4,
  notice: {
    warn: true,
    heading: "Only what this round captured can be shown.",
    text: "The project's root, ~/src/harbor, no longer exists.",
  },
};

/**
 * A screen opened by a move inside the app, before its view arrives. The
 * wordmark stands where the title and the way back will be, and no eye is
 * drawn, since the count is part of the view.
 */
export const Loading: Story = {
  render: () => <ScreenLoading />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvasElement.querySelector(".topbar .wordmark"),
    ).not.toBeNull();
    await expect(canvasElement.querySelector(".topbar .eye")).toBeNull();
    await expect(canvasElement.querySelector(".topbar .back")).toBeNull();
    await expect(canvas.getByRole("status")).toHaveTextContent(
      "Eyeball is connecting",
    );
  },
};

/** An address the view answers 404: a review, a path or a project not there. */
export const NotFoundPlain: Story = {
  render: () => <NotFoundScreen missing={plain} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Nothing here")).toBeVisible();
    await expect(canvasElement.querySelector(".notice")).toBeNull();
    await expect(canvasElement.querySelector(".topbar .eye")).not.toBeNull();
  },
};

/** A path a live read refused, with the banner saying why. */
export const NotFoundRefused: Story = {
  render: () => <NotFoundScreen missing={refused} />,
  play: async ({ canvasElement }) => {
    await expect(canvasElement.querySelector(".notice.warn")).not.toBeNull();
  },
};
