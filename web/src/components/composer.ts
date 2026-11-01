/**
 * Where a composer opens. A composer is keyed by its anchor, so moving to
 * another line or another file is a new composer with an empty field, and the
 * text typed under the old one is not carried over by accident.
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

/** The words of a composer. A decided round sends a comment at once. */
export function composerText(at: ComposerAt, late: boolean): ComposerText {
  const hint = late ? "Sends immediately" : "Saved as you type";
  const button = late ? "Send" : "Add";
  const file: Chip = { level: "file", label: "File", pressed: false };
  const review: Chip = { level: "review", label: "Review", pressed: false };
  switch (at.level) {
    case "review":
      return {
        anchor: "This review",
        placeholder: "Comment on the whole review…",
        chips: [],
        hint,
        button,
      };
    case "file":
      return {
        anchor:
          at.oldLine === undefined
            ? at.path
            : `${at.path}, removed line ${at.oldLine}`,
        placeholder: "Comment on this file…",
        chips: [{ ...file, pressed: true }, review],
        hint,
        button,
      };
    case "line":
      return {
        anchor: `${at.path}:${at.line}`,
        placeholder: `Comment on line ${at.line}…`,
        chips: [
          {
            level: "line",
            label: `Line ${at.line}`,
            pressed: true,
          },
          file,
          review,
        ],
        hint,
        button,
      };
    default:
      return {
        anchor: `${at.path}:${at.first}–${at.last}`,
        placeholder: "Comment on this passage…",
        chips: [
          { level: "passage", label: "Passage", pressed: true },
          file,
          review,
        ],
        hint,
        button,
      };
  }
}
