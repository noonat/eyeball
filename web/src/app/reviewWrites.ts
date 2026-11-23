import { tokenHeaders } from "../../ts/token";
import type {
  CardBody,
  CommentDeleted,
  CommentEdit,
  CommentRequest,
  CommentStored,
  DecisionRequest,
  Refusal,
} from "../api/types";

// The three sentences for an answer that carries none of its own, which are the
// template page's.
const noAnswer = "Cannot reach Eyeball.";
const anError = "Eyeball could not do that.";
const tooSlow =
  "Eyeball did not answer in time. Nothing is known to have been sent.";

// How long a write has to answer. The template page's htmx waited ten seconds.
const timeout = 10_000;

/**
 * How a write ended. A write the server took has its answer. A write it
 * refused, with a 4xx, has the sentence to show, and would be refused again, so
 * the text stays where it was typed. A write that may not have been stored, one
 * that reached no server, timed out or was failed by it, is lost: a comment
 * then becomes an unsent card to retry with the same key.
 */
export type Outcome<T> =
  | { readonly kind: "ok"; readonly value: T }
  | {
      readonly kind: "refused";
      readonly message: string;
      /** The status, which tells a change request with nothing in it, 422. */
      readonly status: number;
    }
  | { readonly kind: "lost"; readonly message: string };

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isComment(value: unknown): boolean {
  return (
    isRecord(value) &&
    typeof value["id"] === "string" &&
    typeof value["href"] === "string" &&
    typeof value["body"] === "string" &&
    typeof value["editable"] === "boolean"
  );
}

function isStored(value: unknown): value is CommentStored {
  return (
    isRecord(value) &&
    isComment(value["comment"]) &&
    typeof value["count"] === "number"
  );
}

function isDeleted(value: unknown): value is CommentDeleted {
  return isRecord(value) && typeof value["count"] === "number";
}

function isRefusal(value: unknown): value is Refusal {
  return isRecord(value) && typeof value["message"] === "string";
}

/** The sentence an answer that was not taken carries, or the fixed one. */
async function sentence(res: Response): Promise<string> {
  try {
    const body: unknown = await res.json();
    return isRefusal(body) && body.message !== "" ? body.message : anError;
  } catch {
    return anError;
  }
}

async function send<T>(
  method: string,
  address: string,
  body: unknown,
  isAnswer: (value: unknown) => value is T,
): Promise<Outcome<T>> {
  return exchange(method, address, body, async (res) => {
    const answer: unknown = await res.json();
    return isAnswer(answer) ? answer : undefined;
  });
}

async function exchange<T>(
  method: string,
  address: string,
  body: unknown,
  read: (res: Response) => Promise<T | undefined>,
  cancel?: AbortSignal,
): Promise<Outcome<T>> {
  const abort = new AbortController();
  const timer = window.setTimeout(() => {
    abort.abort();
  }, timeout);
  // A request the caller cancels must end, and not wait on the browser's cache
  // lock for its address, which held a second request for 20 seconds in
  // Chromium when the first never answered.
  const stop = (): void => {
    abort.abort();
  };
  cancel?.addEventListener("abort", stop);
  try {
    const res = await fetch(address, {
      method,
      signal: abort.signal,
      headers: {
        Accept: "application/json",
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
        // A read carries no token, as the template's htmx sent none.
        ...(method === "GET" ? {} : tokenHeaders()),
      },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    });
    if (res.status >= 500) {
      return { kind: "lost", message: await sentence(res) };
    }
    if (res.status >= 400) {
      return {
        kind: "refused",
        message: await sentence(res),
        status: res.status,
      };
    }
    const answer = await read(res);
    if (answer === undefined) {
      return { kind: "lost", message: anError };
    }
    return { kind: "ok", value: answer };
  } catch {
    return {
      kind: "lost",
      message: abort.signal.aborted ? tooSlow : noAnswer,
    };
  } finally {
    window.clearTimeout(timer);
    cancel?.removeEventListener("abort", stop);
  }
}

function isCardBody(value: unknown): value is CardBody {
  return isRecord(value) && typeof value["kind"] === "string";
}

/**
 * Reads one card's body, at the address its card gave. A body that fails
 * is drawn as a failed line with a retry, so a refusal and a failure to
 * arrive both carry the sentence to show.
 */
export function getBody(
  href: string,
  cancel: AbortSignal,
): Promise<Outcome<CardBody>> {
  return exchange(
    "GET",
    href,
    undefined,
    async (res) => {
      const answer: unknown = await res.json();
      return isCardBody(answer) ? answer : undefined;
    },
    cancel,
  );
}

/** Where a review's comments are written. */
export function commentsAddress(review: string): string {
  return `/api/reviews/${review}/comments`;
}

/** Sends a comment, which the server stores once for one submission key. */
export function postComment(
  review: string,
  request: CommentRequest,
): Promise<Outcome<CommentStored>> {
  return send("POST", commentsAddress(review), request, isStored);
}

/** Saves a comment's new text, at the address its view gave it. */
export function putComment(
  href: string,
  edit: CommentEdit,
): Promise<Outcome<CommentStored>> {
  return send("PUT", href, edit, isStored);
}

/** Deletes a comment, at the address its view gave it. */
export function deleteComment(href: string): Promise<Outcome<CommentDeleted>> {
  return send("DELETE", href, undefined, isDeleted);
}

/**
 * Sends the verdict, to the address the bar's view names. A stored verdict
 * answers with no body. A change request with nothing in it is refused with
 * 422, which the bar draws as its refused state, and any other refusal is a
 * sentence for the failed line.
 */
export function postVerdict(
  action: string,
  request: DecisionRequest,
): Promise<Outcome<true>> {
  return exchange("POST", action, request, (res) =>
    Promise.resolve(res.status === 204 ? true : undefined),
  );
}
