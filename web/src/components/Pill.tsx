import type { ReactNode } from "react";

export type PillTone = "waiting" | "working" | "approved" | "changes" | "plain";

interface PillProps {
  readonly tone: PillTone;
  readonly children: ReactNode;
}

/** A state or a count, as a small rounded label. */
export function Pill({ tone, children }: PillProps) {
  return <span className={`pill ${tone}`}>{children}</span>;
}
