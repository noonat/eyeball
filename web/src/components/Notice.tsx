import type { ReactNode } from "react";
import { cx } from "./cx";
import { Icon, type IconName } from "./Icon";

export type NoticeTone = "neutral" | "warn" | "danger";

const glyphs: Readonly<Record<NoticeTone, IconName>> = {
  neutral: "info",
  warn: "triangle-alert",
  danger: "circle-alert",
};

interface NoticeProps {
  readonly tone: NoticeTone;
  /** Replaces the tone's usual glyph, as the lapsed subscription's bell does. */
  readonly icon?: IconName;
  readonly children: ReactNode;
}

/** A line of explanation, for what the screen could not have prevented. */
export function Notice({ tone, icon, children }: NoticeProps) {
  const toneClass = tone === "neutral" ? undefined : tone;
  return (
    <div className={cx("notice", toneClass)}>
      <Icon glyph name={icon ?? glyphs[tone]} />
      <span>{children}</span>
    </div>
  );
}
