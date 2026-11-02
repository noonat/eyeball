import { expect, test } from "vitest";
import { anchorOf } from "../app/reviewDrafts";
import { composerScenes } from "../fixtures/composers";
import {
  commentRequest,
  type CommentSend,
  type ComposerAt,
} from "./composerAt";

// Every composer answer the template routes gave, read as the page's markup.
const answers = import.meta.glob<string>("../fixtures/review-composer-*.html", {
  query: "?raw",
  import: "default",
  eager: true,
});

/** The file's name, which is what the scenes are keyed by. */
function fileName(path: string): string {
  return path.split("/").pop() ?? path;
}

function byName(a: string, b: string): number {
  return a.localeCompare(b);
}

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
function requestFields(
  at: ComposerAt,
  round: number,
  against: "base" | "",
): Record<string, string> {
  const request = commentRequest(at, {
    round,
    against,
    body: "A point.",
    submission: "key-1",
  });
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

test("every committed composer answer has a scene that wrote it", () => {
  const files = Object.keys(answers).map(fileName);
  const scenes = Object.keys(composerScenes).map((name) => `${name}.html`);
  expect(files.toSorted(byName)).toEqual(scenes.toSorted(byName));
});

// The anchor and the page's round and comparison are read from the request
// that wrote each answer, so the test holds the app's request to the answer
// with no table of its own to keep in step.
test.each(Object.entries(composerScenes))(
  "the request for %s sends its hidden fields",
  (name, query) => {
    const markup = answers[`../fixtures/${name}.html`];
    expect(markup).toBeDefined();
    const at = anchorOf(query);
    expect(at).toBeDefined();
    if (at === undefined || markup === undefined) {
      return;
    }
    const against = query["against"] === "base" ? "base" : "";
    const round = Number(query["round"]);
    expect(requestFields(at, round, against)).toEqual(hiddenFields(markup));
  },
);

test("a passage of several lines sends both ends of its range", () => {
  // Every committed passage answer opens on one line, so the fixtures cannot
  // tell the last line from the first.
  const request = commentRequest(
    { level: "passage", path: "docs/product.md", first: 3, last: 5 },
    { round: 2, against: "", body: "Text.", submission: "key-1" },
  );
  expect([request.start, request.end]).toEqual([3, 5]);
});

const send: CommentSend = {
  round: 1,
  against: "base",
  body: "Text.",
  submission: "k",
};

// A level the chips chose sends the fields that level takes and no others, as
// the template's form did by disabling the rest. The Go test of the JSON route
// sends these same bodies.
test.each([
  {
    name: "a line sent as the file",
    at: { level: "line", path: "a.txt", line: 2 } as const,
    level: "file" as const,
    want: {
      level: "file",
      path: "a.txt",
      start: 0,
      end: 0,
      old_line: 0,
      against: "",
    },
  },
  {
    name: "a line sent as the review",
    at: { level: "line", path: "a.txt", line: 2 } as const,
    level: "review" as const,
    want: {
      level: "review",
      path: "",
      start: 0,
      end: 0,
      old_line: 0,
      against: "",
    },
  },
  {
    name: "a passage sent as the file",
    at: { level: "passage", path: "a.txt", first: 1, last: 2 } as const,
    level: "file" as const,
    want: {
      level: "file",
      path: "a.txt",
      start: 0,
      end: 0,
      old_line: 0,
      against: "",
    },
  },
  {
    name: "a passage sent as the review",
    at: { level: "passage", path: "a.txt", first: 1, last: 2 } as const,
    level: "review" as const,
    want: {
      level: "review",
      path: "",
      start: 0,
      end: 0,
      old_line: 0,
      against: "",
    },
  },
  {
    name: "a removed row sent as the file, which it is",
    at: { level: "file", path: "a.txt", oldLine: 1 } as const,
    level: "file" as const,
    want: {
      level: "file",
      path: "a.txt",
      start: 0,
      end: 0,
      old_line: 1,
      against: "base",
    },
  },
  {
    name: "a removed row sent as the review",
    at: { level: "file", path: "a.txt", oldLine: 1 } as const,
    level: "review" as const,
    want: {
      level: "review",
      path: "",
      start: 0,
      end: 0,
      old_line: 0,
      against: "",
    },
  },
])("$name", ({ at, level, want }) => {
  const request = commentRequest(at, send, level);
  expect({
    level: request.level,
    path: request.path,
    start: request.start,
    end: request.end,
    old_line: request.old_line,
    against: request.against,
  }).toEqual(want);
});

test("a request carries the text and the key it was given", () => {
  const request = commentRequest(
    { level: "review" },
    { round: 3, against: "", body: "Text.", submission: "key-9" },
  );
  expect(request.body).toBe("Text.");
  expect(request.submission).toBe("key-9");
  expect(request.round).toBe(3);
});
