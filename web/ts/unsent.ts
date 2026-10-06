import { closeAnchored, updateGuard } from "./composer";
import { eltOf, targetOf, xhrOf } from "./events";

// What an unsent card holds: the form it came from, so Retry sends the same
// comment, with the same submission key, to the same place.
export interface Pending {
  readonly action: string;
  readonly target: string;
  readonly values: Readonly<Record<string, string>>;
}

const pending = new WeakMap<Element, Pending>();
let retrying: Element | null = null;

// catchFailedSaves turns a comment that could not be saved into an unsent card
// in its list, as the approved frame draws it, and closes the composer. Only a
// save that reached no server, or one the server failed, becomes a card: a
// form the server refused would be refused again, so its text stays in the
// composer with the server's reason.
export function catchFailedSaves(): void {
  document.addEventListener("htmx:sendError", onFailure);
  document.addEventListener("htmx:timeout", onFailure);
  document.addEventListener("htmx:responseError", onFailure);

  document.addEventListener("click", (event) => {
    const target = targetOf(event);
    const card = target?.closest(".comment.unsent") ?? null;
    if (card === null) {
      return;
    }
    if (target?.closest("[data-retry]")) {
      void retry(card);
    } else if (target?.closest("[data-discard]")) {
      card.remove();
      updateGuard();
    }
  });
  keepCards();
}

function onFailure(event: Event): void {
  const form = eltOf(event)?.closest<HTMLFormElement>("form.composer[hx-post]");
  if (!form) {
    return;
  }
  const status = xhrOf(event)?.status ?? 0;
  if (event.type === "htmx:responseError" && status < 500) {
    return;
  }
  toCard(form);
}

function toCard(form: HTMLFormElement): void {
  const template = document.getElementById("unsent-card");
  const shape =
    template instanceof HTMLTemplateElement
      ? template.content.firstElementChild
      : null;
  const copy = shape?.cloneNode(true);
  const where = form.getAttribute("hx-target");
  const into = where === null ? null : document.querySelector(where);
  const holder =
    into?.matches("tr") === true ? into.querySelector(".thread-body") : into;
  if (
    !(copy instanceof Element) ||
    holder === null ||
    holder === undefined ||
    where === null
  ) {
    return;
  }
  const values: Record<string, string> = {};
  for (const [name, value] of new FormData(form)) {
    if (typeof value === "string") {
      values[name] = value;
    }
  }
  const whereText = copy.querySelector(".where");
  const text = copy.querySelector("p");
  if (whereText) {
    whereText.textContent = form.querySelector(".anchor")?.textContent ?? "";
  }
  if (text) {
    text.textContent = values["body"] ?? "";
  }
  pending.set(copy, {
    action: form.getAttribute("hx-post") ?? "",
    target: where,
    values,
  });
  holder.appendChild(copy);
  form.querySelector(":scope > .failed")?.remove();
  if (form.closest("tr.thread, div[id^='thread-'], .filecomposer")) {
    closeAnchored();
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
}

// retry sends a card's comment again. On success the answer replaces the list
// the card sits in, which then holds the stored comment.
async function retry(card: Element): Promise<void> {
  const p = pending.get(card);
  if (p === undefined) {
    return;
  }
  retrying = card;
  try {
    await htmx.ajax("post", p.action, {
      target: p.target,
      swap: "outerHTML",
      values: p.values,
      source: card,
    });
  } finally {
    retrying = null;
    updateGuard();
  }
}

// keepCards carries the other unsent cards across a swap that replaces the
// list they sit in, which another comment's answer does.
function keepCards(): void {
  document.addEventListener("htmx:beforeSwap", (event) => {
    const d: unknown = event instanceof CustomEvent ? event.detail : null;
    const target =
      typeof d === "object" &&
      d !== null &&
      "target" in d &&
      d.target instanceof Element
        ? d.target
        : null;
    if (target === null || target.id === "") {
      return;
    }
    const cards = [...target.querySelectorAll(".comment.unsent")].filter(
      (c) => c !== retrying,
    );
    if (cards.length === 0) {
      return;
    }
    const id = target.id;
    // With no settle delay the swap happens in this task, so the new list is
    // in place by the next one.
    window.setTimeout(() => {
      const again = document.getElementById(id);
      const holder =
        again?.matches("tr") === true
          ? again.querySelector(".thread-body")
          : again;
      for (const card of cards) {
        holder?.appendChild(card);
      }
      updateGuard();
    }, 0);
  });
}

// SavedCard is an unsent card as a draft keeps it.
export interface SavedCard extends Pending {
  readonly where: string;
}

// unsentCards reads every card on the page, for a draft.
export function unsentCards(): SavedCard[] {
  const out: SavedCard[] = [];
  for (const card of document.querySelectorAll(".comment.unsent")) {
    const p = pending.get(card);
    if (p) {
      out.push({
        ...p,
        where: card.querySelector(".where")?.textContent ?? "",
      });
    }
  }
  return out;
}

// restoreCard puts a card a draft kept back in its list, when that list is on
// the page.
export function restoreCard(saved: SavedCard): void {
  const template = document.getElementById("unsent-card");
  const shape =
    template instanceof HTMLTemplateElement
      ? template.content.firstElementChild
      : null;
  const copy = shape?.cloneNode(true);
  const into = document.querySelector(saved.target);
  const holder =
    into?.matches("tr") === true ? into.querySelector(".thread-body") : into;
  if (!(copy instanceof Element) || holder === null || holder === undefined) {
    return;
  }
  const whereText = copy.querySelector(".where");
  const text = copy.querySelector("p");
  if (whereText) {
    whereText.textContent = saved.where;
  }
  if (text) {
    text.textContent = saved.values["body"] ?? "";
  }
  pending.set(copy, {
    action: saved.action,
    target: saved.target,
    values: saved.values,
  });
  holder.appendChild(copy);
}
