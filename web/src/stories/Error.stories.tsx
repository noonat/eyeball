import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { ErrorScreen } from "../components/ErrorScreen";

const meta = {
  title: "Screen/Error",
  parameters: { frame: "screen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** What the failure's sentence and heading say, and that the way back is there. */
async function expectDrawn(
  canvasElement: HTMLElement,
  title: string,
  heading: string,
  text: string,
): Promise<void> {
  const canvas = within(canvasElement);
  // The bar's heading can say what the box's headline says, so the box is
  // searched alone.
  const box = canvasElement.querySelector<HTMLElement>(".empty");
  if (box === null) {
    throw new Error("the error screen drew no box");
  }
  await expect(within(box).getByText(heading)).toBeVisible();
  await expect(within(box).getByText(text)).toBeVisible();
  await expect(canvasElement.querySelector(".topbar h1")).toHaveTextContent(
    title,
  );
  await expect(canvas.getByRole("link", { name: /queue/i })).toHaveAttribute(
    "href",
    "/",
  );
}

/** An address the mux has no route for. */
export const NotFound: Story = {
  render: () => (
    <ErrorScreen
      title="Not found"
      heading="Nothing here"
      text="There is no page at this address."
      waiting={4}
    />
  ),
  play: async ({ canvasElement }) => {
    await expectDrawn(
      canvasElement,
      "Not found",
      "Nothing here",
      "There is no page at this address.",
    );
    await expect(canvasElement.querySelector(".topbar .eye")).not.toBeNull();
  },
};

/** An address that does not answer the method used. */
export const NotAllowed: Story = {
  render: () => (
    <ErrorScreen
      title="Not allowed"
      heading="Not allowed"
      text="This address does not answer that kind of request."
      waiting={4}
    />
  ),
  play: async ({ canvasElement }) => {
    await expectDrawn(
      canvasElement,
      "Not allowed",
      "Not allowed",
      "This address does not answer that kind of request.",
    );
  },
};

/** A raw file past the send limit, opened as a page of its own. */
export const TooLarge: Story = {
  render: () => (
    <ErrorScreen
      title="Too large"
      heading="Too large"
      text="Past the 20 MB the surface sends on its own."
      waiting={4}
    />
  ),
  play: async ({ canvasElement }) => {
    await expectDrawn(
      canvasElement,
      "Too large",
      "Too large",
      "Past the 20 MB the surface sends on its own.",
    );
  },
};

/** A handler's failure, drawn where the waiting count could not be read. */
export const Failed: Story = {
  render: () => (
    <ErrorScreen
      title="Something went wrong"
      heading="This page could not be read"
      text="The daemon's log says why."
      waiting={null}
    />
  ),
  play: async ({ canvasElement }) => {
    await expectDrawn(
      canvasElement,
      "Something went wrong",
      "This page could not be read",
      "The daemon's log says why.",
    );
    // A null count draws no eye, since a shut eye would say nothing waits.
    await expect(canvasElement.querySelector(".topbar .eye")).toBeNull();
  },
};
