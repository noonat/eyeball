import { afterEach, expect, test } from "vitest";
import { forget, remember } from "../../ts/storage";
import { readPlace, recallPlace, storePlace } from "./reviewPlace";

const place = "7:1:";

afterEach(() => {
  forget(`place:${place}`);
});

test("a place stored in the template page's shape is restored", () => {
  remember(`place:${place}`, { scroll: 1234, folded: ["a.txt", "d/b.txt"] });
  expect(recallPlace(place)).toEqual({
    scroll: 1234,
    folded: ["a.txt", "d/b.txt"],
  });
});

test("a place from before every card opened keeps its scroll and drops its list", () => {
  // That entry names the open files, and on a large review it names almost
  // none, so reading it as the folded files would fold nearly every card.
  remember(`place:${place}`, { scroll: 300, open: ["d/05.txt"] });
  expect(recallPlace(place)).toEqual({ scroll: 300, folded: [] });
});

test("an entry that is not a place is no place", () => {
  expect(readPlace(null)).toBeNull();
  expect(readPlace("scroll")).toBeNull();
  expect(readPlace({ folded: ["a.txt"] })).toBeNull();
  expect(readPlace({ scroll: "300" })).toBeNull();
  expect(readPlace({ scroll: 5, folded: ["a.txt", 3, null] })).toEqual({
    scroll: 5,
    folded: ["a.txt"],
  });
});

test("a place the app stores is the shape the template page reads", () => {
  storePlace(place, { scroll: 40, folded: ["a.txt"] });
  const raw = window.localStorage.getItem(`eyeball:place:${place}`);
  const entry: unknown = JSON.parse(raw ?? "null");
  expect(entry).toMatchObject({ value: { scroll: 40, folded: ["a.txt"] } });
});
