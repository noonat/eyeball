// Package capture freezes what a round is asking about, at the moment it asks.
//
// A capture is a base commit and the content of every file that differs from it,
// including files outside the review's scope, because a file opened for context
// has to show what the agent had. Everything else resolves from git at the base,
// which is what keeps a capture cheap. See docs/architecture.md.
//
// A review's base is fixed at its first round. Later rounds are handed the
// commit already recorded on the review rather than resolving one again, so the
// total a reviewer sees before approving is measured from where the work
// started.
//
// An empty base means there is nothing to measure against: a project with no
// git, or a repository with no commits yet. It is never a request to resolve
// HEAD again, and folding those two meanings together is how a later round
// silently re-bases itself onto whatever HEAD had become.
//
// Whether a root is a git project is decided by a .git entry on the filesystem,
// not by asking git. With git missing from PATH the question cannot be put to
// git at all, so its failure could not separate a repository whose tool is
// absent from a directory that was never one.
package capture
