import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type ComponentType, useState } from "react";
import {
  createBrowserRouter,
  Outlet,
  RouterProvider,
  ScrollRestoration,
} from "react-router";
import type { Failure } from "./api/types";
import { ErrorScreen } from "./components/ErrorScreen";
import { FilesRoute, ProjectRoute, ReviewRoute } from "./app/ViewRoutes";
import { QueueRoute } from "./app/QueueRoute";
import { drawnPatterns, type DrawnPattern } from "./app/routes";
import { TapListener } from "./app/TapListener";

const client = new QueryClient();

/**
 * What each address the app draws shows. The type makes a pattern with no
 * screen, and a screen with no pattern, a compile error, so the router's routes
 * are the list `draws` answers from and nothing else.
 */
const screens: Readonly<Record<DrawnPattern, ComponentType>> = {
  "/": QueueRoute,
  "/reviews/:review": ReviewRoute,
  "/reviews/:review/rounds/:round": ReviewRoute,
  "/projects/:project": ProjectRoute,
  "/projects/:project/files/*": FilesRoute,
  "/reviews/:review/files/*": FilesRoute,
  "/reviews/:review/rounds/:round/files/*": FilesRoute,
};

/**
 * What every screen sits in. The browser restores the scroll of a page it
 * loaded, and a move inside the app is not a load, so ScrollRestoration puts a
 * list back where it was when the reader returns to it.
 */
function Root() {
  return (
    <>
      <TapListener />
      <div className="app">
        <Outlet />
      </div>
      <ScrollRestoration />
    </>
  );
}

function createRouter() {
  return createBrowserRouter([
    {
      Component: Root,
      children: drawnPatterns.map((path) => ({
        path,
        Component: screens[path],
      })),
    },
  ]);
}

/**
 * The router is made when the first route draws, so a failure never creates
 * one: creating it reads the address and starts matching it.
 */
function Routed() {
  const [router] = useState(createRouter);
  return <RouterProvider router={router} />;
}

interface AppProps {
  /** What the shell says failed, or null where the address did not. */
  readonly failure: Failure | null;
}

/**
 * The app's providers and its routes. A failure takes the place of every
 * route, since the address that failed may be one the router has no route for.
 */
export function App({ failure }: AppProps) {
  if (failure !== null) {
    return (
      <div className="app">
        <ErrorScreen
          title={failure.title}
          heading={failure.heading}
          text={failure.text}
          waiting={failure.waiting_count}
        />
      </div>
    );
  }
  return (
    <QueryClientProvider client={client}>
      <Routed />
    </QueryClientProvider>
  );
}
