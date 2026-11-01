import { createContext, useContext } from "react";
import type { Comment } from "../api/types";
import type { CommentState } from "./CommentCard";
import { sameAnchor, type ComposerAt } from "./composer";

/**
 * A comment the reader wrote that the server has not stored: on its way, or
 * refused. It keeps its text and the place it was written, so nothing typed is
 * lost, and it is drawn there with how far it has got. Its id is the app's own
 * until the server gives it one.
 */
export interface Pending {
  readonly at: ComposerAt;
  readonly comment: Comment;
  readonly state: CommentState;
}

/**
 * What the review page keeps for the reader, as the components below it read
 * it: which composer is open, which comment is being edited, how far each
 * comment's trip to the server has got, and what a tap does. The app holds
 * these. Nothing here sends a request.
 */
export interface ReviewUi {
  /** The round was decided, so a comment is sent at once. */
  readonly late: boolean;
  /** The composer open under a line, a passage, a file or the review. */
  readonly composer?: ComposerAt | undefined;
  /** The id of the comment shown as its edit form. */
  readonly editing?: string | undefined;
  /** The comments not yet stored, in the order they were written. */
  readonly pending?: readonly Pending[] | undefined;
  readonly onOpen?: ((at: ComposerAt) => void) | undefined;
  readonly onClose?: (() => void) | undefined;
  readonly onSubmit?: ((at: ComposerAt, body: string) => void) | undefined;
  readonly onEdit?: ((id: string) => void) | undefined;
  readonly onSave?: ((id: string, body: string) => void) | undefined;
  readonly onCancelEdit?: (() => void) | undefined;
  readonly onDelete?: ((id: string) => void) | undefined;
  readonly onRetry?: ((id: string) => void) | undefined;
  /** Asks for a deferred card's body, or a held card's, now. */
  readonly onLoad?: ((path: string) => void) | undefined;
}

/** The page's state for a round that takes no comments. */
export const readOnly: ReviewUi = { late: false };

/** The state the review page is drawn with. */
export const ReviewUiContext = createContext<ReviewUi>(readOnly);

/** The state the review page is drawn with. */
export function useReviewUi(): ReviewUi {
  return useContext(ReviewUiContext);
}

/** The pending comments written at one anchor. */
export function pendingAt(ui: ReviewUi, at: ComposerAt): readonly Pending[] {
  return (ui.pending ?? []).filter((p) => sameAnchor(p.at, at));
}
