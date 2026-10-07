package server

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/store"
)

// handleCheck answers GET /agent/check: whether an approved review in the
// caller's project, opened by any agent, has a latest round measured from
// base whose capture matches new's tree. A git hook runs it before letting a
// default branch move from base to new.
//
// It reads and nothing else. The project is looked up and never registered,
// so a repository eyeball has not seen is refused rather than recorded, and
// nothing is acknowledged.
func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	c := callerFromQuery(r)
	if !filepath.IsAbs(c.Dir) {
		return badRequest("the working directory %q is not absolute", c.Dir)
	}
	q := r.URL.Query()
	if q.Get("base") == "" || q.Get("new") == "" {
		return refuse(http.StatusBadRequest, api.CodeMissing, "a check needs the commit the branch moves from and the one it moves to", "run eyeball check --base <old> <new>")
	}
	root, err := capture.FindRoot(c.Dir)
	if err != nil {
		return err
	}
	project, err := s.db.ProjectByRoot(ctx, root.Resolved)
	if errors.Is(err, store.ErrNotFound) {
		message := fmt.Sprintf("no review has been opened in %s", root.Resolved)
		return refuse(http.StatusConflict, api.CodeNotApproved, message, "request a review from the clone that merges, and have it approved")
	}
	if err != nil {
		return err
	}
	base, err := capture.ResolveCommit(ctx, root.Resolved, q.Get("base"))
	if err != nil {
		return err
	}
	target, err := capture.ResolveCommit(ctx, root.Resolved, q.Get("new"))
	if err != nil {
		return err
	}

	rows, err := s.db.s.ReviewsIn(ctx, project.ID)
	if err != nil {
		return err
	}
	differs := ""
	for _, row := range rows {
		if row.State != store.StateApproved {
			continue
		}
		round, err := s.db.s.Round(ctx, row.ReviewID, row.Round)
		if err != nil {
			return err
		}
		if round.BaseCommit != base {
			continue
		}
		files, err := s.db.s.RoundFiles(ctx, round.ID)
		if err != nil {
			return err
		}
		reason, err := capture.Match(ctx, root.Resolved, base, target, files)
		if err != nil {
			return err
		}
		if reason == "" {
			writeJSON(w, http.StatusOK, api.CheckResult{Review: row.ReviewID, Round: row.Round, Title: row.Title, Agent: row.Agent})
			return nil
		}
		if differs == "" {
			differs = fmt.Sprintf("review %d round %d: %s", row.ReviewID, row.Round, reason)
		}
	}
	if differs != "" {
		return refuse(http.StatusConflict, api.CodeTreeDiffers, differs, "request a round of the review with the tree that merges, and have it approved")
	}
	message := fmt.Sprintf("no approved review in %s has a latest round measured from %s", root.Resolved, base)
	return refuse(http.StatusConflict, api.CodeNotApproved, message, "request a review from the clone that merges, against this base, and have it approved")
}
