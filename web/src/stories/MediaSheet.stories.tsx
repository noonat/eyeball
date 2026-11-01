import type { Meta, StoryObj } from "@storybook/react-vite";
import { ScreenSheet } from "./ScreenSheet";
import { checkSheet } from "./sheetChecks";
import { entries } from "./Sheet";
import { mediaStates } from "./sheetStates";
import * as screens from "./File.stories";

const meta = {
  title: "File/Media",
  parameters: { frame: "screen", sheet: false },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const all = entries({ "./File.stories.tsx": screens });

/** File, media and files with no rows */
export const States: Story = {
  render: () => (
    <ScreenSheet
      title="File, media and files with no rows"
      intro={
        <>
          The files a browser displays and the ones it does not: a page in its
          sandbox, images before over after, a recording, a video, a PDF, a file
          past what the surface sends, and a file with no rendering at all.
        </>
      }
      entries={all}
      states={mediaStates}
      notes={[
        <>
          <b>The sheet serves no file bytes.</b> A frame, an image or a player
          points at the raw route, which Storybook does not answer, so each
          shows empty here. Their layout and captions are what the sheet judges.
        </>,
      ]}
    />
  ),
  play: async ({ canvasElement }) => {
    await checkSheet(canvasElement, all, mediaStates);
  },
};
