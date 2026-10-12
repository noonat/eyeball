package server

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/store"
)

// handleCat answers GET /agent/cat: the bytes a file had in the approved
// review, in the caller's project, whose latest round is measured from base. A
// gate that must compare a merged file with what was approved reads it here,
// since the blob store is not a file it may trust.
//
// Like check, it reads and nothing else: the project is looked up and never
// registered, and nothing is acknowledged. The path is the caller's, relative
// to its working directory as every other path is. The content is held in
// memory and sent as JSON, so every answer is JSON and a client can tell a whole
// one from a cut one.
//
// Only a round that holds the file in scope and with content answers. Two
// approved reviews that hold different bytes for it are refused, because
// picking one would let a gate compare against whichever it happened to see
// first. Reviews that hold the same bytes are one answer.
func (s *Server) handleCat(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	c := callerFromQuery(r)
	if !filepath.IsAbs(c.Dir) {
		return badRequest("the working directory %q is not absolute", c.Dir)
	}
	q := r.URL.Query()
	if q.Get("base") == "" || q.Get("path") == "" {
		return refuse(http.StatusBadRequest, api.CodeMissing, "a cat needs the commit the review is measured from and a path", "run eyeball cat --base <commit> <path>")
	}
	root, err := capture.FindRoot(c.Dir)
	if err != nil {
		return err
	}
	project, err := s.db.ProjectByRoot(ctx, root.Resolved)
	if errors.Is(err, store.ErrNotFound) {
		message := fmt.Sprintf("no review has been opened in %s", root.Resolved)
		return refuse(http.StatusConflict, api.CodeNotCaptured, message, "request a review from the clone that merges, and have it approved")
	}
	if err != nil {
		return err
	}
	base, err := capture.ResolveCommit(ctx, root.Resolved, q.Get("base"))
	if err != nil {
		return err
	}
	want, err := capture.ReviewPath(root, c.Dir, q.Get("path"))
	if err != nil {
		return err
	}

	rows, err := s.db.s.ReviewsIn(ctx, project.ID)
	if err != nil {
		return err
	}
	var digest string
	var first int64
	var number int
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
		for _, f := range files {
			if f.Path != want || !f.InScope || f.Digest == "" {
				continue
			}
			if digest != "" && f.Digest != digest {
				message := fmt.Sprintf("approved reviews %d and %d hold different content for %s", first, row.ReviewID, want)
				return refuse(http.StatusConflict, api.CodeAmbiguous, message, "request a round of one review with the tree that merges, and have it approved")
			}
			if digest == "" {
				digest, first, number = f.Digest, row.ReviewID, row.Round
			}
		}
	}
	if digest == "" {
		message := fmt.Sprintf("no approved review in %s has a latest round measured from %s that captured %s", root.Resolved, base, want)
		return refuse(http.StatusConflict, api.CodeNotCaptured, message, "request a review that names the file, measured from this base, and have it approved")
	}

	file, err := s.blobs.Open(digest)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(file)
	if err != nil {
		return errors.Wrap(err, "read the file")
	}
	writeJSON(w, http.StatusOK, api.CatResult{
		Review:  first,
		Round:   number,
		Path:    want,
		Digest:  digest,
		Size:    int64(len(content)),
		Content: content,
	})
	return nil
}
