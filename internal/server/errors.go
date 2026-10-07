package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/store"
)

// refusal is an error a handler answers with as it stands: a status, and the
// body an agent reads.
type refusal struct {
	status int
	body   api.Error
}

// Error is the refusal's message.
func (r *refusal) Error() string {
	return r.body.Message
}

// refuse builds a refusal.
func refuse(status int, code api.Code, message, fix string) error {
	return errors.WithStack(&refusal{status: status, body: api.Error{Code: code, Message: message, Fix: fix}})
}

// mapping is how one sentinel from the store or capture becomes a refusal.
// Name is the sentinel's qualified name, which a test compares against every
// exported Err variable it finds by parsing those packages' source, so a
// sentinel added there without a row here fails the build's tests.
type mapping struct {
	name   string
	err    error
	status int
	code   api.Code
	fix    string
}

// mappings is every sentinel a handler can receive and what it means to an
// agent. A handler with something more specific to say returns its own refusal
// before the error reaches here.
var mappings = []mapping{
	{
		name:   "capture.ErrBadBase",
		err:    capture.ErrBadBase,
		status: http.StatusBadRequest,
		code:   api.CodeBadBase,
		fix:    "name a commit, a branch or a tag with --base, or leave it out to use HEAD",
	},
	{
		name:   "capture.ErrBadCommit",
		err:    capture.ErrBadCommit,
		status: http.StatusBadRequest,
		code:   api.CodeBadCommit,
		fix:    "pass a commit, a branch or a tag that exists in this repository",
	},
	{
		name:   "capture.ErrBaseGained",
		err:    capture.ErrBaseGained,
		status: http.StatusConflict,
		code:   api.CodeBaseGained,
		fix:    "abandon this review and open a new one, which will be measured from the new commit",
	},
	{
		name:   "capture.ErrNoGit",
		err:    capture.ErrNoGit,
		status: http.StatusBadRequest,
		code:   api.CodeNoGit,
		fix:    "install git, or put it on the PATH the daemon runs with, then run eyeball stop so the next command starts a daemon that can find it",
	},
	{
		name:   "capture.ErrNoProject",
		err:    capture.ErrNoProject,
		status: http.StatusBadRequest,
		code:   api.CodeNoProject,
		fix:    "run from inside a git repository, or create an empty .eyeball file at the project root",
	},
	{
		name:   "capture.ErrNoSuchPath",
		err:    capture.ErrNoSuchPath,
		status: http.StatusBadRequest,
		code:   api.CodeNoSuchPath,
		fix:    "check the path, which is relative to the directory the command ran in",
	},
	{
		name:   "capture.ErrNotFound",
		err:    capture.ErrNotFound,
		status: http.StatusNotFound,
		code:   api.CodeNotFound,
		fix:    "check the path",
	},
	{
		name:   "capture.ErrNotTopLevel",
		err:    capture.ErrNotTopLevel,
		status: http.StatusBadRequest,
		code:   api.CodeNotTopLevel,
		fix:    "run from the repository's top level",
	},
	{
		name:   "capture.ErrNothingCaptured",
		err:    capture.ErrNothingCaptured,
		status: http.StatusUnprocessableEntity,
		code:   api.CodeNothingCaptured,
		fix:    "check that the work is written to disk, name different paths, or pass --base with a commit from before the work began",
	},
	{
		name:   "capture.ErrOutsideProject",
		err:    capture.ErrOutsideProject,
		status: http.StatusBadRequest,
		code:   api.CodeOutsideProject,
		fix:    "name paths inside the project, relative to the directory the command ran in",
	},
	{
		// An error carrying it is marked ErrNotFound as well, so the
		// capture.ErrNotFound row, earlier in this table, answers it first,
		// with the same status. The row is here because every sentinel needs
		// one.
		name:   "capture.ErrRulesUnreadable",
		err:    capture.ErrRulesUnreadable,
		status: http.StatusNotFound,
		code:   api.CodeNotFound,
		fix:    "read eyeball.log in the state directory to see why git could not say what it ignores",
	},
	{
		name:   "capture.ErrTooManyFiles",
		err:    capture.ErrTooManyFiles,
		status: http.StatusUnprocessableEntity,
		code:   api.CodeTooManyFiles,
		fix:    "name narrower paths",
	},
	{
		name:   "capture.ErrVanished",
		err:    capture.ErrVanished,
		status: http.StatusConflict,
		code:   api.CodeVanished,
		fix:    "run the same request again once nothing is writing to the working copy",
	},
	{
		name:   "store.ErrAlreadyDecided",
		err:    store.ErrAlreadyDecided,
		status: http.StatusConflict,
		code:   api.CodeAlreadyDecided,
		fix:    "reload the review to see the verdict already given",
	},
	{
		name:   "store.ErrAwaitingVerdict",
		err:    store.ErrAwaitingVerdict,
		status: http.StatusConflict,
		code:   api.CodeAwaitingVerdict,
		fix:    "run eyeball wait with the review's id",
	},
	{
		name:   "store.ErrBaseMoved",
		err:    store.ErrBaseMoved,
		status: http.StatusConflict,
		code:   api.CodeBaseMoved,
		fix:    "abandon this review and open a new one",
	},
	{
		name:   "store.ErrEmptyChangeRequest",
		err:    store.ErrEmptyChangeRequest,
		status: http.StatusBadRequest,
		code:   api.CodeEmptyChangeRequest,
		fix:    "write a comment or add a note saying what to change",
	},
	{
		name:   "store.ErrInvalidPath",
		err:    store.ErrInvalidPath,
		status: http.StatusBadRequest,
		code:   api.CodeInvalidPath,
		fix:    "name files or directories inside the project, not the project root itself",
	},
	{
		name:   "store.ErrMissing",
		err:    store.ErrMissing,
		status: http.StatusBadRequest,
		code:   api.CodeMissing,
		fix:    "supply the missing value",
	},
	{
		name:   "store.ErrNotFound",
		err:    store.ErrNotFound,
		status: http.StatusNotFound,
		code:   api.CodeNotFound,
		fix:    "run eyeball status to list the reviews this agent has",
	},
	{
		name:   "store.ErrNotOpen",
		err:    store.ErrNotOpen,
		status: http.StatusConflict,
		code:   api.CodeNotOpen,
		fix:    "open a new review; this one has ended",
	},
	{
		name:   "store.ErrNotReleased",
		err:    store.ErrNotReleased,
		status: http.StatusConflict,
		code:   api.CodeNotReleased,
		fix:    "wait for the verdict with eyeball wait, which hands over the comments it releases",
	},
	{
		name:   "store.ErrNothingInScope",
		err:    store.ErrNothingInScope,
		status: http.StatusUnprocessableEntity,
		code:   api.CodeNothingInScope,
		fix:    "name the paths the work is in, or pass --base with a commit from before the work began",
	},
	{
		name:   "store.ErrPathClaimed",
		err:    store.ErrPathClaimed,
		status: http.StatusConflict,
		code:   api.CodePathClaimed,
		fix:    "run eyeball status to see the review that holds it",
	},
	{
		name:   "store.ErrRoundClosed",
		err:    store.ErrRoundClosed,
		status: http.StatusConflict,
		code:   api.CodeRoundClosed,
		fix:    "write a new comment instead",
	},
}

// toRefusal turns any error from a handler into the status and body it is
// answered with, and reports whether it was a failure inside the daemon.
func toRefusal(err error) (int, api.Error, bool) {
	var r *refusal
	if errors.As(err, &r) {
		return r.status, r.body, false
	}
	for _, m := range mappings {
		if errors.Is(err, m.err) {
			return m.status, api.Error{Code: m.code, Message: err.Error(), Fix: m.fix}, false
		}
	}
	// The message for a failure stays generic. The error itself can name a
	// file path, a SQL statement or a git command line, and the agent can do
	// nothing with any of them.
	internal := api.Error{
		Code:    api.CodeInternal,
		Message: "the daemon failed to answer",
		Fix:     "read eyeball.log in the state directory, then run the command again",
	}
	return http.StatusInternalServerError, internal, true
}

// writeJSON answers with a JSON body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// badRequest is the refusal for a request body or parameter the daemon could
// not read, which the CLI never sends and a hand-written client might.
func badRequest(format string, args ...any) error {
	return refuse(http.StatusBadRequest, api.CodeBadRequest, fmt.Sprintf(format, args...), "use the eyeball command rather than calling the socket directly")
}
