// Package kind decides what a file is, and therefore how it opens.
//
// Extension first and content second, because an extension is what the agent
// named it and is right nearly always.
//
// SVG is a page, not an image. It can carry script, so its rendering is an img
// element, where script cannot execute, and its bytes are served under the same
// sandbox as HTML.
package kind
