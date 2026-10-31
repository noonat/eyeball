import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { entries } from "./Sheet";
import { ScreenSheet, missingStates, type ScreenState } from "./ScreenSheet";
import * as screens from "./Queue.stories";

const meta = {
  title: "Queue/Sheet",
  parameters: { frame: "screen", sheet: false },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const all = entries({ "./Queue.stories.tsx": screens });

const states: readonly ScreenState[] = [
  { id: "queue-screen--populated", caption: "populated" },
  { id: "queue-screen--after-a-verdict", caption: "after a verdict" },
  { id: "queue-screen--nothing-waiting", caption: "nothing waiting" },
  { id: "queue-screen--not-installed", caption: "not installed yet" },
  { id: "queue-screen--not-allowed", caption: "notifications not allowed" },
  { id: "queue-screen--lapsed", caption: "a lapsed subscription" },
  { id: "queue-screen--awkward-week", caption: "the awkward week" },
  { id: "queue-screen--loading", caption: "connecting, before the first view" },
];

const wide: ScreenState = { id: "queue-screen--wide", caption: "wide layout" };

/** Every state of the queue, light beside dark, for judging on the phone. */
export const States: Story = {
  render: () => (
    <ScreenSheet
      title="Queue"
      intro={
        <>
          The home screen, drawn by its components from the views the API sends.
          It answers one question: what needs attention. Each state is in a 390
          by 800 frame, light beside dark, and each frame has a Full screen
          button.
        </>
      }
      entries={all}
      states={states}
      wide={wide}
      notes={[
        <>
          <b>Rows are in the order the API sends them.</b> Waiting lists the
          longest wait first, which is what the template page draws, and the
          owner approved it so. The static frames in queue.html draw the newest
          first, and the component does not follow them.
        </>,
        <>
          <b>
            The static frame&rsquo;s &ldquo;Not requested yet&rdquo; row has no
            state here.
          </b>{" "}
          A row is Working only once a round has an answer, so no data produces
          it.
        </>,
      ]}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(missingStates(all, [...states, wide])).toEqual([]);
    // A phone frame and a Full screen button for each state and theme, and one
    // each for the wide layout.
    await expect(canvasElement.querySelectorAll(".phone")).toHaveLength(
      states.length * 2,
    );
    await expect(
      canvas.getAllByRole("link", { name: /full screen/i }),
    ).toHaveLength(states.length * 2 + 2);
    // A rule written for the sheet's own headings once matched the queue's
    // section headings inside the frames, which then drew 56px above a section
    // and a 19px heading. A heading in a frame has to compute as it does in a
    // screen with no sheet around it.
    const bare = document.createElement("div");
    bare.className = "app";
    bare.dataset["theme"] = "light";
    bare.innerHTML =
      '<div class="scroll"><div class="page"><div class="section"><h2>Working</h2></div></div></div>';
    document.body.append(bare);
    const bareHeading = bare.querySelector("h2");
    const framed = canvasElement.querySelector(".phone .section h2");
    if (bareHeading === null || framed === null) {
      throw new Error("a queue heading is missing from the sheet");
    }
    const reference = getComputedStyle(bareHeading);
    const inFrame = getComputedStyle(framed);
    for (const property of [
      "margin-top",
      "margin-bottom",
      "font-size",
      "letter-spacing",
    ]) {
      await expect(inFrame.getPropertyValue(property)).toBe(
        reference.getPropertyValue(property),
      );
    }
    bare.remove();
    for (const phone of canvasElement.querySelectorAll(".phone")) {
      await expect(phone.getAttribute("data-theme")).toMatch(/^(light|dark)$/);
    }
  },
};
