// Package viewdoc draws a project, tree or file view as the HTML the template
// pages drew, for tests. The screens are React components, so a test about their
// content has no markup to read from the server, and this gives it markup by the
// old class names. Nothing in the daemon imports it.
package viewdoc
