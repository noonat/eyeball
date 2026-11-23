import type {
  Card,
  CardBody,
  Comment,
  DocPart,
  DocPassage,
  Review,
  Thread,
} from "../api/types";
import { composerId, type ComposerAt } from "../components/composerAt";

/**
 * The cached round with a comment the server stored put where it belongs, and
 * the count of comments the answer carried. The answer is the comment as the
 * round's view sends it, so the round needs no fetch to show it. The place is
 * the anchor the comment was sent to, because the comment says neither its
 * level nor its path.
 */
export function placeComment(
  review: Review,
  dest: ComposerAt,
  comment: Comment,
  count: number,
): Review {
  return withCount(placeAt(review, dest, comment), count);
}

/** The cached round with one comment replaced by what the server stored. */
export function replaceComment(
  review: Review,
  comment: Comment,
  count: number,
): Review {
  return withCount(
    mapComments(review, (list) =>
      list.map((c) => (c.id === comment.id ? comment : c)),
    ),
    count,
  );
}

/** The cached round without a comment the server deleted. */
export function removeComment(
  review: Review,
  id: string,
  count: number,
): Review {
  return withCount(
    mapComments(review, (list) => list.filter((c) => c.id !== id)),
    count,
  );
}

function withCount(review: Review, count: number): Review {
  const acting = review.acting;
  if (acting === null || acting.bar === null) {
    return review;
  }
  return { ...review, acting: { ...acting, bar: { ...acting.bar, count } } };
}

function placeAt(review: Review, dest: ComposerAt, comment: Comment): Review {
  const append = (list: readonly Comment[] | null): Comment[] => [
    ...(list ?? []),
    comment,
  ];
  if (dest.level === "review") {
    const acting = review.acting;
    return acting === null
      ? review
      : { ...review, acting: { ...acting, comments: append(acting.comments) } };
  }
  // A document shown as the page has no card to hold a comment on the file,
  // so it joins the comments the round lists on their own.
  if (dest.level === "file" && review.fresh) {
    return { ...review, comments: append(review.comments) };
  }
  return mapCard(review, dest.path, (card, body) => {
    switch (dest.level) {
      case "file":
        return { ...body, comments: append(body.comments) };
      case "line":
        return {
          ...body,
          threads: placeInThread(card, body, dest.line, comment),
        };
      default:
        return {
          ...body,
          passages: placeInPassage(card, body, dest, comment),
        };
    }
  });
}

/** The threads under a card's lines, with the comment in its line's. */
function placeInThread(
  card: Card,
  body: CardBody,
  line: number,
  comment: Comment,
): Thread[] {
  const threads = body.threads ?? [];
  if (threads.some((t) => t.line === line)) {
    return threads.map((t) =>
      t.line === line
        ? { ...t, comments: [...(t.comments ?? []), comment] }
        : t,
    );
  }
  const made: Thread = {
    id: composerId({ level: "line", path: card.path, line }, card.id),
    line,
    comments: [comment],
  };
  return [...threads, made].toSorted((a, b) => a.line - b.line);
}

/** The passages of a card's document, with the comment in its passage's thread. */
function placeInPassage(
  card: Card,
  body: CardBody,
  dest: Extract<ComposerAt, { level: "passage" }>,
  comment: Comment,
): DocPart[] {
  const edit = (p: DocPassage): DocPassage => {
    if (p.first !== dest.first || p.last !== dest.last) {
      return p;
    }
    const at: ComposerAt = {
      level: "passage",
      path: card.path,
      first: dest.first,
      last: dest.last,
    };
    const thread: Thread = p.thread ?? {
      id: composerId(at, card.id),
      line: 0,
      comments: [],
    };
    return {
      ...p,
      thread: { ...thread, comments: [...(thread.comments ?? []), comment] },
    };
  };
  return (body.passages ?? []).map((part) => ({
    passage: part.passage === null ? null : edit(part.passage),
    fold: part.fold === null ? null : part.fold.map(edit),
  }));
}

/** The round with a card's body changed, where the card has one. */
function mapCard(
  review: Review,
  path: string,
  change: (card: Card, body: CardBody) => CardBody,
): Review {
  return {
    ...review,
    cards: (review.cards ?? []).map((card) =>
      card.path === path && card.body !== null
        ? { ...card, body: change(card, card.body) }
        : card,
    ),
  };
}

/** The round with every list of comments changed, wherever a comment sits. */
function mapComments(
  review: Review,
  change: (list: readonly Comment[]) => Comment[],
): Review {
  const each = (list: readonly Comment[] | null): Comment[] | null =>
    list === null ? null : change(list);
  const thread = (t: Thread): Thread => ({ ...t, comments: each(t.comments) });
  const passage = (p: DocPassage): DocPassage => ({
    ...p,
    thread: p.thread === null ? null : thread(p.thread),
  });
  const part = (p: DocPart): DocPart => ({
    passage: p.passage === null ? null : passage(p.passage),
    fold: p.fold === null ? null : p.fold.map(passage),
  });
  const acting = review.acting;
  return {
    ...review,
    comments: each(review.comments),
    acting:
      acting === null ? null : { ...acting, comments: each(acting.comments) },
    cards: (review.cards ?? []).map((card) => {
      const body = card.body;
      if (body === null) {
        return card;
      }
      return {
        ...card,
        body: {
          ...body,
          comments: each(body.comments),
          threads: body.threads === null ? null : body.threads.map(thread),
          passages: body.passages === null ? null : body.passages.map(part),
        },
      };
    }),
  };
}

/** A stored comment of the round, wherever it sits, or nothing. */
export function findComment(review: Review, id: string): Comment | undefined {
  let found: Comment | undefined;
  mapComments(review, (list) => {
    found ??= list.find((c) => c.id === id);
    return [...list];
  });
  return found;
}

/**
 * The path of the card whose document holds a stored comment on a passage, or
 * nothing where the comment is on no passage.
 */
export function passagePath(review: Review, id: string): string | undefined {
  return (review.cards ?? []).find((card) =>
    (card.body?.passages ?? []).some((part) =>
      [part.passage, ...(part.fold ?? [])].some(
        (p) => p?.thread?.comments?.some((c) => c.id === id) === true,
      ),
    ),
  )?.path;
}

/**
 * The round with the bodies the app fetched put in the cards that have none. A
 * fetched body is kept apart from the round the cache holds, so a round fetched
 * again does not take it away, and is merged here for whatever reads or draws
 * the round.
 */
export function withBodies(
  review: Review,
  bodies: Readonly<Record<string, CardBody>>,
): Review {
  if (review.cards === null || Object.keys(bodies).length === 0) {
    return review;
  }
  return {
    ...review,
    cards: review.cards.map((card) => {
      const body = bodies[card.path];
      return card.body === null && body !== undefined
        ? { ...card, body }
        : card;
    }),
  };
}

/**
 * A card's body after a change to the round, taken from the body alone. The
 * round is given the card with that body, so a change that places a comment in
 * a card finds it, and what the change did to the card's body is read back.
 */
export function changeBody(
  review: Review,
  card: Card,
  body: CardBody,
  change: (round: Review) => Review,
): CardBody {
  const next = change({ ...review, cards: [{ ...card, body }] });
  return next.cards?.[0]?.body ?? body;
}
