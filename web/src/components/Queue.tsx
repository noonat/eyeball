import type { Queue as QueueView } from "../api/types";
import { Banners } from "./Banners";
import { EmptyState } from "./EmptyState";
import { QueueSection } from "./QueueSection";
import { QueueSetup, type Setup } from "./QueueSetup";
import { TopBar } from "./TopBar";

interface QueueProps {
  /** The view the API sent. */
  readonly queue: QueueView;
  /** What this device still has to do for notifications. */
  readonly setup?: Setup | undefined;
  /** Blinks the eye once, as the queue does after a verdict. */
  readonly blink?: boolean | undefined;
  /**
   * What the eye shows when it is not the view's own count: the count from
   * before a verdict, until the lid is shut.
   */
  readonly eyeWaiting?: number | undefined;
  readonly onBlinkStart?: (() => void) | undefined;
  readonly onBlinkEnd?: (() => void) | undefined;
  /** The stream has been down long enough to say so. */
  readonly offline?: boolean | undefined;
  /** The daemon runs a newer build than the one that served this page. */
  readonly updateReady?: boolean | undefined;
  readonly onRetry?: (() => void) | undefined;
  readonly onAllow?: (() => void) | undefined;
  readonly onResubscribe?: (() => void) | undefined;
}

/**
 * The screen the surface opens on: the top bar and the three sections, in the
 * order the API sent each. With nothing waiting, the shut eye stands where the
 * Waiting section would be.
 */
export function Queue({
  queue,
  setup = "ready",
  blink,
  eyeWaiting,
  onBlinkStart,
  onBlinkEnd,
  offline,
  updateReady,
  onRetry,
  onAllow,
  onResubscribe,
}: QueueProps) {
  const waiting = queue.waiting ?? [];
  const working = queue.working ?? [];
  const settled = queue.settled ?? [];
  return (
    <>
      <div className="head">
        <TopBar
          waiting={eyeWaiting ?? queue.waiting_count}
          blink={blink}
          onBlinkStart={onBlinkStart}
          onBlinkEnd={onBlinkEnd}
        />
        <Banners
          offline={offline}
          updateReady={updateReady}
          onRetry={onRetry}
        />
      </div>
      <div className="scroll">
        <div className="page">
          <QueueSetup
            setup={setup}
            onAllow={onAllow}
            onResubscribe={onResubscribe}
          />
          {waiting.length > 0 ? (
            <QueueSection
              title="Waiting on you"
              kind="waiting"
              rows={waiting}
            />
          ) : (
            <EmptyState />
          )}
          {working.length > 0 ? (
            <QueueSection title="Working" kind="working" rows={working} />
          ) : null}
          {settled.length > 0 ? (
            <QueueSection title="Settled" kind="approved" rows={settled} />
          ) : null}
        </div>
      </div>
    </>
  );
}
