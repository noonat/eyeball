import type { ReactNode } from "react";

/**
 * A "page" story sits in the padded column of a screen. A "screen" story is the
 * whole screen, top bar included.
 */
export type Frame = "page" | "screen";

interface FramedProps {
  readonly frame: Frame;
  readonly children: ReactNode;
}

/** The surface's own wrappers, as docs/design's pages write them. */
export function Framed({ frame, children }: FramedProps) {
  if (frame === "screen") {
    return <div className="app">{children}</div>;
  }
  return (
    <div className="app">
      <div className="page">{children}</div>
    </div>
  );
}
