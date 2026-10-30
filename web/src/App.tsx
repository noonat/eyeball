import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Route, Routes } from "react-router";
import { QueueRoute } from "./app/QueueRoute";

const client = new QueryClient();

/**
 * The app's providers and its routes. The shell is served at the queue's
 * address only, so the router has one route. A review is still a template
 * page, which a plain anchor loads in full.
 */
export function App() {
  return (
    <QueryClientProvider client={client}>
      <BrowserRouter>
        <div className="app">
          <Routes>
            <Route path="/" element={<QueueRoute />} />
          </Routes>
        </div>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
