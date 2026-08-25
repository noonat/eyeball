// Package diff compares two captures and marks what moved.
//
// Word marking runs only where a run of removals pairs one to one with the
// additions that replaced it. An unequal run is a rewrite rather than an edit,
// and pairing across one marks the wrong words with confidence.
package diff
