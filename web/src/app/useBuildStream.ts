import { useEffect, useRef, useState } from "react";
import { openStream, type Stream } from "../../ts/connection";
import { newerBuild, pageBuild } from "../../ts/update";
import type { Connection } from "./useQueueStream";

/**
 * Holds a screen's live stream for the connection and the build, which is all a
 * screen with no stamp of its own follows. The stream is opened when the screen
 * opens and closed when it goes, and the notice's own delay keeps the gap
 * between two screens from showing.
 */
export function useBuildStream(): Connection {
  const stream = useRef<Stream | null>(null);
  const [offline, setOffline] = useState(false);
  const [newest, setNewest] = useState("");

  useEffect(() => {
    const opened = openStream("/events", {
      stamp: () => undefined,
      build: setNewest,
      connection: setOffline,
    });
    stream.current = opened;
    return () => {
      opened.close();
      stream.current = null;
    };
  }, []);

  return {
    offline,
    updateReady: newerBuild(pageBuild(), newest),
    retry: () => {
      stream.current?.reopen();
    },
  };
}
