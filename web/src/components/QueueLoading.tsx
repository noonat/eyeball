import { useEffect, useState } from "react";
import { reducedMotion } from "../../ts/blink";
import { Eye, type Lid } from "./Eye";

/** The shortest and longest wait between blinks, in milliseconds. */
const shortest = 1000;
const longest = 5000;

/** The shortest and longest time the eye stays shut, in milliseconds. */
const briefest = 200;
const longestShut = 1000;

/** A time drawn between two bounds, so no two blinks share a rhythm. */
function between(low: number, high: number): number {
  return low + Math.random() * (high - low);
}

/**
 * The queue before its first view arrives: a large open eye in the middle of
 * the screen, blinking now and then, and one line saying what is happening. It
 * has no bar and no notices. A phone cannot tell a daemon that is down from one
 * that is slow, since a tailnet node that is offline drops packets and nothing
 * refuses the connection, so "connecting" is the one thing true of both.
 *
 * With the device asking for no motion the eye stays open.
 */
export function QueueLoading() {
  const [lid, setLid] = useState<Lid | undefined>(undefined);
  const [still] = useState(reducedMotion);

  // A blink is a close, a hold and an open. The close and the open are CSS
  // animations, and the hold is a timer, so the hold can be any length without
  // the curve of either one changing. Each wait starts when the previous blink
  // ends and its length is drawn then, so no two are planned together.
  useEffect(() => {
    if (still) {
      return undefined;
    }
    const step = (next: Lid, after: number): (() => void) => {
      const timer = window.setTimeout(() => {
        setLid(next);
      }, after);
      return () => {
        window.clearTimeout(timer);
      };
    };
    if (lid === undefined) {
      return step("closing", between(shortest, longest));
    }
    if (lid === "closed") {
      return step("opening", between(briefest, longestShut));
    }
    return undefined;
  }, [still, lid]);

  // The three layers of the eye each end their animation, so the first of
  // them moves the lid on and the rest find it already moved.
  const settle = (): void => {
    setLid((now) => {
      if (now === "closing") {
        return "closed";
      }
      return now === "opening" ? undefined : now;
    });
  };

  return (
    <div className="connecting" role="status">
      <Eye waiting={0} splash lid={lid} onBlinkEnd={settle} />
      <p>Eyeball is connecting…</p>
    </div>
  );
}
