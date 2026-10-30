import type { ReactNode } from "react";
import { cx } from "./cx";
import { Icon, type IconName } from "./Icon";

export type ButtonTone = "primary" | "danger" | "ghost" | "accent";

interface ButtonProps {
  readonly children: ReactNode;
  readonly tone?: ButtonTone;
  readonly small?: boolean;
  readonly icon?: IconName;
  /** A control that goes somewhere is a link. One that acts is a button. */
  readonly href?: string;
  readonly onClick?: (() => void) | undefined;
}

/** A pill-shaped control. Approve is the filled one and sits on the right. */
export function Button({
  children,
  tone,
  small,
  icon,
  href,
  onClick,
}: ButtonProps) {
  const className = cx("btn", small && "small", tone);
  const inside = (
    <>
      {icon === undefined ? null : <Icon name={icon} />}
      {children}
    </>
  );
  if (href !== undefined) {
    return (
      <a className={className} href={href}>
        {inside}
      </a>
    );
  }
  return (
    <button type="button" className={className} onClick={onClick}>
      {inside}
    </button>
  );
}
