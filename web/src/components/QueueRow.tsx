import type { Row } from "../api/types";
import { Icon } from "./Icon";
import { Pill } from "./Pill";

/** Which section a row is in, which decides what it prints. */
export type RowKind = "waiting" | "working" | "approved";

interface QueueRowProps {
  readonly row: Row;
  readonly kind: RowKind;
}

/** One review in the queue. The whole row is one link to the review. */
export function QueueRow({ row, kind }: QueueRowProps) {
  return (
    // A review is still a template page, so a plain anchor makes the full page
    // load that needs. The router intercepts only its own Link.
    <a className={`row ${kind}`} href={`/reviews/${row.id}`}>
      <span className="where">
        {row.project === "" ? null : (
          <span className="bit">
            <Icon name="folder" />
            <span className="project">{row.project}</span>
          </span>
        )}
        <span className="bit">
          <Icon name="bot" />
          <span className="agent">{row.agent}</span>
        </span>
      </span>
      <span className="age">{row.age}</span>
      <span className="title">
        {kind === "waiting" ? <mark>{row.title}</mark> : row.title}
      </span>
      {kind === "waiting" && row.gist !== "" ? (
        <span className="gist">{row.gist}</span>
      ) : null}
      <span className="meta">
        <Meta row={row} kind={kind} />
      </span>
    </a>
  );
}

function Meta({ row, kind }: QueueRowProps) {
  if (kind === "working") {
    return <span>Changes requested, round {row.round}</span>;
  }
  if (kind === "approved") {
    return (
      <>
        <Pill tone="approved">Approved</Pill>
        <span>Round {row.round}</span>
      </>
    );
  }
  return (
    <>
      <span>Round {row.round}</span>
      <span>{row.files}</span>
      <span className="counts">
        <span className="plus">+{row.added}</span>{" "}
        <span className="minus">&minus;{row.removed}</span>
      </span>
    </>
  );
}
