package server

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball/internal/store"
	"github.com/noonat/eyeball/internal/view"
)

// maxWriteBytes bounds a write's JSON body. A comment is typed on a phone, so
// a megabyte is far past anything a person wrote, and the bound stops a body
// from being read whole into memory.
const maxWriteBytes = 1 << 20

// refuseJSON answers a write the server will not do with a view.Refusal.
func refuseJSON(w http.ResponseWriter, status int, text string) {
	writeJSON(w, status, view.Refusal{Message: text})
}

// readJSON decodes a write's JSON body into T, and answers a body it cannot
// read with a refusal. A field the type does not hold is refused as well: the
// types are generated from Go, so an unknown field means the app and the server
// disagree.
//
// A body that is not declared as JSON is answered 415 with a Refusal, which
// the app can show, rather than failing later in the decoder. The cross-origin
// check every surface request passes is what turns a cross-site post away. This
// is a second layer behind it, kept for being explicit.
func readJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var out T
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "application/json" {
		refuseJSON(w, http.StatusUnsupportedMediaType, notThisForm)
		return out, false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxWriteBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		refuseJSON(w, http.StatusBadRequest, notThisForm)
		return out, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		refuseJSON(w, http.StatusBadRequest, notThisForm)
		return out, false
	}
	return out, true
}

// handleAddCommentJSON answers POST /api/reviews/{review}/comments: one comment
// on the round the body names.
func (s *Server) handleAddCommentJSON(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	review, ok, err := s.writeReview(w, r)
	if !ok || err != nil {
		return err
	}
	req, ok := readJSON[view.CommentRequest](w, r)
	if !ok {
		return nil
	}
	round, ok, err := s.findRound(w, r, review, int(req.Round))
	if !ok || err != nil {
		return err
	}
	in := commentInput{
		level:       store.Level(req.Level),
		path:        req.Path,
		start:       int(req.Start),
		end:         int(req.End),
		old:         int(req.OldLine),
		againstBase: req.Against == "base",
		body:        req.Body,
		submission:  req.Submission,
		malformed:   req.Start < 0 || req.End < 0 || req.OldLine < 0,
	}
	stored, rj, err := s.storeComment(ctx, review, round, in)
	if err != nil {
		return err
	}
	if rj != nil {
		refuseJSON(w, rj.status, rj.text)
		return nil
	}
	return s.answerStored(ctx, w, review, stored.ID)
}

// handleEditCommentJSON answers PUT /api/reviews/{review}/comments/{comment}.
func (s *Server) handleEditCommentJSON(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	review, ok, err := s.writeReview(w, r)
	if !ok || err != nil {
		return err
	}
	id, ok, err := s.reviewComment(w, r, review)
	if !ok || err != nil {
		return err
	}
	req, ok := readJSON[view.CommentEdit](w, r)
	if !ok {
		return nil
	}
	rj, err := s.changeComment(ctx, id, req.Body)
	if err != nil {
		return err
	}
	if rj != nil {
		refuseJSON(w, rj.status, rj.text)
		return nil
	}
	return s.answerStored(ctx, w, review, id)
}

// answerStored answers a comment that was added or edited with the comment as
// the round's view sends it, and the count the bar shows.
func (s *Server) answerStored(ctx context.Context, w http.ResponseWriter, review store.Review, id int64) error {
	card, err := s.card(ctx, review, id)
	if err != nil {
		return err
	}
	tally, err := s.tally(ctx, review)
	if err != nil {
		return err
	}
	comments := viewCards([]cardView{card})
	writeJSON(w, http.StatusOK, view.CommentStored{Comment: comments[0], Count: int32(tally)})
	return nil
}

// handleDeleteCommentJSON answers DELETE /api/reviews/{review}/comments/
// {comment}, which has no body: the address names everything.
func (s *Server) handleDeleteCommentJSON(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	review, ok, err := s.writeReview(w, r)
	if !ok || err != nil {
		return err
	}
	id, ok, err := s.reviewComment(w, r, review)
	if !ok || err != nil {
		return err
	}
	rj, err := s.removeComment(ctx, id)
	if err != nil {
		return err
	}
	if rj != nil {
		refuseJSON(w, rj.status, rj.text)
		return nil
	}
	tally, err := s.tally(ctx, review)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, view.CommentDeleted{Count: int32(tally)})
	return nil
}

// handleDecideJSON answers POST /api/reviews/{review}/decision with no body
// once the verdict is stored. A change request with nothing in it is a 422 with
// the sentence, which the bar draws as its refused state.
func (s *Server) handleDecideJSON(w http.ResponseWriter, r *http.Request) error {
	review, ok, err := s.writeReview(w, r)
	if !ok || err != nil {
		return err
	}
	req, ok := readJSON[view.DecisionRequest](w, r)
	if !ok {
		return nil
	}
	round, ok, err := s.findRound(w, r, review, int(req.Round))
	if !ok || err != nil {
		return err
	}
	rj, err := s.recordVerdict(r.Context(), review, round, store.Verdict(req.Verdict), req.Note)
	if err != nil {
		return err
	}
	if rj != nil {
		refuseJSON(w, rj.status, rj.text)
		return nil
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
