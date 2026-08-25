// Package capture freezes what a round is asking about, at the moment it asks.
//
// A capture is a base commit and the content of every file that differs from it,
// including files outside the review's scope, because a file opened for context
// has to show what the agent had. Everything else resolves from git at the base,
// which is what keeps a capture cheap. See docs/architecture.md.
package capture
