import { matchPath } from "react-router";

/**
 * The addresses the app draws, as React Router patterns. The server answers
 * each with the shell, and a test holds the two lists to each other: the Go
 * route table's shell routes are these patterns written in net/http's syntax.
 * An address in neither list is a template page, which only a full load opens.
 */
export const drawnPatterns = [
  "/",
  "/reviews/:review",
  "/reviews/:review/rounds/:round",
  "/projects/:project",
  "/projects/:project/files/*",
  "/reviews/:review/files/*",
  "/reviews/:review/rounds/:round/files/*",
] as const;

/** One of the patterns the app draws. */
export type DrawnPattern = (typeof drawnPatterns)[number];

/**
 * Whether the app draws an address, which is the one place that is decided. A
 * link to such an address moves the router and any other link is a full load,
 * and a notification's tap moves the same way. An address on another origin is
 * never drawn, and neither is anything under /api, which no pattern matches.
 */
export function draws(address: string): boolean {
  const url = new URL(address, location.origin);
  if (url.origin !== location.origin) {
    return false;
  }
  return drawnPatterns.some(
    (path) => matchPath({ path, end: true }, url.pathname) !== null,
  );
}
