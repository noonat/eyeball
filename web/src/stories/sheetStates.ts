import type { ScreenState } from "./ScreenSheet";

// The states each screen sheet lays out, by story id. They are listed by hand,
// so ScreenCoverage.stories.tsx fails when a screen story is on none of them.

/** The states on the project sheet. */
export const projectStates: readonly ScreenState[] = [
  { id: "project-screen--populated", caption: "populated" },
  {
    id: "project-screen--worktrees",
    caption: "a main checkout and a worktree",
  },
  {
    id: "project-screen--worktree-committed",
    caption: "the worktree committed",
  },
  { id: "project-screen--clean", caption: "everything committed" },
  { id: "project-screen--not-git", caption: "not a git repository" },
  { id: "screen-arrival--loading", caption: "before the view arrives" },
  {
    id: "screen-arrival--not-found-plain",
    caption: "an address with nothing at it",
  },
  { id: "screen-arrival--not-found-refused", caption: "a live read refused" },
];

/** The states on the tree sheet. */
export const treeStates: readonly ScreenState[] = [
  { id: "tree-screen--root", caption: "the root" },
  { id: "tree-screen--subdirectory", caption: "a subdirectory" },
];

/** The states on the file sheet. */
export const fileStates: readonly ScreenState[] = [
  { id: "file-screen--whole", caption: "marked" },
  { id: "file-screen--plain", caption: "plain" },
  { id: "file-screen--document", caption: "a document" },
  { id: "file-screen--document-source", caption: "a document, as source" },
  { id: "file-screen--context", caption: "context, from the base" },
  { id: "file-screen--live", caption: "live, from disk" },
  { id: "file-screen--image-context", caption: "an unchanged image" },
  { id: "file-screen--mode", caption: "a mode change" },
  { id: "file-screen--worktree", caption: "a worktree&rsquo;s live file" },
];

/** The states on the media sheet. */
export const mediaStates: readonly ScreenState[] = [
  { id: "file-screen--page", caption: "a page" },
  { id: "file-screen--image-changed", caption: "a changed image" },
  { id: "file-screen--image-new", caption: "a new image" },
  { id: "file-screen--image-deleted", caption: "a deleted image" },
  { id: "file-screen--svg", caption: "an SVG, as an image" },
  { id: "file-screen--audio-changed", caption: "a changed recording" },
  { id: "file-screen--video-new", caption: "a new video" },
  { id: "file-screen--pdf", caption: "a PDF" },
  { id: "file-screen--too-large", caption: "past the send limit" },
  { id: "file-screen--binary", caption: "no rendering" },
];

/** The states on the sheet of a review's rounds. */
export const reviewStates: readonly ScreenState[] = [
  { id: "review-screen--round-two", caption: "a document, second round" },
  { id: "review-screen--whole-review", caption: "the whole review" },
  {
    id: "review-screen--round-one-closed",
    caption: "an earlier round, closed",
  },
  { id: "review-screen--code", caption: "code, waiting" },
  { id: "review-screen--approved", caption: "approved" },
  { id: "review-screen--fresh", caption: "a new document" },
  { id: "review-screen--earlier", caption: "reached from the current round" },
  { id: "review-screen--abandoned", caption: "withdrawn by the agent" },
  { id: "review-screen--unmeasurable", caption: "a base git cannot read" },
];

/** The states on the sheet of large rounds and what a card holds. */
export const largeReviewStates: readonly ScreenState[] = [
  { id: "review-screen--deferred", caption: "bodies on their way" },
  { id: "review-screen--held", caption: "a file held back" },
  { id: "review-screen--listed", caption: "twelve files, listed" },
  { id: "review-screen--rendered", caption: "files shown as themselves" },
  { id: "review-states--body-failed", caption: "a body that did not load" },
  {
    id: "review-states--load-failed",
    caption: "a held file that did not load",
  },
];

/** The states on the sheet of comments and composers. */
export const reviewCommentStates: readonly ScreenState[] = [
  { id: "review-states--composer-on-line", caption: "a composer under a line" },
  {
    id: "review-states--composer-on-removed-line",
    caption: "a composer on a removed line",
  },
  {
    id: "review-states--composer-on-passage",
    caption: "a composer under a passage",
  },
  { id: "review-states--composer-on-file", caption: "a composer on a file" },
  {
    id: "review-states--composer-on-decided-round",
    caption: "a composer on a decided round",
  },
  { id: "review-states--editing-a-comment", caption: "a comment being edited" },
  { id: "review-states--sending", caption: "a comment sending" },
  { id: "review-states--unsent", caption: "a comment not saved" },
  { id: "review-states--composer-failed", caption: "a comment refused" },
  { id: "review-states--edit-failed", caption: "a save that failed" },
  {
    id: "review-states--comment-actions-failed",
    caption: "a delete that failed",
  },
  { id: "review-states--unsent-failed", caption: "a retry that failed" },
];

/** The states on the sheet of the decision bar and the screen before its view. */
export const reviewBarStates: readonly ScreenState[] = [
  {
    id: "review-states--bar-while-writing",
    caption: "a comment being written",
  },
  { id: "review-states--bar-offline", caption: "offline" },
  { id: "review-states--bar-failed", caption: "a verdict that failed" },
  {
    id: "review-states--bar-refused",
    caption: "changes requested with nothing",
  },
  { id: "review-states--loading", caption: "before the view arrives" },
];
