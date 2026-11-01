import type { Comment } from "../api/types";
import { CommentCard } from "./CommentCard";
import { CommentEdit } from "./CommentEdit";
import type { ComposerAt } from "./composer";
import { pendingAt, useReviewUi } from "./reviewUi";

/** One stored comment, as a card or as its edit form, by what the page is doing. */
export function CommentItem({ comment }: { readonly comment: Comment }) {
  const ui = useReviewUi();
  if (ui.editing === comment.id) {
    return (
      <CommentEdit
        comment={comment}
        onSave={(body) => {
          ui.onSave?.(comment.id, body);
        }}
        onCancel={ui.onCancelEdit}
      />
    );
  }
  return (
    <CommentCard
      comment={comment}
      onEdit={() => {
        ui.onEdit?.(comment.id);
      }}
      onDelete={() => {
        ui.onDelete?.(comment.id);
      }}
    />
  );
}

interface CommentItemsProps {
  /** The stored comments at the anchor. */
  readonly comments: readonly Comment[] | null;
  /**
   * The anchor, for the comments the reader wrote there that the server has not
   * stored. They follow the stored ones. A list of comments that cannot be
   * written to leaves it out.
   */
  readonly at?: ComposerAt | undefined;
}

/** Comments, one after another, the unstored ones last. */
export function CommentItems({ comments, at }: CommentItemsProps) {
  const ui = useReviewUi();
  const pending = at === undefined ? [] : pendingAt(ui, at);
  return (
    <>
      {(comments ?? []).map((comment) => (
        <CommentItem key={comment.id} comment={comment} />
      ))}
      {pending.map((p) => (
        <CommentCard
          key={p.comment.id}
          comment={p.comment}
          state={p.state}
          onRetry={() => {
            ui.onRetry?.(p.comment.id);
          }}
          onDelete={() => {
            ui.onDelete?.(p.comment.id);
          }}
        />
      ))}
    </>
  );
}

interface CommentGroupProps extends CommentItemsProps {
  /** The element id of the group, which a comment's answer replaces. */
  readonly id: string;
}

/** The comments at one anchor, in an element a composer's answer replaces. */
export function CommentGroup({ id, comments, at }: CommentGroupProps) {
  return (
    <div id={id}>
      <CommentItems comments={comments} at={at} />
    </div>
  );
}

/** The row of a code table that holds the comments under one line. */
export function ThreadRow({ id, comments, at }: CommentGroupProps) {
  return (
    <tr className="thread" id={id}>
      <td colSpan={3}>
        <div className="thread-body">
          <CommentItems comments={comments} at={at} />
        </div>
      </td>
    </tr>
  );
}
