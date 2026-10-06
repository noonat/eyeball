package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/kind"
	"github.com/noonat/eyeball/internal/store"
)

// sandboxPolicy is the policy on every file's bytes. A page opened directly
// runs as it does in the frame, with an opaque origin, so it reaches no
// storage and its writes carry Origin: null, which the cross-origin check
// refuses. There is no default-src, so a page may load what it likes.
const sandboxPolicy = "sandbox allow-scripts; frame-ancestors 'self'"

// tooLargeToSend is what the surface says of a file past kind.MaxSend.
const tooLargeToSend = "Too large to send over a phone connection."

// handleRaw answers GET /reviews/{review}/raw/{round}/{path...}: one file's
// bytes as of a round, where round 0 is the base commit.
//
// A round resolves a path from its capture and then the base, never from the
// working copy. The working copy holds only what the capture skipped, and a
// preview that mixed live bytes into a frozen round would carry no banner to
// say so.
//
// A PDF is served without the sandbox, because Chrome refuses to run its PDF
// viewer under it. That rests on a browser's PDF viewer keeping a document's
// script away from the page around it, which PDFium, PDF.js and PDFKit all
// document.
func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Security-Policy", sandboxPolicy)
	number, err := strconv.Atoi(r.PathValue("round"))
	if err != nil || number < 0 {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	review, ok, err := s.reviewFromPath(r)
	if err != nil || !ok {
		return s.missing(w, err)
	}
	reader, err := s.rawReader(r.Context(), review, number)
	if errors.Is(err, store.ErrNotFound) {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	if err != nil {
		return err
	}

	p := r.PathValue("path")
	size, err := reader.Size(r.Context(), p)
	if errors.Is(err, capture.ErrNotFound) {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	if err != nil {
		return err
	}
	if size > kind.MaxSend {
		s.renderPage(w, http.StatusRequestEntityTooLarge, "error", page{Title: "Too large", Back: "/", BackLabel: "Queue", Body: message{Heading: "Too large", Text: tooLargeToSend}})
		return nil
	}
	rc, _, err := reader.Frozen(r.Context(), p)
	if errors.Is(err, capture.ErrNotFound) {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	if err != nil {
		return err
	}
	// Read whole, since http.ServeContent answers the Range requests iOS
	// Safari needs before it plays a video, and a base-side file comes out of
	// git cat-file as a stream that cannot seek. One code path for both costs
	// up to kind.MaxSend held per request in flight.
	content, err := readAll(rc)
	if err != nil {
		return err
	}
	f := kind.Of(p, content)
	if f.Kind == kind.Other {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	if f.Kind == kind.PDF {
		w.Header().Del("Content-Security-Policy")
	}
	w.Header().Set("Content-Type", f.Type)
	http.ServeContent(w, r, path.Base(p), time.Time{}, bytes.NewReader(content))
	return nil
}

// rawReader reads a review's files as of a round, and round 0 as the base
// commit alone. Every round of a review is measured from the same base, which
// the first round fixed.
func (s *Server) rawReader(ctx context.Context, review store.Review, number int) (*capture.Reader, error) {
	first, err := s.db.s.Round(ctx, review.ID, 1)
	if err != nil {
		return nil, err
	}
	project, err := s.db.s.Project(ctx, review.ProjectID)
	if err != nil {
		return nil, err
	}
	st, err := capture.Inspect(ctx, project.Root, first.BaseCommit, project.InsideCheckout)
	if err != nil {
		return nil, err
	}
	if number == 0 {
		return st.Reader(s.blobs, nil), nil
	}
	round, err := s.db.s.Round(ctx, review.ID, number)
	if err != nil {
		return nil, err
	}
	files, err := s.roundFiles(ctx, round.ID)
	if err != nil {
		return nil, err
	}
	return st.Reader(s.blobs, files), nil
}

// readAll reads rc to its end and closes it.
func readAll(rc io.ReadCloser) ([]byte, error) {
	defer func() { _ = rc.Close() }()
	content, err := io.ReadAll(io.LimitReader(rc, kind.MaxSend+1))
	if err != nil {
		return nil, errors.Wrap(err, "read a file to send")
	}
	return content, nil
}
