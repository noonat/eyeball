import { useEffect } from "react";
import { trackHead } from "../../ts/head";

/**
 * Keeps the sticky head's height on the root element. It is drawn with the
 * first view, since the state before it has no head to measure.
 */
export function HeadTracker() {
  useEffect(() => {
    trackHead();
  }, []);
  return null;
}
