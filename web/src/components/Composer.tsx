import { useLayoutEffect, useRef, type FormEvent, type ReactNode } from "react";
import { Failed } from "./CommentCard";
import { composerText, type ComposerAt } from "./composerAt";
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
  /**
   * What the field holds, when the page keeps it. The page holds it so the text
   * moves with the composer to another anchor and a draft can save it. A
   * composer given none keeps its own.
   */
  readonly text?: string | undefined;
  readonly onText?: ((text: string) => void) | undefined;
  /** The level the chips chose, which is the anchor's until one is pressed. */
  readonly level?: ComposerAt["level"] | undefined;
  /**
   * A count that changes when the place the composer is open at is tapped
   * again, which brings the field into view and focuses it once more.
   */
  readonly focus?: number | undefined;
  /** What the request to send it failed with, shown at the foot of the form. */
  readonly failure?: string | undefined;
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

/**
 * Brings a composer into view and puts the focus in its field. A composer that
 * opens in a fold opens the fold, which a draft reopened there would otherwise
 * leave shut around it. The field takes the focus without scrolling again,
 * since the scroll above has put it in view.
 */
function bringIntoView(element: HTMLFormElement): void {
  const fold = element.closest("details.fold");
  if (fold instanceof HTMLDetailsElement) {
    fold.open = true;
  }
  const opened = element.closest(
    "tr.thread, .filecomposer, div[id^='thread-']",
  );
  (opened ?? element).scrollIntoView({ block: "nearest" });
  element.querySelector("textarea")?.focus({ preventScroll: true });
}

function ComposerForm({
  at,
  late,
  text: held,
  onText,
  level,
  focus,
  failure,
  onSubmit,
  onCancel,
  onLevel,
}: ComposerProps) {
  const text = composerText(at, late, level);
  const form = useRef<HTMLFormElement>(null);
  // An anchored composer is brought into view when it opens, and again each
  // time its place is tapped, which on iOS is how a reader gets the keyboard
  // back. The review's own box is on the page already.
  useLayoutEffect(() => {
    if (form.current !== null && at.level !== "review") {
      bringIntoView(form.current);
    }
    // As the composer opens, and not as text is typed: a composer moved to
    // another anchor mounts again in the new place, and its level is fixed
    // for as long as it is mounted.
  }, [at.level]);
  useLayoutEffect(() => {
    if (form.current !== null && focus !== undefined && focus > 0) {
      bringIntoView(form.current);
    }
  }, [focus]);
  const submit = (event: FormEvent<HTMLFormElement>): void => {
    event.preventDefault();
    const field = event.currentTarget.elements.namedItem("body");
    if (field instanceof HTMLTextAreaElement) {
      onSubmit?.(field.value);
    }
  };
  return (
    <form className="composer" ref={form} onSubmit={submit}>
      <div className="anchor">{text.anchor}</div>
      <textarea
        className="field"
        name="body"
        aria-label="Comment"
        placeholder={text.placeholder}
        {...(held === undefined
          ? {}
          : {
              value: held,
              onChange: (event) => {
                onText?.(event.currentTarget.value);
              },
            })}
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
      {failure === undefined ? null : <Failed text={failure} />}
    </form>
  );
}
