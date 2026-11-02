import { useEffect, useRef, type MouseEvent } from "react";
import type { Card, CardBody, CodeRow } from "../api/types";
import { AppLink } from "./AppLink";
import { CodeTable } from "./CodeTable";
import { Failed } from "./CommentCard";
import { CommentGroup, CommentItems, ThreadRow } from "./CommentList";
import { Composer } from "./Composer";
import { composerId, sameAnchor, type ComposerAt } from "./composerAt";
import { Counts } from "./Counts";
import { DocumentBody } from "./DocumentBody";
import { Icon } from "./Icon";
import { ModeLine } from "./ModeLine";
import { RenderedFile } from "./RenderedFile";
import { composerProps, pendingAt, useReviewUi } from "./reviewUi";

interface ReviewCardProps {
  readonly card: Card;
}

/**
 * One file of the round, open unless the reader folded it: its path, what it moved, a link to the file on
 * its own, and below them its body. A card the round sent no body for shows a
 * skeleton the size of the rows it will hold, and a held card shows its size
 * and a link to load it.
 */
export function ReviewCard({ card }: ReviewCardProps) {
  const ui = useReviewUi();
  const body = card.body;
  const open = ui.folded?.has(card.path) !== true;
  const failure = ui.failures?.[`card:${card.path}`];
  const element = useRef<HTMLDetailsElement>(null);
  const load = ui.onLoad;
  const wanted =
    load !== undefined &&
    open &&
    card.deferred &&
    body === null &&
    failure === undefined;
  // A body is asked for once its card is open and within a screen of view. The
  // observer's root is the viewport, since the document is what scrolls, and
  // nothing between a card and the viewport clips, so the margin reaches a
  // screen below it. An observer is made only while a body is wanted, so a card
  // unfolded on screen asks at once and a folded one asks for nothing. The
  // template page's htmx intersect trigger could do neither: it has no margin,
  // and telling an open card from a folded one needs a trigger filter, which
  // its allowEval: false turned off.
  useEffect(() => {
    if (!wanted || element.current === null) {
      return undefined;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          load(card.path);
        }
      },
      { rootMargin: "100% 0px" },
    );
    observer.observe(element.current);
    return () => {
      observer.disconnect();
    };
  }, [wanted, load, card.path]);
  const retry =
    load === undefined
      ? undefined
      : () => {
          load(card.path);
        };
  return (
    <details
      className="file"
      id={card.id}
      data-path={card.path}
      open={open}
      ref={element}
      onToggle={(event) => {
        ui.onFold?.(card.path, event.currentTarget.open);
      }}
    >
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
      {body !== null ? (
        <CardBodyView card={card} body={body} />
      ) : card.held !== "" ? (
        <Held card={card} failure={failure} onRetry={retry} />
      ) : (
        <>
          {failure === undefined ? null : (
            <Failed text={failure} onRetry={retry} />
          )}
          <Skeleton card={card} />
        </>
      )}
    </details>
  );
}

interface HeldProps {
  readonly card: Card;
  /** What the last Load said, where it failed. */
  readonly failure: string | undefined;
  readonly onRetry: (() => void) | undefined;
}

/** A card past the size a card shows on its own. */
function Held({ card, failure, onRetry }: HeldProps) {
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
      {failure === undefined ? null : (
        <Failed text={failure} onRetry={onRetry} />
      )}
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
        <Composer {...composerProps(ui, fileAt)} id={body.slot} />
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
  const open = ui.composer;
  return (
    <DocumentBody
      passages={body.passages ?? []}
      path={card.path}
      selected={(passage) =>
        open !== undefined &&
        passage.first > 0 &&
        sameAnchor(open, {
          level: "passage",
          path: card.path,
          first: passage.first,
          last: passage.last,
        })
      }
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
          return <Composer {...composerProps(ui, at)} id={id} cards={cards} />;
        }
        return (passage.thread?.comments?.length ?? 0) === 0 &&
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

/** The anchor a tap on a row of code opens a composer at. */
function rowAt(path: string, row: CodeRow): ComposerAt {
  return row.kind === "del"
    ? { level: "file", path, oldLine: row.number }
    : { level: "line", path, line: row.number };
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
        <Composer {...composerProps(ui, at)} id={composerId(at, card.id)} />
      ) : null;
    }
    const at: ComposerAt = { level: "line", path: card.path, line: row.number };
    const thread = threads.get(row.number);
    const cards = <CommentItems comments={thread?.comments ?? null} at={at} />;
    if (open !== undefined && sameAnchor(open, at)) {
      return (
        <Composer
          {...composerProps(ui, at)}
          id={thread?.id ?? composerId(at, card.id)}
          cards={cards}
        />
      );
    }
    // A line left with no comment carries no row, as the template's script
    // dropped the empty one.
    if (
      (thread?.comments?.length ?? 0) === 0 &&
      pendingAt(ui, at).length === 0
    ) {
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
  const selected = (row: CodeRow): boolean => {
    const open = ui.composer;
    if (open === undefined || !acting || row.kind === "hunk") {
      return false;
    }
    return sameAnchor(open, rowAt(card.path, row));
  };
  return <CodeTable rows={body.rows ?? []} under={under} selected={selected} />;
}
