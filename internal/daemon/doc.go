// Package daemon owns the process's lifetime: one instance, started on demand,
// stopped explicitly.
//
// Single instance comes from an exclusive lock held for the process's life
// rather than from the socket file existing, because a crash leaves the socket
// behind and a stale socket must not stop the next daemon from starting.
//
// Shutdown runs in one order and the lock is released last: waits are ended,
// the lock file is marked stopping, the server unlinks the socket and gives a
// capture in progress time to finish, the store closes, then the lock goes. A closing Go unix listener unlinks whatever is at its
// path, so a daemon that released the lock first would delete its successor's
// socket.
//
// A started daemon is detached: its own session, stdin and stdout on
// /dev/null, stderr appended to the log, and the state directory as its
// working directory. One that inherited the starting command's stdout would
// keep an agent's harness waiting for end-of-file until the daemon exited.
//
// It does not exit when idle. It is started by an agent and used by a person
// hours later, from a phone, by tapping a notification.
package daemon
