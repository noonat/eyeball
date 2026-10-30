import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import type { Queue as QueueView } from "../api/types";
import { Queue } from "../components/Queue";
import { awkward, nothingWaiting, populated } from "../fixtures";
import awkwardHtml from "../fixtures/queue-awkward.html?raw";
import nothingWaitingHtml from "../fixtures/queue-nothing-waiting.html?raw";
import populatedHtml from "../fixtures/queue-populated.html?raw";
import { screenMarkup, templateMarkup } from "./markup";

const meta = {
  title: "Queue/Screen",
  component: Queue,
  parameters: { frame: "screen" },
} satisfies Meta<typeof Queue>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The first n waiting rows, with the eye and the count following them. */
function firstWaiting(queue: QueueView, n: number): QueueView {
  return {
    ...queue,
    waiting: (queue.waiting ?? []).slice(0, n),
    waiting_count: n,
  };
}

/**
 * A data state is the markup the template page renders for the same server
 * state. Both are normalized the same way, so a class changed in a component
 * fails here.
 */
function sameAsTemplate(html: string): NonNullable<Story["play"]> {
  return async ({ canvasElement }) => {
    await expect(screenMarkup(canvasElement)).toEqual(templateMarkup(html));
  };
}

/** Three projects, four agents, every section. */
export const Populated: Story = {
  args: { queue: populated },
  play: sameAsTemplate(populatedHtml),
};

/**
 * The queue after a verdict: the reviewed row is gone from Waiting, the eye
 * holds one fewer, and it blinks once as the only confirmation.
 */
export const AfterAVerdict: Story = {
  args: { queue: firstWaiting(populated, 2), blink: true },
};

/** The shut eye stands where Waiting would be, and Working follows it. */
export const NothingWaiting: Story = {
  args: { queue: nothingWaiting },
  play: sameAsTemplate(nothingWaitingHtml),
};

/** Neither step is done, so the notice and both steps lead the queue. */
export const NotInstalled: Story = {
  args: { queue: firstWaiting(populated, 1), setup: "not-installed" },
};

/** Installed, with one step left, which is a tap. */
export const NotAllowed: Story = {
  args: { queue: firstWaiting(populated, 2), setup: "not-allowed" },
};

/** A subscription that stopped working, which nothing else on the phone says. */
export const Lapsed: Story = {
  args: { queue: firstWaiting(populated, 2), setup: "lapsed" },
};

/**
 * Titles that wrap to three lines, a path-shaped agent name, a review waiting
 * since Tuesday, an eleven-file change, and eight waiting at once.
 */
export const AwkwardWeek: Story = {
  args: { queue: awkward },
  play: sameAsTemplate(awkwardHtml),
};

/**
 * The wide layout, which a 390 pixel frame cannot show. Open it full screen at
 * a desktop width.
 */
export const Wide: Story = {
  args: { queue: populated },
  parameters: { sheet: false },
};
