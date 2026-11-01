import type { DocPart } from "../api/types";

interface DocumentBodyProps {
  /** A markdown file rendered by the server, as its passages and folds. */
  readonly passages: readonly DocPart[];
}

/**
 * A fold's markup, as render writes it: a details element holding a run of
 * unchanged passages, which opens with no script. The count is a number the
 * server counted, so nothing in the line is text from a document.
 */
function foldOpen(count: number): string {
  return `<details class="fold"><summary>${count} unchanged passages</summary>\n`;
}

/** The passages joined into the one piece of markup the template page wrote. */
function joinPassages(passages: readonly DocPart[]): string {
  let out = "";
  for (const part of passages) {
    if (part.passage !== null) {
      out += part.passage.html;
      continue;
    }
    out += foldOpen(part.fold?.length ?? 0);
    for (const passage of part.fold ?? []) {
      out += passage.html;
    }
    out += "</details>\n";
  }
  return out;
}

/**
 * A markdown file as a document. This is the one component that inserts markup
 * as it is. The server made each passage's HTML with goldmark, raw HTML off,
 * and marked the words that changed, so it is trusted only for that. Nothing
 * else in the app may set markup: `react/no-danger` is on, and .oxlintrc.json
 * turns it off for this file alone.
 */
export function DocumentBody({ passages }: DocumentBodyProps) {
  return (
    <div
      className="prose"
      dangerouslySetInnerHTML={{ __html: joinPassages(passages) }}
    />
  );
}
