import type { FormEvent } from "react";
import { Icon } from "./Icon";
import { Notice } from "./Notice";
import { Pill } from "./Pill";

interface TallyProps {
  /** The comments the round holds. */
  readonly count: number;
}

/** What the round holds, above the verdict's buttons. */
export function Tally({ count }: TallyProps) {
  return (
    <div className="tally" id="tally">
      {count === 0 ? (
        <span>No comments yet</span>
      ) : (
        <>
          <Icon name="message-square" />
          <Pill tone="plain">
            {count} comment{count === 1 ? "" : "s"}
          </Pill>
          <span>Ready to send</span>
        </>
      )}
    </div>
  );
}

/** What stands between the reader and a verdict, which the bar says. */
export interface BarGuard {
  /** Comments the server did not take, which the verdict would leave behind. */
  readonly unsent?: number | undefined;
  /** A comment is still being written. */
  readonly unsaved?: boolean | undefined;
  /** The stream is down, so a verdict could not be sent. */
  readonly offline?: boolean | undefined;
}

interface DecisionBarProps extends BarGuard {
  readonly count: number;
  /** The verdict note as typed. */
  readonly note?: string | undefined;
  /**
   * Called as the note is typed. A bar given it holds the note as the page
   * keeps it, which a draft saves, and a bar given none keeps its own.
   */
  readonly onNote?: ((note: string) => void) | undefined;
  /** A change request with nothing in it was refused. */
  readonly refused?: boolean | undefined;
  readonly onDecide?:
    | ((verdict: "changes" | "approve", note: string) => void)
    | undefined;
}

/**
 * The decision bar of an open round: the count, a note to send with the
 * verdict, and the two verdicts. A change request with no comment and no note
 * is refused, and the bar says so. Each thing that would lose work, or fail to
 * send, is a line above the count.
 */
export function DecisionBar({
  count,
  note,
  onNote,
  refused,
  unsent,
  unsaved,
  offline,
  onDecide,
}: DecisionBarProps) {
  const submit = (event: FormEvent<HTMLFormElement>): void => {
    event.preventDefault();
    const native = event.nativeEvent;
    const submitter = native instanceof SubmitEvent ? native.submitter : null;
    const field = event.currentTarget.elements.namedItem("note");
    const verdict =
      submitter instanceof HTMLButtonElement ? submitter.value : "";
    if (
      field instanceof HTMLTextAreaElement &&
      (verdict === "changes" || verdict === "approve")
    ) {
      onDecide?.(verdict, field.value);
    }
  };
  return (
    <form className="decide" id="bar" onSubmit={submit}>
      {refused === true ? (
        <Notice tone="warn">
          <strong>Requesting changes needs something to act on.</strong> Add a
          comment, or write a note with the verdict.
        </Notice>
      ) : null}
      {unsent === undefined || unsent === 0 ? null : (
        <div id="unsent">
          <Notice tone="danger">
            <strong>
              <span id="unsent-count">
                {unsent} comment{unsent === 1 ? "" : "s"}
              </span>{" "}
              did not save.
            </strong>{" "}
            Retry it, or delete it, before deciding.
          </Notice>
        </div>
      )}
      {unsaved === true ? (
        <div id="unsaved">
          <Notice tone="danger">
            <strong>A comment is still being written.</strong> Add it, or cancel
            it, before deciding.
          </Notice>
        </div>
      ) : null}
      <Tally count={count} />
      <textarea
        className="field"
        name="note"
        aria-label="Verdict note"
        placeholder="A note with the verdict…"
        {...(onNote === undefined
          ? { defaultValue: note }
          : {
              value: note ?? "",
              onChange: (event) => {
                onNote(event.currentTarget.value);
              },
            })}
      />
      <div className="buttons">
        <button
          type="submit"
          className="btn danger"
          name="verdict"
          value="changes"
        >
          Request changes
        </button>
        <button
          type="submit"
          className="btn primary"
          name="verdict"
          value="approve"
        >
          Approve
        </button>
      </div>
      {offline === true ? (
        <div id="whynot">
          <span className="why-not">
            You can&rsquo;t submit a review while offline.
          </span>
        </div>
      ) : null}
    </form>
  );
}
