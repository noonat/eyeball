import type { FormEvent, ReactNode } from "react";
import { composerText, type ComposerAt } from "./composer";
import { Icon } from "./Icon";

interface ComposerProps {
  /** Where the composer opens, which decides its words and its chips. */
  readonly at: ComposerAt;
  /** The element id the composer opens in. */
  readonly id: string;
  /** The round was decided, so a comment is sent at once. */
  readonly late: boolean;
  /** The comments already on the anchor, which stay in view above the field. */
  readonly cards?: ReactNode;
  readonly onSubmit?: ((body: string) => void) | undefined;
  readonly onCancel?: (() => void) | undefined;
  readonly onLevel?: ((level: ComposerAt["level"]) => void) | undefined;
}

/**
 * A composer, in the place its anchor gives it: a row under a line, a group
 * under a passage, the file's own slot, or the form alone for the review. The
 * comments already on a line or a passage stay above the field, so the text
 * it is about is on screen while a reply is typed.
 */
export function Composer(props: ComposerProps) {
  const { at, id, cards } = props;
  const form = <ComposerForm {...props} />;
  if (
    at.level === "line" ||
    (at.level === "file" && at.oldLine !== undefined)
  ) {
    return (
      <tr className="thread" id={id}>
        <td colSpan={3}>
          <div className="thread-body">
            {cards}
            {form}
          </div>
        </td>
      </tr>
    );
  }
  if (at.level === "passage") {
    return (
      <div id={id}>
        {cards}
        {form}
      </div>
    );
  }
  if (at.level === "file") {
    return (
      <div className="filecomposer" id={id}>
        {form}
      </div>
    );
  }
  return form;
}

function ComposerForm({
  at,
  late,
  onSubmit,
  onCancel,
  onLevel,
}: ComposerProps) {
  const text = composerText(at, late);
  const submit = (event: FormEvent<HTMLFormElement>): void => {
    event.preventDefault();
    const field = event.currentTarget.elements.namedItem("body");
    if (field instanceof HTMLTextAreaElement) {
      onSubmit?.(field.value);
    }
  };
  return (
    <form className="composer" onSubmit={submit}>
      <div className="anchor">{text.anchor}</div>
      <textarea
        className="field"
        name="body"
        aria-label="Comment"
        placeholder={text.placeholder}
      />
      {late && text.chips.length > 0 ? (
        <span className="hint">{text.hint}</span>
      ) : null}
      <div className="foot">
        {text.chips.length > 0 ? (
          <span className="levels">
            {text.chips.map((chip) => (
              <button
                key={chip.level}
                type="button"
                className="chip"
                aria-pressed={chip.pressed}
                onClick={() => {
                  onLevel?.(chip.level);
                }}
              >
                {chip.label}
              </button>
            ))}
          </span>
        ) : (
          <span className="hint">{text.hint}</span>
        )}
        <span className="acts">
          {at.level === "review" ? null : (
            <button
              type="button"
              className="btn small ghost"
              onClick={() => {
                onCancel?.();
              }}
            >
              <Icon name="x" />
              Cancel
            </button>
          )}
          <button type="submit" className="btn small accent">
            {text.button}
          </button>
        </span>
      </div>
    </form>
  );
}
