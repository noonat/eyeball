import { Eye } from "./Eye";
import { Icon } from "./Icon";

interface Back {
  readonly label: string;
  readonly href: string;
}

interface TopBarProps {
  /**
   * How many reviews wait, shown by the eye. Null where the count is not known,
   * and the bar then draws no eye, since a shut one would say nothing waits.
   */
  readonly waiting: number | null;
  /** A screen below the queue names its title and the way back. */
  readonly back?: Back;
  readonly title?: string;
  /** Blinks the eye once, as a screen does after a verdict. */
  readonly blink?: boolean | undefined;
  readonly onBlinkStart?: (() => void) | undefined;
  readonly onBlinkEnd?: (() => void) | undefined;
}

/** The bar every screen starts with: the wordmark or a way back, and the eye. */
export function TopBar({
  waiting,
  back,
  title,
  blink,
  onBlinkStart,
  onBlinkEnd,
}: TopBarProps) {
  return (
    <div className="topbar">
      {back === undefined ? (
        <span className="wordmark">eyeball</span>
      ) : (
        <>
          <a className="back" href={back.href}>
            <Icon name="chevron-left" />
            {back.label}
          </a>
          <h1>{title}</h1>
        </>
      )}
      {waiting === null ? null : (
        <Eye
          waiting={waiting}
          blink={blink}
          onBlinkStart={onBlinkStart}
          onBlinkEnd={onBlinkEnd}
        />
      )}
    </div>
  );
}
