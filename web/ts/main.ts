import { followTheme } from "./theme";

// The bundle is loaded without defer so the theme is set before the first
// paint. A deferred script would paint light first on a dark device.
followTheme();
