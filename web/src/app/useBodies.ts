import { useQueryClient, useQueries } from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState } from "react";
import type { Card, CardBody, Review } from "../api/types";
import { viewKey } from "../api/views";
import { getBody } from "./reviewWrites";

/** The query key of a card's body: the cards route's own address, query included. */
export function bodyKey(card: Card): readonly [string, string] {
  return viewKey(card.body_href);
}

/** Matches the query of any card body of a round. */
export function isBodyOf(
  review: string,
  round: number,
): (query: { readonly queryKey: readonly unknown[] }) => boolean {
  const prefix = `/api/reviews/${review}/rounds/${String(round)}/cards/`;
  return ({ queryKey }) =>
    queryKey[0] === "view" &&
    typeof queryKey[1] === "string" &&
    queryKey[1].startsWith(prefix);
}

/** What a screen needs from the bodies it fetches. */
export interface Bodies {
  /** The body held for each path, fetched now or when the review was last open. */
  readonly data: Readonly<Record<string, CardBody>>;
  /**
   * The failure of each card's last request, by "card:" and the path. It goes as
   * soon as the card asks again, since a query with no data starts over, and a
   * card that failed asks again only from its retry.
   */
  readonly failures: Readonly<Record<string, string>>;
  /**
   * Asks for a card's body unless it has one or one is on its way. It is the
   * card's retry as well, and a held card's Load.
   */
  readonly load: (path: string) => void;
  /** Reads the bodies held again, as a newer round makes them stale. */
  readonly refreshAll: () => void;
  /** Reads one body again, as a comment on one of its passages moves its folds. */
  readonly refresh: (path: string) => void;
}

interface Held {
  readonly data: Readonly<Record<string, CardBody>>;
  readonly failures: Readonly<Record<string, string>>;
}

/**
 * The bodies of the cards a long review sends without one, and of a held card
 * the reader loads, as queries of their own. The query cache is where server
 * data lives, so a review the reader returns to draws its bodies at once and
 * reads them again behind them, and a fetch of the round cannot touch a body.
 *
 * A query is enabled for a path the screen has asked for, or whose body the
 * cache already holds. A disabled one still returns what the cache holds. It
 * fails at once, with no retry, so the failed line is there when the reader
 * looks. A request ends when the screen goes, since the query's signal is passed
 * on.
 */
export function useBodies(review: Review): Bodies {
  const client = useQueryClient();
  const cards = (review.cards ?? []).filter((card) => card.body === null);
  const [asked, setAsked] = useState<ReadonlySet<string>>(
    () =>
      new Set(
        cards
          .filter((card) => client.getQueryData(bodyKey(card)) !== undefined)
          .map((card) => card.path),
      ),
  );
  const paths = cards.map((card) => card.path).join("\0");
  const combine = useCallback(
    (
      results: readonly {
        readonly data: CardBody | undefined;
        readonly error: Error | null;
        readonly isFetching: boolean;
      }[],
    ): Held => {
      const names = paths === "" ? [] : paths.split("\0");
      const data: Record<string, CardBody> = {};
      const failures: Record<string, string> = {};
      results.forEach((result, i) => {
        const path = names[i];
        if (path === undefined) {
          return;
        }
        if (result.data !== undefined) {
          data[path] = result.data;
        }
        if (result.error !== null) {
          failures[`card:${path}`] = result.error.message;
        }
      });
      return { data, failures };
    },
    [paths],
  );
  const held = useQueries({
    queries: cards.map((card) => ({
      queryKey: bodyKey(card),
      queryFn: async ({ signal }: { signal: AbortSignal }) => {
        const outcome = await getBody(card.body_href, signal);
        if (outcome.kind !== "ok") {
          throw new Error(outcome.message);
        }
        return outcome.value;
      },
      enabled: asked.has(card.path),
      retry: false,
      // A body is read again with the round, inside the hold. A read on focus
      // would give its comments the state of a verdict given elsewhere while
      // the bar is still drawn.
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
    })),
    combine,
  });

  // The newest cards and asks, read by a callback the cards hold on to, so it
  // keeps one identity and a card's observer is not made again for each render.
  const newest = useRef({ cards: review.cards, asked });
  useEffect(() => {
    newest.current = { cards: review.cards, asked };
  });
  const load = useCallback(
    (path: string): void => {
      const card = (newest.current.cards ?? []).find((c) => c.path === path);
      if (card === undefined) {
        return;
      }
      const key = bodyKey(card);
      if (client.getQueryState(key)?.fetchStatus === "fetching") {
        return;
      }
      if (newest.current.asked.has(path)) {
        void client.refetchQueries({ queryKey: key });
      } else {
        setAsked((all) => new Set(all).add(path));
      }
    },
    [client],
  );
  const refreshAll = useCallback((): void => {
    void client.invalidateQueries({
      predicate: isBodyOf(review.id, review.round),
    });
  }, [client, review.id, review.round]);
  const refresh = useCallback(
    (path: string): void => {
      const card = (review.cards ?? []).find((c) => c.path === path);
      if (card !== undefined) {
        void client.invalidateQueries({ queryKey: bodyKey(card) });
      }
    },
    [client, review.cards],
  );
  return { ...held, load, refreshAll, refresh };
}
