package api

import "time"

// Code identifies a refusal. It is stable: an agent may branch on it, and the
// manual lists every value.
type Code string

const (
	// CodeAlreadyDecided is a verdict on a round that already has one.
	CodeAlreadyDecided Code = "already_decided"
	// CodeAwaitingVerdict is a next round asked for while the last one waits.
	CodeAwaitingVerdict Code = "awaiting_verdict"
	// CodeBadBase is a base that does not name a commit.
	CodeBadBase Code = "bad_base"
	// CodeBadRequest is a request the daemon could not read.
	CodeBadRequest Code = "bad_request"
	// CodeBaseGained is a later round on a review opened before the
	// repository had a commit.
	CodeBaseGained Code = "base_gained"
	// CodeBaseMoved is a round measured from a base other than the review's.
	CodeBaseMoved Code = "base_moved"
	// CodeConnectionLost is a connection the daemon closed before answering.
	// The client reports it: the daemon cannot.
	CodeConnectionLost Code = "connection_lost"
	// CodeDaemonStopping is a request cut short because the daemon is exiting.
	CodeDaemonStopping Code = "daemon_stopping"
	// CodeDaemonUnavailable is a daemon that could not be reached or started.
	// The client reports it.
	CodeDaemonUnavailable Code = "daemon_unavailable"
	// CodeEmptyChangeRequest is a change request with no comment and no note.
	CodeEmptyChangeRequest Code = "empty_change_request"
	// CodeInternal is a failure inside the daemon. Its message never carries
	// the underlying error, which is in the daemon's log.
	CodeInternal Code = "internal"
	// CodeInvalidPath is a path that is empty, absolute where it cannot be, or
	// names the project root.
	CodeInvalidPath Code = "invalid_path"
	// CodeMissing is a required field left empty.
	CodeMissing Code = "missing"
	// CodeNoGit is a git repository on a machine with no git on PATH.
	CodeNoGit Code = "no_git"
	// CodeNoProject is a working directory with no project above it.
	CodeNoProject Code = "no_project"
	// CodeNoSuchPath is a review path the project does not have, outside git.
	CodeNoSuchPath Code = "no_such_path"
	// CodeNotFound is an id that names nothing.
	CodeNotFound Code = "not_found"
	// CodeNotOpen is a review that has already been approved or abandoned.
	CodeNotOpen Code = "not_open"
	// CodeNotReleased is a reply to a comment the agent cannot have seen.
	CodeNotReleased Code = "not_released"
	// CodeNotTopLevel is a project root below its repository's top level.
	CodeNotTopLevel Code = "not_top_level"
	// CodeNothingCaptured is a first round where nothing differs from the
	// base.
	CodeNothingCaptured Code = "nothing_captured"
	// CodeNothingInScope is a first round where nothing under the review's
	// paths differs from the base.
	CodeNothingInScope Code = "nothing_in_scope"
	// CodeOutsideProject is a review path that leaves the project root.
	CodeOutsideProject Code = "outside_project"
	// CodePathClaimed is a path an open review in the project already covers.
	CodePathClaimed Code = "path_claimed"
	// CodePathUnmatched is a first-round path that is neither on disk nor
	// covered by anything captured.
	CodePathUnmatched Code = "path_unmatched"
	// CodeRoundClosed is an edit to a comment whose round has been released.
	CodeRoundClosed Code = "round_closed"
	// CodeTooManyFiles is a walk outside git that passed its file limit.
	CodeTooManyFiles Code = "too_many_files"
	// CodeUsage is a command line the CLI could not parse.
	CodeUsage Code = "usage"
	// CodeVanished is a file that disappeared while it was being captured.
	CodeVanished Code = "vanished"
	// CodeWrongAgent is a review opened under another agent name.
	CodeWrongAgent Code = "wrong_agent"
	// CodeWrongProject is a review that belongs to another project.
	CodeWrongProject Code = "wrong_project"
)

// Error is a refusal: what happened, and what resolves it.
type Error struct {
	// Code is the stable identifier for the refusal.
	Code Code `json:"code"`
	// Message says what happened.
	Message string `json:"message"`
	// Fix is the command or action that resolves it.
	Fix string `json:"fix"`
}

// ErrorBody is the whole body of every refusal.
type ErrorBody struct {
	// Error is the refusal.
	Error Error `json:"error"`
}

// Caller says who is asking: the directory the command ran in and the agent
// name it was given, by flag or by environment. An empty agent means the
// daemon names it from the project root.
type Caller struct {
	// Dir is the caller's working directory, as the shell spells it.
	Dir string `json:"dir"`
	// Agent is the explicit agent name, empty when none was given.
	Agent string `json:"agent"`
	// Home is the caller's home directory, which a default agent name is
	// written relative to. It is the caller's and not the daemon's, because
	// the daemon inherits whichever agent started it.
	Home string `json:"home"`
}

// RequestBody opens a review, or asks for the next round of one.
type RequestBody struct {
	// Caller is who is asking.
	Caller
	// Paths are the files and directories a new review claims. Empty with
	// Review set.
	Paths []string `json:"paths"`
	// Note is the agent's brief for the reviewer.
	Note string `json:"note"`
	// Title is what the queue shows, defaulting to the note's first line.
	Title string `json:"title"`
	// Base is the ref a first round is measured from, defaulting to HEAD.
	Base string `json:"base"`
	// Review is the review to ask the next round of, zero for a new review.
	Review int64 `json:"review"`
}

// ReceiptBody names the comments the agent's client printed.
type ReceiptBody struct {
	// Caller is who is asking.
	Caller
	// Comments are the ids printed.
	Comments []int64 `json:"comments"`
	// Round is the number of the round whose verdict was printed, and zero
	// when none was.
	Round int `json:"round"`
}

// ReplyBody is the agent's reply to one comment.
type ReplyBody struct {
	// Caller is who is asking.
	Caller
	// Body is the reply.
	Body string `json:"body"`
}

// AbandonBody withdraws a review.
type AbandonBody struct {
	// Caller is who is asking.
	Caller
}

// Daemon describes the running daemon.
type Daemon struct {
	// PID is the daemon's process id.
	PID int `json:"pid"`
	// Started is when the daemon started.
	Started time.Time `json:"started"`
	// Build identifies the binary the daemon is running.
	Build string `json:"build"`
	// StateDir is where the daemon keeps its database, blobs and socket.
	StateDir string `json:"state_dir"`
	// Surface is the reviewer's surface on the tailnet.
	Surface Surface `json:"surface"`
}

// Surface is the reviewer's surface on the tailnet, as the daemon reports it.
type Surface struct {
	// State is off, starting, serving or failed.
	State string `json:"state"`
	// URL is the surface's address while it is serving.
	URL string `json:"url"`
	// Reason says why it failed and what the owner does about it.
	Reason string `json:"reason"`
}

// Project is the project a command ran in.
type Project struct {
	// Name is what the surface displays.
	Name string `json:"name"`
	// Root is the project's root with symlinks resolved, which is its
	// identity.
	Root string `json:"root"`
}

// Review is one review as an agent sees it.
type Review struct {
	// ID is the review's id, which every other command takes.
	ID int64 `json:"id"`
	// Title is what the queue shows.
	Title string `json:"title"`
	// Agent is the agent that opened it.
	Agent string `json:"agent"`
	// State is waiting, changes_requested, approved or abandoned.
	State State `json:"state"`
	// Round is the latest round's number.
	Round int `json:"round"`
	// Paths are the files and directories the review claims.
	Paths []string `json:"paths"`
	// Project is the project the review belongs to.
	Project Project `json:"project"`
}

// State is where a review stands from the agent's side.
type State string

const (
	// StateWaiting is a review whose latest round has no decision.
	StateWaiting State = "waiting"
	// StateChangesRequested is a review whose latest round asked for changes.
	StateChangesRequested State = "changes_requested"
	// StateApproved is a review that ended in an approval.
	StateApproved State = "approved"
	// StateAbandoned is a review the agent withdrew.
	StateAbandoned State = "abandoned"
)

// Size is how much a round moved since the round before it, counting only
// paths inside the review's scope.
type Size struct {
	// Files is the number of paths that moved.
	Files int `json:"files"`
	// Added is the number of lines added.
	Added int `json:"added"`
	// Removed is the number of lines removed.
	Removed int `json:"removed"`
}

// RequestResult is what opening a review or a round returns.
type RequestResult struct {
	// Review is the review the round belongs to.
	Review Review `json:"review"`
	// Size is what the round moved.
	Size Size `json:"size"`
	// Next says what to do now.
	Next string `json:"next"`
}

// Kind is which of the outcomes a wait returned.
type Kind string

const (
	// KindVerdict is a decided latest round.
	KindVerdict Kind = "verdict"
	// KindAbandoned is a withdrawn review.
	KindAbandoned Kind = "abandoned"
	// KindComments is late comments on a review whose latest round still
	// waits.
	KindComments Kind = "comments"
	// KindStillWaiting is a wait the client gave up on at its timeout. The
	// client writes it: the daemon never sends it.
	KindStillWaiting Kind = "still_waiting"
)

// Verdict is the reviewer's decision on a round.
type Verdict string

const (
	// VerdictApprove ends the review.
	VerdictApprove Verdict = "approve"
	// VerdictChanges asks for another round.
	VerdictChanges Verdict = "changes"
)

// Comment is one point the reviewer made, as the agent receives it.
type Comment struct {
	// ID is what eyeball reply takes.
	ID int64 `json:"id"`
	// Round is the number of the round it was written on.
	Round int `json:"round"`
	// Level is line, passage, file or review.
	Level string `json:"level"`
	// Path is the file it is about, empty for a review-level comment.
	Path string `json:"path"`
	// StartLine and EndLine bound it, zero where the level has no range.
	StartLine int `json:"start_line"`
	// EndLine is the last line it covers.
	EndLine int `json:"end_line"`
	// Quoted is the text the range covers.
	Quoted string `json:"quoted"`
	// Body is what the reviewer wrote.
	Body string `json:"body"`
	// Late says it was written after its round's verdict, or on a withdrawn
	// review.
	Late bool `json:"late"`
	// Reply is the agent's reply, empty if none.
	Reply string `json:"reply"`
}

// WaitResult is what a wait returns.
type WaitResult struct {
	// Kind is verdict, abandoned, comments or still_waiting.
	Kind Kind `json:"kind"`
	// Review is the review waited on.
	Review Review `json:"review"`
	// Verdict is the latest round's decision, empty unless Kind is verdict.
	Verdict Verdict `json:"verdict"`
	// Note is the reviewer's note on the round, empty if none.
	Note string `json:"note"`
	// DecidedAt is when the verdict was given, and null without one. It is
	// always present, so a --format template that names it does not fail on
	// an answer with no verdict.
	DecidedAt *time.Time `json:"decided_at"`
	// Comments are the comments being handed over.
	Comments []Comment `json:"comments"`
	// Next says what to do now.
	Next string `json:"next"`
}

// StatusRow is one review in a status listing.
type StatusRow struct {
	// ID is the review's id.
	ID int64 `json:"id"`
	// Title is what the queue shows.
	Title string `json:"title"`
	// Agent is the agent that opened it.
	Agent string `json:"agent"`
	// Round is the latest round's number.
	Round int `json:"round"`
	// State is waiting, changes_requested, approved or abandoned.
	State State `json:"state"`
	// Size is what the latest round moved.
	Size Size `json:"size"`
	// RequestedAt is when the latest round was asked for.
	RequestedAt time.Time `json:"requested_at"`
	// Undelivered counts released comments the agent has not acknowledged.
	Undelivered int `json:"undelivered"`
}

// StatusResult is what eyeball status returns.
type StatusResult struct {
	// Daemon is the daemon answering.
	Daemon Daemon `json:"daemon"`
	// Project is the project the command ran in.
	Project Project `json:"project"`
	// Agent is the name the caller is known by.
	Agent string `json:"agent"`
	// Reviews are the caller's reviews, or every review with --all.
	Reviews []StatusRow `json:"reviews"`
	// Others counts reviews in the project under other agent names, which
	// --all lists.
	Others int `json:"others"`
	// Next says what to do now.
	Next string `json:"next"`
}

// File is one path a round captured.
type File struct {
	// Path is relative to the project root.
	Path string `json:"path"`
	// Digest is the sha256 of the content, empty for a deleted file.
	Digest string `json:"digest"`
	// Size is the content's length in bytes.
	Size int64 `json:"size"`
	// InScope says whether the review's paths cover it.
	InScope bool `json:"in_scope"`
}

// Round is one round of a review as eyeball show reports it.
type Round struct {
	// Number counts from one.
	Number int `json:"number"`
	// Note is the agent's brief for that round.
	Note string `json:"note"`
	// Base is the commit the round was measured from, empty outside git.
	Base string `json:"base"`
	// Verdict is the decision on the round, empty while it waits.
	Verdict Verdict `json:"verdict"`
	// VerdictNote is the reviewer's note on the round, empty if none.
	VerdictNote string `json:"verdict_note"`
	// DecidedAt is when it was decided, and null while it waits.
	DecidedAt *time.Time `json:"decided_at"`
	// Size is what the round moved.
	Size Size `json:"size"`
}

// ShowResult is what eyeball show returns: a review's record, readable by
// anything, with no comments, which are the agent's to receive.
type ShowResult struct {
	// Review is the review.
	Review Review `json:"review"`
	// Rounds are every round, first to latest.
	Rounds []Round `json:"rounds"`
	// Files are the latest round's captured files with their digests.
	Files []File `json:"files"`
}

// AbandonResult is what withdrawing a review returns.
type AbandonResult struct {
	// Review is the review, now abandoned.
	Review Review `json:"review"`
	// Next says what to do now.
	Next string `json:"next"`
}

// ReplyResult is what a reply returns.
type ReplyResult struct {
	// Comment is the comment replied to, with the reply.
	Comment Comment `json:"comment"`
	// Next says what to do now.
	Next string `json:"next"`
}

// ReceiptResult is what acknowledging comments returns.
type ReceiptResult struct {
	// Comments are the ids the agent acknowledged.
	Comments []int64 `json:"comments"`
}

// StopResult is what eyeball stop prints.
type StopResult struct {
	// Stopped says a daemon was running and was asked to stop. It is false
	// when there was nothing to stop.
	Stopped bool `json:"stopped"`
	// Daemon is the daemon that stopped, and null when none was running.
	Daemon *Daemon `json:"daemon"`
}
