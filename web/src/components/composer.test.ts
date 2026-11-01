import { expect, test } from "vitest";
import { commentRequest } from "./composer";
import type { CommentSend, ComposerAt } from "./composer";

// Every composer answer the template routes gave, read as the page's markup.
const answers = import.meta.glob<string>("../fixtures/review-composer-*.html", {
  query: "?raw",
  import: "default",
  eager: true,
});

// Where each answer opened and what the page around it was showing. The page
// is not in the answer, so the round and the comparison are written here.
interface Opened {
  readonly at: ComposerAt;
  readonly round: number;
  readonly against: "base" | "";
}

const opened: Readonly<Record<string, Opened>> = {
  "review-composer-review.html": {
    at: { level: "review" },
    round: 1,
    against: "",
  },
  "review-composer-late-review.html": {
    at: { level: "review" },
    round: 1,
    against: "",
  },
  "review-composer-file.html": {
    at: { level: "file", path: "internal/upload/backoff.go" },
    round: 1,
    against: "",
  },
  "review-composer-late.html": {
    at: { level: "file", path: "internal/approved/limit.go" },
    round: 1,
    against: "",
  },
  "review-composer-removed.html": {
    at: { level: "file", path: "internal/upload/worker.go", oldLine: 13 },
    round: 1,
    against: "",
  },
  "review-composer-whole.html": {
    at: { level: "file", path: "docs/product.md" },
    round: 2,
    against: "base",
  },
  "review-composer-line.html": {
    at: { level: "line", path: "internal/upload/worker.go", line: 13 },
    round: 1,
    against: "",
  },
  "review-composer-line-empty.html": {
    at: { level: "line", path: "internal/upload/worker.go", line: 12 },
    round: 1,
    against: "",
  },
  "review-composer-passage.html": {
    at: { level: "passage", path: "docs/product.md", first: 9, last: 9 },
    round: 2,
    against: "",
  },
  "review-composer-passage-commented.html": {
    at: { level: "passage", path: "docs/product.md", first: 7, last: 7 },
    round: 2,
    against: "",
  },
};

/** The hidden fields of the answer's form that carry a value. */
function hiddenFields(markup: string): Record<string, string> {
  const doc = new DOMParser().parseFromString(markup, "text/html");
  const fields: Record<string, string> = {};
  for (const input of doc.querySelectorAll("input[type=hidden]")) {
    const name = input.getAttribute("name");
    const value = input.getAttribute("value") ?? "";
    // The key is drawn by the page's script when it first sends, so the
    // template's field is always empty and the app's is not compared.
    if (name !== null && name !== "submission" && value !== "") {
      fields[name] = value;
    }
  }
  return fields;
}

/**
 * The fields of a request that carry a value, under the names the form sent
 * them as. The server reads zero and empty as absent.
 */
function requestFields(row: Opened): Record<string, string> {
  const send: CommentSend = {
    round: row.round,
    against: row.against,
    body: "A point.",
    submission: "key-1",
  };
  const request = commentRequest(row.at, send);
  const named: readonly (readonly [string, string | number])[] = [
    ["round", request.round],
    ["level", request.level],
    ["path", request.path],
    ["start", request.start],
    ["end", request.end],
    ["oldline", request.old_line],
    ["against", request.against],
  ];
  const fields: Record<string, string> = {};
  for (const [name, value] of named) {
    if (value !== "" && value !== 0) {
      fields[name] = String(value);
    }
  }
  return fields;
}

function byName(a: string, b: string): number {
  return a.localeCompare(b);
}

/** The file's name, which is what the rows above are keyed by. */
function fileName(path: string): string {
  return path.split("/").pop() ?? path;
}

test("every committed composer answer has a row here", () => {
  const files = Object.keys(answers).map(fileName);
  expect(files.toSorted(byName)).toEqual(Object.keys(opened).toSorted(byName));
});

test.each(Object.entries(answers))(
  "the request for %s sends its hidden fields",
  (path, markup) => {
    const row = opened[fileName(path)];
    expect(row).toBeDefined();
    if (row === undefined) {
      return;
    }
    expect(requestFields(row)).toEqual(hiddenFields(markup));
  },
);

test("a request carries the text and the key it was given", () => {
  const request = commentRequest(
    { level: "review" },
    { round: 3, against: "", body: "Text.", submission: "key-9" },
  );
  expect(request.body).toBe("Text.");
  expect(request.submission).toBe("key-9");
  expect(request.round).toBe(3);
});

test("a passage of several lines sends both ends of its range", () => {
  // Every committed passage answer opens on one line, so the fixtures cannot
  // tell the last line from the first.
  const request = commentRequest(
    { level: "passage", path: "docs/product.md", first: 3, last: 5 },
    { round: 2, against: "", body: "Text.", submission: "key-1" },
  );
  expect([request.start, request.end]).toEqual([3, 5]);
});
