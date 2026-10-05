// Package cli is the command line an agent drives.
//
// Every command speaks HTTP over the daemon's unix socket, so there is one set
// of handlers and --json is the response body rather than a second code path.
// Flags parse in any position: argument order is what a model gets wrong.
//
// wait acknowledges the comments it printed only after printing them, so a
// command killed in between leaves them owed rather than lost. Delivery is at
// least once by design.
//
// Run prints everything it has to say and returns only an exit status, so main
// is the one place that exits. The manual eyeball doc prints is manual.md, held
// to every command, flag, refusal code and JSON field by a test.
package cli
