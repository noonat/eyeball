import type { Meta, StoryObj } from "@storybook/react-vite";
import { ScreenSheet } from "./ScreenSheet";
import { checkSheet } from "./sheetChecks";
import { entries } from "./Sheet";
import { treeStates } from "./sheetStates";
import * as screens from "./Tree.stories";

const meta = {
  title: "Tree/Sheet",
  parameters: { frame: "screen", sheet: false },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const all = entries({ "./Tree.stories.tsx": screens });

/** Tree */
export const States: Story = {
  render: () => (
    <ScreenSheet
      title="Tree"
      intro={
        <>
          A review&rsquo;s file tree, drawn from the view the API sends. Files
          the review changes are bold, so browsing for context never loses track
          of what the verdict is about.
        </>
      }
      entries={all}
      states={treeStates}
      notes={[
        <>
          <b>A subdirectory holding changed files opens in place</b>, one level,
          with the counts the review page carries.
        </>,
      ]}
    />
  ),
  play: async ({ canvasElement }) => {
    await checkSheet(canvasElement, all, treeStates);
  },
};
