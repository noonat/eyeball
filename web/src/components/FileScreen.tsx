import type { File, Toolbar } from "../api/types";
import { AppLink } from "./AppLink";
import { CodeTable } from "./CodeTable";
import { Crumb } from "./Crumb";
import { DocumentBody } from "./DocumentBody";
import { EmptyBox } from "./EmptyBox";
import { Icon } from "./Icon";
import { RenderedFile } from "./RenderedFile";
import { Screen } from "./Screen";
import { ViewNotices } from "./ViewNotice";

interface FileScreenProps {
  readonly title: string;
  readonly back: string;
  readonly backLabel: string;
  readonly waiting: number | null;
  /** The file the API sent. */
  readonly file: File;
  readonly offline?: boolean | undefined;
  readonly updateReady?: boolean | undefined;
  readonly onRetry?: (() => void) | undefined;
}

/**
 * One file at one round, or live from a working copy: its path, the banners
 * saying where it comes from, the toolbar, and the file itself in whatever form
 * the server chose.
 */
export function FileScreen({ file, ...bar }: FileScreenProps) {
  // The mode line sits outside the file's box when the file has none of its
  // own: a rendering or a document. Over rows it is the box's first line.
  const modeOutside = file.body === "rendered" || file.body === "document";
  return (
    <Screen {...bar}>
      <Crumb icon="file-text" dir={file.dir} name={file.name} />
      <ViewNotices notices={file.notices} />
      {file.toolbar === null ? null : <FileToolbar toolbar={file.toolbar} />}
      {file.mode !== "" && modeOutside ? (
        <div className="file">
          <ModeLine mode={file.mode} />
        </div>
      ) : null}
      <FileBody file={file} />
    </Screen>
  );
}

function ModeLine({ mode }: { readonly mode: string }) {
  return (
    <div className="modeline">
      <Icon name="triangle-alert" />
      <span>{mode}</span>
    </div>
  );
}

function FileBody({ file }: { readonly file: File }) {
  if (file.body === "rendered") {
    return file.rendered === null ? null : (
      <RenderedFile rendered={file.rendered} />
    );
  }
  if (file.body === "deleted") {
    return <EmptyBox as="span" headline="Deleted" text={file.deleted} />;
  }
  if (file.body === "document") {
    return <DocumentBody html={file.document} />;
  }
  return (
    <div className="file">
      {file.mode === "" ? null : <ModeLine mode={file.mode} />}
      {file.body === "binary" ? (
        <div className="toobig">Binary file, not shown as a diff.</div>
      ) : file.body === "too_large" ? (
        <div className="toobig">Too large to show as a diff.</div>
      ) : (
        <CodeTable rows={file.rows ?? []} />
      )}
    </div>
  );
}

/** The counts, and the control choosing between two views of the file. */
function FileToolbar({ toolbar }: { readonly toolbar: Toolbar }) {
  return (
    <div className="toolbar">
      {toolbar.note !== "" ? (
        <span className="counts">
          <span className="hint">{toolbar.note}</span>
        </span>
      ) : toolbar.counted !== "" ? (
        <span className="counts">
          <span className="plus">+{toolbar.added}</span>{" "}
          <span className="minus">&minus;{toolbar.removed}</span>{" "}
          <span className="hint">{toolbar.counted}</span>
        </span>
      ) : null}
      {toolbar.marked !== "" ? (
        <Segmented
          first={{ label: "Marked", href: toolbar.marked }}
          second={{ label: "Plain", href: toolbar.plain }}
          secondShown={toolbar.show_plain}
        />
      ) : toolbar.rendered !== "" ? (
        <Segmented
          first={{ label: toolbar.rendered_label, href: toolbar.rendered }}
          second={{ label: "Source", href: toolbar.source }}
          secondShown={toolbar.show_source}
        />
      ) : null}
    </div>
  );
}

interface Choice {
  readonly label: string;
  readonly href: string;
}

interface SegmentedProps {
  readonly first: Choice;
  readonly second: Choice;
  /** The second view is the one shown. */
  readonly secondShown: boolean;
}

/** Two views of one file, the one shown marked. */
function Segmented({ first, second, secondShown }: SegmentedProps) {
  return (
    <span className="segmented">
      <AppLink href={first.href} current={!secondShown}>
        {first.label}
      </AppLink>
      <AppLink href={second.href} current={secondShown}>
        {second.label}
      </AppLink>
    </span>
  );
}
