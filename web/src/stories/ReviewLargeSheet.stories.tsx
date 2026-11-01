import type { Meta, StoryObj } from "@storybook/react-vite";
import { ScreenSheet } from "./ScreenSheet";
import { checkSheet } from "./sheetChecks";
import { entries } from "./Sheet";
import { largeReviewStates } from "./sheetStates";
import * as rounds from "./Review.stories";
import * as states from "./ReviewStates.stories";

const meta = {
  title: "Review/Large",
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
          // The sheet draws each frame in both themes, so the list of files
          // is in the document twice. Each story alone has one.
          { id: "landmark-unique", enabled: false },
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

/** Review, large rounds and what a card holds */
export const States: Story = {
  render: () => (
    <ScreenSheet
      title="Review, large rounds and what a card holds"
      intro={
        <>
          A round past ten files or three hundred lines sends no bodies with the
          view, so each card opens as a skeleton as tall as its rows will be. A
          file past five hundred changed lines is held behind a line to load it.
          A round of more than ten files has a list of them above the cards. The
          last frame holds an image, a page, a video, a file too large to send,
          a binary and a mode change.
        </>
      }
      entries={all}
      states={largeReviewStates}
    />
  ),
  play: async ({ canvasElement }) => {
    await checkSheet(canvasElement, all, largeReviewStates);
  },
};
