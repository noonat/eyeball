import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { useEffect, useMemo, useRef, useState } from "react";
import { clearBlinkMark, setBlinkMark } from "../../ts/blink";
import { queueKey } from "../api/queue";
import type { CardBody, Comment, Review } from "../api/types";
import { viewKey, type ViewAnswer } from "../api/views";
import {
  commentRequest,
  composerText,
  sameAnchor,
  type ComposerAt,
} from "../components/composerAt";
import type { Pending, ReviewUi } from "../components/reviewUi";
import {
  anchorOf,
  queryOf,
  recallDrafts,
  requestAt,
  requestOf,
  storeDrafts,
  targetOf,
  valuesOf,
  type Drafts,
  type SavedCard,
} from "./reviewDrafts";
import {
  findComment,
  passagePath,
  placeComment,
  removeComment,
  replaceComment,
  changeBody,
  withBodies,
} from "./reviewCache";
import {
  deleteComment,
  postComment,
  postVerdict,
  putComment,
} from "./reviewWrites";
import { bodyKey, useBodies } from "./useBodies";
import { useReviewScroll } from "./useReviewScroll";
import type { ReviewStream } from "./useReviewStream";
import type { CommentRequest } from "../api/types";

type Level = ComposerAt["level"];

/**
 * A comment the reader wrote that the server has not stored. It keeps the
 * request it was sent as, key included, so a retry sends the same comment and a
 * send that was stored after all is not stored twice.
 */
export interface Outbox {
  /** The app's own id, until the server gives the comment one. */
  readonly id: string;
  readonly request: CommentRequest;
  /** The anchor line the composer showed, which the card repeats. */
  readonly where: string;
  readonly state: "sending" | "unsent";
}

/** What a draft is made of, read from the state it was saved from. */
export interface Held {
  readonly note: string;
  readonly reviewText: string;
  readonly composer: ComposerAt | undefined;
  readonly composerText: string;
  readonly edits: Readonly<Record<string, string>>;
  readonly outbox: readonly Outbox[];
}

/** The draft a round's state makes, in the shape the template saved. */
export function draftOf(held: Held, review: Review, against: string): Drafts {
  const d: {
    -readonly [K in keyof Drafts]: Drafts[K];
  } = {};
  if (held.note.trim() !== "") {
    d.note = held.note;
  }
  if (held.reviewText.trim() !== "") {
    d.review = held.reviewText;
  }
  if (held.composer !== undefined && held.composerText.trim() !== "") {
    d.anchored = {
      query: queryOf(held.composer, against),
      text: held.composerText,
    };
  }
  const edits = Object.entries(held.edits).filter(
    ([, text]) => text.trim() !== "",
  );
  if (edits.length > 0) {
    d.edits = Object.fromEntries(edits);
  }
  const unsent = held.outbox
    .filter((item) => item.state === "unsent")
    .map((item) => savedCard(item, review));
  if (unsent.length > 0) {
    d.unsent = unsent;
  }
  return d;
}

/** An unsent card as the template saved one: its form's values and its list. */
function savedCard(item: Outbox, review: Review): SavedCard {
  const at = requestAt(item.request);
  const cardId =
    at.level === "review"
      ? ""
      : ((review.cards ?? []).find((c) => c.path === at.path)?.id ?? "");
  return {
    action: `/reviews/${review.id}/comments`,
    target: targetOf(at, cardId),
    values: valuesOf(item.request),
    where: item.where,
  };
}

/** The cards a draft kept, put back as unsent, with the app's own ids. */
export function outboxOf(saved: readonly SavedCard[]): Outbox[] {
  return saved.flatMap((card) => {
    const request = requestOf(card.values);
    return request === undefined
      ? []
      : [
          {
            id: `pending-${crypto.randomUUID()}`,
            request,
            where: card.where,
            state: "unsent" as const,
          },
        ];
  });
}

/** The comment an outbox entry is drawn as. */
function pendingComment(item: Outbox): Comment {
  return {
    id: item.id,
    href: "",
    where: item.where,
    quoted: "",
    body: item.request.body,
    late: false,
    editable: false,
  };
}

/** Whether a round has a card for a path, so a draft there can be reopened. */
function hasCard(review: Review, at: ComposerAt): boolean {
  return (
    at.level === "review" ||
    (review.cards ?? []).some((card) => card.path === at.path)
  );
}

/**
 * Everything the review page keeps for the reader that the server does not:
 * the open composer and what it holds, the open edit forms, comments on their
 * way or refused, the note, and what a request that failed said. The page
 * saves it as a draft as it changes, and takes it back on load.
 *
 * Each answer goes into the cached round, so a comment never waits on a fetch
 * of it. The state is held here and not in the cached view, so a round fetched
 * again, as the stream asks, keeps what was typed.
 */
export function useReview(
  latest: Review,
  address: string,
  stream: ReviewStream,
  failed: boolean,
) {
  const client = useQueryClient();
  const navigate = useNavigate();
  // What the screen shows is the cached round once nothing typed would be lost
  // by it. A round fetched after a verdict is given elsewhere has no bar, and
  // draws a composer as a late one, so it waits for the text on the page to be
  // stored, sent or cleared, as the template page's reload did. The reader's
  // own writes are put in what is shown at once.
  const [shown, setShown] = useState(latest);
  // The bodies of a long review's cards, which the round sends without, are
  // queries of their own and merged into the round for everything that reads or
  // draws it. A round fetched again cannot take one away.
  const bodies = useBodies(shown);
  const review = useMemo(
    () => withBodies(shown, bodies.data),
    [shown, bodies.data],
  );
  const acting = review.acting;
  const open = acting?.state === "open";
  const late = acting?.state === "late";
  const against = acting?.against === "base" ? "base" : "";
  const writing = acting !== null;
  // The draft is read once, as the page opens, so the state starts as it was
  // left and nothing saves over it first. A decided round keeps none.
  const [saved] = useState<Drafts>(() =>
    open ? recallDrafts(review.id, review.round) : {},
  );
  // The place a draft reopens the composer, when the round still has the file.
  const [savedAt] = useState<ComposerAt | undefined>(() => {
    const at = anchorOf(saved.anchored?.query ?? {});
    return at !== undefined && at.level !== "review" && hasCard(review, at)
      ? at
      : undefined;
  });
  const [composer, setComposer] = useState<ComposerAt | undefined>(savedAt);
  const [composerTyped, setComposerTyped] = useState(() =>
    savedAt === undefined ? "" : (saved.anchored?.text ?? ""),
  );
  const [composerLevel, setComposerLevel] = useState<Level | undefined>();
  const [focusTick, setFocusTick] = useState(0);
  const [reviewText, setReviewText] = useState(() => saved.review ?? "");
  const [note, setNote] = useState(() => saved.note ?? "");
  const [edits, setEdits] = useState<Readonly<Record<string, string>>>(() =>
    Object.fromEntries(
      Object.entries(saved.edits ?? {}).filter(
        ([id]) => findComment(review, id) !== undefined,
      ),
    ),
  );
  const [outbox, setOutbox] = useState<readonly Outbox[]>(() =>
    outboxOf(saved.unsent ?? []),
  );
  const [failures, setFailures] = useState<Readonly<Record<string, string>>>(
    {},
  );

  // The newest of every piece of state, for an answer that arrives later.
  const now = useRef({
    composer,
    composerTyped,
    reviewText,
    review,
    shown,
    outbox,
  });
  useEffect(() => {
    now.current = {
      composer,
      composerTyped,
      reviewText,
      review,
      shown,
      outbox,
    };
  });

  const fail = (key: string, message: string | undefined): void => {
    setFailures((all) => {
      const { [key]: _gone, ...rest } = all;
      return message === undefined ? rest : { ...rest, [key]: message };
    });
  };

  const change = (fn: (view: Review) => Review): void => {
    client.setQueryData<ViewAnswer<Review>>(viewKey(address), (old) =>
      old?.found === true ? { found: true, body: fn(old.body) } : old,
    );
    // The round as shown takes the change as an update of its own state, so two
    // answers handled before a render each build on the one before, and a round
    // taken up in between is built on too. A card with no body is left alone by
    // the change. Each body in its entry takes it from the entry's own latest
    // value, with the round's other parts beside it.
    setShown(fn);
    const base = now.current.shown;
    for (const card of base.cards ?? []) {
      if (card.body === null) {
        client.setQueryData<CardBody>(bodyKey(card), (old) =>
          old === undefined ? old : changeBody(base, card, old, fn),
        );
      }
    }
    // A fetch already in flight may have read the store before this write, and
    // its answer would take the comment off the screen once it arrives. It is
    // cancelled and fetched again, so the fetch that is shown starts after the
    // store. That holds for the round and for each body.
    for (const key of [
      viewKey(address),
      ...(base.cards ?? []).map((card) => bodyKey(card)),
    ]) {
      if (client.isFetching({ queryKey: key, exact: true }) > 0) {
        void client.invalidateQueries({ queryKey: key, exact: true });
      }
    }
  };

  // The round is fetched again after a comment on a passage is added or
  // deleted. The comment is put where it belongs from the answer, so it shows
  // at once, but whether a document's passage sits in a fold depends on whether
  // it has a comment, and that rule is the server's: a copy of it here would
  // have nothing checking that the two agree. The fetch draws the folds as the
  // server decides them, and a card's body is read again for the same reason,
  // since the round's fetch does not carry it.
  const refetch = (path: string | undefined): void => {
    void client.invalidateQueries({ queryKey: viewKey(address), exact: true });
    if (path !== undefined) {
      bodies.refresh(path);
    }
  };

  const settle = (item: Outbox): void => {
    setOutbox((all) => all.filter((o) => o.id !== item.id));
  };

  const mark = (item: Outbox, state: Outbox["state"]): void => {
    setOutbox((all) => {
      const others = all.filter((o) => o.id !== item.id);
      return all.some((o) => o.id === item.id)
        ? all.map((o) => (o.id === item.id ? { ...o, state } : o))
        : [...others, { ...item, state }];
    });
  };

  /**
   * Sends one comment. A refusal is the caller's to answer, since it would be
   * refused again and the text belongs back where it was typed. A comment that
   * may not have been stored becomes, or stays, an unsent card.
   */
  const transmit = async (
    item: Outbox,
    retry: boolean,
    refused: (message: string) => void,
  ): Promise<void> => {
    const outcome = await postComment(review.id, item.request);
    if (outcome.kind === "ok") {
      settle(item);
      fail(`comment:${item.id}`, undefined);
      const { comment, count } = outcome.value;
      change((view) =>
        placeComment(view, requestAt(item.request), comment, count),
      );
      if (item.request.level === "passage") {
        refetch(item.request.path);
      }
      return;
    }
    if (outcome.kind === "refused" && !retry) {
      settle(item);
      refused(outcome.message);
      return;
    }
    mark(item, "unsent");
    fail(`comment:${item.id}`, retry ? outcome.message : undefined);
  };

  const submit = (at: ComposerAt, body: string): void => {
    const own = at.level === "review";
    const level = own ? "review" : (composerLevel ?? at.level);
    const request = commentRequest(
      at,
      {
        round: review.round,
        against,
        body,
        submission: crypto.randomUUID(),
      },
      level,
    );
    const item: Outbox = {
      id: `pending-${crypto.randomUUID()}`,
      request,
      where: composerText(at, late).anchor,
      state: "sending",
    };
    setOutbox((all) => [...all, item]);
    if (own) {
      setReviewText("");
      fail("review", undefined);
    } else {
      setComposer(undefined);
      setComposerTyped("");
      setComposerLevel(undefined);
      fail("composer", undefined);
    }
    void transmit(item, false, (message) => {
      const held = now.current;
      if (own) {
        // The box is back as it was, unless something was typed into it since.
        if (held.reviewText.trim() === "") {
          setReviewText(body);
          fail("review", message);
          return;
        }
      } else if (held.composer === undefined && held.composerTyped === "") {
        setComposer(at);
        setComposerTyped(body);
        setComposerLevel(level === at.level ? undefined : level);
        fail("composer", message);
        return;
      }
      // What was typed since is not to be lost for putting this back, so the
      // refused comment waits as an unsent card.
      mark(item, "unsent");
      fail(`comment:${item.id}`, message);
    });
  };

  const retry = (id: string): void => {
    const item = now.current.outbox.find((o) => o.id === id);
    if (item === undefined) {
      return;
    }
    fail(`comment:${id}`, undefined);
    mark(item, "sending");
    void transmit(item, true, () => undefined);
  };

  const find = (id: string): Comment | undefined =>
    findComment(now.current.review, id);

  const edit = (id: string): void => {
    const comment = find(id);
    if (comment !== undefined) {
      setEdits((all) => ({ ...all, [id]: all[id] ?? comment.body }));
    }
  };

  const closeEdit = (id: string): void => {
    setEdits((all) => {
      const { [id]: _gone, ...rest } = all;
      return rest;
    });
    fail(`edit:${id}`, undefined);
  };

  const save = (id: string, body: string): void => {
    const comment = find(id);
    if (comment === undefined) {
      return;
    }
    fail(`edit:${id}`, undefined);
    void putComment(comment.href, { body }).then((outcome) => {
      if (outcome.kind === "ok") {
        const { comment: stored, count } = outcome.value;
        change((view) => replaceComment(view, stored, count));
        closeEdit(id);
      } else {
        fail(`edit:${id}`, outcome.message);
      }
    });
  };

  const remove = (id: string): void => {
    const item = now.current.outbox.find((o) => o.id === id);
    if (item !== undefined) {
      settle(item);
      fail(`comment:${id}`, undefined);
      return;
    }
    const comment = find(id);
    if (comment === undefined) {
      return;
    }
    fail(`comment:${id}`, undefined);
    const passage = passagePath(now.current.review, id);
    void deleteComment(comment.href).then((outcome) => {
      if (outcome.kind === "ok") {
        change((view) => removeComment(view, id, outcome.value.count));
        closeEdit(id);
        if (passage !== undefined) {
          refetch(passage);
        }
      } else {
        fail(`comment:${id}`, outcome.message);
      }
    });
  };

  // The draft is saved as the state changes, which is what "Saved as you type"
  // says. A decided round sends a comment at once and keeps none.
  useEffect(() => {
    if (!open) {
      return;
    }
    storeDrafts(
      review.id,
      review.round,
      draftOf(
        {
          note,
          reviewText,
          composer,
          composerText: composerTyped,
          edits,
          outbox,
        },
        review,
        against,
      ),
    );
  }, [
    open,
    review,
    against,
    note,
    reviewText,
    composer,
    composerTyped,
    edits,
    outbox,
  ]);

  const typed =
    reviewText.trim() !== "" ||
    composerTyped.trim() !== "" ||
    Object.values(edits).some((text) => text.trim() !== "");
  // A comment on its way counts as text not yet stored, as it did on the
  // template page, where its text stayed in the form until the answer came.
  const sending = outbox.some((o) => o.state === "sending");
  const unsaved = typed || sending;
  const unsent = outbox.filter((o) => o.state === "unsent").length;
  const offline = stream.connection.offline;
  // What holds a verdict back: text not stored, a comment that did not save, or
  // no way to reach the daemon.
  const blocked = unsaved || unsent > 0 || offline;
  // What holds a refetch back, and the screen's change to what it fetched: all
  // of that and the note too, which is no comment but is typed text.
  const holding = unsaved || outbox.length > 0 || note.trim() !== "";

  const [barRefused, setBarRefused] = useState(false);
  const [barFailure, setBarFailure] = useState<string | undefined>();
  const [deciding, setDeciding] = useState(false);

  const decide = (verdict: "changes" | "approve", text: string): void => {
    const action = review.acting?.bar?.action;
    if (blocked || deciding || action === undefined) {
      return;
    }
    setBarRefused(false);
    setBarFailure(undefined);
    // Both verdicts lower the waiting count, so both blink. The mark is set as
    // the verdict is sent, for the queue the router moves to, and cleared when
    // the verdict is not stored.
    setBlinkMark({ review: review.id, count: review.waiting_count });
    setDeciding(true);
    void postVerdict(action, {
      round: review.round,
      verdict,
      note: text,
    }).then((outcome) => {
      if (outcome.kind === "ok") {
        // The queue the router moves to is fetched fresh. Drawn from the view cached
        // before the verdict, it would show the count and the row as they were,
        // and under reduced motion, where the new count shows at once, the old
        // one would show first. Patching the cached view to look as the server
        // would is a guess, so it is dropped.
        client.removeQueries({ queryKey: queueKey });
        void navigate("/");
        return;
      }
      clearBlinkMark();
      setDeciding(false);
      if (outcome.kind === "refused" && outcome.status === 422) {
        setBarRefused(true);
      } else {
        setBarFailure(outcome.message);
      }
    });
  };

  // The screen follows the stream. A stamp that differs from the round's own
  // says something moved, and the round is fetched once nothing typed would be
  // lost, and once for each stamp, so a view that never reaches the stream's
  // stamp is not fetched for ever.
  const asked = useRef("");
  // A stamp whose fetch did not arrive is asked for again. The connection that
  // dropped may have taken the fetch with it, and the stream that reconnects
  // sends the same stamp, so a stamp remembered as asked would never be fetched.
  // A fetch that failed clears it the same way.
  useEffect(() => {
    if (offline || failed) {
      asked.current = "";
    }
    if (
      stream.stamp !== "" &&
      stream.stamp !== latest.stamp &&
      stream.stamp !== asked.current &&
      !holding &&
      !deciding &&
      !offline
    ) {
      asked.current = stream.stamp;
      void client.invalidateQueries({ queryKey: viewKey(address) });
    }
  }, [
    stream.stamp,
    latest.stamp,
    holding,
    deciding,
    offline,
    failed,
    client,
    address,
  ]);
  // Taking up the newer round is state set while drawing, which React allows
  // for the component's own state, so no frame draws the old round once
  // nothing holds it.
  if (latest !== shown && !holding && !deciding) {
    setShown(latest);
  }

  // A round that moved makes the bodies held stale: after a verdict given
  // elsewhere their comments still show Edit and Delete. They are read again,
  // each kept as it is until the new one arrives. A body far from the reader is
  // read too, which costs a request each for the cards the reader scrolled
  // past. Dropping them to load again when they come near was rejected, since a
  // body above the reader changing back to a skeleton moves what is read.
  const stamp = useRef(shown.stamp);
  const refreshAll = bodies.refreshAll;
  useEffect(() => {
    if (stamp.current !== shown.stamp) {
      stamp.current = shown.stamp;
      refreshAll();
    }
  }, [shown.stamp, refreshAll]);

  const pending: Pending[] = outbox.map((item) => ({
    at: requestAt(item.request),
    comment: pendingComment(item),
    state: item.state,
  }));

  const scroll = useReviewScroll(review, bodies.data);
  // What every round has, written or not: the bodies its cards ask for and the
  // folds the reader makes.
  const base: Partial<ReviewUi> = {
    late: false,
    onLoad: bodies.load,
    folded: scroll.folded,
    onFold: scroll.onFold,
  };
  const ui: Partial<ReviewUi> = writing
    ? {
        ...base,
        late,
        composer,
        composerText: composerTyped,
        composerLevel,
        onComposerText: setComposerTyped,
        onLevel: setComposerLevel,
        reviewText,
        onReviewText: setReviewText,
        editing: Object.keys(edits),
        editText: edits,
        onEditText: (id, text) => {
          setEdits((all) => ({ ...all, [id]: text }));
        },
        failures: { ...failures, ...bodies.failures },
        pending,
        composerFocus: focusTick,
        onOpen: (at) => {
          // The composer moves, and keeps what was typed. A tap on the place it
          // is already open at opens it again as well, which refocuses the
          // field, scrolls it into view and clears the level the chips chose
          // and the line a failed send left, as the template's did. On iOS that
          // tap is how a reader gets the keyboard back after dismissing it.
          setComposer(at);
          setComposerLevel(undefined);
          fail("composer", undefined);
          if (composer !== undefined && sameAnchor(composer, at)) {
            setFocusTick((tick) => tick + 1);
          }
        },
        onClose: () => {
          setComposer(undefined);
          setComposerTyped("");
          setComposerLevel(undefined);
          fail("composer", undefined);
        },
        onSubmit: submit,
        onEdit: edit,
        onSave: save,
        onCancelEdit: closeEdit,
        onDelete: remove,
        onRetry: retry,
      }
    : { ...base, failures: bodies.failures };

  return {
    review,
    ui,
    bar: {
      note,
      onNote: setNote,
      unsaved,
      unsent,
      offline,
      held: blocked,
      refused: barRefused,
      ...(barFailure === undefined ? {} : { failure: barFailure }),
    },
    onDecide: decide,
    tapping: writing,
  };
}
