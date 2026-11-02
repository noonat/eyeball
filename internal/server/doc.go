// Package server is the HTTP surface: routes, handlers, and the live stream that
// carries updates and doubles as the connection state.
//
// Handlers answer with a view for the app to draw, a page for what the app does
// not draw, or the shell that loads the app. Each declares the narrow interface
// it needs rather than depending on a package-wide one.
package server
