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
//
// A page reads a round through a Reader built by State.Reader, after Inspect
// has settled the project's state once for the request: whether the root
// exists, whether the project has lost its repository, whether the base still
// exists, and which ignore rules a live read follows. Lost is decided from the
// filesystem alone, before any git call, because git in a lost repository
// either fails or answers for an outer one. Every state but two refuses a live
// read, and a refused live read never stops a captured file from opening.
//
// A page locates a path before it reads one, in a fixed order: a deletion the
// round records, a file the capture holds, a directory the capture holds files
// under, the base's one entry at the path, and last the disk, where the state
// allows a live read. Nothing lists recursively to decide it, so a directory
// is never read as a file.
//
// A live read, of a file no frozen source holds, refuses a path with a .git
// segment or a NUL, a path through a symlinked directory or into a directory
// holding another repository, a path git ignores, and a path inside a
// submodule. It reads through an os.Root on the project root, which refuses in
// the kernel a path that leaves it, so a directory swapped for a link after
// the checks still cannot lead outside. A link reads as its target text. The
// walk and Freeze still check parents with linkedParent: they write to the
// blob store at an agent's request, not to a page.
//
// A directory lists one level from the sources the state allows: the base and
// the capture for a round with a base, the capture alone for a git project
// with none or for any state that refuses a live read, and otherwise the disk
// and the capture. A listing from disk passes the live read's directory checks,
// goes through the same os.Root, and drops what git ignores where the project
// follows ignore rules, so the tree never names a file the live read refuses.
//
// The project view's working copy is the capture's listing asked against HEAD
// with nothing frozen: no Freeze and no blob. Only a git project is listed,
// since git lists a marker project's changes against another repository's
// top level, and a path that vanishes mid-listing is dropped rather than
// failing the page.
package capture
