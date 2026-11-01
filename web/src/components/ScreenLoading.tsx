import { QueueLoading } from "./QueueLoading";
import { TopBar } from "./TopBar";

/**
 * A screen whose view has not arrived, on a move inside the app. The bar is
 * there at once, but its title and back link are part of the view, so it shows
 * the wordmark the queue's bar has and no eye: a guessed label would change
 * when the view came, and a shut eye would say nothing waits. Below it, the
 * same connecting eye the queue shows on a cold launch, since the cause is the
 * same: a slow daemon and a down one look alike from here.
 */
export function ScreenLoading() {
  return (
    <>
      <div className="head">
        <TopBar waiting={null} />
      </div>
      <QueueLoading />
    </>
  );
}
