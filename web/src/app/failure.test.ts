import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, test } from "vitest";
import { readFailure } from "./failure";

afterEach(() => {
  document.head.querySelector("meta[name='eyeball-error']")?.remove();
  document.body.replaceChildren();
  window.history.replaceState(null, "", "/");
});

function tag(content: string): void {
  const meta = document.createElement("meta");
  meta.name = "eyeball-error";
  meta.content = content;
  document.head.append(meta);
}

const failure = {
  title: "Too large",
  heading: "Too large",
  text: "Past the 20 MB the surface sends on its own.",
  waiting_count: 3,
};

test("a shell with no tag has no failure", () => {
  expect(readFailure(document)).toBeNull();
});

test("a tag is read as the failure it holds", () => {
  tag(JSON.stringify(failure));
  expect(readFailure(document)).toEqual(failure);
});

test("a tag that is not a failure is no failure", () => {
  tag("not json");
  expect(readFailure(document)).toBeNull();
  document.head.querySelector("meta[name='eyeball-error']")?.remove();
  tag(JSON.stringify({ title: "Too large" }));
  expect(readFailure(document)).toBeNull();
});

async function draw(failureToShow: ReturnType<typeof readFailure>) {
  // The router is made when it first draws, and reads the address then.
  window.history.replaceState(null, "", "/reviews/1/raw/1/big.png");
  const { App } = await import("../App");
  const host = document.createElement("div");
  document.body.append(host);
  createRoot(host).render(createElement(App, { failure: failureToShow }));
  return host;
}

test("a failure is drawn in place of every route", async () => {
  const host = await draw(failure);
  await expect
    .poll(() => host.querySelector(".empty .headline")?.textContent)
    .toBe("Too large");
  expect(host.querySelector(".empty p:not(.headline)")?.textContent).toBe(
    failure.text,
  );
  expect(host.querySelector(".topbar h1")?.textContent).toBe("Too large");
  expect(host.querySelector(".topbar .eye")).not.toBeNull();
  // No router is mounted, so the way back is an anchor that loads the queue.
  expect(host.querySelector(".topbar a.back")?.getAttribute("href")).toBe("/");
});

test("with no failure the app draws no error screen and routes as usual", async () => {
  const host = await draw(null);
  // The address has no route, so the router's default error page is what it
  // draws. It never draws the failure's box.
  await expect.poll(() => host.textContent).not.toBe("");
  expect(host.querySelector(".empty .headline")?.textContent).not.toBe(
    "Too large",
  );
  expect(host.querySelector(".topbar h1")).toBeNull();
});
