import type { ReactNode } from "react";
import type { Card, CardBody, Comment, Review } from "../api/types";
import { AppLink } from "./AppLink";
import { CommentGroup, CommentItems } from "./CommentList";
import { Composer } from "./Composer";
import type { ComposerAt } from "./composer";
import { Counts } from "./Counts";
import { DecisionBar, type BarGuard } from "./DecisionBar";
import { DocumentBody } from "./DocumentBody";
import { Icon } from "./Icon";
import { Notice } from "./Notice";
import { Pill } from "./Pill";
import { ReviewCard } from "./ReviewCard";
import { ReviewUiContext, readOnly, type ReviewUi } from "./reviewUi";
import { Screen } from "./Screen";

interface ReviewScreenProps {
  /** The view of one round of a review. */
  readonly review: Review;
  /**
   * Bodies the app fetched for cards the round sent none with, by path. A card
   * with no body in the view or here shows a skeleton.
   */
  readonly bodies?: Readonly<Record<string, CardBody>> | undefined;
  /** The page's own state: the open composer, a comment being edited. */
  readonly ui?: Partial<ReviewUi> | undefined;
  /** The verdict note as typed, and what stands between the reader and a verdict. */
  readonly bar?: (BarGuard & { note?: string; refused?: boolean }) | undefined;
  readonly onDecide?:
    | ((verdict: "changes" | "approve", note: string) => void)
    | undefined;
  readonly offline?: boolean | undefined;
  readonly updateReady?: boolean | undefined;
  readonly onRetry?: (() => void) | undefined;
}

/**
 * One round of a review: the agent's note, who and what it is about, the
 * verdict if there is one, the comparison, a card for each file that moved,
 * the comments that belong to none, and for a round that can be commented on
 * the review's own composer and the decision bar.
 */
export function ReviewScreen({
  review,
  bodies,
  ui,
  bar,
  onDecide,
  ...notices
}: ReviewScreenProps) {
  const acting = review.acting;
  const state: ReviewUi = {
    ...readOnly,
    late: acting?.state === "late",
    ...ui,
  };
  const cards = review.cards ?? [];
  const fresh = review.fresh ? cards[0] : undefined;
  const reviewAt: ComposerAt = { level: "review" };
  return (
    <ReviewUiContext.Provider value={state}>
      <Screen
        title={review.title}
        back={review.back}
        backLabel={review.back_label}
        waiting={review.waiting_count}
        {...notices}
        acting={acting?.state}
        footer={
          acting?.bar == null ? null : (
            <DecisionBar
              count={acting.bar.count}
              onDecide={onDecide}
              {...bar}
            />
          )
        }
      >
        <Letter review={review} />
        <Brief review={review} />
        <Earlier review={review} />
        <Verdict review={review} />
        {review.closed === "" ? null : <Discussion review={review} />}
        {review.decided === null || review.decided.note === null ? null : (
          <VerdictNote note={review.decided.note} />
        )}
        {acting === null ? null : (
          <>
            <Composer
              at={reviewAt}
              id="comments-review"
              late={state.late}
              onSubmit={(text) => {
                state.onSubmit?.(reviewAt, text);
              }}
            />
            <CommentGroup
              id={acting.list_id}
              comments={acting.comments}
              at={reviewAt}
            />
          </>
        )}
        <Toolbar review={review} fresh={fresh} />
        {review.listed ? <FileList cards={cards} /> : null}
        {fresh !== undefined ? (
          <DocumentBody passages={fresh.body?.passages ?? []} />
        ) : (
          cards.map((card) => (
            <ReviewCard
              key={card.id}
              card={card}
              fetched={bodies?.[card.path]}
            />
          ))
        )}
        <div className="tree">
          <AppLink className="treerow" href={review.all_files}>
            <Icon name="folder" />
            <span className="nm">All files</span>
            <Icon name="chevron-right" />
          </AppLink>
        </div>
        {review.closed === "" ? <Discussion review={review} /> : null}
      </Screen>
    </ReviewUiContext.Provider>
  );
}

/** The agent's note, signed with the agent. */
function Letter({ review }: { readonly review: Review }) {
  return (
    <div className="note letter">
      {(review.note ?? []).map((paragraph, i) => (
        <p key={i}>{paragraph}</p>
      ))}
      <span className="sig">
        <span className="agent">{review.agent}</span>
      </span>
    </div>
  );
}

/** What the review is: its state, its round, where it came from and its title. */
function Brief({ review }: { readonly review: Review }) {
  const decided = review.decided;
  return (
    <div className="brief">
      <div className="where">
        {review.abandoned ? (
          <Pill tone="plain">Abandoned</Pill>
        ) : decided !== null ? (
          <Pill tone={decided.approved ? "approved" : "changes"}>
            {decided.pill}
          </Pill>
        ) : (
          <Pill tone="waiting">Waiting {review.waiting}</Pill>
        )}
        <Pill tone="plain">Round {review.round}</Pill>
        <span className="who">
          <AppLink className="bit" href={review.project_href}>
            <Icon name="folder" />
            <span className="project">{review.project}</span>
          </AppLink>
          <span className="bit">
            <Icon name="bot" />
            <span className="agent">{review.agent}</span>
          </span>
        </span>
      </div>
      <h2 className="title">{review.review_title}</h2>
    </div>
  );
}

/** The rounds before this one, each a link. */
function Earlier({ review }: { readonly review: Review }) {
  const earlier = review.earlier ?? [];
  if (earlier.length === 0) {
    return null;
  }
  return (
    <div className="section">
      <h2>
        Earlier rounds <span className="count">{earlier.length}</span>
      </h2>
      <div className="tree">
        {earlier.map((row) => (
          <AppLink key={row.href} className="treerow" href={row.href}>
            <Icon name="message-square" />
            <span className="nm">{row.label}</span>
            <span className="hint">{row.age}</span>
            <Icon name="chevron-right" />
          </AppLink>
        ))}
      </div>
    </div>
  );
}

/** What was decided, or that the round is closed, and why nothing is measured. */
function Verdict({ review }: { readonly review: Review }) {
  const decided = review.decided;
  return (
    <>
      {review.closed !== "" ? (
        <Notice tone="neutral">{review.closed}</Notice>
      ) : decided === null ? null : decided.approved ? (
        <Notice tone="neutral" icon="check">
          <strong>Approved.</strong> {decided.sentence}
        </Notice>
      ) : (
        <Notice tone="neutral" icon="message-square">
          <strong>Changes requested.</strong> {decided.sentence}
        </Notice>
      )}
      {review.unmeasurable === "" ? null : (
        <Notice tone="warn">
          {review.unmeasurable} Each file is shown as this round captured it.
        </Notice>
      )}
    </>
  );
}

/** The note that went out with the verdict. */
function VerdictNote({ note }: { readonly note: readonly string[] }) {
  return (
    <div className="note">
      <span className="from">Verdict note</span>
      {note.map((paragraph, i) => (
        <p key={i}>{paragraph}</p>
      ))}
    </div>
  );
}

/** The comments that belong to no card, and what was done about them. */
function Discussion({ review }: { readonly review: Review }) {
  return (
    <>
      <Section title="Comments" comments={review.comments} />
      <Section title="What was done" comments={review.replies} />
    </>
  );
}

interface SectionProps {
  readonly title: string;
  readonly comments: readonly Comment[] | null;
}

function Section({ title, comments }: SectionProps): ReactNode {
  if (comments === null || comments.length === 0) {
    return null;
  }
  return (
    <div className="section">
      <h2>
        {title} <span className="count">{comments.length}</span>
      </h2>
      <div>
        <CommentItems comments={comments} />
      </div>
    </div>
  );
}

interface ToolbarProps {
  readonly review: Review;
  readonly fresh: Card | undefined;
}

/** The totals, and the control choosing what the change is measured against. */
function Toolbar({ review, fresh }: ToolbarProps) {
  return (
    <div className="toolbar">
      {fresh !== undefined ? (
        <>
          <span className="counts">
            <b className="files">1 file, new</b>
          </span>
          <a className="btn small" href={fresh.raw_diff}>
            Raw diff
          </a>
        </>
      ) : review.unmeasurable !== "" ? (
        <span className="counts">
          <b className="files">{review.files}</b>
        </span>
      ) : (
        <span className="counts">
          <b className="files">{review.files}</b>&nbsp;
          <span className="plus">+{review.added}</span>{" "}
          <span className="minus">&minus;{review.removed}</span>
        </span>
      )}
      {review.against === null ? null : (
        <span className="segmented">
          {/* A query alone is relative to the page it is on, which only the
              browser knows until the review has a route of its own. */}
          <a
            href="?against=previous"
            aria-current={review.against.base ? undefined : true}
          >
            {review.against.previous}
          </a>
          <a
            href="?against=base"
            aria-current={review.against.base ? true : undefined}
          >
            Whole review
          </a>
        </span>
      )}
    </div>
  );
}

/** A list of the files, each a link to its card, for a round past ten. */
function FileList({ cards }: { readonly cards: readonly Card[] }) {
  return (
    <nav className="tree filelist" aria-label="Files">
      {cards.map((card) => (
        <a key={card.id} className="treerow" href={`#${card.id}`}>
          <Icon name="file-text" />
          <span className="nm">
            <bdi>{card.path}</bdi>
          </span>
          {card.unmeasured ? null : (
            <Counts added={card.added} removed={card.removed} />
          )}
        </a>
      ))}
    </nav>
  );
}
