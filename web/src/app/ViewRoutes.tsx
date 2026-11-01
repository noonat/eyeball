import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { useLocation } from "react-router";
import { blinkOnLoad } from "../../ts/blink";
import { fetchView, isFiles, isProject, viewKey } from "../api/views";
import { FilesScreen } from "../components/FilesScreen";
import { NotFoundScreen } from "../components/NotFoundScreen";
import { ProjectScreen } from "../components/ProjectScreen";
import { ScreenLoading } from "../components/ScreenLoading";
import { HeadTracker } from "./HeadTracker";
import { notices } from "./notices";
import { useBuildStream } from "./useBuildStream";

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
