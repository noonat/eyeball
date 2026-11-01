import type { Connection } from "./useQueueStream";

/** What a connection makes a screen show: its two notices and the retry. */
export function notices(connection: Connection) {
  return {
    offline: connection.offline,
    updateReady: connection.updateReady,
    onRetry: connection.retry,
  };
}
