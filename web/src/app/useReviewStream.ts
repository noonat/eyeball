import { useEffect, useRef, useState } from "react";
import { openStream, type Stream } from "../../ts/connection";
import { newerBuild, pageBuild } from "../../ts/update";
import type { Connection } from "./useQueueStream";

/** What a review's stream says: the connection, and the stamp it last sent. */
export interface ReviewStream {
  readonly connection: Connection;
  /** The newest stamp the stream sent, and empty before the first. */
  readonly stamp: string;
}

/**
 * Holds a review's live stream, `/events?review=N`. Each stamp it sends names
 * the review's state, and the screen compares it with the stamp of the view it
 * holds. The stream is opened when the screen opens and closed when it goes,
 * and a stream that reconnects sends the current stamp again, so a change missed
 * while it was down is found when it returns.
 */
export function useReviewStream(url: string): ReviewStream {
  const stream = useRef<Stream | null>(null);
  const [offline, setOffline] = useState(false);
  const [newest, setNewest] = useState("");
  const [stamp, setStamp] = useState("");

  useEffect(() => {
    const opened = openStream(url, {
      stamp: setStamp,
      build: setNewest,
      connection: setOffline,
    });
    stream.current = opened;
    return () => {
      opened.close();
      stream.current = null;
    };
  }, [url]);

  return {
    stamp,
    connection: {
      offline,
      updateReady: newerBuild(pageBuild(), newest),
      retry: () => {
        stream.current?.reopen();
      },
    },
  };
}
