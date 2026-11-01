import { cx } from "./cx";

/**
 * Where the splash eye's lid is in a blink. It closes, stays closed for as long
 * as its owner holds it, and opens. An open eye has no lid state.
 */
export type Lid = "closing" | "closed" | "opening";

interface EyeProps {
  /** How many reviews wait. Zero shuts the eye. */
  readonly waiting: number;
  /** The empty state's size. It is decoration, so it has no label. */
  readonly large?: boolean;
  /**
   * The eye as the bar draws it, scaled up for a screen of its own: the ink
   * outline and the iris disc, with a 0 in it since no count is known. It is
   * decoration, so it has no label, and `waiting` is not read.
   */
  readonly splash?: boolean;
  /** The splash eye's blink. The caller moves it on when an animation ends. */
  readonly lid?: Lid | undefined;
  /** Plays the blink that confirms a verdict, once, as the eye mounts. */
  readonly blink?: boolean | undefined;
  /** The blink's animation began, after its delay. */
  readonly onBlinkStart?: (() => void) | undefined;
  /** The blink's animation ended. */
  readonly onBlinkEnd?: (() => void) | undefined;
}

/** The top bar's eye: open with the waiting count as its iris, or shut. */
export function Eye({
  waiting,
  large,
  splash,
  lid,
  blink,
  onBlinkStart,
  onBlinkEnd,
}: EyeProps) {
  const shut = waiting === 0;
  if (splash) {
    return (
      <span
        className={cx("eye splash", lid)}
        onAnimationStart={onBlinkStart}
        onAnimationEnd={onBlinkEnd}
        aria-hidden="true"
      >
        <span className="iris">0</span>
      </span>
    );
  }
  if (large) {
    return (
      <span className={shut ? "eye lg shut" : "eye lg"} aria-hidden="true" />
    );
  }
  if (shut) {
    return (
      <span
        className={cx("eye shut", blink && "blink")}
        onAnimationStart={onBlinkStart}
        onAnimationEnd={onBlinkEnd}
        role="img"
        aria-label="Nothing waiting"
      />
    );
  }
  return (
    <span
      className={cx("eye", blink && "blink")}
      onAnimationStart={onBlinkStart}
      onAnimationEnd={onBlinkEnd}
      role="img"
      aria-label={`${waiting} waiting`}
    >
      <span className="iris">{waiting}</span>
    </span>
  );
}
