import { useEffect, useState } from "react";
import {
  allowPush,
  installed,
  pushState,
  resubscribePush,
  type PushState,
} from "../../ts/push";
import type { Setup } from "../components/QueueSetup";

/** What this device has to do for notifications, and the taps that do it. */
export interface Push {
  readonly setup: Setup;
  readonly allow: () => void;
  readonly resubscribe: () => void;
}

function setupOf(state: PushState | null): Setup {
  if (state === null || state === "active") {
    return "ready";
  }
  if (state === "lapsed") {
    return "lapsed";
  }
  return installed() ? "not-allowed" : "not-installed";
}

/**
 * Keeps this device's push subscription current, as every load of the queue
 * does, and says which setup notice applies. Nothing shows until the device
 * has answered.
 */
export function usePush(): Push {
  const [state, setState] = useState<PushState | null>(null);
  useEffect(() => {
    void pushState().then(setState);
  }, []);
  return {
    setup: setupOf(state),
    allow: () => {
      void allowPush().then((next) => {
        if (next !== null) {
          setState(next);
        }
      });
    },
    resubscribe: () => {
      void resubscribePush().then(setState);
    },
  };
}
