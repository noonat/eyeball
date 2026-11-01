import type { Project, WorkingCopy } from "../api/types";
import { Counts } from "./Counts";
import { cx } from "./cx";
import { EmptyBox } from "./EmptyBox";
import { Icon } from "./Icon";
import { Notice } from "./Notice";
import { Pill } from "./Pill";
import { QueueRow } from "./QueueRow";
import { Screen } from "./Screen";
import { ViewNotice, ViewNotices } from "./ViewNotice";

interface ProjectScreenProps {
  /** The view the API sent. */
  readonly project: Project;
  /** The stream has been down long enough to say so. */
  readonly offline?: boolean | undefined;
  /** The daemon runs a newer build than the one that served this page. */
  readonly updateReady?: boolean | undefined;
  readonly onRetry?: (() => void) | undefined;
}

/**
 * One project: its open reviews, which can be acted on, then each working copy
 * as it is on disk now. The working copy is live and read only.
 */
export function ProjectScreen({
  project,
  offline,
  updateReady,
  onRetry,
}: ProjectScreenProps) {
  const copies = project.copies ?? [];
  const first = copies[0];
  return (
    <Screen
      title={project.title}
      back={project.back}
      backLabel={project.back_label}
      waiting={project.waiting_count}
      offline={offline}
      updateReady={updateReady}
      onRetry={onRetry}
    >
      <div className="brief">
        <div className="where">
          <span className="agent">{project.root}</span>
        </div>
        <h2 className="title">{project.name}</h2>
      </div>
      {project.named || first === undefined ? null : (
        <ViewNotices notices={first.notices} />
      )}
      {project.open > 0 ? (
        <div className="section">
          <h2>
            Open reviews <span className="count">{project.open}</span>
          </h2>
          <div className="rows">
            {(project.waiting ?? []).map((row) => (
              <QueueRow key={row.id} row={row} kind="waiting" />
            ))}
            {(project.working ?? []).map((row) => (
              <QueueRow key={row.id} row={row} kind="working" />
            ))}
          </div>
        </div>
      ) : null}
      {project.named ? (
        <>
          {project.live ? (
            <Notice tone="neutral">
              Everything changed on disk right now, live, in each working copy.
              Read only: a verdict belongs to a review.
            </Notice>
          ) : null}
          {copies.map((copy) => (
            <div className="section" key={`${copy.name}:${copy.hint}`}>
              <h2>
                {copy.name}
                <CopyCount count={copy.count} />
              </h2>
              <div className="hint">{copy.hint}</div>
              <ViewNotices notices={copy.notices} />
              <CopyRows copy={copy} />
            </div>
          ))}
        </>
      ) : first === undefined ? null : (
        <div className="section">
          <h2>
            Working copy
            <CopyCount count={first.count} />
          </h2>
          {(first.rows ?? []).length > 0 ? (
            <Notice tone="neutral">
              Everything changed on disk right now, live. Read only: a verdict
              belongs to a review.
            </Notice>
          ) : null}
          <CopyRows copy={first} />
        </div>
      )}
    </Screen>
  );
}

/** How many files a working copy lists, after its name. */
function CopyCount({ count }: { readonly count: string }) {
  return count === "" ? null : (
    <>
      {" "}
      <span className="count">{count}</span>
    </>
  );
}

/** A working copy's files, or the box saying why there are none. */
function CopyRows({ copy }: { readonly copy: WorkingCopy }) {
  const rows = copy.rows ?? [];
  if (rows.length === 0) {
    return copy.empty === null ? null : (
      <EmptyBox
        as="span"
        headline={copy.empty.headline}
        text={copy.empty.text}
      />
    );
  }
  return (
    <>
      {copy.capped === null ? null : <ViewNotice notice={copy.capped} />}
      <div className="tree">
        {rows.map((row) => (
          <a
            className={cx("treerow", !row.claimed && "inscope")}
            href={row.href}
            key={row.href}
          >
            <Icon name="file-text" />
            <span className="nm">{row.path}</span>
            {row.claimed ? (
              <Pill tone="plain">In review</Pill>
            ) : (
              <Counts added={row.added} removed={row.removed} />
            )}
          </a>
        ))}
      </div>
    </>
  );
}
