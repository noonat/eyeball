package server

import (
	"net/http"
	"sort"

	"github.com/noonat/eyeball/internal/view"
)

// handleReviewView answers GET /api/reviews/{review} and GET /api/reviews/
// {review}/rounds/{round}: one round of a review, with the bodies the sending
// thresholds allow.
func (s *Server) handleReviewView(w http.ResponseWriter, r *http.Request) error {
	return s.serveView(w, r, s.roundAnswer)
}

// handleCardView answers GET /api/reviews/{review}/rounds/{round}/cards/
// {path...}: one card's body. It compares only its own file, so a long review
// does not compare every file again for each body it asks for. The round's view
// and this route both build the body with cardBody, and a Go test finds the
// two equal for every card.
func (s *Server) handleCardView(w http.ResponseWriter, r *http.Request) error {
	_, page, ok, err := s.roundPageOf(r, r.PathValue("path"))
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	if ok {
		for _, f := range page.Shown {
			if f.Path == r.PathValue("path") {
				writeJSON(w, http.StatusOK, cardBody(f))
				return nil
			}
		}
	}
	writeJSON(w, http.StatusNotFound, view.NotFound{WaitingCount: s.waitingCount(r.URL.Path)})
	return nil
}

// reviewView is a round's page as the JSON view holds it.
func reviewView(p page, body reviewPage, waiting *int32) view.Review {
	out := view.Review{
		Title:        p.Title,
		Back:         p.Back,
		BackLabel:    p.BackLabel,
		WaitingCount: waiting,
		ID:           body.ID,
		Round:        int32(body.Round),
		ReviewTitle:  body.Title,
		Project:      body.Project,
		ProjectHref:  body.ProjectHref,
		Agent:        body.Agent,
		Abandoned:    body.Abandoned,
		Waiting:      body.Waiting,
		Note:         body.Note,
		Files:        body.Files,
		Added:        body.Added,
		Removed:      body.Removed,
		Listed:       body.Listed,
		Fresh:        body.Fresh != nil,
		Comments:     viewComments(body.Comments),
		Replies:      viewComments(body.Replies),
		Closed:       body.Closed,
		AllFiles:     body.AllFiles,
		Stream:       body.Stream,
		Stamp:        body.Stamp,
		Place:        body.Place,
		Unmeasurable: body.Unmeasurable,
	}
	if d := body.Decided; d != nil {
		out.Decided = &view.Decided{Approved: d.Approved, Pill: d.Pill, Sentence: d.Sentence, Note: d.Note}
	}
	if a := body.Against; a != nil {
		out.Against = &view.Against{Previous: a.Previous, Base: a.Base}
	}
	for _, e := range body.Earlier {
		out.Earlier = append(out.Earlier, view.EarlierRound(e))
	}
	if a := body.Acting; a != nil {
		acting := &view.Acting{State: a.State, Against: a.Against, ListID: a.List.ID, Comments: viewCards(a.List.Cards)}
		if a.Bar != nil {
			acting.Bar = &view.Bar{Action: "/api" + a.Bar.Action, Count: int32(a.Bar.Count)}
		}
		out.Acting = acting
	}
	for _, f := range body.Shown {
		out.Cards = append(out.Cards, reviewCard(f))
	}
	return out
}

// reviewCard is one file of a round as the JSON view holds it. A deferred or
// held card has no body.
func reviewCard(f fileView) view.Card {
	card := view.Card{
		Path:          f.Path,
		ID:            f.CardID(),
		Added:         f.Added,
		Removed:       f.Removed,
		Unmeasured:    f.Unmeasured,
		Whole:         f.Whole,
		RawDiff:       f.RawDiff,
		Held:          f.Held,
		HeldHint:      f.HeldHint,
		Deferred:      f.Deferred,
		BodyHref:      "/api" + f.BodyHref,
		SkeletonHunks: int32(f.SkeletonHunks()),
		SkeletonLines: int32(f.SkeletonLines()),
	}
	if !f.Deferred && f.Held == "" {
		body := cardBody(f)
		card.Body = &body
	}
	return card
}

// cardBody is what a card shows for one file, with the comments on it. The
// round's view and the cards route both build a body here, so a body fetched
// later is the one the round would have sent.
func cardBody(f fileView) view.CardBody {
	body := view.CardBody{Mode: f.Mode, Slot: f.Slot}
	switch {
	case f.Rendered != nil:
		body.Kind = view.FileBodyRendered
		body.Rendered = viewRendering(f.Rendered)
	case f.Deleted:
		body.Kind = view.FileBodyDeleted
	case f.IsDocument:
		body.Kind = view.FileBodyDocument
		body.Passages = docParts(f.Parts, func(first, last int) *view.Thread {
			cards := f.PassageCards[[2]int{first, last}]
			if len(cards) == 0 {
				return nil
			}
			return &view.Thread{ID: passageID(f.Path, first, last), Comments: viewCards(cards)}
		})
	case f.Diff.Binary:
		body.Kind = view.FileBodyBinary
	case f.Diff.TooLarge:
		body.Kind = view.FileBodyTooLarge
	default:
		body.Kind = view.FileBodyRows
		body.Rows = codeRows(f.Diff)
	}
	if f.List == nil {
		body.Comments = viewComments(f.Comments)
		return body
	}
	body.ListID = f.List.ID
	body.Comments = viewCards(f.List.Cards)
	lines := make([]int, 0, len(f.Threads))
	for line := range f.Threads {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	for _, line := range lines {
		t := f.Threads[line]
		body.Threads = append(body.Threads, view.Thread{ID: t.ID, Line: int32(line), Comments: viewCards(t.Cards)})
	}
	return body
}

// viewComments is the comments a page lists with no way to change them.
func viewComments(comments []commentView) []view.Comment {
	var out []view.Comment
	for _, c := range comments {
		out = append(out, view.Comment{ID: c.ID, Where: c.Where, Quoted: c.Quoted, Body: c.Body, Late: c.Late})
	}
	return out
}

// viewCards is the comments of a round that can be commented on.
func viewCards(cards []cardView) []view.Comment {
	var out []view.Comment
	for _, c := range cards {
		out = append(out, view.Comment{
			ID:       c.ID,
			Href:     "/api" + c.Href,
			Where:    c.Where,
			Quoted:   c.Quoted,
			Body:     c.Body,
			Late:     c.Late,
			Editable: c.Editable,
		})
	}
	return out
}
