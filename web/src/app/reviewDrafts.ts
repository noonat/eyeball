import { forget, recall, remember } from "../../ts/storage";
import type { CommentRequest, CommentLevel } from "../api/types";
import { composerId, type ComposerAt } from "../components/composerAt";

/**
 * What a draft keeps for one round of one review, in the shape the template
 * page saved it. A page loaded from the daemon is the template's until the
 * review moves to the app for good, and both read and write the same device's
 * storage, so a draft written by one is restored by the other. The shape is
 * the template's, key for key: a card the app saved must be one the template
 * can put back.
 */
export interface Drafts {
  /** The verdict's note. */
  readonly note?: string;
  /** The review's own box. */
  readonly review?: string;
  /** The anchored composer, with where it was opened. */
  readonly anchored?: {
    readonly query: Readonly<Record<string, string>>;
    readonly text: string;
  };
  /** Each open edit form, by its comment's id. */
  readonly edits?: Readonly<Record<string, string>>;
  /** Every comment the server did not take. */
  readonly unsent?: readonly SavedCard[];
}

/**
 * An unsent card as it is saved: the form it came from, so a retry sends the
 * same comment with the same key to the same place. The action and the target
 * are the template's, the form route and the list's selector. The app sends to
 * the JSON route and finds the place from the values, and keeps the other two
 * so the template can retry what the app saved.
 */
export interface SavedCard {
  readonly action: string;
  readonly target: string;
  readonly values: Readonly<Record<string, string>>;
  readonly where: string;
}

/** Where a round's draft is kept. */
export function draftKey(review: string, round: number): string {
  return `drafts:${review}:${String(round)}`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

/** The entries of a value that are strings, or none if it is not a record. */
function strings(value: unknown): Record<string, string> {
  const out: Record<string, string> = {};
  if (isRecord(value)) {
    for (const [key, entry] of Object.entries(value)) {
      if (typeof entry === "string") {
        out[key] = entry;
      }
    }
  }
  return out;
}

function savedCard(value: unknown): SavedCard | undefined {
  if (!isRecord(value)) {
    return undefined;
  }
  const { action, target, where } = value;
  if (
    typeof action !== "string" ||
    typeof target !== "string" ||
    typeof where !== "string"
  ) {
    return undefined;
  }
  return { action, target, where, values: strings(value["values"]) };
}

/**
 * What a stored value holds, keeping only the parts that have the shape. A
 * draft is read from storage, which another version of the page may have
 * written, so each part is checked and one that is wrong is dropped alone.
 */
export function readDrafts(saved: unknown): Drafts {
  if (!isRecord(saved)) {
    return {};
  }
  const out: {
    -readonly [K in keyof Drafts]: Drafts[K];
  } = {};
  if (typeof saved["note"] === "string") {
    out.note = saved["note"];
  }
  if (typeof saved["review"] === "string") {
    out.review = saved["review"];
  }
  const anchored = saved["anchored"];
  if (isRecord(anchored) && typeof anchored["text"] === "string") {
    out.anchored = {
      query: strings(anchored["query"]),
      text: anchored["text"],
    };
  }
  if (isRecord(saved["edits"])) {
    out.edits = strings(saved["edits"]);
  }
  if (Array.isArray(saved["unsent"])) {
    const cards: unknown[] = saved["unsent"];
    out.unsent = cards.flatMap((card) => savedCard(card) ?? []);
  }
  return out;
}

/** A round's saved draft, or an empty one. */
export function recallDrafts(review: string, round: number): Drafts {
  return readDrafts(recall(draftKey(review, round)));
}

/** Saves a draft, or forgets the round's when it holds nothing. */
export function storeDrafts(review: string, round: number, d: Drafts): void {
  const key = draftKey(review, round);
  if (Object.keys(d).length === 0) {
    forget(key);
  } else {
    remember(key, d);
  }
}

/**
 * The query a tap opened a composer with, as the template wrote it: the path,
 * the level and the lines, and for a removed row its number and the page's
 * comparison, which is the one place the comparison is kept.
 */
export function queryOf(
  at: ComposerAt,
  against: string,
): Record<string, string> {
  switch (at.level) {
    case "review":
      return { level: "review" };
    case "file":
      return at.oldLine === undefined
        ? { level: "file", path: at.path }
        : {
            level: "file",
            path: at.path,
            oldline: String(at.oldLine),
            against,
          };
    case "line":
      return { level: "line", path: at.path, start: String(at.line) };
    default:
      return {
        level: "passage",
        path: at.path,
        start: String(at.first),
        end: String(at.last),
      };
  }
}

function line(value: string | undefined): number | undefined {
  if (value === undefined || !/^[0-9]+$/.test(value)) {
    return undefined;
  }
  const n = Number(value);
  return n > 0 ? n : undefined;
}

/**
 * The anchor a query names, or nothing where it names none. One reading serves
 * a draft reopened and the composer answers the template routes gave, so a
 * query the template wrote is read the way the app writes one.
 */
export function anchorOf(
  query: Readonly<Record<string, string>>,
): ComposerAt | undefined {
  const path = query["path"];
  switch (query["level"]) {
    case "review":
      return { level: "review" };
    case "file": {
      if (path === undefined || path === "") {
        return undefined;
      }
      const old = line(query["oldline"]);
      return old === undefined
        ? { level: "file", path }
        : { level: "file", path, oldLine: old };
    }
    case "line": {
      const at = line(query["start"]);
      return path === undefined || path === "" || at === undefined
        ? undefined
        : { level: "line", path, line: at };
    }
    case "passage": {
      const first = line(query["start"]);
      const last = line(query["end"]);
      return path === undefined ||
        path === "" ||
        first === undefined ||
        last === undefined
        ? undefined
        : { level: "passage", path, first, last };
    }
    default:
      return undefined;
  }
}

/**
 * The selector of the list a comment is drawn in, which the template's cards
 * were saved with as their target. A file's comments are in its own list, the
 * element a comment's id names less its prefix, and every other anchor's are in
 * the element its composer opens in.
 */
export function targetOf(at: ComposerAt, cardId: string): string {
  if (at.level === "review") {
    return "#comments-review";
  }
  if (at.level === "file") {
    return `#comments-${cardId.replace(/^file-/, "")}`;
  }
  return `#${composerId(at, cardId)}`;
}

/** The form a comment was sent as, which is what the template saved and sent. */
export function valuesOf(request: CommentRequest): Record<string, string> {
  const values: Record<string, string> = {
    round: String(request.round),
    level: request.level,
    body: request.body,
    submission: request.submission,
  };
  if (request.path !== "") {
    values["path"] = request.path;
  }
  if (request.start > 0) {
    values["start"] = String(request.start);
  }
  if (request.end > 0) {
    values["end"] = String(request.end);
  }
  if (request.old_line > 0) {
    values["oldline"] = String(request.old_line);
  }
  if (request.against !== "") {
    values["against"] = request.against;
  }
  return values;
}

function isLevel(value: string | undefined): value is CommentLevel {
  return (
    value === "review" ||
    value === "file" ||
    value === "line" ||
    value === "passage"
  );
}

/**
 * The request a saved card's values make, or nothing where they cannot be
 * sent. The key is the card's own, so a retry of a comment that was stored
 * after all is stored once.
 */
export function requestOf(
  values: Readonly<Record<string, string>>,
): CommentRequest | undefined {
  const level = values["level"];
  const round = line(values["round"]);
  if (!isLevel(level) || round === undefined) {
    return undefined;
  }
  return {
    round,
    level,
    path: values["path"] ?? "",
    start: line(values["start"]) ?? 0,
    end: line(values["end"]) ?? 0,
    old_line: line(values["oldline"]) ?? 0,
    against: values["against"] ?? "",
    body: values["body"] ?? "",
    submission: values["submission"] ?? "",
  };
}

/** The anchor a comment sent as a request is drawn at, where it is stored. */
export function requestAt(request: CommentRequest): ComposerAt {
  switch (request.level) {
    case "review":
      return { level: "review" };
    case "file":
      return { level: "file", path: request.path };
    case "line":
      return { level: "line", path: request.path, line: request.start };
    default:
      return {
        level: "passage",
        path: request.path,
        first: request.start,
        last: request.end,
      };
  }
}
