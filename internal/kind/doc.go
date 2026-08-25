// Package kind decides what a file is, and therefore how it opens.
//
// Extension first and content second, because an extension is what the agent
// named it and is right nearly always.
//
// SVG is a page, not an image. It can carry script, so it renders through the
// sandbox like any other document. Shown through an img element it cannot
// execute, which is how a thumbnail draws it.
package kind
