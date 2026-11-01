import type { Files } from "../api/types";
import { FileScreen } from "./FileScreen";
import { TreeScreen } from "./TreeScreen";

interface FilesScreenProps {
  /** The view of a review's file address, which is a tree or a file. */
  readonly files: Files;
  readonly offline?: boolean | undefined;
  readonly updateReady?: boolean | undefined;
  readonly onRetry?: (() => void) | undefined;
}

/** The screen a file address draws: a directory's tree or one file's page. */
export function FilesScreen({ files, ...notices }: FilesScreenProps) {
  const bar = {
    title: files.title,
    back: files.back,
    backLabel: files.back_label,
    waiting: files.waiting_count,
    ...notices,
  };
  if (files.kind === "tree" && files.tree !== null) {
    return <TreeScreen {...bar} tree={files.tree} />;
  }
  if (files.kind === "file" && files.file !== null) {
    return <FileScreen {...bar} file={files.file} />;
  }
  return null;
}
