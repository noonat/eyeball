import type { Meta, StoryObj } from "@storybook/react-vite";
import { FilesScreen } from "../components/FilesScreen";
import { treeRoot, treeSubdir } from "../fixtures";
import rootHtml from "../fixtures/tree-root.html?raw";
import subdirHtml from "../fixtures/tree-subdir.html?raw";
import { sameAsApproved } from "./approved";

const meta = {
  title: "Tree/Screen",
  component: FilesScreen,
  parameters: { frame: "screen" },
} satisfies Meta<typeof FilesScreen>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The project at the round under review, from its root. */
export const Root: Story = {
  args: { files: treeRoot },
  play: sameAsApproved(rootHtml),
};

/** A directory, with the subdirectory holding changed files open in place. */
export const Subdirectory: Story = {
  args: { files: treeSubdir },
  play: sameAsApproved(subdirHtml),
};
