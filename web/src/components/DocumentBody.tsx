import type { DocumentHTML } from "../api/types";

interface DocumentBodyProps {
  /** A markdown file rendered by the server. */
  readonly html: DocumentHTML;
}

/**
 * A markdown file as a document. This is the one component that inserts markup
 * as it is. The server made the HTML with goldmark, raw HTML off, and marked
 * the words that changed, so it is trusted only for that. Nothing else in the
 * app may set markup. A lint rule that enforces it comes with the shell.
 */
export function DocumentBody({ html }: DocumentBodyProps) {
  return <div className="prose" dangerouslySetInnerHTML={{ __html: html }} />;
}
