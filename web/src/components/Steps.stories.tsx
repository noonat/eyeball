import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "./Button";
import { Step, Steps } from "./Steps";

const meta = {
  title: "Foundations/Steps",
  component: Steps,
} satisfies Meta<typeof Steps>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Installed, and notifications still to allow. */
export const FirstRun: Story = {
  args: {
    children: (
      <>
        <Step
          number={1}
          done
          what="Installed to the home screen"
          why="iOS delivers web push only to an installed page."
        />
        <Step
          number={2}
          what="Allow notifications"
          why="One request per review. Comments and verdicts send none."
          action={
            <Button tone="accent" small href="#">
              Allow
            </Button>
          }
        />
      </>
    ),
  },
};
