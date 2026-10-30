import { cx } from "./cx";

interface EyeProps {
  /** How many reviews wait. Zero shuts the eye. */
  readonly waiting: number;
  /** The empty state's size. It is decoration, so it has no label. */
  readonly large?: boolean;
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
  blink,
  onBlinkStart,
  onBlinkEnd,
}: EyeProps) {
  const shut = waiting === 0;
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
