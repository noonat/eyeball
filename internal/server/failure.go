package server

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball/internal/view"
)

// The failures the surface answers on its own. Each is the same words wherever
// it is shown, so a person's error screen and a script's plain text agree.
var (
	failNotFound   = view.Failure{Title: "Not found", Heading: "Nothing here", Text: "There is no page at this address."}
	failNotAllowed = view.Failure{Title: "Not allowed", Heading: "Not allowed", Text: "This address does not answer that kind of request."}
	failNoToken    = view.Failure{Title: "Not allowed", Heading: "Not allowed", Text: noToken}
	failTooLarge   = view.Failure{Title: "Too large", Heading: "Too large", Text: pastSendLimit}
	failFailed     = view.Failure{Title: "Something went wrong", Heading: "This page could not be read", Text: "The daemon's log says why."}
)

// fail answers a request the surface cannot serve. A browser says what it is
// loading in Sec-Fetch-Dest, and only "document" is a page a person reads, so
// only that gets the shell with the failure in its head. An image, a frame, a
// fetch and curl get the sentence as text, since a page is no use to them.
// Accept cannot decide this: a frame sends the same text/html a page does.
//
// A shell gets the surface's own Content-Security-Policy. A raw file's sandbox
// policy is set before the file is read, and under it the shell would have no
// origin to read its own storage from.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, status int, f view.Failure) {
	h := w.Header()
	if r.Header.Get("Sec-Fetch-Dest") != "document" {
		h.Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(f.Text + "\n"))
		return
	}
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	f.WaitingCount = s.waitingCount("failure")
	tag, err := json.Marshal(f)
	if err == nil {
		err = s.writeShell(w, status, page{Title: f.Title, Failure: string(tag)})
	}
	if err != nil {
		s.log.Error("render failure", "status", status, "err", err)
		http.Error(w, f.Text, status)
	}
}

// writeShell writes the shell with the status. The page's build fields are
// filled here.
func (s *Server) writeShell(w http.ResponseWriter, status int, data page) error {
	data.VAPID = s.vapidPublic()
	data.Build = s.daemon.Build
	data.Token = s.buildToken()
	// A clone, since executing a template escapes its tree in place and the
	// shared one is read by every request.
	t, err := s.pages.Clone()
	if err != nil {
		return errors.WithStack(err)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "shell", data); err != nil {
		return errors.WithStack(err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
	return nil
}
