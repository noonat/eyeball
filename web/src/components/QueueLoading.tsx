import { Banners } from "./Banners";

interface QueueLoadingProps {
  /** The stream has been down long enough to say so. */
  readonly offline?: boolean | undefined;
  /** The daemon runs a newer build than the one that served this page. */
  readonly updateReady?: boolean | undefined;
  readonly onRetry?: (() => void) | undefined;
}

/**
 * The queue before its first view arrives. The bar has no count to show, and a
 * shut eye would say nothing is waiting, so it holds the wordmark alone. A
 * launch with the daemon down stays here, which is why the notices are drawn.
 */
export function QueueLoading({
  offline,
  updateReady,
  onRetry,
}: QueueLoadingProps) {
  return (
    <>
      <div className="head">
        <div className="topbar">
          <span className="wordmark">eyeball</span>
        </div>
        <Banners
          offline={offline}
          updateReady={updateReady}
          onRetry={onRetry}
        />
      </div>
      <div className="scroll" />
    </>
  );
}
