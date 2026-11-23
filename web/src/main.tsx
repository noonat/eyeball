import { createRoot } from "react-dom/client";
import { followTheme } from "../ts/theme";
import { takeToken } from "../ts/token";
import { refreshOnTap } from "../ts/update";
import { registerWorker } from "../ts/worker";
import { prune } from "../ts/storage";
import { restoreViewport } from "../ts/viewport";
import { App } from "./App";
import { readFailure } from "./app/failure";

// The bundle is loaded without defer so the theme is set before the first
// paint, as the template pages' bundle does. The token is taken at once too,
// so the fragment leaves the address bar before the router reads it.
followTheme();
takeToken();

function mount(): void {
  const root = document.getElementById("root");
  if (root === null) {
    return;
  }
  // An entry the page's own memory kept past its age is dropped as the app
  // opens, as the template bundle did.
  prune();
  registerWorker();
  refreshOnTap();
  restoreViewport();
  createRoot(root).render(<App failure={readFailure(document)} />);
}

// The script runs in the head, before the root element is parsed.
if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", mount);
} else {
  mount();
}
