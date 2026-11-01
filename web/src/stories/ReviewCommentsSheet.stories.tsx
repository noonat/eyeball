import type { Meta, StoryObj } from "@storybook/react-vite";
import { ScreenSheet } from "./ScreenSheet";
import { checkSheet } from "./sheetChecks";
import { entries } from "./Sheet";
import { reviewCommentStates } from "./sheetStates";
import * as rounds from "./Review.stories";
import * as states from "./ReviewStates.stories";

const meta = {
  title: "Review/Comments",
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

/** Review, comments and composers */
export const States: Story = {
  render: () => (
    <ScreenSheet
      title="Review, comments and composers"
      intro={
        <>
          A composer opens where the reader taps: under a line, under a removed
          line, under a passage, in a file&rsquo;s own slot, and on a decided
          round, where a comment goes out at once. A comment is edited in place.
          A comment on its way shows its text with no actions, and one the
          server did not take stays where it was written, dashed, with a way to
          retry it.
        </>
      }
      entries={all}
      states={reviewCommentStates}
      notes={[
        <>
          <b>Sending and Not saved are new states.</b> No template drew them.
          Both keep the comment&rsquo;s text on screen: the first marks the card
          with the word Sending, and the second is the dashed card the template
          page kept in a template for its script to clone. The stylesheet draws
          its Not saved mark.
        </>,
      ]}
    />
  ),
  play: async ({ canvasElement }) => {
    await checkSheet(canvasElement, all, reviewCommentStates);
  },
};
