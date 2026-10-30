import type { Row } from "../api/types";
import { QueueRow, type RowKind } from "./QueueRow";

interface QueueSectionProps {
  readonly title: string;
  readonly kind: RowKind;
  /** The rows in the order the API sent them. */
  readonly rows: readonly Row[];
}

/** A titled group of queue rows, with how many it holds. */
export function QueueSection({ title, kind, rows }: QueueSectionProps) {
  return (
    <div className="section">
      <h2>
        {title} <span className="count">{rows.length}</span>
      </h2>
      <div className="rows">
        {rows.map((row) => (
          <QueueRow key={row.id} row={row} kind={kind} />
        ))}
      </div>
    </div>
  );
}
