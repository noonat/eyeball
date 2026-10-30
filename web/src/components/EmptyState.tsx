import { Eye } from "./Eye";

/** The queue when nothing waits: a large shut eye and one line. */
export function EmptyState() {
  return (
    <div className="empty">
      <Eye waiting={0} large />
      <span className="headline">Nothing waiting</span>
    </div>
  );
}
