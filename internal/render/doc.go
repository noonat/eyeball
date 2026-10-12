// Package render turns a capture into what the reviewer reads: markdown as a
// document, code as a highlighted diff, and the templates around both.
//
// Highlighting happens here rather than in the browser, because the server has
// already parsed the file and shipping a second parser to a phone to re-derive
// the same answer buys nothing.
//
// A document is untrusted, since an agent wrote it. Raw HTML is never
// rendered. Links, autolinks and images go through an allowlist rather than
// goldmark's denylist: http, https and mailto as written, a fragment alone,
// and a relative or root-relative path resolved against the document's
// directory, through capture.CleanPath, to the address the caller writes for
// it. A path cleaning to the project's root goes to the root, which CleanPath
// refuses. A fragment alone is kept only on a page writing heading ids, and
// goes to the document's own page elsewhere. Anything else renders as its
// text, with no a element.
//
// What changed in a document is marked block by block. Each top-level block
// is keyed by a parse and a rendering of its own, with heading ids off, raw
// HTML and destinations written as the source has them, and whitespace
// collapsed outside pre, then hashed one to a line for the line diff. The
// pairing is internal/diff's word-marking rule: equal runs pair one to one,
// an unequal run's replaced text goes on its last new block, and a removal
// with nothing in its place is a block of its own. Nothing is marked when the
// old side has no blocks.
//
// Front matter is one block, shown as YAML in a code block with both of its
// delimiters. It is front matter only when the document's first line is ---
// and a later line is ---. Otherwise the first --- is the rule goldmark makes
// of it.
package render
