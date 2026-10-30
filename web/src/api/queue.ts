import type { Queue, Row } from "./types";

/** The query key of the queue's view. */
export const queueKey = ["queue"] as const;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isRow(value: unknown): value is Row {
  return (
    isRecord(value) &&
    typeof value["id"] === "string" &&
    typeof value["project"] === "string" &&
    typeof value["agent"] === "string" &&
    typeof value["title"] === "string" &&
    typeof value["age"] === "string" &&
    typeof value["round"] === "number" &&
    typeof value["files"] === "string" &&
    typeof value["added"] === "string" &&
    typeof value["removed"] === "string" &&
    typeof value["gist"] === "string"
  );
}

function isSection(value: unknown): value is Row[] | null {
  return value === null || (Array.isArray(value) && value.every(isRow));
}

/** Whether a decoded body has the shape the generated types promise. */
export function isQueue(value: unknown): value is Queue {
  return (
    isRecord(value) &&
    isSection(value["waiting"]) &&
    isSection(value["working"]) &&
    isSection(value["settled"]) &&
    typeof value["waiting_count"] === "number" &&
    typeof value["stamp"] === "string"
  );
}

/**
 * Reads the queue's view. A body that is not the shape the types say is an
 * error, so a server and an app that disagree fail here and not on a screen.
 */
export async function fetchQueue(): Promise<Queue> {
  const res = await fetch("/api/", { headers: { Accept: "application/json" } });
  if (!res.ok) {
    throw new Error(`GET /api/ answered ${res.status}`);
  }
  const body: unknown = await res.json();
  if (!isQueue(body)) {
    throw new Error("GET /api/ answered a body that is not the queue's view");
  }
  return body;
}
