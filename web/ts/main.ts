import { blinkOnLoad } from "./blink";
import { trackHead } from "./head";
import { prune } from "./storage";
import { followStream } from "./stream";
import { refreshOnTap } from "./update";
import { registerWorker } from "./worker";
import { restoreViewport } from "./viewport";
import { followTheme } from "./theme";
import { answerTaps, load } from "./tap";
import { takeToken } from "./token";

// The bundle is loaded without defer so the theme is set before the first
// paint. A deferred script would paint light first on a dark device. The
// token is taken at once too, so the fragment leaves the address bar before
// anything else can read it.
followTheme();
takeToken();
// The bundle runs in the head before the body is parsed, so what reads the page
// waits for it.
document.addEventListener("DOMContentLoaded", () => {
  registerWorker();
  answerTaps(load);
  trackHead();
  refreshOnTap();
  followStream();
  prune();
  blinkOnLoad();
  restoreViewport();
});
