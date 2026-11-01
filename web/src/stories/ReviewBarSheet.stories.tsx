import type { Meta, StoryObj } from "@storybook/react-vite";
import { ScreenSheet } from "./ScreenSheet";
import { checkSheet } from "./sheetChecks";
import { entries } from "./Sheet";
import { reviewBarStates } from "./sheetStates";
import * as rounds from "./Review.stories";
import * as states from "./ReviewStates.stories";

const meta = {
  title: "Review/Bar",
  parameters: {
    frame: "screen",
    sheet: false,
    // Each card's summary holds the link to its file, as the approved page
    // did, which axe reports as nested interactive controls. See
    // Review/Screen.
    a11y: {
      config: {
        rules: [
          { id: "nested-interactive", selector: "summary", enabled: false },
        ],
      },
    },
  },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const all = entries({
  "./Review.stories.tsx": rounds,
  "./ReviewStates.stories.tsx": states,
});

/** Review, the decision bar and the screen before its view */
export const States: Story = {
  render: () => (
    <ScreenSheet
      title="Review, the decision bar and the screen before its view"
      intro={
        <>
          The bar says what stands between the reader and a verdict: a comment
          still being written, a stream that is down, or a request for changes
          that has nothing to act on. The last frame is the screen before its
          view arrives, with the wordmark where the title will be.
        </>
      }
      entries={all}
      states={reviewBarStates}
    />
  ),
  play: async ({ canvasElement }) => {
    await checkSheet(canvasElement, all, reviewBarStates);
  },
};
