// Package api holds the agent API's wire types, shared by the daemon's handlers
// and the CLI.
//
// These types are the stable schema an agent reads through --json and
// --format, so a field added, renamed or removed here is a change to that
// contract. The manual eyeball doc prints lists every field of every response
// type by its dotted path, and a test fails when one is missing. See
// docs/architecture.md.
package api
