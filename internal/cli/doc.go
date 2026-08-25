// Package cli is the command line an agent drives.
//
// Every command speaks HTTP over the daemon's unix socket, so there is one set
// of handlers and --json is the response body rather than a second code path.
// Flags parse in any position: argument order is what a model gets wrong.
package cli
