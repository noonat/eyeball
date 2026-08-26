// Package blob stores the content of captured files, addressed by the sha256 of
// the bytes.
//
// Put returns a digest only after its content is on disk. It writes to a
// temporary name, fsyncs the file, renames it into place, and fsyncs the
// directories the rename passed through. A crash therefore leaves an
// unreferenced blob, which nothing reads, rather than a digest recorded with no
// file to match it.
//
// Every call that takes a digest refuses one that is not 64 lowercase hex
// characters, before touching the filesystem, because the digest is what the
// path is built from. Nothing is verified on read. See docs/architecture.md.
package blob
