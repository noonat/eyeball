interface EmptyBoxProps {
  readonly headline: string;
  readonly text: string;
  /**
   * The element the headline is. The project view and a deleted file use a
   * span, and the tree and the not found page a paragraph, as their template
   * pages did.
   */
  readonly as: "span" | "p";
}

/** The box a screen shows in place of rows it has none of. */
export function EmptyBox({ headline, text, as }: EmptyBoxProps) {
  return (
    <div className="empty">
      {as === "span" ? (
        <span className="headline">{headline}</span>
      ) : (
        <p className="headline">{headline}</p>
      )}
      <p>{text}</p>
    </div>
  );
}
