import type { Meta, StoryObj } from "@storybook/react-vite";
import { ScreenSheet } from "./ScreenSheet";
import { checkSheet } from "./sheetChecks";
import { entries } from "./Sheet";
import { errorStates } from "./sheetStates";
import * as screens from "./Error.stories";

const meta = {
  title: "Error/Sheet",
  parameters: { frame: "screen", sheet: false },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const all = entries({ "./Error.stories.tsx": screens });

/** Every failure the server answers a page load with, light beside dark. */
export const States: Story = {
  render: () => (
    <ScreenSheet
      title="Errors"
      intro={
        <>
          The screen a person sees when the server fails a page load, with the
          sentence the server sends. Each state is in a 390 by 800 frame, light
          beside dark, and each frame has a Full screen button.
        </>
      }
      entries={all}
      states={errorStates}
      notes={[
        <>
          <b>A write refused for its token has no state of its own.</b> It draws
          the same screen as the 405, with the sentence about the address
          eyeball serve printed.
        </>,
      ]}
    />
  ),
  play: async ({ canvasElement }) => {
    await checkSheet(canvasElement, all, errorStates);
  },
};
