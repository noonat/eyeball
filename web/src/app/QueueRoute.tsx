import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { blinks, takeBlinkMark } from "../../ts/blink";
import { trackHead } from "../../ts/head";
import { setBadge } from "../../ts/push";
import { fetchQueue, queueKey } from "../api/queue";
import type { Queue as QueueView } from "../api/types";
import { Banners } from "../components/Banners";
import { Queue } from "../components/Queue";
import { useQueueStream } from "./useQueueStream";
import { usePush } from "./usePush";

/** Whether the eye blinks for the review a verdict was just sent for. */
function blinkFor(marked: string | null, queue: QueueView): boolean {
  if (marked === null) {
    return false;
  }
  return blinks(marked, [
    ...(queue.waiting ?? []).map((row) => ({ id: row.id, waiting: true })),
    ...(queue.working ?? []).map((row) => ({ id: row.id, waiting: false })),
    ...(queue.settled ?? []).map((row) => ({ id: row.id, waiting: false })),
  ]);
}

/**
 * Keeps the sticky head's height on the root element. It is keyed by what the
 * route draws, so it measures the head that is on screen after the first view
 * replaces the one before it.
 */
function HeadTracker() {
  useEffect(() => {
    trackHead();
  }, []);
  return null;
}

/** The queue, drawn from the API and kept current by the live stream. */
export function QueueRoute() {
  const { data } = useQuery({ queryKey: queueKey, queryFn: fetchQueue });
  const connection = useQueueStream();
  const push = usePush();
  // The mark is read once, on the load the verdict returned to, and the eye
  // blinks for the first view that has the row.
  const [marked] = useState(takeBlinkMark);
  const blink = data !== undefined && blinkFor(marked, data);

  useEffect(() => {
    if (data !== undefined) {
      setBadge(data.waiting_count);
    }
  }, [data]);

  if (data === undefined) {
    // Before the first view the bar has no count to show, and a shut eye would
    // say nothing is waiting.
    return (
      <>
        <div className="head">
          <div className="topbar">
            <span className="wordmark">eyeball</span>
          </div>
          <Banners
            offline={connection.offline}
            updateReady={connection.updateReady}
            onRetry={connection.retry}
          />
        </div>
        <div className="scroll" />
        <HeadTracker key="loading" />
      </>
    );
  }
  return (
    <>
      <Queue
        queue={data}
        setup={push.setup}
        blink={blink}
        offline={connection.offline}
        updateReady={connection.updateReady}
        onRetry={connection.retry}
        onAllow={push.allow}
        onResubscribe={push.resubscribe}
      />
      <HeadTracker key="loaded" />
    </>
  );
}
