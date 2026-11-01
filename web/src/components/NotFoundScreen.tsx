import type { NotFound } from "../api/types";
import { EmptyBox } from "./EmptyBox";
import { Screen } from "./Screen";
import { ViewNotice } from "./ViewNotice";

interface NotFoundScreenProps {
  /** The body the API sent with its 404. */
  readonly missing: NotFound;
  readonly offline?: boolean | undefined;
  readonly updateReady?: boolean | undefined;
  readonly onRetry?: (() => void) | undefined;
}

/**
 * An address with nothing at it. The view carries no title or way back, so the
 * bar is the one the template's error page had: "Not found", back to the
 * queue. A refused live read says why in a banner above the box.
 */
export function NotFoundScreen({ missing, ...notices }: NotFoundScreenProps) {
  return (
    <Screen
      title="Not found"
      back="/"
      backLabel="Queue"
      waiting={missing.waiting_count}
      {...notices}
    >
      {missing.notice === null ? null : <ViewNotice notice={missing.notice} />}
      <EmptyBox
        as="p"
        headline="Nothing here"
        text="There is no page at this address."
      />
    </Screen>
  );
}
