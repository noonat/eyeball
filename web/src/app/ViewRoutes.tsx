import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { useLocation } from "react-router";
import { blinkOnLoad } from "../../ts/blink";
import { fetchView, isFiles, isProject, isReview, viewKey } from "../api/views";
import type { NotFound, Review } from "../api/types";
import { FilesScreen } from "../components/FilesScreen";
import { NotFoundScreen } from "../components/NotFoundScreen";
import { ProjectScreen } from "../components/ProjectScreen";
import { ReviewScreen } from "../components/ReviewScreen";
import { ScreenLoading } from "../components/ScreenLoading";
import { HeadTracker } from "./HeadTracker";
import { notices } from "./notices";
import { useBuildStream } from "./useBuildStream";
import { useReview } from "./useReview";
import { useReviewStream } from "./useReviewStream";
import { useTaps } from "./useTaps";

/** Names the document, as the template pages' titles did. */
function useDocumentTitle(title: string | undefined): void {
  useEffect(() => {
    if (title !== undefined) {
      document.title = `${title} · Eyeball`;
    }
  }, [title]);
}

/**
 * The address the screen draws, path and query. The view is the one at /api
 * plus this, so a `?copy=` reaches the server unchanged.
 */
function useAddress(): string {
  const { pathname, search } = useLocation();
  return pathname + search;
}

/** A project's page, drawn from its view. */
export function ProjectRoute() {
  const address = useAddress();
  const { data } = useQuery({
    queryKey: viewKey(address),
    queryFn: ({ signal }) => fetchView(address, isProject, signal),
  });
  const connection = useBuildStream();
  useEffect(blinkOnLoad, []);
  useDocumentTitle(
    data === undefined ? undefined : data.found ? data.body.title : "Not found",
  );
  if (data === undefined) {
    return <ScreenLoading {...notices(connection)} />;
  }
  return (
    <>
      {data.found ? (
        <ProjectScreen project={data.body} {...notices(connection)} />
      ) : (
        <NotFoundScreen missing={data.body} {...notices(connection)} />
      )}
      <HeadTracker />
    </>
  );
}

/** A tree or a file, drawn from the view of the review's file address. */
export function FilesRoute() {
  const address = useAddress();
  const { data } = useQuery({
    queryKey: viewKey(address),
    queryFn: ({ signal }) => fetchView(address, isFiles, signal),
  });
  const connection = useBuildStream();
  useEffect(blinkOnLoad, []);
  useDocumentTitle(
    data === undefined ? undefined : data.found ? data.body.title : "Not found",
  );
  if (data === undefined) {
    return <ScreenLoading {...notices(connection)} />;
  }
  return (
    <>
      {data.found ? (
        <FilesScreen files={data.body} {...notices(connection)} />
      ) : (
        <NotFoundScreen missing={data.body} {...notices(connection)} />
      )}
      <HeadTracker />
    </>
  );
}

/** One round of a review, drawn from the view of its address. */
export function ReviewRoute() {
  const address = useAddress();
  const { data, isError } = useQuery({
    queryKey: viewKey(address),
    queryFn: ({ signal }) => fetchView(address, isReview, signal),
    // The stream says when the round moved, and says it again when it
    // reconnects. A refetch on focus or on coming back online, outside the
    // hold, would draw a round the held text has not been let go of.
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  useDocumentTitle(
    data === undefined ? undefined : data.found ? data.body.title : "Not found",
  );
  if (data === undefined) {
    return <ReviewArriving />;
  }
  return (
    <>
      {data.found ? (
        // The address keys the screen, so a move to another round or another
        // comparison starts afresh, and a draft written for either is read
        // back. A newer round at the same address is not a new screen: it is
        // taken up through the hold, since a remount would drop what the
        // reader typed.
        <ReviewView
          key={address}
          latest={data.body}
          address={address}
          failed={isError}
        />
      ) : (
        <ReviewMissing missing={data.body} />
      )}
      <HeadTracker />
    </>
  );
}

/**
 * The screen before the view arrives. It holds the build stream, which is all
 * there is to follow until the view names a review's own.
 */
function ReviewArriving() {
  return <ScreenLoading {...notices(useBuildStream())} />;
}

/** The 404 of a review that is not there, which has no stamp to follow. */
function ReviewMissing({ missing }: { readonly missing: NotFound }) {
  return <NotFoundScreen missing={missing} {...notices(useBuildStream())} />;
}

interface ReviewViewProps {
  /** The newest round the cache holds, which the screen takes up when it can. */
  readonly latest: Review;
  readonly address: string;
  /** The last fetch of the round failed, so the round held is not the newest. */
  readonly failed: boolean;
}

/** The screen of a round, with what the reader has typed and sent. */
function ReviewView({ latest, address, failed }: ReviewViewProps) {
  const stream = useReviewStream(latest.stream);
  const state = useReview(latest, address, stream, failed);
  useTaps(state.tapping, state.ui.onOpen ?? noOpen, state.held);
  return (
    <ReviewScreen
      review={state.review}
      ui={state.ui}
      bar={state.bar}
      onDecide={state.onDecide}
      {...notices(stream.connection)}
    />
  );
}

function noOpen(): void {
  return undefined;
}
