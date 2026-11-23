import { expect, test } from "vitest";
import type { Comment, Review } from "../api/types";
import { reviewCode, reviewDeferred } from "../fixtures";
import {
  findComment,
  placeComment,
  removeComment,
  replaceComment,
  changeBody,
  withBodies,
} from "./reviewCache";

const stored: Comment = {
  id: "9",
  href: "/api/reviews/1/comments/9",
  where: "line 2",
  quoted: "",
  body: "Why two?",
  late: false,
  editable: true,
};

// A deferred card of the long review, with the body the app fetched for it,
// which is the body of a card of a short one.
function fetchedBody(): { review: Review; path: string; merged: Review } {
  const card = reviewDeferred.cards?.[0];
  const body = reviewCode.cards?.[0]?.body;
  if (card === undefined || body === undefined || body === null) {
    throw new Error("the fixtures lack a card and a body");
  }
  return {
    review: reviewDeferred,
    path: card.path,
    merged: withBodies(reviewDeferred, { [card.path]: body }),
  };
}

test("a body fetched for a deferred card is merged into the round", () => {
  const { review, merged } = fetchedBody();
  expect(review.cards?.[0]?.body).toBeNull();
  expect(merged.cards?.[0]?.body?.kind).toBe("rows");
});

test("a comment written on a fetched body is placed in that body, and the merge into a newer round keeps it", () => {
  const { review, path, merged } = fetchedBody();
  const card = merged.cards?.[0];
  const old = card?.body;
  if (card === undefined || old === null || old === undefined) {
    throw new Error("the merged round has no body");
  }
  const body = changeBody(review, card, old, (round) =>
    placeComment(round, { level: "line", path, line: 2 }, stored, 1),
  );
  const thread = body.threads?.find((t) => t.line === 2);
  expect(thread?.comments?.map((c) => c.id)).toEqual(["9"]);
  // A round fetched again has no bodies, and the merge puts the body back with
  // its comment.
  const again = withBodies({ ...review, stamp: "newer" }, { [path]: body });
  expect(findComment(again, "9")?.body).toBe("Why two?");
});

test("two comments placed one after the other in a body are both kept", () => {
  const { review, path, merged } = fetchedBody();
  const card = merged.cards?.[0];
  const old = card?.body;
  if (card === undefined || old === null || old === undefined) {
    throw new Error("the merged round has no body");
  }
  const place = (id: string) => (round: Review) =>
    placeComment(round, { level: "line", path, line: 2 }, { ...stored, id }, 1);
  const first = changeBody(review, card, old, place("9"));
  const second = changeBody(review, card, first, place("10"));
  expect(
    second.threads?.find((t) => t.line === 2)?.comments?.map((c) => c.id),
  ).toEqual(["9", "10"]);
});

test("a comment on a fetched body is edited and deleted there", () => {
  const { path, merged } = fetchedBody();
  const placed = placeComment(
    merged,
    { level: "line", path, line: 2 },
    stored,
    1,
  );
  const edited = replaceComment(
    placed,
    { ...stored, body: "Why two lines?" },
    1,
  );
  expect(findComment(edited, "9")?.body).toBe("Why two lines?");
  const gone = removeComment(edited, "9", 0);
  expect(findComment(gone, "9")).toBeUndefined();
});

test("a card that has its own body keeps it", () => {
  const card = reviewCode.cards?.[0];
  if (card === undefined || card.body === null) {
    throw new Error("the fixture lacks a card with a body");
  }
  const merged = withBodies(reviewCode, {
    [card.path]: { ...card.body, mode: "changed" },
  });
  expect(merged.cards?.[0]?.body).toEqual(card.body);
});
