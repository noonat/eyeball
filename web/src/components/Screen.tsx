import type { ReactNode } from "react";
import { Banners } from "./Banners";
import { TopBar } from "./TopBar";

interface ScreenProps {
  /** The view's title, which the bar's heading and the document share. */
  readonly title: string;
  /** Where the bar's back link goes, and what it says. */
  readonly back: string;
  readonly backLabel: string;
  /** The view's waiting count, or null where it could not be read. */
  readonly waiting: number | null;
  /** The stream has been down long enough to say so. */
  readonly offline?: boolean | undefined;
  /** The daemon runs a newer build than the one that served this page. */
  readonly updateReady?: boolean | undefined;
  readonly onRetry?: (() => void) | undefined;
  readonly children: ReactNode;
  /**
   * What sits in the scrolling column after the page: the decision bar, which
   * the stylesheet pins to the bottom of it.
   */
  readonly footer?: ReactNode;
  /**
   * What the page can be commented on as: "open" or "late", and absent where
   * it cannot. app.css selects `.page[data-acting]` to make rows, passages and
   * files tappable, since iOS Safari sends no click from an element it does
   * not take to be clickable.
   */
  readonly acting?: string | undefined;
}

/**
 * A screen below the queue: the top bar with its way back, the notices under
 * it, and the padded column the screen's content sits in.
 */
export function Screen({
  title,
  back,
  backLabel,
  waiting,
  offline,
  updateReady,
  onRetry,
  children,
  footer,
  acting,
}: ScreenProps) {
  return (
    <>
      <div className="head">
        <TopBar
          waiting={waiting}
          back={{ label: backLabel, href: back }}
          title={title}
        />
        <Banners
          offline={offline}
          updateReady={updateReady}
          onRetry={onRetry}
        />
      </div>
      <div className="scroll">
        <div className="page" data-acting={acting}>
          {children}
        </div>
        {footer}
      </div>
    </>
  );
}
