import type { MouseEvent } from "react";
import type { Card, CardBody, CodeRow } from "../api/types";
import { AppLink } from "./AppLink";
import { CodeTable } from "./CodeTable";
import { CommentGroup, CommentItems, ThreadRow } from "./CommentList";
import { Composer } from "./Composer";
import { composerId, sameAnchor, type ComposerAt } from "./composer";
import { Counts } from "./Counts";
import { DocumentBody } from "./DocumentBody";
import { Icon } from "./Icon";
import { ModeLine } from "./ModeLine";
import { RenderedFile } from "./RenderedFile";
import { pendingAt, useReviewUi } from "./reviewUi";

interface ReviewCardProps {
  readonly card: Card;
  /**
   * The body the app fetched for a card the round did not send one with, or
   * for a held card the reader loaded.
   */
  readonly fetched?: CardBody | undefined;
}

/**
 * One file of the round, open: its path, what it moved, a link to the file on
 * its own, and below them its body. A card the round sent no body for shows a
 * skeleton the size of the rows it will hold, and a held card shows its size
 * and a link to load it.
 */
export function ReviewCard({ card, fetched }: ReviewCardProps) {
  const body = card.body ?? fetched;
  return (
    <details className="file" id={card.id} open>
      <summary>
        <span className="path">
          <bdi>{card.path}</bdi>
        </span>
        {card.unmeasured ? null : (
          <Counts added={card.added} removed={card.removed} />
        )}
        {card.raw_diff === "" ? (
          <AppLink
            className="fileopen"
            href={card.whole}
            aria-label="Whole file"
            title="Whole file"
          >
            <Icon name="maximize-2" />
          </AppLink>
        ) : (
          <AppLink
            className="fileopen"
            href={card.raw_diff}
            aria-label="Raw diff"
            title="Raw diff"
          >
            <Icon name="file-text" />
          </AppLink>
        )}
      </summary>
      {body !== undefined && body !== null ? (
        <CardBodyView card={card} body={body} />
      ) : card.held !== "" ? (
        <Held card={card} />
      ) : (
        <Skeleton card={card} />
      )}
    </details>
  );
}

/** A card past the size a card shows on its own. */
function Held({ card }: { readonly card: Card }) {
  const ui = useReviewUi();
  const load = (event: MouseEvent): void => {
    event.preventDefault();
    ui.onLoad?.(card.path);
  };
  return (
    <div className="filefacts">
      <span className="kind">{card.held} changed lines</span>
      <span className="hint">{card.held_hint}</span>
      <a className="load" href={card.whole} onClick={load}>
        Load anyway
      </a>
    </div>
  );
}

/**
 * What a card shows while its body is on its way. It is as tall as the rows
 * will be, from the hunks and lines the round counted, so the body arriving
 * moves nothing below it. A body that is not rows has no counts, and the
 * skeleton keeps its default height.
 */
function Skeleton({ card }: { readonly card: Card }) {
  if (card.skeleton_lines === 0) {
    return <div className="skeleton" />;
  }
  return (
    <div
      className="skeleton"
      data-hunks={card.skeleton_hunks}
      data-lines={card.skeleton_lines}
      // The stylesheet sizes the skeleton from these two properties.
      ref={(element) => {
        element?.style.setProperty("--hunks", String(card.skeleton_hunks));
        element?.style.setProperty("--lines", String(card.skeleton_lines));
      }}
    />
  );
}

interface BodyProps {
  readonly card: Card;
  readonly body: CardBody;
}

/** What a card shows for its file, then the comments on the file. */
export function CardBodyView({ card, body }: BodyProps) {
  const ui = useReviewUi();
  const fileAt: ComposerAt = { level: "file", path: card.path };
  const slotOpen =
    ui.composer !== undefined &&
    sameAnchor(ui.composer, fileAt) &&
    body.slot !== "";
  return (
    <>
      {body.mode === "" ? null : <ModeLine mode={body.mode} />}
      <FileBody card={card} body={body} />
      {body.list_id === "" ? (
        <CommentItems comments={body.comments} />
      ) : (
        <CommentGroup id={body.list_id} comments={body.comments} at={fileAt} />
      )}
      {body.slot === "" ? null : slotOpen ? (
        <Composer
          at={fileAt}
          id={body.slot}
          late={ui.late}
          onSubmit={(text) => {
            ui.onSubmit?.(fileAt, text);
          }}
          onCancel={ui.onClose}
        />
      ) : (
        <div id={body.slot} />
      )}
    </>
  );
}

function FileBody({ card, body }: BodyProps) {
  switch (body.kind) {
    case "rendered":
      return body.rendered === null ? null : (
        <RenderedFile rendered={body.rendered} card />
      );
    case "deleted":
      return <div className="toobig">Deleted as of this round.</div>;
    case "document":
      return (
        <div className="filebody">
          <CardDocument card={card} body={body} />
        </div>
      );
    case "binary":
      return <div className="toobig">Binary file, not shown as a diff.</div>;
    case "too_large":
      return <div className="toobig">Too large to show as a diff.</div>;
    default:
      return <CardRows card={card} body={body} />;
  }
}

/** A card's document, with the comments on a passage under it. */
export function CardDocument({ card, body }: BodyProps) {
  const ui = useReviewUi();
  return (
    <DocumentBody
      passages={body.passages ?? []}
      renderAfter={(passage) => {
        if (passage.first === 0 || body.list_id === "") {
          return null;
        }
        const at: ComposerAt = {
          level: "passage",
          path: card.path,
          first: passage.first,
          last: passage.last,
        };
        const id = passage.thread?.id ?? composerId(at, card.id);
        const cards = (
          <CommentItems comments={passage.thread?.comments ?? null} at={at} />
        );
        if (ui.composer !== undefined && sameAnchor(ui.composer, at)) {
          return (
            <Composer
              at={at}
              id={id}
              late={ui.late}
              cards={cards}
              onSubmit={(text) => {
                ui.onSubmit?.(at, text);
              }}
              onCancel={ui.onClose}
            />
          );
        }
        return passage.thread === null &&
          pendingAt(ui, at).length === 0 ? null : (
          <CommentGroup
            id={id}
            comments={passage.thread?.comments ?? null}
            at={at}
          />
        );
      }}
    />
  );
}

/** A card's rows of code, with a thread under a line that has comments. */
function CardRows({ card, body }: BodyProps) {
  const ui = useReviewUi();
  const threads = new Map(
    (body.threads ?? []).map((thread) => [thread.line, thread]),
  );
  const acting = body.list_id !== "";
  const under = (row: CodeRow) => {
    if (!acting || row.kind === "hunk") {
      return null;
    }
    const open = ui.composer;
    if (row.kind === "del") {
      const at: ComposerAt = {
        level: "file",
        path: card.path,
        oldLine: row.number,
      };
      return open !== undefined && sameAnchor(open, at) ? (
        <Composer
          at={at}
          id={composerId(at, card.id)}
          late={ui.late}
          onSubmit={(text) => {
            ui.onSubmit?.(at, text);
          }}
          onCancel={ui.onClose}
        />
      ) : null;
    }
    const at: ComposerAt = { level: "line", path: card.path, line: row.number };
    const thread = threads.get(row.number);
    const cards = <CommentItems comments={thread?.comments ?? null} at={at} />;
    if (open !== undefined && sameAnchor(open, at)) {
      return (
        <Composer
          at={at}
          id={thread?.id ?? composerId(at, card.id)}
          late={ui.late}
          cards={cards}
          onSubmit={(text) => {
            ui.onSubmit?.(at, text);
          }}
          onCancel={ui.onClose}
        />
      );
    }
    if (thread === undefined && pendingAt(ui, at).length === 0) {
      return null;
    }
    return (
      <ThreadRow
        id={thread?.id ?? composerId(at, card.id)}
        comments={thread?.comments ?? null}
        at={at}
      />
    );
  };
  return <CodeTable rows={body.rows ?? []} under={under} />;
}
