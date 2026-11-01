import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type ComponentType } from "react";
import {
  createBrowserRouter,
  Outlet,
  RouterProvider,
  ScrollRestoration,
} from "react-router";
import { FilesRoute, ProjectRoute } from "./app/ViewRoutes";
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

const router = createBrowserRouter([
  {
    Component: Root,
    children: drawnPatterns.map((path) => ({
      path,
      Component: screens[path],
    })),
  },
]);

/** The app's providers and its routes. */
export function App() {
  return (
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}
