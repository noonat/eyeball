// Package blob stores the content of captured files, addressed by the sha256 of
// the bytes.
//
// A round holds only what differs from its base commit, so this is proportional
// to what an agent changed rather than to the size of a repository. Writes go to
// a temporary name and rename into place, so a reader never sees a partial file,
// and re-capturing identical bytes is a rename over the path that already holds
// them.
package blob
