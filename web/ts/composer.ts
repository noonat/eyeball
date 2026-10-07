import { classes } from "./classes";
import { eltOf, succeeded, targetOf, verbOf } from "./events";
import { isOffline, unsavedText } from "./state";

// The page's acting data, set by the server on .page: which review and round
// every write names, and whether the round is open to taps.
export interface Acting {
  readonly review: string;
  readonly round: string;
  readonly against: string;
}

export function acting(): Acting | null {
  const page = document.querySelector<HTMLElement>(".page[data-acting='open']");
  const { review, round } = page?.dataset ?? {};
  if (page === null || review === undefined || round === undefined) {
    return null;
  }
  return { review, round, against: page.dataset["against"] ?? "" };
}

// openComposers wires a tap on a line to the one anchored composer.
export function openComposers(): void {
  document.addEventListener("click", (event) => {
    const target = targetOf(event);
    const chip = target?.closest<HTMLButtonElement>(".composer .chip");
    if (chip) {
      switchLevel(chip);
      return;
    }
    if (target?.closest(".composer [data-cancel]")) {
      closeAnchored();
      clearSelection();
      updateGuard();
      return;
    }
    const at = acting();
    const block = target?.closest<HTMLElement>(".prose .blk[data-lines]");
    if (at !== null && block) {
      openPassage(event, at, block);
      return;
    }
    const file = target?.closest<HTMLElement>("details.file[data-path]");
    if (at === null || file === null || file === undefined) {
      return;
    }
    const row = target?.closest<HTMLElement>("tr[data-line], tr[data-old]");
    const path = file.dataset["path"] ?? "";
    if (row) {
      const query: Record<string, string> = { path };
      if (row.dataset["line"] !== undefined) {
        query["level"] = "line";
        query["start"] = row.dataset["line"];
      } else {
        query["level"] = "file";
        query["oldline"] = row.dataset["old"] ?? "";
        query["against"] = at.against;
      }
      void anchor(at, file, row, query);
      return;
    }
    // A file shown as itself takes a comment on the file, as the line naming
    // a file with no rows does. A player and the Open link keep their taps.
    if (
      target?.closest(".toobig, .filefacts, .rendered .shot, .rendered .cap") &&
      !target.closest("a")
    ) {
      void anchor(at, file, null, { level: "file", path });
    }
  });
}

// openPassage opens the composer under a tapped passage. A link inside it still
// follows, and the hint is the one control that shows what a changed passage
// replaced, as the owner decided, so a tap anywhere else on a changed passage
// cancels its summary's toggle.
function openPassage(event: Event, at: Acting, block: HTMLElement): void {
  const target = targetOf(event);
  if (target?.closest("a") || target?.closest(".hint")) {
    return;
  }
  const [start, end] = (block.dataset["lines"] ?? "").split("-");
  const path = block.closest<HTMLElement>("[data-path]")?.dataset["path"];
  if (start === undefined || end === undefined || path === undefined) {
    return;
  }
  if (block.tagName === "DETAILS") {
    event.preventDefault();
  }
  const file = block.closest<HTMLElement>("[data-path]");
  if (file) {
    void anchor(at, file, block, { level: "passage", path, start, end });
  }
}

// anchor moves the one composer to a new place, keeping what was typed. A
// second composer for each tap would scatter half-written comments through a
// long review.
export async function anchor(
  at: Acting,
  file: HTMLElement,
  row: HTMLElement | null,
  query: Record<string, string>,
  text = "",
): Promise<void> {
  const typed = closeAnchored() || text;
  clearSelection();
  row?.classList.add(classes.sel);
  const params = new URLSearchParams({ round: at.round, ...query });
  const url = `/reviews/${at.review}/composer?${params.toString()}`;
  // A line's composer opens in the row under the line, replacing the row its
  // comments already sit in, or inserted after the line when it has none.
  // Anything else fills the file's slot.
  let opened: Element | null = null;
  if (row) {
    const thread = row.nextElementSibling;
    if (thread?.matches(threadGroup) === true) {
      await htmx.ajax("get", url, { target: thread, swap: "outerHTML" });
    } else {
      await htmx.ajax("get", url, { target: row, swap: "afterend" });
    }
    opened = row.nextElementSibling;
  } else {
    const slot = file.querySelector<HTMLElement>("[id^='slot-']");
    if (slot === null) {
      return;
    }
    await htmx.ajax("get", url, { target: slot, swap: "outerHTML" });
    opened = document.getElementById(slot.id);
  }
  const form = opened?.querySelector<HTMLFormElement>("form.composer");
  const field = form?.querySelector<HTMLTextAreaElement>("textarea");
  if (form) {
    // Kept so a draft can reopen the composer where it was.
    form.dataset["query"] = JSON.stringify(query);
  }
  if (field && typed !== "") {
    field.value = typed;
  }
  opened?.scrollIntoView({ block: "nearest" });
  field?.focus({ preventScroll: true });
  updateGuard();
}

function clearSelection(): void {
  for (const sel of document.querySelectorAll(`.${classes.sel}`)) {
    sel.classList.remove(classes.sel);
  }
}

// anchoredForm finds a composer opened at a line or a file, rather than the
// review's own box or a comment's edit form.
const anchoredForm =
  "tr.thread form.composer[hx-post], div[id^='thread-'] form.composer[hx-post], .filecomposer[id^='slot-'] form.composer";

// threadGroup finds the group under a line or a passage that holds its
// comments.
const threadGroup = "tr.thread, div[id^='thread-']";

// closeAnchored puts an anchored composer's slot back empty and returns what
// it held.
export function closeAnchored(): string {
  const form = document.querySelector<HTMLFormElement>(anchoredForm);
  if (form === null) {
    return "";
  }
  const typed = form.querySelector("textarea")?.value ?? "";
  const slot = form.closest<HTMLElement>(".filecomposer[id^='slot-']");
  if (slot) {
    const empty = document.createElement("div");
    empty.id = slot.id;
    slot.replaceWith(empty);
    return typed;
  }
  const thread = form.closest(threadGroup);
  form.remove();
  dropEmptyThread(thread);
  return typed;
}

// dropEmptyThread removes a line's row once nothing is left in it, so a line
// with no comments carries no empty row.
function dropEmptyThread(thread: Element | null): void {
  const holder =
    thread?.matches("tr") === true
      ? thread.querySelector(".thread-body")
      : thread;
  if (thread && holder?.childElementCount === 0) {
    thread.remove();
  }
}

// switchLevel moves the composer's text to another level without a request:
// the fields that level does not take are disabled, so the form sends only
// what the server accepts, and the answer goes to that level's list.
function switchLevel(chip: HTMLButtonElement): void {
  const form = chip.closest("form");
  const level = chip.dataset["level"];
  if (form === null || level === undefined) {
    return;
  }
  const takes: Record<string, readonly string[]> = {
    review: [],
    file: ["path", "oldline", "against"],
    line: ["path", "start", "end"],
    passage: ["path", "start", "end"],
  };
  const allowed = takes[level] ?? [];
  for (const input of form.querySelectorAll<HTMLInputElement>(
    "input[type='hidden']",
  )) {
    if (input.name === "level") {
      input.value = level;
    } else if (
      ["path", "start", "end", "oldline", "against"].includes(input.name)
    ) {
      input.disabled = !allowed.includes(input.name);
    }
  }
  const target = chip.dataset["target"];
  if (target !== undefined) {
    form.setAttribute("hx-target", target);
  }
  for (const other of form.querySelectorAll(".chip")) {
    other.setAttribute("aria-pressed", String(other === chip));
  }
}

// keyComments gives a comment its submission key the first time it is sent,
// and keeps it for every retry, so a send that was stored after all is not
// stored twice. The capture phase runs before htmx's own listener on the form.
export function keyComments(): void {
  document.addEventListener(
    "submit",
    (event) => {
      const key = targetOf(event)?.querySelector<HTMLInputElement>(
        "input[name='submission']",
      );
      if (key && key.value === "") {
        key.value = crypto.randomUUID();
      }
    },
    true,
  );
}

// settleComposers resets a composer once its comment is stored: an anchored
// one closes, and the review's own box empties for the next comment.
export function settleComposers(): void {
  document.addEventListener("htmx:afterRequest", (event) => {
    const form = eltOf(event)?.closest("form.composer") ?? null;
    if (!succeeded(event) || form === null || verbOf(event) !== "post") {
      updateGuard();
      return;
    }
    // A line comment's answer replaces the row the form was in, so a form no
    // longer on the page was an anchored one too.
    if (
      !form.isConnected ||
      form.closest(`.filecomposer[id^='slot-'], ${threadGroup}`)
    ) {
      closeAnchored();
      clearSelection();
    } else {
      const field = form.querySelector("textarea");
      const key = form.querySelector<HTMLInputElement>(
        "input[name='submission']",
      );
      if (field) {
        field.value = "";
      }
      if (key) {
        key.value = "";
      }
    }
    updateGuard();
  });
}

// updateGuard shows the bar's notice and marks both verdicts unavailable while
// text is unsaved. A verdict sent then would release the round without that
// comment.
export function updateGuard(): void {
  const notice = document.getElementById("unsaved");
  const whyNot = document.getElementById("whynot");
  const held = unsavedText(false);
  const down = isOffline();
  const unsent = document.querySelectorAll(".comment.unsent").length;
  if (notice) {
    notice.hidden = !held;
  }
  const unsentNotice = document.getElementById("unsent");
  const count = document.getElementById("unsent-count");
  if (unsentNotice) {
    unsentNotice.hidden = unsent === 0;
  }
  if (count) {
    count.textContent =
      unsent === 1 ? "1 comment" : `${String(unsent)} comments`;
  }
  if (whyNot) {
    whyNot.hidden = !down;
  }
  for (const button of document.querySelectorAll("#bar .buttons button")) {
    if (held || down || unsent > 0) {
      button.setAttribute("aria-disabled", "true");
    } else {
      button.removeAttribute("aria-disabled");
    }
  }
}

// guardVerdicts keeps the guard current as text is typed, and cancels a
// verdict sent while it holds. aria-disabled does not stop a button.
export function guardVerdicts(): void {
  document.addEventListener("input", updateGuard);
  document.addEventListener("eyeball:connection", updateGuard);
  // A line comment's answer replaces the row its form was in, so htmx's
  // afterRequest for it fires on a detached form and never reaches the
  // document. The outline matters only while a composer is open, so every
  // swap clears it once none is.
  document.addEventListener("htmx:afterSwap", () => {
    for (const thread of document.querySelectorAll(threadGroup)) {
      dropEmptyThread(thread);
    }
    if (document.querySelector(anchoredForm) === null) {
      clearSelection();
    }
    updateGuard();
  });
  document.addEventListener("htmx:beforeRequest", (event) => {
    if (
      eltOf(event)?.id === "bar" &&
      (unsavedText(false) ||
        isOffline() ||
        document.querySelector(".comment.unsent") !== null)
    ) {
      updateGuard();
      event.preventDefault();
    }
  });
}
