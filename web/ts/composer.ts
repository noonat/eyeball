import { classes } from "./classes";
import { eltOf, succeeded, targetOf, verbOf } from "./events";

// The page's acting data, set by the server on .page: which review and round
// every write names, and whether the round is open to taps.
interface Acting {
  readonly review: string;
  readonly round: string;
  readonly against: string;
}

function acting(): Acting | null {
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
    const at = acting();
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
    if (target?.closest(".toobig")) {
      void anchor(at, file, null, { level: "file", path });
    }
  });
}

// anchor moves the one composer to a new place, keeping what was typed. A
// second composer for each tap would scatter half-written comments through a
// long review.
async function anchor(
  at: Acting,
  file: HTMLElement,
  row: HTMLElement | null,
  query: Record<string, string>,
): Promise<void> {
  const typed = closeAnchored();
  for (const sel of document.querySelectorAll(`tr.${classes.sel}`)) {
    sel.classList.remove(classes.sel);
  }
  row?.classList.add(classes.sel);
  const slot = file.querySelector<HTMLElement>("[id^='slot-']");
  if (slot === null) {
    return;
  }
  const params = new URLSearchParams({ round: at.round, ...query });
  await htmx.ajax(
    "get",
    `/reviews/${at.review}/composer?${params.toString()}`,
    {
      target: slot,
      swap: "outerHTML",
    },
  );
  const opened = document.getElementById(slot.id);
  const field = opened?.querySelector<HTMLTextAreaElement>("textarea");
  if (field && typed !== "") {
    field.value = typed;
  }
  opened?.scrollIntoView({ block: "nearest" });
  field?.focus({ preventScroll: true });
  updateGuard();
}

// closeAnchored puts an anchored composer's slot back empty and returns what
// it held.
function closeAnchored(): string {
  const open = document.querySelector<HTMLElement>(
    ".filecomposer[id^='slot-']",
  );
  if (open === null) {
    return "";
  }
  const typed = open.querySelector("textarea")?.value ?? "";
  const slot = document.createElement("div");
  slot.id = open.id;
  open.replaceWith(slot);
  return typed;
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
    if (form.closest(".filecomposer[id^='slot-']")) {
      closeAnchored();
      for (const sel of document.querySelectorAll(`tr.${classes.sel}`)) {
        sel.classList.remove(classes.sel);
      }
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

// unsaved says whether any composer or edit form holds text not yet stored.
// The verdict's note is not a comment, so it does not count.
function unsaved(): boolean {
  for (const field of document.querySelectorAll<HTMLTextAreaElement>(
    "form.composer textarea",
  )) {
    if (field.value.trim() !== "") {
      return true;
    }
  }
  return false;
}

// updateGuard shows the bar's notice and marks both verdicts unavailable while
// text is unsaved. A verdict sent then would release the round without that
// comment.
function updateGuard(): void {
  const notice = document.getElementById("unsaved");
  const held = unsaved();
  if (notice) {
    notice.hidden = !held;
  }
  for (const button of document.querySelectorAll("#bar .buttons button")) {
    if (held) {
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
  document.addEventListener("htmx:afterSwap", updateGuard);
  document.addEventListener("htmx:beforeRequest", (event) => {
    if (eltOf(event)?.id === "bar" && unsaved()) {
      updateGuard();
      event.preventDefault();
    }
  });
}
