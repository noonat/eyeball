interface CountsProps {
  /** The lines added and removed, written out, and empty for none. */
  readonly added: string;
  readonly removed: string;
}

/** A row's changed-line counts, or nothing where neither side moved. */
export function Counts({ added, removed }: CountsProps) {
  if (added === "" && removed === "") {
    return null;
  }
  return (
    <span className="counts">
      {added === "" ? null : <span className="plus">+{added}</span>}
      {added !== "" && removed !== "" ? " " : null}
      {removed === "" ? null : <span className="minus">&minus;{removed}</span>}
    </span>
  );
}
