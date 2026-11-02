import type { FormEvent } from "react";
import type { Comment } from "../api/types";
import { Failed } from "./CommentCard";
import { Icon } from "./Icon";

interface CommentEditProps {
  readonly comment: Comment;
  /**
   * What the field holds, when the page keeps it so a draft can. The field is
   * the comment's own text until then.
   */
  readonly text?: string | undefined;
  readonly onText?: ((text: string) => void) | undefined;
  /** What the request to save it failed with. */
  readonly failure?: string | undefined;
  readonly onSave?: ((body: string) => void) | undefined;
  readonly onCancel?: (() => void) | undefined;
}

/** A comment's text in a field, in place of the comment, while it is edited. */
export function CommentEdit({
  comment,
  text,
  onText,
  failure,
  onSave,
  onCancel,
}: CommentEditProps) {
  const submit = (event: FormEvent<HTMLFormElement>): void => {
    event.preventDefault();
    const field = event.currentTarget.elements.namedItem("body");
    if (field instanceof HTMLTextAreaElement) {
      onSave?.(field.value);
    }
  };
  return (
    <form className="composer" id={`comment-${comment.id}`} onSubmit={submit}>
      <div className="anchor">{comment.where}</div>
      <textarea
        className="field"
        name="body"
        aria-label="Edit comment"
        {...(text === undefined
          ? { defaultValue: comment.body }
          : {
              value: text,
              onChange: (event) => {
                onText?.(event.currentTarget.value);
              },
            })}
      />
      <div className="foot">
        <span className="hint">Editing</span>
        <span className="acts">
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
          <button type="submit" className="btn small accent">
            Save
          </button>
        </span>
      </div>
      {failure === undefined ? null : <Failed text={failure} />}
    </form>
  );
}
