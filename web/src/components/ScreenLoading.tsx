import { Banners } from "./Banners";
import { QueueLoading } from "./QueueLoading";
import { TopBar } from "./TopBar";

interface ScreenLoadingProps {
  /** The stream has been down long enough to say so. */
  readonly offline?: boolean | undefined;
  /** The daemon runs a newer build than the one that served this page. */
  readonly updateReady?: boolean | undefined;
  readonly onRetry?: (() => void) | undefined;
}

/**
 * A screen whose view has not arrived, on a move inside the app. The bar is
 * there at once, but its title and back link are part of the view, so it shows
 * the wordmark the queue's bar has and no eye: a guessed label would change
 * when the view came, and a shut eye would say nothing waits. Below it, the
 * same connecting eye the queue shows on a cold launch, since the cause is the
 * same: a slow daemon and a down one look alike from here. The notices sit under
 * the bar, so a daemon that is down says so.
 */
export function ScreenLoading({
  offline,
  updateReady,
  onRetry,
}: ScreenLoadingProps) {
  return (
    <>
      <div className="head">
        <TopBar waiting={null} />
        <Banners
          offline={offline}
          updateReady={updateReady}
          onRetry={onRetry}
        />
      </div>
      <QueueLoading />
    </>
  );
}
