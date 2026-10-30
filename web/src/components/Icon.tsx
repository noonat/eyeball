import { cx } from "./cx";

/** The names in docs/design/icons.txt that a component draws. */
export type IconName =
  | "chevron-left"
  | "chevron-right"
  | "folder"
  | "bot"
  | "message-square"
  | "check"
  | "x"
  | "pencil"
  | "trash-2"
  | "refresh-cw"
  | "info"
  | "triangle-alert"
  | "circle-alert"
  | "bell"
  | "file-text"
  | "clock"
  | "chevron-down"
  | "maximize-2";

interface IconProps {
  readonly name: IconName;
  /** Draws at the step size, 22px, instead of following the text. */
  readonly large?: boolean;
  /** Marks the icon as a notice's glyph, which the stylesheet sizes. */
  readonly glyph?: boolean;
}

/** One lucide icon, drawn as a CSS mask so it takes the color of its text. */
export function Icon({ name, large, glyph }: IconProps) {
  return (
    <span
      className={cx(glyph && "glyph", "i", `i-${name}`, large && "lg")}
      aria-hidden="true"
    />
  );
}
