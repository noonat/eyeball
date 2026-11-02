import { createContext, useContext } from "react";
import type { Comment } from "../api/types";
import type { CommentState } from "./CommentCard";
import { sameAnchor, type ComposerAt } from "./composerAt";

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
  /** The composer open under a line, a passage or a file. */
  readonly composer?: ComposerAt | undefined;
  /**
   * What the open composer holds, and the level its chips chose to send it as.
   * They are the page's state and not the composer's, so the text moves with
   * the composer to another anchor.
   */
  readonly composerText?: string | undefined;
  readonly composerLevel?: ComposerAt["level"] | undefined;
  /**
   * Counts the taps on the place the composer is open at, each of which brings
   * the field back into view with the focus.
   */
  readonly composerFocus?: number | undefined;
  readonly onComposerText?: ((text: string) => void) | undefined;
  readonly onLevel?: ((level: ComposerAt["level"]) => void) | undefined;
  /** What the review's own box holds. */
  readonly reviewText?: string | undefined;
  readonly onReviewText?: ((text: string) => void) | undefined;
  /**
   * The comments shown as their edit forms, any number at once, with what each
   * holds.
   */
  readonly editing?: readonly string[] | undefined;
  readonly editText?: Readonly<Record<string, string>> | undefined;
  readonly onEditText?: ((id: string, text: string) => void) | undefined;
  /**
   * The line a request that failed leaves beside its control, by the control:
   * "composer", "review", "edit:" and a comment's id, or "comment:" and a
   * comment's id for its actions.
   */
  readonly failures?: Readonly<Record<string, string>> | undefined;
  /** The comments not yet stored, in the order they were written. */
  readonly pending?: readonly Pending[] | undefined;
  readonly onOpen?: ((at: ComposerAt) => void) | undefined;
  readonly onClose?: (() => void) | undefined;
  readonly onSubmit?: ((at: ComposerAt, body: string) => void) | undefined;
  readonly onEdit?: ((id: string) => void) | undefined;
  readonly onSave?: ((id: string, body: string) => void) | undefined;
  readonly onCancelEdit?: ((id: string) => void) | undefined;
  readonly onDelete?: ((id: string) => void) | undefined;
  readonly onRetry?: ((id: string) => void) | undefined;
  /**
   * Asks for a deferred card's body, or a held card's, now. A deferred card
   * asks as it comes within a screen of view, and again from its retry. The
   * failure of the request is in `failures` under "card:" and the path.
   */
  readonly onLoad?: ((path: string) => void) | undefined;
  /**
   * The paths of the cards the reader folded. A card is open unless it is here,
   * so a card the round sends is open and a file new to the round opens with
   * the rest.
   */
  readonly folded?: ReadonlySet<string> | undefined;
  readonly onFold?: ((path: string, open: boolean) => void) | undefined;
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

/** What the open composer is given, from the page's state. */
export function composerProps(ui: ReviewUi, at: ComposerAt) {
  return {
    at,
    late: ui.late,
    text: ui.composerText,
    onText: ui.onComposerText,
    level: ui.composerLevel,
    failure: ui.failures?.["composer"],
    focus: ui.composerFocus,
    onLevel: ui.onLevel,
    onSubmit: (text: string) => {
      ui.onSubmit?.(at, text);
    },
    onCancel: ui.onClose,
  };
}
