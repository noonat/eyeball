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
