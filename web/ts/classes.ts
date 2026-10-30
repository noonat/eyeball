// Every class the client writes, in one place, because the template class check
// only sees classes written in a template. A Go test holds these to app.css.
export const classes = Object.freeze({
  sel: "sel",
  failed: "failed",
  whyNot: "why-not",
  offline: "offline",
  retry: "retry",
} as const);
