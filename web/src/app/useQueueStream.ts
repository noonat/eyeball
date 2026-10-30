import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { newerBuild, pageBuild } from "../../ts/update";
import { openStream, type Stream } from "../../ts/connection";
import { isQueue, queueKey } from "../api/queue";

/** What the stream says about the daemon, for the notices to draw. */
export interface Connection {
  readonly offline: boolean;
  readonly updateReady: boolean;
  readonly retry: () => void;
}

/** The stamp of the view the cache holds, empty before the first. */
function heldStamp(client: QueryClient): string {
  const held: unknown = client.getQueryData(queueKey);
  return isQueue(held) ? held.stamp : "";
}

/**
 * Holds the queue's live stream. Each stamp it sends is compared with the
 * stamp of the view the cache holds, and a difference refetches the view. A
 * stream that reconnects sends the current stamp again, so a change missed
 * while it was down is fetched when it returns.
 *
 * A fetch that was already under way when a stamp arrived may have read the
 * store before the change, and its answer clears the invalidation the stamp
 * made. So each time a fetch of the view succeeds, whatever it returned, the
 * view is compared with the newest stamp the stream has sent, and fetched once
 * more if they differ. That is one catch-up for each stamp the stream sends, so
 * a view that never reaches the stream's stamp is fetched once and not for
 * ever.
 */
export function useQueueStream(): Connection {
  const client = useQueryClient();
  const stream = useRef<Stream | null>(null);
  const [offline, setOffline] = useState(false);
  const [newest, setNewest] = useState("");

  useEffect(() => {
    let latest = "";
    let caughtUp = "";
    const refetch = (): void => {
      // A fetch in flight is left to finish. One that read the store before
      // the change is covered by the catch-up when it succeeds.
      void client.invalidateQueries(
        { queryKey: queueKey },
        { cancelRefetch: false },
      );
    };
    const opened = openStream("/events?queue=1", {
      stamp: (stamp) => {
        latest = stamp;
        if (stamp !== heldStamp(client)) {
          refetch();
        }
      },
      build: setNewest,
      connection: setOffline,
    });
    stream.current = opened;
    const unsubscribe = client.getQueryCache().subscribe((event) => {
      if (
        event.type === "updated" &&
        event.action.type === "success" &&
        event.query.queryKey[0] === queueKey[0] &&
        latest !== "" &&
        latest !== heldStamp(client) &&
        caughtUp !== latest
      ) {
        caughtUp = latest;
        void client.invalidateQueries({ queryKey: queueKey });
      }
    });
    return () => {
      unsubscribe();
      opened.close();
      stream.current = null;
    };
  }, [client]);

  return {
    offline,
    updateReady: newerBuild(pageBuild(), newest),
    retry: () => {
      stream.current?.reopen();
    },
  };
}
