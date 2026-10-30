import { createRoot } from "react-dom/client";
import "../../docs/design/app.css";
import { EmptyState } from "./components/EmptyState";

// The queue spec gives this entry the router. Until then it draws one
// foundation component, so the bundle has something to prove it can render.
const root = document.getElementById("root");
if (root !== null) {
  createRoot(root).render(
    <div className="app">
      <div className="page">
        <EmptyState />
      </div>
    </div>,
  );
}
