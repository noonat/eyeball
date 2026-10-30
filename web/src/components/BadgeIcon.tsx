interface BadgeIconProps {
  /** How many reviews wait, drawn on the installed icon. */
  readonly waiting: number;
}

/** The installed home-screen icon with its badge. */
export function BadgeIcon({ waiting }: BadgeIconProps) {
  return (
    <span
      className="badgeicon"
      role="img"
      aria-label={`eyeball, ${waiting} waiting`}
    >
      <span className="badge">{waiting}</span>
    </span>
  );
}
