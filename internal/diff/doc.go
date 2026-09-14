// Package diff compares two captures and marks what moved.
//
// Word marking runs only where a run of removals pairs one to one with the
// additions that replaced it. An unequal run is a rewrite rather than an edit,
// and pairing across one marks the wrong words with confidence.
//
// The line comparison is go-udiff, which is x/tools' two-sided Myers. It is
// bidirectional and bounded, so a change too large to search exactly still
// reads as a diff rather than as the whole file replaced. What is written here
// is everything above that: the hunks, their context and numbering, the size a
// file is not diffed past, and the marking.
//
// The package takes bytes and returns values. No git, no store, no filesystem,
// so every adversarial shape is a table row rather than a fixture repository.
package diff
