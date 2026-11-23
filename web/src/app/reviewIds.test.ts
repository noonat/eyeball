import { expect, test } from "vitest";
import type { Card, CardBody, Review } from "../api/types";
import { isReview } from "../api/views";
import { composerId } from "../components/composerAt";
import { targetOf } from "./reviewDrafts";

// The views the server sent, which `make api-check` keeps equal to what a fresh
// run of Go writes. The app builds the id of a thread or a slot that does not
// exist yet from the card's id, and Go builds the same ids from the path. Each
// side formats its own, so a change to either fails here once the fixtures are
// written again.
const views = import.meta.glob<unknown>("../fixtures/review-*.json", {
  import: "default",
  eager: true,
});

function reviews(): readonly Review[] {
  return Object.values(views).filter(isReview);
}

function bodies(): readonly { card: Card; body: CardBody }[] {
  return reviews().flatMap((review) =>
    (review.cards ?? []).flatMap((card) =>
      card.body === null ? [] : [{ card, body: card.body }],
    ),
  );
}

test("the id of a line's thread is the one the server sends", () => {
  const threads = bodies().flatMap(({ card, body }) =>
    (body.threads ?? []).map((thread) => ({ card, thread })),
  );
  expect(threads.length).toBeGreaterThan(0);
  for (const { card, thread } of threads) {
    const at = { level: "line", path: card.path, line: thread.line } as const;
    expect(composerId(at, card.id)).toBe(thread.id);
    expect(targetOf(at, card.id)).toBe(`#${thread.id}`);
  }
});

test("the id of a passage's thread is the one the server sends", () => {
  const passages = bodies().flatMap(({ card, body }) =>
    (body.passages ?? []).flatMap((part) =>
      [part.passage, ...(part.fold ?? [])].flatMap((passage) =>
        passage?.thread == null
          ? []
          : [{ card, passage, thread: passage.thread }],
      ),
    ),
  );
  expect(passages.length).toBeGreaterThan(0);
  for (const { card, passage, thread } of passages) {
    const at = {
      level: "passage",
      path: card.path,
      first: passage.first,
      last: passage.last,
    } as const;
    expect(composerId(at, card.id)).toBe(thread.id);
  }
});

test("the ids of a file's slot and its list are the ones the server sends", () => {
  const slots = bodies().filter(({ body }) => body.slot !== "");
  expect(slots.length).toBeGreaterThan(0);
  for (const { card, body } of slots) {
    const at = { level: "file", path: card.path } as const;
    expect(composerId(at, card.id)).toBe(body.slot);
    expect(targetOf(at, card.id)).toBe(`#${body.list_id}`);
  }
});

test("the id of the review's own list is the one the server sends", () => {
  const lists = reviews().flatMap((review) =>
    review.acting === null ? [] : [review.acting.list_id],
  );
  expect(lists.length).toBeGreaterThan(0);
  for (const id of lists) {
    expect(composerId({ level: "review" }, "")).toBe(id);
  }
});
