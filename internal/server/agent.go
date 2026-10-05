package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/store"
)

// titleLimit is how long a title taken from the note may be. A guess at what
// reads well in a queue row, to be tuned once there is a queue to look at.
const titleLimit = 72

// caller is who sent a request, once the daemon has found its project.
type caller struct {
	dir     string
	root    capture.Root
	project store.Project
	agent   string
}

// resolve finds the caller's project, registering it on first sight, and the
// name the caller is known by.
func (s *Server) resolve(ctx context.Context, c api.Caller) (caller, error) {
	if !filepath.IsAbs(c.Dir) {
		return caller{}, badRequest("the working directory %q is not absolute", c.Dir)
	}
	root, err := capture.FindRoot(c.Dir)
	if err != nil {
		return caller{}, err
	}
	name, err := capture.ProjectName(ctx, root.Resolved)
	if err != nil {
		// The name is for display only and is read once, when the project is
		// first seen. Failing the request over it would be worse than a
		// project named after its directory.
		s.log.Warn("project name", "root", root.Resolved, "err", err)
		name = ""
	}
	project, err := s.db.EnsureProject(ctx, root.Resolved, name)
	if err != nil {
		return caller{}, err
	}
	agent := capture.AgentName(c.Agent, "", root, s.home)
	return caller{dir: c.Dir, root: root, project: project, agent: agent}, nil
}

// callerFromQuery reads the caller from a GET request's query parameters.
func callerFromQuery(r *http.Request) api.Caller {
	q := r.URL.Query()
	return api.Caller{Dir: q.Get("dir"), Agent: q.Get("agent")}
}

// decode reads a JSON request body into v.
func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return badRequest("the request body is not valid JSON: %v", err)
	}
	return nil
}

// pathID reads a numeric path parameter.
func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, refuse(http.StatusBadRequest, api.CodeNotFound, fmt.Sprintf("%q is not a %s id", r.PathValue(name), name), "pass the number eyeball status lists")
	}
	return id, nil
}

// ownReview reads a review and refuses it unless the caller may act on it: the
// same project, and the same agent name.
//
// The project check is what stops an agent in one project from freezing its own
// working copy into another project's review with request --review. The agent
// check is product.md's own slice. Both refusals say how to proceed, because
// the usual cause is an agent that lost its context and forgot a name.
func (s *Server) ownReview(ctx context.Context, c caller, reviewID int64) (store.Review, error) {
	review, err := s.db.Review(ctx, reviewID)
	if err != nil {
		return store.Review{}, err
	}
	if review.ProjectID != c.project.ID {
		other, err := s.db.Project(ctx, review.ProjectID)
		if err != nil {
			return store.Review{}, err
		}
		message := fmt.Sprintf("review %d belongs to the project at %s, not %s", reviewID, other.Root, c.project.Root)
		return store.Review{}, refuse(http.StatusForbidden, api.CodeWrongProject, message, "run the command from inside "+other.Root)
	}
	if review.Agent != c.agent {
		message := fmt.Sprintf("review %d was opened by %s, and this command runs as %s", reviewID, review.Agent, c.agent)
		fix := fmt.Sprintf("pass --agent %q, or set EYEBALL_AGENT, if that agent is this one", review.Agent)
		return store.Review{}, refuse(http.StatusForbidden, api.CodeWrongAgent, message, fix)
	}
	return review, nil
}

// latest reads a review's newest round and its decision, which is nil while
// the round waits.
func (s *Server) latest(ctx context.Context, review store.Review) (store.Round, *store.Decision, error) {
	round, err := s.db.Round(ctx, review.ID, review.LatestRound)
	if err != nil {
		return store.Round{}, nil, err
	}
	decision, err := s.db.Decision(ctx, round.ID)
	if errors.Is(err, store.ErrNotFound) {
		return round, nil, nil
	}
	if err != nil {
		return store.Round{}, nil, err
	}
	return round, &decision, nil
}

// agentState is where a review stands from the agent's side. Waiting and
// working are not stored states: they follow from whether the latest round has
// a decision.
func agentState(review store.Review, decision *store.Decision) api.State {
	switch {
	case review.State == store.StateApproved:
		return api.StateApproved
	case review.State == store.StateAbandoned:
		return api.StateAbandoned
	case decision == nil:
		return api.StateWaiting
	default:
		return api.StateChangesRequested
	}
}

// apiReview is a review as the wire shows it.
func (s *Server) apiReview(ctx context.Context, review store.Review) (api.Review, error) {
	project, err := s.db.Project(ctx, review.ProjectID)
	if err != nil {
		return api.Review{}, err
	}
	_, decision, err := s.latest(ctx, review)
	if err != nil {
		return api.Review{}, err
	}
	return api.Review{
		ID:      review.ID,
		Title:   review.Title,
		Agent:   review.Agent,
		State:   agentState(review, decision),
		Round:   review.LatestRound,
		Paths:   review.Paths,
		Project: api.Project{Name: project.Name, Root: project.Root},
	}, nil
}

// handleDaemon reports the daemon.
func (s *Server) handleDaemon(w http.ResponseWriter, _ *http.Request) error {
	writeJSON(w, http.StatusOK, s.daemon)
	return nil
}

// handleStop answers, then asks the daemon to exit. The shutdown cannot run in
// here: it waits for active requests, and this request is one of them.
func (s *Server) handleStop(w http.ResponseWriter, _ *http.Request) error {
	daemon := s.daemon
	writeJSON(w, http.StatusOK, api.StopResult{Stopped: true, Daemon: &daemon})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	s.stop()
	return nil
}

// handleRequest opens a review, or asks for the next round of one.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) error {
	var body api.RequestBody
	if err := decode(r, &body); err != nil {
		return err
	}
	if strings.TrimSpace(body.Note) == "" {
		return refuse(http.StatusBadRequest, api.CodeMissing, "a request needs a note saying what to look at", `pass -m "what to look at"`)
	}
	if body.Review != 0 && len(body.Paths) > 0 {
		return refuse(http.StatusBadRequest, api.CodeUsage, "a next round takes no paths, because a review's scope is fixed when it opens", fmt.Sprintf(`run eyeball request --review %d -m "..." with no paths`, body.Review))
	}
	if body.Review != 0 && body.Base != "" {
		return refuse(http.StatusBadRequest, api.CodeUsage, "a next round takes no base, because a review's base is fixed at its first round", fmt.Sprintf(`run eyeball request --review %d -m "..." without --base`, body.Review))
	}
	if body.Review == 0 && len(body.Paths) == 0 {
		return refuse(http.StatusBadRequest, api.CodeMissing, "a new review needs at least one path", `name the files or directories to review: eyeball request <paths...> -m "..."`)
	}
	c, err := s.resolve(r.Context(), body.Caller)
	if err != nil {
		return err
	}
	var result api.RequestResult
	if body.Review != 0 {
		result, err = s.nextRound(r.Context(), c, body)
	} else {
		result, err = s.firstRound(r.Context(), c, body)
	}
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

// firstRound opens a review.
func (s *Server) firstRound(ctx context.Context, c caller, body api.RequestBody) (api.RequestResult, error) {
	paths := make([]string, 0, len(body.Paths))
	for _, p := range body.Paths {
		rel, err := capture.ReviewPath(c.root, c.dir, p)
		if err != nil {
			return api.RequestResult{}, err
		}
		normalized, err := store.NormalizePath(rel)
		if err != nil {
			return api.RequestResult{}, err
		}
		paths = append(paths, normalized)
	}
	base, err := capture.ResolveBase(ctx, c.root.Resolved, body.Base)
	if err != nil {
		return api.RequestResult{}, err
	}
	req, err := capture.Freeze(ctx, capture.Options{
		Root:  c.root.Resolved,
		Base:  base,
		Paths: paths,
		Round: 1,
		Blobs: s.blobs,
	})
	if err != nil {
		return api.RequestResult{}, err
	}
	if err := refuseUnmatched(c.root.Resolved, paths, req.Files); err != nil {
		return api.RequestResult{}, err
	}
	req.Note = strings.TrimSpace(body.Note)
	title := strings.TrimSpace(body.Title)
	if title == "" {
		title = defaultTitle(body.Note)
	}
	review, round, err := s.db.OpenReview(ctx, store.NewReview{
		ProjectID: c.project.ID,
		Agent:     c.agent,
		Title:     title,
		Paths:     paths,
		Request:   req,
	})
	if err != nil {
		return api.RequestResult{}, s.explainClaim(ctx, c, err)
	}
	return s.requestResult(ctx, review, round)
}

// nextRound asks for the next round of the caller's own review.
func (s *Server) nextRound(ctx context.Context, c caller, body api.RequestBody) (api.RequestResult, error) {
	review, err := s.ownReview(ctx, c, body.Review)
	if err != nil {
		return api.RequestResult{}, err
	}
	if review.State != store.StateOpen {
		return api.RequestResult{}, refuse(http.StatusConflict, api.CodeNotOpen, fmt.Sprintf("review %d is %s", review.ID, review.State), "open a new review with eyeball request <paths...>")
	}
	last, decision, err := s.latest(ctx, review)
	if err != nil {
		return api.RequestResult{}, err
	}
	// Checked before capturing, so an agent asking twice is refused without a
	// capture's worth of git calls. The store checks again in its transaction.
	if decision == nil {
		message := fmt.Sprintf("review %d round %d is still waiting for a verdict", review.ID, last.Number)
		return api.RequestResult{}, refuse(http.StatusConflict, api.CodeAwaitingVerdict, message, fmt.Sprintf("run eyeball wait %d", review.ID))
	}
	first, err := s.db.Round(ctx, review.ID, 1)
	if err != nil {
		return api.RequestResult{}, err
	}
	stored, err := s.db.RoundFiles(ctx, last.ID)
	if err != nil {
		return api.RequestResult{}, err
	}
	previous := make([]store.File, 0, len(stored))
	for _, f := range stored {
		previous = append(previous, f.File)
	}
	req, err := capture.Freeze(ctx, capture.Options{
		Root:     c.root.Resolved,
		Base:     first.BaseCommit,
		Paths:    review.Paths,
		Round:    last.Number + 1,
		Previous: previous,
		Blobs:    s.blobs,
	})
	if err != nil {
		return api.RequestResult{}, err
	}
	req.Note = strings.TrimSpace(body.Note)
	round, err := s.db.OpenRound(ctx, review.ID, req)
	if err != nil {
		return api.RequestResult{}, err
	}
	review.LatestRound = round.Number
	return s.requestResult(ctx, review, round)
}

// requestResult is what a request answers with, including the one instruction
// in the tool that says to stop.
func (s *Server) requestResult(ctx context.Context, review store.Review, round store.Round) (api.RequestResult, error) {
	wire, err := s.apiReview(ctx, review)
	if err != nil {
		return api.RequestResult{}, err
	}
	next := fmt.Sprintf("stop here. Run eyeball wait %d and do not continue this work until it returns with the verdict.", review.ID)
	return api.RequestResult{
		Review: wire,
		Size:   api.Size{Files: round.Files, Added: round.Added, Removed: round.Removed},
		Next:   next,
	}, nil
}

// refuseUnmatched refuses a first-round path that is neither on disk nor
// covered by anything captured, a deletion included. It is almost always a
// typo, and the review would otherwise claim a scope that holds nothing.
func refuseUnmatched(root string, paths []string, files []store.File) error {
	for _, p := range paths {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(p))); err == nil {
			continue
		}
		covered := false
		for _, f := range files {
			if store.Covers([]string{p}, f.Path) {
				covered = true
				break
			}
		}
		if !covered {
			message := fmt.Sprintf("%s is not in the project and nothing captured is under it", p)
			return refuse(http.StatusBadRequest, api.CodePathUnmatched, message, "check the path, which is relative to the directory the command ran in")
		}
	}
	return nil
}

// explainClaim replaces the overlap refusal's generic fix with the step that
// fits the review holding the path.
func (s *Server) explainClaim(ctx context.Context, c caller, err error) error {
	var claimed *store.ClaimedError
	if !errors.As(err, &claimed) {
		return err
	}
	message := fmt.Sprintf("%s is covered by %s, claimed by review %d (%s)", claimed.Path, claimed.Claimed, claimed.ReviewID, claimed.Title)
	if claimed.Agent != c.agent {
		fix := fmt.Sprintf("review %d belongs to agent %s: pass --agent %q if that is this agent, or name other paths", claimed.ReviewID, claimed.Agent, claimed.Agent)
		return refuse(http.StatusConflict, api.CodePathClaimed, message, fix)
	}
	holder, rerr := s.db.Review(ctx, claimed.ReviewID)
	if rerr != nil {
		return rerr
	}
	_, decision, rerr := s.latest(ctx, holder)
	if rerr != nil {
		return rerr
	}
	fix := fmt.Sprintf("review %d is waiting for a verdict: run eyeball wait %d", claimed.ReviewID, claimed.ReviewID)
	if decision != nil {
		fix = fmt.Sprintf(`to ask again on review %d, run eyeball request --review %d -m "what changed"`, claimed.ReviewID, claimed.ReviewID)
	}
	return refuse(http.StatusConflict, api.CodePathClaimed, message, fix)
}

// defaultTitle is the first line of the note that is not blank, cut at a word
// boundary to fit a queue row.
func defaultTitle(note string) string {
	for line := range strings.Lines(note) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) <= titleLimit {
			return line
		}
		// Cut on a character boundary, so a title with no space in its first
		// bytes is still valid UTF-8.
		end := titleLimit
		for end > 0 && !utf8.RuneStart(line[end]) {
			end--
		}
		cut := line[:end]
		if i := strings.LastIndexByte(cut, ' '); i > 0 {
			cut = cut[:i]
		}
		return strings.TrimSpace(cut)
	}
	return ""
}

// handleWait blocks until the review has something for the agent: a verdict,
// a withdrawal, or comments written after a verdict.
func (s *Server) handleWait(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "review")
	if err != nil {
		return err
	}
	c, err := s.resolve(r.Context(), callerFromQuery(r))
	if err != nil {
		return err
	}
	if _, err := s.ownReview(r.Context(), c, id); err != nil {
		return err
	}

	// Subscribe before reading. Reading first leaves a gap in which a
	// verdict lands unseen, and the agent then waits forever on a review that
	// was answered.
	changed, unsubscribe := s.hub.subscribe()
	defer unsubscribe()
	stopping := func() error {
		// The daemon cancels every request's context before it shuts down.
		// Answering with nothing would be a 200 with an empty body, which a
		// client reads as a verdict with no comments.
		return refuse(http.StatusServiceUnavailable, api.CodeDaemonStopping, "the daemon stopped while this wait was open", fmt.Sprintf("run eyeball wait %d again", id))
	}
	for {
		// A signal and the cancellation can arrive together, and select picks
		// either. Reading on a canceled context would fail as an internal
		// error rather than say the daemon is stopping.
		if r.Context().Err() != nil {
			return stopping()
		}
		select {
		case <-s.stopping:
			return stopping()
		default:
		}
		result, ready, err := s.waitState(r.Context(), id)
		if err != nil {
			if r.Context().Err() != nil {
				return stopping()
			}
			return err
		}
		if hook := s.afterRead.Load(); hook != nil {
			(*hook)()
		}
		if ready {
			writeJSON(w, http.StatusOK, result)
			return nil
		}
		select {
		case <-changed:
		case <-s.stopping:
			return stopping()
		case <-r.Context().Done():
			return stopping()
		}
	}
}

// waitState reads what a wait would return now, and whether it is ready to.
//
// The first kind that applies wins: a withdrawn review, then a decided latest
// round, then late comments the agent has not acknowledged. A verdict batch
// holds every comment on the decided round, delivered or not, so running wait
// again returns the same batch, plus every released comment from an earlier
// round the agent has not acknowledged, so none is lost once a later round
// exists.
func (s *Server) waitState(ctx context.Context, reviewID int64) (api.WaitResult, bool, error) {
	review, err := s.db.Review(ctx, reviewID)
	if err != nil {
		return api.WaitResult{}, false, err
	}
	last, decision, err := s.latest(ctx, review)
	if err != nil {
		return api.WaitResult{}, false, err
	}
	released, err := s.db.Released(ctx, reviewID)
	if err != nil {
		return api.WaitResult{}, false, err
	}
	wire, err := s.apiReview(ctx, review)
	if err != nil {
		return api.WaitResult{}, false, err
	}
	result := api.WaitResult{Review: wire, Comments: []api.Comment{}}

	switch {
	case review.State == store.StateAbandoned:
		result.Kind = api.KindAbandoned
		result.Comments = pick(released, func(c store.ReleasedComment) bool { return c.Delivered.IsZero() })
		result.Next = "the review was withdrawn. Nothing is waiting on it."
		return result, true, nil
	case decision != nil:
		result.Kind = api.KindVerdict
		result.Verdict = api.Verdict(decision.Verdict)
		result.Note = decision.Note
		decided := decision.Decided
		result.DecidedAt = &decided
		result.Comments = pick(released, func(c store.ReleasedComment) bool {
			return c.Round == last.Number || c.Delivered.IsZero()
		})
		result.Next = verdictNext(review.ID, decision.Verdict)
		return result, true, nil
	}
	late := pick(released, func(c store.ReleasedComment) bool { return c.Late && c.Delivered.IsZero() })
	if len(late) > 0 {
		result.Kind = api.KindComments
		result.Comments = late
		result.Next = fmt.Sprintf("these comments arrived after an earlier verdict. Round %d is still waiting: run eyeball wait %d again and do not continue until it returns.", last.Number, review.ID)
		return result, true, nil
	}
	return api.WaitResult{}, false, nil
}

// verdictNext is what to do after a verdict.
func verdictNext(reviewID int64, verdict store.Verdict) string {
	if verdict == store.VerdictApprove {
		return "approved. The work may continue."
	}
	return fmt.Sprintf(`changes requested. Make the changes, reply to a comment with eyeball reply <comment> -m "what was done" where it helps, then run eyeball request --review %d -m "what changed".`, reviewID)
}

// pick converts the released comments that keep returns true for.
func pick(in []store.ReleasedComment, keep func(store.ReleasedComment) bool) []api.Comment {
	out := []api.Comment{}
	for _, c := range in {
		if keep(c) {
			out = append(out, apiComment(c))
		}
	}
	return out
}

// apiComment is a released comment as the wire shows it.
func apiComment(c store.ReleasedComment) api.Comment {
	return api.Comment{
		ID:        c.ID,
		Round:     c.Round,
		Level:     string(c.Level),
		Path:      c.Path,
		StartLine: c.StartLine,
		EndLine:   c.EndLine,
		Quoted:    c.Quoted,
		Body:      c.Body,
		Late:      c.Late,
		Reply:     c.Reply,
	}
}

// handleReceipt stamps the comments the agent's client printed.
func (s *Server) handleReceipt(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "review")
	if err != nil {
		return err
	}
	var body api.ReceiptBody
	if err := decode(r, &body); err != nil {
		return err
	}
	c, err := s.resolve(r.Context(), body.Caller)
	if err != nil {
		return err
	}
	if _, err := s.ownReview(r.Context(), c, id); err != nil {
		return err
	}
	if err := s.db.MarkDelivered(r.Context(), id, body.Comments); err != nil {
		return err
	}
	ids := body.Comments
	if ids == nil {
		ids = []int64{}
	}
	writeJSON(w, http.StatusOK, api.ReceiptResult{Comments: ids})
	return nil
}

// handleAbandon withdraws a review.
func (s *Server) handleAbandon(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "review")
	if err != nil {
		return err
	}
	var body api.AbandonBody
	if err := decode(r, &body); err != nil {
		return err
	}
	c, err := s.resolve(r.Context(), body.Caller)
	if err != nil {
		return err
	}
	if _, err := s.ownReview(r.Context(), c, id); err != nil {
		return err
	}
	if err := s.db.Abandon(r.Context(), id); err != nil {
		return err
	}
	review, err := s.db.Review(r.Context(), id)
	if err != nil {
		return err
	}
	wire, err := s.apiReview(r.Context(), review)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, api.AbandonResult{Review: wire, Next: "withdrawn. Nothing is waiting on this review now."})
	return nil
}

// handleReply stores the agent's reply to one released comment.
func (s *Server) handleReply(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "comment")
	if err != nil {
		return err
	}
	var body api.ReplyBody
	if err := decode(r, &body); err != nil {
		return err
	}
	c, err := s.resolve(r.Context(), body.Caller)
	if err != nil {
		return err
	}
	reviewID, err := s.db.CommentReview(r.Context(), id)
	if err != nil {
		return err
	}
	if _, err := s.ownReview(r.Context(), c, reviewID); err != nil {
		return err
	}
	if err := s.db.Reply(r.Context(), id, body.Body); err != nil {
		return err
	}
	released, err := s.db.Released(r.Context(), reviewID)
	if err != nil {
		return err
	}
	for _, rc := range released {
		if rc.ID == id {
			writeJSON(w, http.StatusOK, api.ReplyResult{Comment: apiComment(rc), Next: "replied. The reviewer sees it with the comment."})
			return nil
		}
	}
	return errors.Newf("comment %d was replied to and is not released", id)
}

// handleStatus lists the caller's reviews.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) error {
	c, err := s.resolve(r.Context(), callerFromQuery(r))
	if err != nil {
		return err
	}
	all := r.URL.Query().Get("all") == "1"
	everyone, err := s.db.ReviewsIn(r.Context(), c.project.ID)
	if err != nil {
		return err
	}
	listed := everyone
	if !all {
		if listed, err = s.db.ReviewsFor(r.Context(), c.project.ID, c.agent); err != nil {
			return err
		}
	}
	result := api.StatusResult{
		Daemon:  s.daemon,
		Project: api.Project{Name: c.project.Name, Root: c.project.Root},
		Agent:   c.agent,
		Reviews: []api.StatusRow{},
	}
	for _, row := range everyone {
		if row.Agent != c.agent {
			result.Others++
		}
	}
	for _, row := range listed {
		if !all && row.State != store.StateOpen && row.Undelivered == 0 {
			continue
		}
		result.Reviews = append(result.Reviews, statusRow(row))
	}
	result.Next = statusNext(result, all)
	writeJSON(w, http.StatusOK, result)
	return nil
}

// statusRow is a listing row as the wire shows it.
func statusRow(row store.ReviewRow) api.StatusRow {
	state := api.StateWaiting
	switch {
	case row.State == store.StateApproved:
		state = api.StateApproved
	case row.State == store.StateAbandoned:
		state = api.StateAbandoned
	case row.Verdict != "":
		state = api.StateChangesRequested
	}
	return api.StatusRow{
		ID:          row.ReviewID,
		Title:       row.Title,
		Agent:       row.Agent,
		Round:       row.Round,
		State:       state,
		Size:        api.Size{Files: row.Files, Added: row.Added, Removed: row.Removed},
		RequestedAt: row.RequestedAt,
		Undelivered: row.Undelivered,
	}
}

// statusNext is what status suggests, from what it found.
func statusNext(result api.StatusResult, all bool) string {
	var lines []string
	for _, row := range result.Reviews {
		switch {
		case row.State == api.StateWaiting:
			lines = append(lines, fmt.Sprintf("review %d is waiting for a verdict: run eyeball wait %d", row.ID, row.ID))
		case row.Undelivered > 0 || row.State == api.StateChangesRequested:
			lines = append(lines, fmt.Sprintf("review %d has comments to read: run eyeball wait %d", row.ID, row.ID))
		}
	}
	if !all && result.Others > 0 {
		lines = append(lines, fmt.Sprintf("%d more reviews in this project belong to other agent names: run eyeball status --all to list them", result.Others))
	}
	if len(lines) == 0 {
		return "nothing is waiting."
	}
	return strings.Join(lines, "\n")
}

// handleShow reads any review's record. It is the one route with no project or
// agent check and no receipt, because a gate reading an approval is not the
// agent that opened the review, and must not mark the agent's comments read.
func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "review")
	if err != nil {
		return err
	}
	review, err := s.db.Review(r.Context(), id)
	if err != nil {
		return err
	}
	wire, err := s.apiReview(r.Context(), review)
	if err != nil {
		return err
	}
	result := api.ShowResult{Review: wire, Rounds: []api.Round{}, Files: []api.File{}}
	var latestID int64
	for n := 1; n <= review.LatestRound; n++ {
		round, err := s.db.Round(r.Context(), id, n)
		if err != nil {
			return err
		}
		entry := api.Round{
			Number: round.Number,
			Note:   round.Note,
			Base:   round.BaseCommit,
			Size:   api.Size{Files: round.Files, Added: round.Added, Removed: round.Removed},
		}
		decision, err := s.db.Decision(r.Context(), round.ID)
		switch {
		case err == nil:
			entry.Verdict = api.Verdict(decision.Verdict)
			decided := decision.Decided.Truncate(time.Millisecond)
			entry.DecidedAt = &decided
		case !errors.Is(err, store.ErrNotFound):
			return err
		}
		result.Rounds = append(result.Rounds, entry)
		latestID = round.ID
	}
	files, err := s.db.RoundFiles(r.Context(), latestID)
	if err != nil {
		return err
	}
	for _, f := range files {
		result.Files = append(result.Files, api.File{Path: f.Path, Digest: f.Digest, Size: f.Size, InScope: f.InScope})
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}
