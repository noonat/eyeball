import type { CommentRequest } from "../api/types";

/**
 * Where a composer opens. A composer is keyed by its anchor, which names the
 * element it opens in. The text typed in it is not part of the anchor: a tap
 * on another line moves the composer there and keeps what was typed, as the
 * template page's did. A second composer for each tap would scatter
 * half-written comments through a long review.
 */
export type ComposerAt =
  | { readonly level: "review" }
  | {
      readonly level: "file";
      readonly path: string;
      /** A removed row's number on the side it was removed from. */
      readonly oldLine?: number | undefined;
    }
  | {
      readonly level: "line";
      readonly path: string;
      readonly line: number;
    }
  | {
      readonly level: "passage";
      readonly path: string;
      readonly first: number;
      readonly last: number;
    };

/** A key no other anchor shares, for the composer open there. */
export function composerKey(at: ComposerAt): string {
  switch (at.level) {
    case "review":
      return "review";
    case "file":
      return at.oldLine === undefined
        ? `file:${at.path}`
        : `file:${at.path}:o${at.oldLine}`;
    case "line":
      return `line:${at.path}:${at.line}`;
    default:
      return `passage:${at.path}:${at.first}-${at.last}`;
  }
}

/** Whether two anchors are the same place. */
export function sameAnchor(a: ComposerAt, b: ComposerAt): boolean {
  return composerKey(a) === composerKey(b);
}

/**
 * The element id a composer opens in, from the id of the card it is under. A
 * card's id is "file-" and a hash of its path, which the server also writes
 * into every thread and list id, so the two agree without the app hashing.
 */
export function composerId(at: ComposerAt, cardId: string): string {
  const key = cardId.replace(/^file-/, "");
  switch (at.level) {
    case "review":
      return "comments-review";
    case "file":
      return at.oldLine === undefined
        ? `slot-${key}`
        : `thread-${key}-o${at.oldLine}`;
    case "line":
      return `thread-${key}-${at.line}`;
    default:
      return `thread-${key}-p${at.first}-${at.last}`;
  }
}

/** One level a comment's text can be sent to. */
export interface Chip {
  readonly level: ComposerAt["level"];
  readonly label: string;
  readonly pressed: boolean;
}

/** What a composer says, which depends on where it opens and the round. */
export interface ComposerText {
  /** The line naming what the comment attaches to, above the field. */
  readonly anchor: string;
  readonly placeholder: string;
  /** The levels the text can move between, none for the review's own. */
  readonly chips: readonly Chip[];
  /** The line beside the button. */
  readonly hint: string;
  readonly button: string;
}

/**
 * The words of a composer. A decided round sends a comment at once. The chips
 * switch what the text is sent as without moving the composer, so the words
 * above them stay those of the anchor and only the pressed chip follows the
 * level chosen.
 */
export function composerText(
  at: ComposerAt,
  late: boolean,
  chosen: ComposerAt["level"] = at.level,
): ComposerText {
  const hint = late ? "Sends immediately" : "Saved as you type";
  const button = late ? "Send" : "Add";
  const text = anchorText(at);
  return {
    ...text,
    chips: text.chips.map((chip) => ({
      ...chip,
      pressed: chip.level === chosen,
    })),
    hint,
    button,
  };
}

const fileChip: Chip = { level: "file", label: "File", pressed: false };
const reviewChip: Chip = { level: "review", label: "Review", pressed: false };

/** What an anchor says, before the round and the chosen level are known. */
function anchorText(
  at: ComposerAt,
): Pick<ComposerText, "anchor" | "placeholder" | "chips"> {
  switch (at.level) {
    case "review":
      return {
        anchor: "This review",
        placeholder: "Comment on the whole review…",
        chips: [],
      };
    case "file":
      return {
        anchor:
          at.oldLine === undefined
            ? at.path
            : `${at.path}, removed line ${at.oldLine}`,
        placeholder: "Comment on this file…",
        chips: [fileChip, reviewChip],
      };
    case "line":
      return {
        anchor: `${at.path}:${at.line}`,
        placeholder: `Comment on line ${at.line}…`,
        chips: [
          { level: "line", label: `Line ${at.line}`, pressed: false },
          fileChip,
          reviewChip,
        ],
      };
    default:
      return {
        anchor: `${at.path}:${at.first}–${at.last}`,
        placeholder: "Comment on this passage…",
        chips: [
          { level: "passage", label: "Passage", pressed: false },
          fileChip,
          reviewChip,
        ],
      };
  }
}

/** What a request carries that the anchor does not say. */
export interface CommentSend {
  /** The number of the round the reader was looking at. */
  readonly round: number;
  /** The page's comparison, which a removed row's quote needs. */
  readonly against: "base" | "";
  readonly body: string;
  /** The key a retry sends again, so the comment is stored once. */
  readonly submission: string;
}

/**
 * The request a composer sends for its anchor, sent as a level the chips chose.
 * It carries what the template's hidden fields did and no more: a level takes
 * only the fields its form left enabled, so a line switched to the file sends
 * the path alone, and the comparison goes with a removed row's old line, which
 * is the one place a quote reads from the page's comparison. A line sends its
 * own number as the end of its range.
 */
export function commentRequest(
  at: ComposerAt,
  send: CommentSend,
  level: ComposerAt["level"] = at.level,
): CommentRequest {
  const request: CommentRequest = {
    round: send.round,
    level,
    path: "",
    start: 0,
    end: 0,
    old_line: 0,
    against: "",
    body: send.body,
    submission: send.submission,
  };
  if (level === "review" || at.level === "review") {
    return { ...request, level: "review" };
  }
  if (level === "file") {
    const removed = at.level === "file" ? at.oldLine : undefined;
    return {
      ...request,
      path: at.path,
      ...(removed === undefined
        ? {}
        : { old_line: removed, against: send.against }),
    };
  }
  if (at.level === "line") {
    return { ...request, path: at.path, start: at.line, end: at.line };
  }
  if (at.level === "passage") {
    return { ...request, path: at.path, start: at.first, end: at.last };
  }
  return request;
}
