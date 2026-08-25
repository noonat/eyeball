// Package store is the SQLite database holding reviews, rounds, comments,
// decisions and push subscriptions.
//
// Writes run with synchronous=FULL rather than the usual NORMAL. Losing a
// comment is the failure this must not have, and under WAL the default is
// durable against a process crash but not against the machine losing power.
package store
