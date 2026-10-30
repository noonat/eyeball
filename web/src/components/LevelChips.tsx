interface Level {
  readonly label: string;
  readonly pressed?: boolean;
}

interface LevelChipsProps {
  readonly levels: readonly Level[];
  readonly onChoose?: (label: string) => void;
}

/** The levels a comment can attach to, with the chosen one pressed. */
export function LevelChips({ levels, onChoose }: LevelChipsProps) {
  return (
    <div className="levels">
      {levels.map((level) => (
        <button
          key={level.label}
          type="button"
          className="chip"
          aria-pressed={level.pressed === true ? "true" : undefined}
          onClick={() => {
            onChoose?.(level.label);
          }}
        >
          {level.label}
        </button>
      ))}
    </div>
  );
}
