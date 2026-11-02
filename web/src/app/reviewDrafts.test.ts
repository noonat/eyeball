import { afterEach, expect, test } from "vitest";
import { forget, recall, remember } from "../../ts/storage";
import { reviewCode } from "../fixtures";
import {
  anchorOf,
  draftKey,
  queryOf,
  readDrafts,
  recallDrafts,
  requestOf,
  storeDrafts,
  valuesOf,
} from "./reviewDrafts";
import { draftOf, outboxOf } from "./useReview";

const key = draftKey("7", 1);

afterEach(() => {
  forget(key);
});

// What the template page saved for one round, as its script wrote it: the
// verdict's note, the review's box, the composer with the query it was opened
// by, an edit form by its comment, and a card it could not send with the form it
// came from.
const template = {
  note: "A note in progress",
  review: "Half a review comment",
  anchored: {
    query: { path: "a.txt", level: "line", start: "2" },
    text: "About line two",
  },
  edits: { "5": "Edited text" },
  unsent: [
    {
      action: "/reviews/7/comments",
      target: "#comments-review",
      where: "This review",
      values: {
        round: "1",
        level: "review",
        body: "Never sent",
        submission: "first-key",
      },
    },
  ],
};

test("a draft of all five parts, stored as the template stored it, is restored", () => {
  remember(key, template);
  const saved = recallDrafts("7", 1);
  expect(saved.note).toBe("A note in progress");
  expect(saved.review).toBe("Half a review comment");
  expect(saved.anchored?.text).toBe("About line two");
  expect(anchorOf(saved.anchored?.query ?? {})).toEqual({
    level: "line",
    path: "a.txt",
    line: 2,
  });
  expect(saved.edits).toEqual({ "5": "Edited text" });
  expect(saved.unsent).toEqual(template.unsent);
});

test("the unsent card's retry sends the key it was first sent under", () => {
  remember(key, template);
  const [item] = outboxOf(recallDrafts("7", 1).unsent ?? []);
  if (item === undefined) {
    throw new Error("the card was not restored");
  }
  expect(item.state).toBe("unsent");
  expect(item.request.submission).toBe("first-key");
  expect(item.request.body).toBe("Never sent");
  expect(item.where).toBe("This review");
  // Saved again, as the app saves a card, it keeps the same key, and the
  // values read back make the same request.
  const again = valuesOf(item.request);
  expect(again["submission"]).toBe("first-key");
  expect(requestOf(again)).toEqual(item.request);
});

test("a draft the app saves is the shape the template reads", () => {
  const saved = draftOf(
    {
      note: "A note in progress",
      reviewText: "Half a review comment",
      composer: { level: "line", path: "a.txt", line: 2 },
      composerText: "About line two",
      edits: { "5": "Edited text" },
      outbox: [
        {
          id: "pending-1",
          request: {
            round: 1,
            level: "review",
            path: "",
            start: 0,
            end: 0,
            old_line: 0,
            against: "",
            body: "Never sent",
            submission: "first-key",
          },
          where: "This review",
          state: "unsent",
        },
        {
          id: "pending-2",
          request: {
            round: 1,
            level: "review",
            path: "",
            start: 0,
            end: 0,
            old_line: 0,
            against: "",
            body: "Still on its way",
            submission: "second-key",
          },
          where: "This review",
          state: "sending",
        },
      ],
    },
    { ...reviewCode, id: "7" },
    "",
  );
  storeDrafts("7", 1, saved);
  expect(recall(key)).toEqual({
    note: "A note in progress",
    review: "Half a review comment",
    anchored: template.anchored,
    edits: { "5": "Edited text" },
    unsent: template.unsent,
  });
});

test("a draft with nothing in it is forgotten", () => {
  remember(key, template);
  storeDrafts("7", 1, {});
  expect(recall(key)).toBeNull();
});

test("a part with the wrong shape is dropped alone", () => {
  const saved = readDrafts({
    note: 3,
    review: "Kept",
    anchored: { query: "x", text: 4 },
    edits: { "1": "kept", "2": 9 },
    unsent: [{ action: "x" }, template.unsent[0]],
  });
  expect(saved.note).toBeUndefined();
  expect(saved.review).toBe("Kept");
  expect(saved.anchored).toBeUndefined();
  expect(saved.edits).toEqual({ "1": "kept" });
  expect(saved.unsent).toEqual(template.unsent);
});

test("the query a tap opened a composer with is the template's", () => {
  expect(queryOf({ level: "line", path: "a.txt", line: 2 }, "base")).toEqual({
    level: "line",
    path: "a.txt",
    start: "2",
  });
  expect(
    queryOf({ level: "passage", path: "d.md", first: 3, last: 5 }, ""),
  ).toEqual({ level: "passage", path: "d.md", start: "3", end: "5" });
  expect(queryOf({ level: "file", path: "a.txt", oldLine: 3 }, "base")).toEqual(
    { level: "file", path: "a.txt", oldline: "3", against: "base" },
  );
  expect(queryOf({ level: "file", path: "a.txt" }, "base")).toEqual({
    level: "file",
    path: "a.txt",
  });
});
