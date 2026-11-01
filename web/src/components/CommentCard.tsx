import type { KeyboardEvent, MouseEvent } from "react";
import type { Comment } from "../api/types";
import { Icon } from "./Icon";

/**
 * Where a comment is in its trip to the server. A comment the reader just
 * sent is sending until the answer comes, and one the server could not take
 * stays unsent, with its text, until it is retried or deleted. A comment in
 * neither state has no mark.
 */
export type CommentState = "sending" | "unsent";

interface CommentCardProps {
  readonly comment: Comment;
  readonly state?: CommentState | undefined;
  readonly onEdit?: (() => void) | undefined;
  readonly onDelete?: (() => void) | undefined;
  readonly onRetry?: (() => void) | undefined;
}

/**
 * One comment: what it is attached to, the text it quotes, and what the
 * reviewer wrote. A comment of a round that can still change has Edit and
 * Delete. One written after the verdict says so. A comment a round lists with
 * no way to change it has no address of its own, and so no id.
 */
export function CommentCard({
  comment,
  state,
  onEdit,
  onDelete,
  onRetry,
}: CommentCardProps) {
  const own = comment.href !== "";
  const id = own ? `comment-${comment.id}` : undefined;
  const className = state === "unsent" ? "comment unsent" : "comment";
  return (
    <div className={className} id={id}>
      <div className="anchor">
        <span className="where">{comment.where}</span>
        {comment.late ? <span>after the verdict</span> : null}
        {state === "sending" ? <span>Sending…</span> : null}
      </div>
      {comment.quoted === "" ? null : (
        <blockquote className="quoted">{comment.quoted}</blockquote>
      )}
      <p>{comment.body}</p>
      {state === "unsent" ? (
        <div className="acts">
          <Action icon="refresh-cw" onAct={onRetry} button>
            Retry now
          </Action>
          <Action icon="trash-2" onAct={onDelete} button>
            Delete
          </Action>
        </div>
      ) : comment.editable && state === undefined ? (
        <div className="acts">
          <Action icon="pencil" onAct={onEdit} href={`#${id}`}>
            Edit
          </Action>
          <Action icon="trash-2" onAct={onDelete} href={`#${id}`}>
            Delete
          </Action>
        </div>
      ) : null}
    </div>
  );
}

interface ActionProps {
  readonly icon: "refresh-cw" | "trash-2" | "pencil";
  readonly onAct: (() => void) | undefined;
  /** A link to the card itself, for an action that moves nothing. */
  readonly href?: string;
  /** An action with no address is a button by role, as an anchor can be. */
  readonly button?: boolean;
  readonly children: string;
}

/** One of a card's actions: a link to the card, or a button by role. */
function Action({ icon, onAct, href, button, children }: ActionProps) {
  const press = (event: MouseEvent): void => {
    event.preventDefault();
    onAct?.();
  };
  const key = (event: KeyboardEvent): void => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onAct?.();
    }
  };
  if (button === true) {
    return (
      <a role="button" tabIndex={0} onClick={press} onKeyDown={key}>
        <Icon name={icon} />
        {children}
      </a>
    );
  }
  return (
    <a href={href} onClick={press}>
      <Icon name={icon} />
      {children}
    </a>
  );
}
