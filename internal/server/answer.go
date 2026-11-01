package server

import (
	"net/http"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball/internal/view"
)

// pageAnswer is what a page address answers, before it is either drawn by a
// template or sent as JSON. A page route and its view under /api share one
// function that builds it, so the two cannot answer the same address
// differently.
type pageAnswer struct {
	// missing says the address has nothing at it, which both forms answer with
	// 404.
	missing bool
	// notice says why a live read was refused, on a missing answer, and is nil
	// where the path is simply not there.
	notice *notice
	// page is the top bar and the body the page template reads.
	page page
}

// notFound is the answer for an address with nothing at it. A notice says why,
// where a refused live read is the reason.
func notFound(n *notice) pageAnswer {
	return pageAnswer{missing: true, notice: n}
}

// answerer builds the answer for a request.
type answerer func(r *http.Request) (pageAnswer, error)

// serveView answers a page route's view under /api with JSON.
//
// The waiting count is read here, once, as renderPage reads it for the eye. A
// count that cannot be read is sent as null, as the page draws no eye.
func (s *Server) serveView(w http.ResponseWriter, r *http.Request, answer answerer) error {
	ans, err := answer(r)
	if err != nil {
		return err
	}
	count := s.waitingCount(r.URL.Path)
	w.Header().Set("Cache-Control", "no-store")
	if ans.missing {
		writeJSON(w, http.StatusNotFound, view.NotFound{WaitingCount: count, Notice: viewNoticePtr(ans.notice)})
		return nil
	}
	switch body := ans.page.Body.(type) {
	case projectPage:
		writeJSON(w, http.StatusOK, projectView(ans.page, body, count))
	case treePage:
		writeJSON(w, http.StatusOK, treeFilesView(ans.page, body, count))
	case filePage:
		writeJSON(w, http.StatusOK, fileFilesView(ans.page, body, count))
	default:
		return errors.Newf("no view for page body %T", ans.page.Body)
	}
	return nil
}

// handleFileView answers GET /api/reviews/{review}/files/{path...}.
func (s *Server) handleFileView(w http.ResponseWriter, r *http.Request) error {
	return s.serveView(w, r, s.fileAnswer)
}

// handleRoundFileView answers GET /api/reviews/{review}/rounds/{round}/files/
// {path...}.
func (s *Server) handleRoundFileView(w http.ResponseWriter, r *http.Request) error {
	return s.serveView(w, r, s.fileAnswer)
}

// handleProjectView answers GET /api/projects/{project}.
func (s *Server) handleProjectView(w http.ResponseWriter, r *http.Request) error {
	return s.serveView(w, r, s.projectAnswer)
}

// handleProjectFileView answers GET /api/projects/{project}/files/{path...}.
func (s *Server) handleProjectFileView(w http.ResponseWriter, r *http.Request) error {
	return s.serveView(w, r, s.projectFileAnswer)
}
