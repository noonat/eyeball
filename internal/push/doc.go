// Package push sends one notification per review request, and nothing else.
//
// A subscription that a push service reports as gone is marked lapsed rather
// than retried, and the surface says so: a queue that looks empty because
// nothing is arriving is the failure notification exists to prevent.
package push
