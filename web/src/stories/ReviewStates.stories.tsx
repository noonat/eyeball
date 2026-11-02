import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { Comment } from "../api/types";
import { pick } from "./must";
import { ReviewScreen } from "../components/ReviewScreen";
import { ScreenLoading } from "../components/ScreenLoading";
import {
  reviewApproved,
  reviewCode,
  reviewDoc,
  reviewFresh,
} from "../fixtures";

const meta = {
  title: "Review/States",
  component: ReviewScreen,
  parameters: {
    frame: "screen",
    a11y: {
      // A card's summary holds the link to the file on its own, as the
      // approved page did. See Review/Screen.
      config: {
        rules: [
          { id: "nested-interactive", selector: "summary", enabled: false },
        ],
      },
    },
  },
} satisfies Meta<typeof ReviewScreen>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A comment the app has not stored yet. It has no address and no id of its own. */
function pending(
  id: number,
  where: string,
  body: string,
  quoted = "",
): Comment {
  return {
    id: `pending-${id}`,
    href: "",
    where,
    quoted,
    body,
    late: false,
    editable: false,
  };
}

/** A comment on its way: the text is shown, with no actions, and says so. */
export const Sending: Story = {
  args: {
    review: reviewCode,
    ui: {
      pending: [
        {
          at: { level: "review" },
          comment: pending(
            1,
            "This review",
            "Where is the retry limit tested?",
          ),
          state: "sending",
        },
      ],
    },
  },
  play: async ({ canvasElement }) => {
    const card = pick(canvasElement, ".comment:not([id])");
    await expect(within(card).getByText("Sending…")).toBeVisible();
    await expect(card.querySelector(".acts")).toBeNull();
  },
};

/**
 * A comment the server did not take. It stays where it was written, dashed and marked Not saved by the stylesheet,
 * with its text and a way to retry or delete it, and the
 * bar says the verdict would leave it behind.
 */
export const Unsent: Story = {
  args: {
    review: reviewCode,
    ui: {
      pending: [
        {
          at: { level: "line", path: "internal/upload/worker.go", line: 22 },
          comment: pending(
            2,
            "internal/upload/worker.go:22",
            "This second send ignores the context.",
            "\treturn w.send(ctx)",
          ),
          state: "unsent",
        },
      ],
    },
    bar: { unsent: 1, held: true },
  },
  play: async ({ canvasElement }) => {
    const card = pick(canvasElement, ".comment.unsent");
    const scope = within(card);
    await expect(
      scope.getByRole("button", { name: "Retry now" }),
    ).toBeVisible();
    await expect(scope.getByRole("button", { name: "Delete" })).toBeVisible();
    await expect(
      within(pick(canvasElement, ".decide")).getByText(/did not save/),
    ).toBeVisible();
  },
};

/** A composer open under a line, which has no comment yet. */
export const ComposerOnLine: Story = {
  args: {
    review: reviewCode,
    ui: {
      composer: { level: "line", path: "internal/upload/worker.go", line: 12 },
    },
  },
  play: async ({ canvasElement }) => {
    const row = canvasElement.querySelector("tr.thread .composer");
    await expect(row).not.toBeNull();
    await expect(
      row?.querySelector(".chip[aria-pressed='true']"),
    ).toHaveTextContent("Line 12");
  },
};

/** A composer open on a removed line, which comments on the file and quotes it. */
export const ComposerOnRemovedLine: Story = {
  args: {
    review: reviewCode,
    ui: {
      composer: {
        level: "file",
        path: "internal/upload/worker.go",
        oldLine: 3,
      },
    },
  },
  play: async ({ canvasElement }) => {
    await expect(
      canvasElement.querySelector("tr.thread .composer .anchor"),
    ).toHaveTextContent("internal/upload/worker.go, removed line 3");
  },
};

/** A composer open on a passage that has no comment yet. */
export const ComposerOnPassage: Story = {
  args: {
    review: reviewDoc,
    ui: {
      composer: {
        level: "passage",
        path: "docs/product.md",
        first: 9,
        last: 9,
      },
    },
  },
  play: async ({ canvasElement }) => {
    await expect(
      canvasElement.querySelector(".prose .composer .anchor"),
    ).toHaveTextContent("docs/product.md:9–9");
  },
};

/** A composer open in a file's own slot, under its comments. */
export const ComposerOnFile: Story = {
  args: {
    review: reviewCode,
    ui: {
      composer: { level: "file", path: "internal/upload/backoff.go" },
    },
  },
  play: async ({ canvasElement }) => {
    await expect(
      canvasElement.querySelector(".filecomposer .composer"),
    ).not.toBeNull();
  },
};

/** A composer on a decided round: a comment goes out at once, and it says so. */
export const ComposerOnDecidedRound: Story = {
  args: {
    review: reviewApproved,
    ui: {
      composer: { level: "file", path: "internal/approved/limit.go" },
    },
  },
  play: async ({ canvasElement }) => {
    await expect(
      within(pick(canvasElement, ".filecomposer")).getByText(
        "Sends immediately",
      ),
    ).toBeVisible();
  },
};

/** A comment shown as the field it is edited in. */
export const EditingAComment: Story = {
  args: {
    review: reviewCode,
    ui: { editing: ["5"] },
  },
  play: async ({ canvasElement }) => {
    await expect(
      canvasElement.querySelector(
        "tr.thread form.composer #comment-5, tr.thread form#comment-5",
      ),
    ).not.toBeNull();
  },
};

/**
 * A comment the server refused stays in its composer with the server's
 * sentence at the foot of the form, and what was typed is still there.
 */
export const ComposerFailed: Story = {
  args: {
    review: reviewCode,
    ui: {
      composer: { level: "line", path: "internal/upload/worker.go", line: 12 },
      composerText: "   ",
      failures: { composer: "A comment needs some text." },
    },
  },
  play: async ({ canvasElement }) => {
    const composer = pick(canvasElement, "tr.thread .composer");
    await expect(within(composer).getByRole("alert")).toHaveTextContent(
      "A comment needs some text.",
    );
    await expect(composer.querySelector("textarea")).toHaveValue("   ");
  },
};

/** A save that reached no server says so in the edit form, which keeps its text. */
export const EditFailed: Story = {
  args: {
    review: reviewCode,
    ui: {
      editing: ["5"],
      editText: { "5": "A sharper note." },
      failures: { "edit:5": "Cannot reach eyeball." },
    },
  },
  play: async ({ canvasElement }) => {
    const form = pick(canvasElement, "form#comment-5");
    await expect(within(form).getByRole("alert")).toHaveTextContent(
      "Cannot reach eyeball.",
    );
    await expect(form.querySelector("textarea")).toHaveValue("A sharper note.");
  },
};

/** A delete that got no answer in time says so after the comment's actions. */
export const CommentActionsFailed: Story = {
  args: {
    review: reviewCode,
    ui: {
      failures: {
        "comment:5":
          "eyeball did not answer in time. Nothing is known to have been sent.",
      },
    },
  },
  play: async ({ canvasElement }) => {
    const card = pick(canvasElement, "#comment-5");
    await expect(within(card).getByRole("alert")).toHaveTextContent(
      "eyeball did not answer in time. Nothing is known to have been sent.",
    );
    await expect(
      within(card).getByRole("link", { name: "Delete" }),
    ).toBeVisible();
  },
};

/** A retry that failed again says so beside the card's Retry and Delete. */
export const UnsentFailed: Story = {
  args: {
    review: reviewCode,
    ui: {
      pending: [
        {
          at: { level: "line", path: "internal/upload/worker.go", line: 22 },
          comment: pending(
            3,
            "internal/upload/worker.go:22",
            "This second send ignores the context.",
          ),
          state: "unsent",
        },
      ],
      failures: { "comment:pending-3": "Cannot reach eyeball." },
    },
    bar: { unsent: 1, held: true },
  },
  play: async ({ canvasElement }) => {
    const card = pick(canvasElement, ".comment.unsent");
    await expect(within(card).getByRole("alert")).toHaveTextContent(
      "Cannot reach eyeball.",
    );
    await expect(
      within(card).getByRole("button", { name: "Retry now" }),
    ).toBeVisible();
  },
};

/** A comment is still being written, so the verdict waits. */
export const BarWhileWriting: Story = {
  args: { review: reviewCode, bar: { unsaved: true, held: true } },
  play: async ({ canvasElement }) => {
    await expect(
      within(pick(canvasElement, ".decide")).getByText(
        "A comment is still being written.",
      ),
    ).toBeVisible();
  },
};

/** The stream is down, so a verdict could not be sent. */
export const BarOffline: Story = {
  args: { review: reviewCode, bar: { offline: true, held: true } },
  play: async ({ canvasElement }) => {
    await expect(
      within(pick(canvasElement, ".decide")).getByText(
        /can.t submit a review while offline/,
      ),
    ).toBeVisible();
  },
};

/** A request for changes with nothing to act on is refused, with the note kept. */
export const BarRefused: Story = {
  args: {
    review: reviewFresh,
    bar: { refused: true, note: "" },
  },
  play: async ({ canvasElement }) => {
    await expect(
      within(pick(canvasElement, ".decide")).getByText(
        "Requesting changes needs something to act on.",
      ),
    ).toBeVisible();
  },
};

/** A verdict that could not be sent says so at the foot of the bar, note kept. */
export const BarFailed: Story = {
  args: {
    review: reviewCode,
    bar: { failure: "Cannot reach eyeball.", note: "Close it out." },
  },
  play: async ({ canvasElement }) => {
    const bar = pick(canvasElement, ".decide");
    await expect(within(bar).getByRole("alert")).toHaveTextContent(
      "Cannot reach eyeball.",
    );
    await expect(bar.querySelector("textarea")).toHaveValue("Close it out.");
  },
};

/**
 * The screen before its view arrives, which is the one every screen below the
 * queue shows. The title, the way back and the count are part of the view, so
 * the bar has the wordmark and no eye.
 */
export const Loading: Story = {
  args: { review: reviewCode },
  render: () => <ScreenLoading />,
  play: async ({ canvasElement }) => {
    await expect(
      canvasElement.querySelector(".topbar .wordmark"),
    ).not.toBeNull();
    await expect(canvasElement.querySelector(".topbar .eye")).toBeNull();
  },
};
