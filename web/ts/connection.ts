// The live stream's connection, with no page in it. The React app wires it to
// its state, so the delay before a loss shows, and what counts as a loss, are
// written once.

// noticeDelay is how long a stream may be down before the loss is reported, so
// a phone waking from sleep does not flash it. Two seconds is a guess.
const noticeDelay = 2000;

export interface StreamEvents {
  // stamp is a stamp the stream sent.
  stamp(stamp: string): void;
  // build is the build the daemon runs, sent on every connect, so a daemon
  // restarted on a newer build is heard when the stream reconnects.
  build(build: string): void;
  // connection is called with true once the stream has been down for
  // noticeDelay, or the browser reports itself offline, and with false when
  // the stream opens.
  connection(down: boolean): void;
}

export interface Stream {
  // reopen closes the stream and opens a new one, which is what Retry does.
  reopen(): void;
  close(): void;
}

// openStream opens an EventSource and reports what it hears. EventSource
// reconnects on its own with backoff, so an error is a loss and not an end.
export function openStream(url: string, on: StreamEvents): Stream {
  let source: EventSource | null = null;
  let timer: number | undefined;

  const open = (): void => {
    source?.close();
    source = new EventSource(url);
    source.addEventListener("build", (event) => {
      if (event instanceof MessageEvent && typeof event.data === "string") {
        on.build(event.data);
      }
    });
    source.addEventListener("open", () => {
      window.clearTimeout(timer);
      timer = undefined;
      on.connection(false);
    });
    source.addEventListener("error", () => {
      timer ??= window.setTimeout(() => {
        on.connection(true);
      }, noticeDelay);
    });
    source.addEventListener("message", (event) => {
      if (typeof event.data === "string") {
        on.stamp(event.data);
      }
    });
  };
  // navigator.onLine is trusted only when it says false.
  const offline = (): void => {
    on.connection(true);
  };
  window.addEventListener("offline", offline);
  open();
  return {
    reopen: open,
    close: () => {
      window.removeEventListener("offline", offline);
      window.clearTimeout(timer);
      timer = undefined;
      source?.close();
      source = null;
    },
  };
}
