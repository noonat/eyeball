// Package daemon owns the process's lifetime: one instance, started on demand,
// stopped explicitly.
//
// Single instance comes from an exclusive lock held for the process's life
// rather than from the socket file existing, because a crash leaves the socket
// behind and a stale socket must not stop the next daemon from starting.
//
// It does not exit when idle. It is started by an agent and used by a person
// hours later, from a phone, by tapping a notification.
package daemon
