import { acting, anchor, updateGuard } from "./composer";
import { forget, recall, remember } from "./storage";
import { restoreCard, type SavedCard, unsentCards } from "./unsent";

// What a draft keeps for one round of one review: the verdict's note, the
// review's own box, the anchored composer with where it was opened, each open
// edit form by its comment, and every unsent card.
interface Drafts {
  note?: string;
  review?: string;
  anchored?: { readonly query: Record<string, string>; readonly text: string };
  edits?: Record<string, string>;
  unsent?: SavedCard[];
}

function isDrafts(v: unknown): v is Drafts {
  return typeof v === "object" && v !== null;
}

let restoring = false;

// keepDrafts saves what is typed as it is typed, which is what the tearout's
// "Saved as you type" promises, and puts it back on load. Each draft clears as
// the text it holds is stored, sent or discarded, because a save reads the
// page as it is.
export function keepDrafts(): void {
  const at = acting();
  if (at === null) {
    return;
  }
  const key = `drafts:${at.review}:${at.round}`;
  const save = (): void => {
    if (!restoring) {
      store(key);
    }
  };
  document.addEventListener("input", save);
  document.addEventListener("htmx:afterSwap", () => window.setTimeout(save, 0));
  document.addEventListener("click", () => window.setTimeout(save, 0));
  void restore(key);
}

function store(key: string): void {
  const d: Drafts = {};
  const note =
    document.querySelector<HTMLTextAreaElement>("#bar textarea")?.value ?? "";
  const review =
    document.querySelector<HTMLTextAreaElement>(
      ".page > form.composer textarea",
    )?.value ?? "";
  if (note.trim() !== "") {
    d.note = note;
  }
  if (review.trim() !== "") {
    d.review = review;
  }
  const open = document.querySelector<HTMLFormElement>(
    "form.composer[data-query]",
  );
  const text = open?.querySelector("textarea")?.value ?? "";
  if (open && text.trim() !== "") {
    const query: unknown = JSON.parse(open.dataset["query"] ?? "{}");
    if (typeof query === "object" && query !== null) {
      d.anchored = {
        query: Object.fromEntries(
          Object.entries(query).map(([k, v]) => [k, String(v)]),
        ),
        text,
      };
    }
  }
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "form.composer[hx-put]",
  )) {
    const body = form.querySelector("textarea")?.value ?? "";
    const id = form.id.replace(/^comment-/, "");
    if (body.trim() !== "" && id !== "") {
      d.edits = { ...d.edits, [id]: body };
    }
  }
  const unsent = unsentCards();
  if (unsent.length > 0) {
    d.unsent = unsent;
  }
  if (Object.keys(d).length === 0) {
    forget(key);
  } else {
    remember(key, d);
  }
}

async function restore(key: string): Promise<void> {
  const saved = recall(key);
  const at = acting();
  if (!isDrafts(saved) || at === null) {
    return;
  }
  restoring = true;
  try {
    const note = document.querySelector<HTMLTextAreaElement>("#bar textarea");
    if (note && saved.note !== undefined) {
      note.value = saved.note;
    }
    const review = document.querySelector<HTMLTextAreaElement>(
      ".page > form.composer textarea",
    );
    if (review && saved.review !== undefined) {
      review.value = saved.review;
    }
    for (const card of saved.unsent ?? []) {
      restoreCard(card);
    }
    for (const [id, body] of Object.entries(saved.edits ?? {})) {
      const card = document.getElementById(`comment-${id}`);
      if (card === null) {
        continue;
      }
      await htmx.ajax("get", `/reviews/${at.review}/comments/${id}?edit=1`, {
        target: card,
        swap: "outerHTML",
      });
      const field = document.querySelector<HTMLTextAreaElement>(
        `#comment-${id} textarea`,
      );
      if (field) {
        field.value = body;
      }
    }
    if (saved.anchored) {
      await reopen(saved.anchored.query, saved.anchored.text);
    }
  } finally {
    restoring = false;
    updateGuard();
  }
}

// reopen opens the anchored composer where a draft says it was, when that
// place is still on the page.
async function reopen(
  query: Record<string, string>,
  text: string,
): Promise<void> {
  const at = acting();
  const path = query["path"] ?? "";
  const file = [...document.querySelectorAll<HTMLElement>("[data-path]")].find(
    (e) => e.dataset["path"] === path,
  );
  if (at === null || file === undefined) {
    return;
  }
  const selector =
    query["level"] === "line"
      ? `tr[data-line="${query["start"] ?? ""}"]`
      : query["level"] === "passage"
        ? `.blk[data-lines="${query["start"] ?? ""}-${query["end"] ?? ""}"]`
        : query["oldline"] !== undefined
          ? `tr[data-old="${query["oldline"]}"]`
          : "";
  const row = selector === "" ? null : await arrival(file, selector);
  if (row === null && query["level"] !== "file") {
    return;
  }
  // A passage with no comment saved under it can sit in a fold, which would
  // hide the composer reopened there.
  const fold = row?.closest<HTMLDetailsElement>("details.fold");
  if (fold) {
    fold.open = true;
  }
  await anchor(at, file, row, query, text);
}

// arrival finds an element in a file card, waiting for a collapsed card's body
// to load first, since a card reopened on load fetches its body after the
// draft is read. It gives up after three seconds.
async function arrival(
  file: HTMLElement,
  selector: string,
): Promise<HTMLElement | null> {
  for (let waited = 0; waited < 3000; waited += 100) {
    const found = file.querySelector<HTMLElement>(selector);
    if (found !== null) {
      return found;
    }
    await new Promise((resolve) => window.setTimeout(resolve, 100));
  }
  return null;
}
