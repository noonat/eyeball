import type { Meta, StoryObj } from "@storybook/react-vite";
import { ScreenSheet } from "./ScreenSheet";
import { checkSheet } from "./sheetChecks";
import { entries } from "./Sheet";
import { fileStates } from "./sheetStates";
import * as screens from "./File.stories";

const meta = {
  title: "File/Sheet",
  parameters: { frame: "screen", sheet: false },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const all = entries({ "./File.stories.tsx": screens });

/** File, code and documents */
export const States: Story = {
  render: () => (
    <ScreenSheet
      title="File, code and documents"
      intro={
        <>
          A file at a round, or live from a working copy: code with the change
          marked or plain, a markdown file as a document or as source, a file
          the review does not touch, and one nothing captured. The images,
          recordings, pages and documents are on the media sheet.
        </>
      }
      entries={all}
      states={fileStates}
      notes={[
        <>
          <b>
            A file opened for context carries a banner saying which context it
            is.
          </b>{" "}
          Without it a reviewer could confuse a file under review with one that
          is not.
        </>,
      ]}
    />
  ),
  play: async ({ canvasElement }) => {
    await checkSheet(canvasElement, all, fileStates);
  },
};
