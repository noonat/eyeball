import {
  guardVerdicts,
  keyComments,
  openComposers,
  settleComposers,
} from "./composer";
import { keepDrafts } from "./drafts";
import { reportFailures } from "./failures";
import { keepPlace } from "./place";
import { prune } from "./storage";
import { followPush } from "./push";
import { followStream } from "./stream";
import { refreshOnTap } from "./update";
import { registerWorker } from "./worker";
import { catchFailedSaves } from "./unsent";
import { followTheme } from "./theme";
import { sendToken, takeToken } from "./token";

// The bundle is loaded without defer so the theme is set before the first
// paint. A deferred script would paint light first on a dark device. The
// token is taken at once too, so the fragment leaves the address bar before
// anything else can read it.
followTheme();
takeToken();
sendToken();
reportFailures();
keyComments();
openComposers();
settleComposers();
guardVerdicts();
catchFailedSaves();
// The stream reads the page's stamp, which is in the body, and the bundle runs
// in the head before the body is parsed.
document.addEventListener("DOMContentLoaded", () => {
  registerWorker();
  refreshOnTap();
  followStream();
  followPush();
  prune();
  keepPlace();
  keepDrafts();
});
