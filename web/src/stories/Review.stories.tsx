import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { ReviewScreen } from "../components/ReviewScreen";
import {
  reviewAbandoned,
  reviewApproved,
  reviewCards,
  reviewCode,
  reviewDeferred,
  reviewDoc,
  reviewDocRound1,
  reviewDocWhole,
  reviewEarlier,
  reviewFresh,
  reviewHeld,
  reviewListed,
  reviewUnmeasurable,
} from "../fixtures";
import reviewAbandonedHtml from "../fixtures/review-abandoned.html?raw";
import reviewApprovedHtml from "../fixtures/review-approved.html?raw";
import reviewCardsHtml from "../fixtures/review-cards.html?raw";
import reviewCodeHtml from "../fixtures/review-code.html?raw";
import reviewDeferredHtml from "../fixtures/review-deferred.html?raw";
import reviewDocHtml from "../fixtures/review-doc.html?raw";
import reviewDocRound1Html from "../fixtures/review-doc-round1.html?raw";
import reviewDocWholeHtml from "../fixtures/review-doc-whole.html?raw";
import reviewEarlierHtml from "../fixtures/review-earlier.html?raw";
import reviewFreshHtml from "../fixtures/review-fresh.html?raw";
import reviewHeldHtml from "../fixtures/review-held.html?raw";
import reviewListedHtml from "../fixtures/review-listed.html?raw";
import reviewUnmeasurableHtml from "../fixtures/review-unmeasurable.html?raw";
import { sameAsApproved } from "./approved";

const meta = {
  title: "Review/Screen",
  component: ReviewScreen,
  parameters: {
    frame: "screen",
    a11y: {
      // A card's summary holds the link to the file on its own, as the
      // approved page did. axe fails a focusable element inside a summary,
      // and the markup is the approved markup, so the rule is off for the
      // summary alone. Every other violation still fails the story.
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

/** A document in its second round, folded to what changed. */
export const RoundTwo: Story = {
  args: { review: reviewDoc },
  play: sameAsApproved(reviewDocHtml),
};

/** The same round against the whole review. */
export const WholeReview: Story = {
  args: { review: reviewDocWhole },
  play: sameAsApproved(reviewDocWholeHtml),
};

/** An earlier round of a document, closed, with the replies to its comments. */
export const RoundOneClosed: Story = {
  args: { review: reviewDocRound1 },
  play: sameAsApproved(reviewDocRound1Html),
};

/** A code review waiting, with a comment at each level. */
export const Code: Story = {
  args: { review: reviewCode },
  play: sameAsApproved(reviewCodeHtml),
};

/** An approved round, a comment sent with the verdict and one after it. */
export const Approved: Story = {
  args: { review: reviewApproved },
  play: sameAsApproved(reviewApprovedHtml),
};

/** A new document on its own in a first round, with nothing marked. */
export const Fresh: Story = {
  args: { review: reviewFresh },
  play: sameAsApproved(reviewFreshHtml),
};

/** A review the agent withdrew. */
export const Abandoned: Story = {
  args: { review: reviewAbandoned },
  play: sameAsApproved(reviewAbandonedHtml),
};

/** A round whose base git can no longer read. */
export const Unmeasurable: Story = {
  args: { review: reviewUnmeasurable },
  play: sameAsApproved(reviewUnmeasurableHtml),
};

/** Past the sending thresholds: every card opens empty, with a skeleton. */
export const Deferred: Story = {
  args: { review: reviewDeferred },
  play: async (context) => {
    await sameAsApproved(reviewDeferredHtml)(context);
    await expect(
      context.canvasElement.querySelectorAll(".skeleton"),
    ).toHaveLength(3);
  },
};

/** A file past the card limit shows its size and a way to load it. */
export const Held: Story = {
  args: { review: reviewHeld },
  play: sameAsApproved(reviewHeldHtml),
};

/** A round of twelve files, with the list of them above the cards. */
export const Listed: Story = {
  args: { review: reviewListed },
  play: sameAsApproved(reviewListedHtml),
};

/** An image, a page, a video, a file too large to send, a binary and a mode. */
export const Rendered: Story = {
  args: { review: reviewCards },
  play: sameAsApproved(reviewCardsHtml),
};

/** An earlier round reached from the current one. */
export const Earlier: Story = {
  args: { review: reviewEarlier },
  play: sameAsApproved(reviewEarlierHtml),
};
