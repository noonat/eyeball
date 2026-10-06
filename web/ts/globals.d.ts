import type htmxType from "../vendor/htmx.esm";

declare global {
  // htmx.min.js defines the global; its own declarations type it.
  const htmx: typeof htmxType;
}
