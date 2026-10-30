interface EyeProps {
  /** How many reviews wait. Zero shuts the eye. */
  readonly waiting: number;
  /** The empty state's size. It is decoration, so it has no label. */
  readonly large?: boolean;
}

/** The top bar's eye: open with the waiting count as its iris, or shut. */
export function Eye({ waiting, large }: EyeProps) {
  const shut = waiting === 0;
  if (large) {
    return (
      <span className={shut ? "eye lg shut" : "eye lg"} aria-hidden="true" />
    );
  }
  if (shut) {
    return (
      <span className="eye shut" role="img" aria-label="Nothing waiting" />
    );
  }
  return (
    <span className="eye" role="img" aria-label={`${waiting} waiting`}>
      <span className="iris">{waiting}</span>
    </span>
  );
}
