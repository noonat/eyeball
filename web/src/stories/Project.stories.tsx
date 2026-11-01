import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProjectScreen } from "../components/ProjectScreen";
import {
  projectClean,
  projectNotGit,
  projectPopulated,
  projectWorktreeCommitted,
  projectWorktrees,
} from "../fixtures";
import cleanHtml from "../fixtures/project-clean.html?raw";
import notGitHtml from "../fixtures/project-not-git.html?raw";
import populatedHtml from "../fixtures/project-populated.html?raw";
import worktreeCommittedHtml from "../fixtures/project-worktree-committed.html?raw";
import worktreesHtml from "../fixtures/project-worktrees.html?raw";
import { sameAsApproved } from "./approved";

const meta = {
  title: "Project/Screen",
  component: ProjectScreen,
  parameters: { frame: "screen" },
} satisfies Meta<typeof ProjectScreen>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Open reviews over the working copy, live, with claimed files marked. */
export const Populated: Story = {
  args: { project: projectPopulated },
  play: sameAsApproved(populatedHtml),
};

/** A main checkout and a worktree, each with its own section. */
export const Worktrees: Story = {
  args: { project: projectWorktrees },
  play: sameAsApproved(worktreesHtml),
};

/** The worktree committed, so its section lists nothing. */
export const WorktreeCommitted: Story = {
  args: { project: projectWorktreeCommitted },
  play: sameAsApproved(worktreeCommittedHtml),
};

/** Everything committed, with one review still open. */
export const Clean: Story = {
  args: { project: projectClean },
  play: sameAsApproved(cleanHtml),
};

/** A project git cannot see: the warning names what is lost. */
export const NotGit: Story = {
  args: { project: projectNotGit },
  play: sameAsApproved(notGitHtml),
};
