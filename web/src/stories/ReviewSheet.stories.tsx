import type { Meta, StoryObj } from "@storybook/react-vite";
import { ScreenSheet } from "./ScreenSheet";
import { checkSheet } from "./sheetChecks";
import { entries } from "./Sheet";
import { reviewStates } from "./sheetStates";
import * as rounds from "./Review.stories";
import * as states from "./ReviewStates.stories";

const meta = {
  title: "Review/Rounds",
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

/** Review, one round at a time */
export const States: Story = {
  render: () => (
    <ScreenSheet
      title="Review, one round at a time"
      intro={
        <>
          A round of a review: the agent&rsquo;s note, who and what it is about,
          the verdict if there is one, the comparison, a card for each file that
          moved, and the decision bar. A document is folded to what changed, a
          closed round is read from the current one, and a review the agent
          withdrew or whose base is gone says so above its files.
        </>
      }
      entries={all}
      states={reviewStates}
      notes={[
        <>
          <b>A card holds the link to its file in its summary.</b> That is the
          approved markup, and axe reports a link inside a summary as nested
          interactive controls. The rule is off for the summary alone, and it is
          the one known accessibility fault of this screen.
        </>,
      ]}
    />
  ),
  play: async ({ canvasElement }) => {
    await checkSheet(canvasElement, all, reviewStates);
  },
};
