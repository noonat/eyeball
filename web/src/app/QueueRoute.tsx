import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import {
  blinkGiveUp,
  blinks,
  lidShutAfter,
  reducedMotion,
  takeBlinkMark,
} from "../../ts/blink";
import { trackHead } from "../../ts/head";
import { setBadge } from "../../ts/push";
import { fetchQueue, queueKey } from "../api/queue";
import type { Queue as QueueView } from "../api/types";
import { Queue } from "../components/Queue";
import { QueueLoading } from "../components/QueueLoading";
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
 * Keeps the sticky head's height on the root element. It is drawn with the
 * first view, since the state before it has no head to measure.
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
  const [mark] = useState(takeBlinkMark);
  const blink = data !== undefined && blinkFor(mark?.review ?? null, data);
  // The eye draws the count from before the verdict and changes to the view's
  // while the lid is shut. With no motion there is no lid, so the new count is
  // shown at once.
  const [still] = useState(reducedMotion);
  const old = blink && !still ? (mark?.count ?? null) : null;
  const [shut, setShut] = useState(false);
  const closing = useRef<number | undefined>(undefined);

  // The animation drives the change: the lid is shut lidShutAfter after the
  // blink starts, and the end of the blink is the backstop. A fixed delay from
  // the first paint would land at the wrong moment on a slow device, since the
  // blink starts when the browser gets to it. The timer below is for a browser
  // whose animation never starts, and shows the new count late rather than
  // never.
  const startBlink = (): void => {
    window.clearTimeout(closing.current);
    closing.current = window.setTimeout(() => {
      setShut(true);
    }, lidShutAfter);
  };
  const endBlink = (): void => {
    window.clearTimeout(closing.current);
    setShut(true);
  };
  useEffect(() => {
    if (old === null) {
      return undefined;
    }
    const giveUp = window.setTimeout(() => {
      setShut(true);
    }, blinkGiveUp);
    return () => {
      window.clearTimeout(giveUp);
      window.clearTimeout(closing.current);
    };
  }, [old]);

  useEffect(() => {
    if (data !== undefined) {
      setBadge(data.waiting_count);
    }
  }, [data]);

  if (data === undefined) {
    return <QueueLoading />;
  }
  return (
    <>
      <Queue
        queue={data}
        setup={push.setup}
        blink={blink}
        eyeWaiting={old !== null && !shut ? old : undefined}
        onBlinkStart={startBlink}
        onBlinkEnd={endBlink}
        offline={connection.offline}
        updateReady={connection.updateReady}
        onRetry={connection.retry}
        onAllow={push.allow}
        onResubscribe={push.resubscribe}
      />
      <HeadTracker />
    </>
  );
}
