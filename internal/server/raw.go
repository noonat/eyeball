package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
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

// pastSendLimit is what the surface says of a file past kind.MaxSend. It
// names the limit rather than a connection, since the surface cannot tell
// wifi from cellular.
const pastSendLimit = "Past the 20 MB the surface sends on its own."

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
	if size > kind.MaxSend && r.URL.Query().Get("load") != "1" {
		s.renderPage(w, http.StatusRequestEntityTooLarge, "error", page{Title: "Too large", Back: "/", BackLabel: "Queue", Body: message{Heading: "Too large", Text: pastSendLimit}})
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
	defer func() { _ = rc.Close() }()
	content, cleanup, err := seekable(rc, size)
	if err != nil {
		return err
	}
	defer cleanup()
	head := make([]byte, 8000)
	n, err := io.ReadFull(content, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return errors.Wrapf(err, "read %s", p)
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return errors.Wrapf(err, "rewind %s", p)
	}
	f := kind.Of(p, head[:n])
	if f.Kind == kind.Other {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	if f.Kind == kind.PDF {
		w.Header().Del("Content-Security-Policy")
	}
	w.Header().Set("Content-Type", f.Type)
	http.ServeContent(w, r, path.Base(p), time.Time{}, content)
	return nil
}

// seekable makes a file's bytes seekable, since http.ServeContent answers the
// Range requests iOS Safari needs before it plays a video. A blob is a file on
// disk and seeks already. A base-side file comes out of git cat-file as a
// stream: within the send limit it is read into memory, which holds up to
// kind.MaxSend per request in flight, and past it, where only a reviewer's
// Load anyway reaches, it is copied to a temporary file first.
func seekable(rc io.ReadCloser, size int64) (io.ReadSeeker, func(), error) {
	if rs, ok := rc.(io.ReadSeeker); ok {
		return rs, func() {}, nil
	}
	if size <= kind.MaxSend {
		content, err := io.ReadAll(io.LimitReader(rc, kind.MaxSend+1))
		if err != nil {
			return nil, nil, errors.Wrap(err, "read a file to send")
		}
		return bytes.NewReader(content), func() {}, nil
	}
	tmp, err := os.CreateTemp("", "eyeball-raw-*")
	if err != nil {
		return nil, nil, errors.Wrap(err, "make a file to send from")
	}
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}
	if _, err := io.Copy(tmp, rc); err != nil {
		cleanup()
		return nil, nil, errors.Wrap(err, "copy a file to send")
	}
	return tmp, cleanup, nil
}

// rawReader reads a review's files as of a round, and round 0 as the base
// commit alone. Every round of a review is measured from the same base, which
// the first round fixed, and is read in the working copy its round came from.
func (s *Server) rawReader(ctx context.Context, review store.Review, number int) (*capture.Reader, error) {
	round, err := s.db.s.Round(ctx, review.ID, max(number, 1))
	if err != nil {
		return nil, err
	}
	project, err := s.db.s.Project(ctx, review.ProjectID)
	if err != nil {
		return nil, err
	}
	root, err := s.copyRoot(ctx, round)
	if err != nil {
		return nil, err
	}
	// Every round carries the base the first one fixed.
	st, err := capture.Inspect(ctx, root, round.BaseCommit, project.InsideCheckout)
	if err != nil {
		return nil, err
	}
	if number == 0 {
		return st.Reader(s.blobs, nil), nil
	}
	files, err := s.roundFiles(ctx, round.ID)
	if err != nil {
		return nil, err
	}
	return st.Reader(s.blobs, files), nil
}
