import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { useLocation } from "react-router";
import { blinkOnLoad } from "../../ts/blink";
import { fetchView, isFiles, isProject, isReview, viewKey } from "../api/views";
import type { Review } from "../api/types";
import { FilesScreen } from "../components/FilesScreen";
import { NotFoundScreen } from "../components/NotFoundScreen";
import { ProjectScreen } from "../components/ProjectScreen";
import { ReviewScreen } from "../components/ReviewScreen";
import { ScreenLoading } from "../components/ScreenLoading";
import { HeadTracker } from "./HeadTracker";
import { notices } from "./notices";
import { useBuildStream } from "./useBuildStream";
import { useReview } from "./useReview";
import { useTaps } from "./useTaps";
import type { Connection } from "./useQueueStream";

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
    queryFn: () => fetchView(address, isProject),
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
    queryFn: () => fetchView(address, isFiles),
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
  const { data } = useQuery({
    queryKey: viewKey(address),
    queryFn: () => fetchView(address, isReview),
  });
  const connection = useBuildStream();
  useDocumentTitle(
    data === undefined ? undefined : data.found ? data.body.title : "Not found",
  );
  if (data === undefined) {
    return <ScreenLoading {...notices(connection)} />;
  }
  return (
    <>
      {data.found ? (
        // The round's own place keys what the page keeps for the reader, so a
        // move to another round or another comparison starts it afresh, and a
        // draft written for either is read back.
        <ReviewView
          key={data.body.place}
          review={data.body}
          address={address}
          connection={connection}
        />
      ) : (
        <NotFoundScreen missing={data.body} {...notices(connection)} />
      )}
      <HeadTracker />
    </>
  );
}

interface ReviewViewProps {
  readonly review: Review;
  readonly address: string;
  readonly connection: Connection;
}

/** The screen of a round, with what the reader has typed and sent. */
function ReviewView({ review, address, connection }: ReviewViewProps) {
  const state = useReview(review, address);
  useTaps(state.tapping, state.ui.onOpen ?? noOpen);
  return (
    <ReviewScreen
      review={review}
      ui={state.ui}
      bar={{ ...state.bar, offline: connection.offline }}
      {...notices(connection)}
    />
  );
}

function noOpen(): void {
  return undefined;
}
