// Package render turns a capture into what the reviewer reads: markdown as a
// document, code as a highlighted diff, and the templates around both.
//
// Highlighting happens here rather than in the browser, because the server has
// already parsed the file and shipping a second parser to a phone to re-derive
// the same answer buys nothing.
package render
