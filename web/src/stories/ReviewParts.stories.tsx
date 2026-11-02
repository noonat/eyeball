import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import type { Comment } from "../api/types";
import { CommentCard } from "../components/CommentCard";
import { CommentGroup, ThreadRow } from "../components/CommentList";
import { CommentEdit } from "../components/CommentEdit";
import { Composer } from "../components/Composer";
import { composerId, type ComposerAt } from "../components/composerAt";
import { DecisionBar, Tally } from "../components/DecisionBar";
import { reviewApproved, reviewCode, reviewDoc } from "../fixtures";
import addFile from "../fixtures/review-add-file.html?raw";
import addLate from "../fixtures/review-add-late.html?raw";
import addLine from "../fixtures/review-add-line.html?raw";
import addLineSecond from "../fixtures/review-add-line-second.html?raw";
import addPassage from "../fixtures/review-add-passage.html?raw";
import addReview from "../fixtures/review-add-review.html?raw";
import commentCard from "../fixtures/review-comment.html?raw";
import commentEdit from "../fixtures/review-comment-edit.html?raw";
import commentLate from "../fixtures/review-comment-late.html?raw";
import commentReleased from "../fixtures/review-comment-released.html?raw";
import composerFile from "../fixtures/review-composer-file.html?raw";
import composerLate from "../fixtures/review-composer-late.html?raw";
import composerLateReview from "../fixtures/review-composer-late-review.html?raw";
import composerLine from "../fixtures/review-composer-line.html?raw";
import composerLineEmpty from "../fixtures/review-composer-line-empty.html?raw";
import composerPassage from "../fixtures/review-composer-passage.html?raw";
import composerPassageCommented from "../fixtures/review-composer-passage-commented.html?raw";
import composerRemoved from "../fixtures/review-composer-removed.html?raw";
import composerRemovedBase from "../fixtures/review-composer-removed-base.html?raw";
import composerReview from "../fixtures/review-composer-review.html?raw";
import decideRefused from "../fixtures/review-decide-refused-422.html?raw";
import decideRefusedNote from "../fixtures/review-decide-refused-note-422.html?raw";
import deleteTally from "../fixtures/review-delete.html?raw";
import { sameFragment } from "./approved";
import { must } from "./must";

const meta = {
  title: "Review/Parts",
  parameters: { frame: "page" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Where a fragment is drawn, so a play function compares what is inside. */
function Fragment({ children }: { readonly children: ReactNode }) {
  return <div data-fragment>{children}</div>;
}

/** The same for a fragment that is a row of a code table. */
function Row({ children }: { readonly children: ReactNode }) {
  return (
    <table>
      <tbody data-fragment>{children}</tbody>
    </table>
  );
}

// The comments the fragments hold, as the round views send them.
const workerCard = reviewCode.cards?.[1];
const lineComment = must(
  workerCard?.body?.threads?.[0]?.comments?.[0],
  "comment on the worker's line",
);
const lateComment = must(reviewApproved.acting?.comments?.[0], "late comment");
const releasedComment = must(
  reviewApproved.cards?.[0]?.body?.comments?.[0],
  "released comment",
);
const passageThread = reviewDoc.cards?.[0]?.body?.passages
  ?.map((part) => part.passage?.thread)
  .find((thread) => thread !== null && thread !== undefined);
const passageComment = must(passageThread?.comments?.[0], "passage comment");

const backoff = must(reviewCode.cards?.[0]?.id, "backoff card");
const worker = must(workerCard?.id, "worker card");
const product = must(reviewDoc.cards?.[0]?.id, "product card");
const limit = must(reviewApproved.cards?.[0]?.id, "limit card");

/** A comment built as the server's answer for a write, which no view holds. */
function comment(
  id: number,
  where: string,
  body: string,
  quoted = "",
  late = false,
): Comment {
  return {
    id: String(id),
    href: `/reviews/8/comments/${id}`,
    where,
    quoted,
    body,
    late,
    editable: !late,
  };
}

const at = {
  review: { level: "review" },
  backoff: { level: "file", path: "internal/upload/backoff.go" },
  line: { level: "line", path: "internal/upload/worker.go", line: 13 },
  empty: { level: "line", path: "internal/upload/worker.go", line: 12 },
  removed: {
    level: "file",
    path: "internal/upload/worker.go",
    oldLine: 13,
  },
  passage: { level: "passage", path: "docs/product.md", first: 9, last: 9 },
  commented: { level: "passage", path: "docs/product.md", first: 7, last: 7 },
  product: { level: "file", path: "docs/product.md" },
  limit: { level: "file", path: "internal/approved/limit.go" },
} as const satisfies Record<string, ComposerAt>;

/** The review's own composer. */
export const ComposerReview: Story = {
  render: () => (
    <Fragment>
      <Composer at={at.review} id="comments-review" late={false} />
    </Fragment>
  ),
  play: sameFragment(composerReview),
};

/** A file's composer, in the file's slot. */
export const ComposerFile: Story = {
  render: () => (
    <Fragment>
      <Composer
        at={at.backoff}
        id={composerId(at.backoff, backoff)}
        late={false}
      />
    </Fragment>
  ),
  play: sameFragment(composerFile),
};

/** A line's composer, under the line, with the comment already on it above. */
export const ComposerLine: Story = {
  render: () => (
    <Row>
      <Composer
        at={at.line}
        id={composerId(at.line, worker)}
        late={false}
        cards={<CommentCard comment={lineComment} />}
      />
    </Row>
  ),
  play: sameFragment(composerLine),
};

/** A line with no comment yet. */
export const ComposerLineEmpty: Story = {
  render: () => (
    <Row>
      <Composer at={at.empty} id={composerId(at.empty, worker)} late={false} />
    </Row>
  ),
  play: sameFragment(composerLineEmpty),
};

/** A removed row's composer, which is the file's comment quoting the row. */
export const ComposerRemoved: Story = {
  render: () => (
    <Row>
      <Composer
        at={at.removed}
        id={composerId(at.removed, worker)}
        late={false}
      />
    </Row>
  ),
  play: sameFragment(composerRemoved),
};

/** A passage's composer, in a group under the passage. */
export const ComposerPassage: Story = {
  render: () => (
    <Fragment>
      <Composer
        at={at.passage}
        id={composerId(at.passage, product)}
        late={false}
      />
    </Fragment>
  ),
  play: sameFragment(composerPassage),
};

/** A passage that has a comment, with the composer below it. */
export const ComposerPassageCommented: Story = {
  render: () => (
    <Fragment>
      <Composer
        at={at.commented}
        id={passageThread?.id ?? ""}
        late={false}
        cards={<CommentCard comment={passageComment} />}
      />
    </Fragment>
  ),
  play: sameFragment(composerPassageCommented),
};

/**
 * A removed row's composer on the whole-review comparison. The comparison is in
 * a hidden field, which the markup comparison leaves out, and the request test
 * holds it.
 */
export const ComposerRemovedWhole: Story = {
  render: () => (
    <Row>
      <Composer
        at={at.removed}
        id={composerId(at.removed, worker)}
        late={false}
      />
    </Row>
  ),
  play: sameFragment(composerRemovedBase),
};

/** On a decided round the hint says a comment is sent at once. */
export const ComposerLate: Story = {
  render: () => (
    <Fragment>
      <Composer at={at.limit} id={composerId(at.limit, limit)} late />
    </Fragment>
  ),
  play: sameFragment(composerLate),
};

/** The review's own composer on a decided round. */
export const ComposerLateReview: Story = {
  render: () => (
    <Fragment>
      <Composer at={at.review} id="comments-review" late />
    </Fragment>
  ),
  play: sameFragment(composerLateReview),
};

/** A comment of an open round, with Edit and Delete. */
export const CommentOpen: Story = {
  render: () => (
    <Fragment>
      <CommentCard comment={lineComment} />
    </Fragment>
  ),
  play: sameFragment(commentCard),
};

/** The same comment as the field it is edited in. */
export const CommentEditing: Story = {
  render: () => (
    <Fragment>
      <CommentEdit comment={lineComment} />
    </Fragment>
  ),
  play: sameFragment(commentEdit),
};

/** A comment written after the verdict, which cannot change. */
export const CommentAfterVerdict: Story = {
  render: () => (
    <Fragment>
      <CommentCard comment={lateComment} />
    </Fragment>
  ),
  play: sameFragment(commentLate),
};

/** A comment that went out with the verdict, which cannot change. */
export const CommentReleased: Story = {
  render: () => (
    <Fragment>
      <CommentCard comment={releasedComment} />
    </Fragment>
  ),
  play: sameFragment(commentReleased),
};

/** What the server answers when a verdict asks for changes with nothing in it. */
export const BarRefused: Story = {
  render: () => (
    <Fragment>
      <DecisionBar count={0} refused />
    </Fragment>
  ),
  play: sameFragment(decideRefused),
};

/** The same with a note of only spaces, which counts as none. */
export const BarRefusedNote: Story = {
  render: () => (
    <Fragment>
      <DecisionBar count={0} refused note="  " />
    </Fragment>
  ),
  play: sameFragment(decideRefusedNote),
};

/** The count after a comment is deleted. */
export const TallyAfterDelete: Story = {
  render: () => (
    <Fragment>
      <Tally count={5} />
    </Fragment>
  ),
  play: sameFragment(deleteTally),
};

/** The answer to a comment on a line: its thread, then the count. */
export const AnswerLine: Story = {
  render: () => (
    <Row>
      <ThreadRow
        id="thread-4356bb35505e-6"
        comments={[
          comment(
            16,
            "internal/forms/a.go:6",
            "Why not return the error?",
            'return errors.New("later")',
          ),
        ]}
      />
    </Row>
  ),
  play: sameFragment(addLine.replace(/<div class="tally"[\s\S]*$/, "")),
};

/** The second comment on the same line joins the first. */
export const AnswerLineSecond: Story = {
  render: () => (
    <Row>
      <ThreadRow
        id="thread-4356bb35505e-6"
        comments={[
          comment(
            16,
            "internal/forms/a.go:6",
            "Why not return the error?",
            'return errors.New("later")',
          ),
          comment(
            17,
            "internal/forms/a.go:6",
            "A second thought on the same line.",
            'return errors.New("later")',
          ),
        ]}
      />
    </Row>
  ),
  play: sameFragment(addLineSecond.replace(/<div class="tally"[\s\S]*$/, "")),
};

/** The answer to a comment on a passage. */
export const AnswerPassage: Story = {
  render: () => (
    <Fragment>
      <CommentGroup
        id="thread-d38ea5e7ffac-p7-7"
        comments={[
          comment(
            18,
            "internal/forms/doc/forms.md:7",
            "This sentence needs a number.",
            "A key that was stored stores nothing new.",
          ),
        ]}
      />
    </Fragment>
  ),
  play: sameFragment(addPassage.replace(/<div class="tally"[\s\S]*$/, "")),
};

/** The answer to a comment on a file: the file's list. */
export const AnswerFile: Story = {
  render: () => (
    <Fragment>
      <CommentGroup
        id="comments-4356bb35505e"
        comments={[
          comment(15, "internal/forms/a.go", "This file wants a test."),
        ]}
      />
    </Fragment>
  ),
  play: sameFragment(addFile.replace(/<div class="tally"[\s\S]*$/, "")),
};

/** The answer to a comment on the review: its list. */
export const AnswerReview: Story = {
  render: () => (
    <Fragment>
      <CommentGroup
        id="comments-review"
        comments={[
          comment(14, "This review", "Say what the failure looks like."),
        ]}
      />
    </Fragment>
  ),
  play: sameFragment(addReview.replace(/<div class="tally"[\s\S]*$/, "")),
};

/** On a decided round the new comment is marked as sent after the verdict. */
export const AnswerLate: Story = {
  render: () => (
    <Fragment>
      <CommentGroup
        id="comments-1789eb48ac9e"
        comments={[
          {
            ...releasedComment,
            href: `/reviews/3/comments/8`,
            editable: false,
          },
          {
            ...comment(
              20,
              "internal/approved/limit.go",
              "Sent at once.",
              "",
              true,
            ),
            href: "/reviews/3/comments/20",
          },
        ]}
      />
    </Fragment>
  ),
  play: sameFragment(addLate),
};
