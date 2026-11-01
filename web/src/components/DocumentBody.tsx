import type { ReactNode } from "react";
import type { DocPart, DocPassage } from "../api/types";
import { cx } from "./cx";

interface DocumentBodyProps {
  /** A markdown file rendered by the server, as its passages and folds. */
  readonly passages: readonly DocPart[];
  /**
   * What follows a passage as an element of its own, such as the comments on
   * it. A file page has none.
   */
  readonly renderAfter?: ((passage: DocPassage) => ReactNode) | undefined;
}

/**
 * A markdown file as a document. A passage's block element is drawn here, from
 * its range and whether it changed, and only what is inside it is the server's
 * markup. So a thread can sit between two passages as an element of its own,
 * and the stylesheet's rules about a passage's neighbors see the same
 * elements the template page had.
 *
 * This is the one component that inserts markup as it is. The server made it
 * with goldmark, raw HTML off, and marked the words that changed, so it is
 * trusted only for that. Nothing else in the app may set markup:
 * `react/no-danger` is on, and .oxlintrc.json turns it off for this file
 * alone.
 */
export function DocumentBody({ passages, renderAfter }: DocumentBodyProps) {
  return (
    <div className="prose">
      {passages.map((part, i) =>
        part.passage !== null ? (
          <Passage key={i} passage={part.passage} renderAfter={renderAfter} />
        ) : (
          <details key={i} className="fold">
            <summary>{part.fold?.length ?? 0} unchanged passages</summary>
            {(part.fold ?? []).map((passage, j) => (
              <Passage key={j} passage={passage} renderAfter={renderAfter} />
            ))}
          </details>
        ),
      )}
    </div>
  );
}

interface PassageProps {
  readonly passage: DocPassage;
  readonly renderAfter: ((passage: DocPassage) => ReactNode) | undefined;
}

function Passage({ passage, renderAfter }: PassageProps) {
  return (
    <>
      <div
        className={cx("blk", passage.changed && "changed")}
        data-lines={
          passage.first > 0 ? `${passage.first}-${passage.last}` : undefined
        }
        dangerouslySetInnerHTML={{ __html: passage.html }}
      />
      {renderAfter?.(passage)}
    </>
  );
}
