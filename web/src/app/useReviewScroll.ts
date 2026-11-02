import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { NavigationType, useLocation, useNavigationType } from "react-router";
import type { Review } from "../api/types";
import { holdPlace, measureRows } from "./cardLayout";
import { recallPlace, storePlace } from "./reviewPlace";

// What ends the place being set again as bodies arrive: the reader's first
// tap, scroll or key. Until then it would undo what they did, and a tap in the
// list of files during those seconds was pulled back to the saved place.
const reader = ["pointerdown", "click", "wheel", "touchmove", "keydown"];

// How long a saved place is set again at most, for a reader who does nothing.
const settle = 3000;

/** What the screen keeps of the reader's position. */
export interface Scroll {
  /** The paths of the cards the reader folded. */
  readonly folded: ReadonlySet<string>;
  readonly onFold: (path: string, open: boolean) => void;
}

/**
 * Everything that positions the review screen: the skeleton's measured heights,
 * the card held at the top while others change height, the reader's place with
 * its folds, and a jump to the element an address names.
 *
 * The place and the jump are applied in effects, after the router's. The
 * router scrolls in a layout effect at the root on every move, to the top
 * where it finds no saved position and no element the hash names, and a
 * layout effect of this screen would run before it, since the screen is drawn
 * ahead of the router's component. A screen drawn in the same commit as the
 * move, from a cached view, is where the order shows.
 *
 * `arrived` changes as a body arrives, which moves what lies below it.
 */
export function useReviewScroll(review: Review, arrived: unknown): Scroll {
  const { hash, key } = useLocation();
  const type = useNavigationType();
  const [saved] = useState(() => recallPlace(review.place));
  // An address with a hash is a jump, and the jump wins over the saved place
  // where the reader did not come back. A router move, or a page load with no
  // place saved, is one. A reload or a move back keeps the place, which is what
  // the reader last left, and a reload after a jump would otherwise return to
  // the card the jump named.
  const [jumping] = useState(
    () => hash !== "" && (type !== NavigationType.Pop || saved === null),
  );
  const [folded, setFolded] = useState<ReadonlySet<string>>(
    () => new Set(saved?.folded ?? []),
  );
  const onFold = (path: string, open: boolean): void => {
    setFolded((all) => {
      if (all.has(path) !== open) {
        return all;
      }
      const next = new Set(all);
      if (open) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  };

  const cards = (review.cards ?? []).map((card) => card.path);
  const deferred = (review.cards ?? []).some((card) => card.deferred);
  const ids = (review.cards ?? []).map((card) => card.id).join(" ");

  // A skeleton's heights depend on the device, so they are measured before the
  // first paint of a screen that has one, and once for as long as the app runs.
  useLayoutEffect(() => {
    if (deferred) {
      measureRows();
    }
  }, [deferred]);

  // Cards come with the round, so the hold starts again when they change.
  useEffect(() => holdPlace(ids === "" ? [] : ids.split(" ")), [ids]);

  const now = useRef({ cards, folded });
  useEffect(() => {
    now.current = { cards, folded };
  });
  const armed = useRef(false);
  const scroll = useRef(0);
  const restoring = useRef(false);
  const place = review.place;

  const save = useCallback(
    (at: number, folds: ReadonlySet<string>): void => {
      storePlace(place, {
        scroll: at,
        folded: now.current.cards.filter((path) => folds.has(path)),
      });
    },
    [place],
  );

  useEffect(() => {
    // Nothing is saved until the saved place has been applied and the reader
    // has taken over. The router's scroll to the top, and a place applied to a
    // page that is still too short, would otherwise be saved over the one
    // being restored.
    const arm = (): void => {
      armed.current = true;
      scroll.current = window.scrollY;
    };
    let timer = 0;
    const stop = (): void => {
      restoring.current = false;
      window.clearTimeout(timer);
      for (const name of reader) {
        document.removeEventListener(name, stop, true);
      }
      arm();
    };
    if (saved !== null && !jumping) {
      restoring.current = true;
      window.scrollTo(0, saved.scroll);
      for (const name of reader) {
        document.addEventListener(name, stop, true);
      }
      timer = window.setTimeout(stop, settle);
    } else {
      arm();
    }
    let pending = 0;
    const onScroll = (): void => {
      if (!armed.current) {
        return;
      }
      scroll.current = window.scrollY;
      window.clearTimeout(pending);
      pending = window.setTimeout(() => {
        save(scroll.current, now.current.folded);
      }, 200);
    };
    // The page is left by a load too, which a router move does not tell.
    const onHide = (): void => {
      if (armed.current) {
        save(window.scrollY, now.current.folded);
      }
    };
    window.addEventListener("scroll", onScroll);
    window.addEventListener("pagehide", onHide);
    return () => {
      window.removeEventListener("scroll", onScroll);
      window.removeEventListener("pagehide", onHide);
      window.clearTimeout(pending);
      window.clearTimeout(timer);
      for (const name of reader) {
        document.removeEventListener(name, stop, true);
      }
      // The last scroll the handler saw, since by now the router may have
      // moved the page for the screen that replaces this one.
      if (armed.current) {
        save(scroll.current, now.current.folded);
      }
    };
    // The place is read once, as the screen opens, so the effect runs once.
  }, [saved, jumping, save]);

  // A body arriving moves what lies below it, so the saved place is set again
  // until the reader takes over.
  const applied = useRef(arrived);
  useEffect(() => {
    if (applied.current === arrived) {
      return;
    }
    applied.current = arrived;
    if (restoring.current && saved !== null) {
      window.scrollTo(0, saved.scroll);
    }
  }, [arrived, saved]);

  // A fold is saved as it is made.
  const seen = useRef(folded);
  useEffect(() => {
    if (seen.current === folded) {
      return;
    }
    seen.current = folded;
    if (armed.current) {
      save(window.scrollY, folded);
    }
  }, [folded, save]);

  // The element a router move's address names, once it is drawn. The router
  // scrolls to it at the move when it is there, and to the top when it is not
  // yet, which is where a comment in a view fetched after the move is. The
  // target is scrolled to below the page's header and its card's, by the
  // stylesheet's scroll margins. A reader who has begun to scroll is left alone.
  const jumped = useRef("");
  useEffect(() => {
    const stop = (): void => {
      jumped.current = key;
    };
    for (const name of reader) {
      document.addEventListener(name, stop, true);
    }
    return () => {
      for (const name of reader) {
        document.removeEventListener(name, stop, true);
      }
    };
  }, [key]);
  useEffect(() => {
    if (
      hash === "" ||
      (type === NavigationType.Pop && !jumping) ||
      jumped.current === key
    ) {
      return;
    }
    const target = elementOf(hash);
    if (target !== null) {
      target.scrollIntoView();
      jumped.current = key;
    }
  });

  return { folded, onFold };
}

function elementOf(hash: string): Element | null {
  try {
    return document.getElementById(decodeURIComponent(hash.slice(1)));
  } catch {
    return null;
  }
}
