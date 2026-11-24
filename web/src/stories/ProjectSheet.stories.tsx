import type { Meta, StoryObj } from "@storybook/react-vite";
import { ScreenSheet } from "./ScreenSheet";
import { checkSheet } from "./sheetChecks";
import { entries } from "./Sheet";
import { projectStates } from "./sheetStates";
import * as screens from "./Project.stories";
import * as arrival from "./Arrival.stories";

const meta = {
  title: "Project/Sheet",
  parameters: { frame: "screen", sheet: false },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const all = entries({
  "./Project.stories.tsx": screens,
  "./Arrival.stories.tsx": arrival,
});

/** Project, and the two states no template draws */
export const States: Story = {
  render: () => (
    <ScreenSheet
      title="Project, and the two states no template draws"
      intro={
        <>
          The project view, drawn by its components from the views the API
          sends, then the two states every screen below the queue shares: the
          moment before its view arrives, and an address with nothing at it.
          Each state is in a 390 by 800 frame, light beside dark, and each frame
          has a Full screen button.
        </>
      }
      entries={all}
      states={projectStates}
      notes={[
        <>
          <b>Before the view arrives, the bar shows the wordmark and no eye.</b>{" "}
          The title and the way back are part of the view, so a label drawn now
          would change when the view came, and a shut eye would say nothing
          waits. The eye returns with the count.
        </>,
        <>
          <b>A 404 draws the error screen&rsquo;s bar and box.</b> The view
          carries a waiting count and, where a live read was refused, the banner
          saying why. It has no title or way back, so the bar says Not found and
          leads to the queue.
        </>,
        <>
          <b>The working copy is live.</b> Its rows read the disk when the
          screen opens, and the screen says once that they are read only.
        </>,
      ]}
    />
  ),
  play: async ({ canvasElement }) => {
    await checkSheet(canvasElement, all, projectStates);
  },
};
