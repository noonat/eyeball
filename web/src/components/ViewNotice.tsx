import type { Notice as NoticeView } from "../api/types";
import { Notice } from "./Notice";

interface ViewNoticeProps {
  readonly notice: NoticeView;
}

/** A banner the server wrote: a bold opening where it has one, then its text. */
export function ViewNotice({ notice }: ViewNoticeProps) {
  return (
    <Notice tone={notice.warn ? "warn" : "neutral"}>
      {notice.heading === "" ? null : (
        <>
          <strong>{notice.heading}</strong>{" "}
        </>
      )}
      {notice.text}
    </Notice>
  );
}

interface ViewNoticesProps {
  /** The banners in the order the server sent them, or null for none. */
  readonly notices: readonly NoticeView[] | null;
}

/** Banners in order. */
export function ViewNotices({ notices }: ViewNoticesProps) {
  return (
    <>
      {(notices ?? []).map((notice, i) => (
        <ViewNotice key={i} notice={notice} />
      ))}
    </>
  );
}
