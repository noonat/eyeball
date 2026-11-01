import type { Tree } from "../api/types";
import { Counts } from "./Counts";
import { Crumb } from "./Crumb";
import { cx } from "./cx";
import { EmptyBox } from "./EmptyBox";
import { Icon } from "./Icon";
import { Screen } from "./Screen";
import { ViewNotices } from "./ViewNotice";

interface TreeScreenProps {
  readonly title: string;
  readonly back: string;
  readonly backLabel: string;
  readonly waiting: number | null;
  /** The directory the API sent. */
  readonly tree: Tree;
  readonly offline?: boolean | undefined;
  readonly updateReady?: boolean | undefined;
  readonly onRetry?: (() => void) | undefined;
}

/**
 * A directory at one round. Directories come first, a subdirectory holding a
 * file the review changes is open in place, and the files the review changes
 * are bold with their counts.
 */
export function TreeScreen({ tree, ...bar }: TreeScreenProps) {
  const rows = tree.rows ?? [];
  return (
    <Screen {...bar}>
      <Crumb icon="folder" dir={tree.dir} name={tree.name} />
      <ViewNotices notices={tree.notices} />
      {rows.length > 0 ? (
        <div className="tree">
          {rows.map((row) => (
            <a
              className={cx(
                "treerow",
                row.indent && "indent",
                row.in_scope && "inscope",
              )}
              href={row.href}
              key={row.href}
            >
              {row.dir ? (
                <>
                  <Icon name="folder" />
                  <span className="nm">{row.name}</span>
                  <Icon name="chevron-right" />
                </>
              ) : (
                <>
                  <Icon name="file-text" />
                  <span className="nm">{row.name}</span>
                  <Counts added={row.added} removed={row.removed} />
                </>
              )}
            </a>
          ))}
        </div>
      ) : (
        <EmptyBox as="p" headline="Nothing here" text={tree.empty} />
      )}
    </Screen>
  );
}
