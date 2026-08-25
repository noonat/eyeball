// Package server is the HTTP surface: routes, handlers, and the live stream that
// carries updates and doubles as the connection state.
//
// Handlers answer with a fragment as readily as with a page, because every
// interaction replaces a piece of what is on screen. Each declares the narrow
// interface it needs rather than depending on a package-wide one.
package server
