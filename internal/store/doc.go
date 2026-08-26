// Package store is the SQLite database holding reviews, rounds, comments,
// decisions and push subscriptions.
//
// Writes run with synchronous=FULL rather than the usual NORMAL. Losing a
// comment is the failure this must not have, and under WAL the default is
// durable against a process crash but not against the machine losing power.
//
// Timestamps are TEXT in TimeLayout, which is fixed width and always UTC, so a
// text sort is a chronological sort and a listing can order on the column.
//
// IDs are integers, because an agent types them and a browser shows them, and
// an opaque token would buy unguessability worth nothing on a surface with no
// authentication. A round is addressed by its number within its review.
//
// A review is open, approved or abandoned. Approving its latest round ends it,
// and asking for changes leaves it open for the next round. Waiting and working
// are not states: a review is waiting when its latest round has no decision and
// working when it does. See docs/architecture.md.
package store
