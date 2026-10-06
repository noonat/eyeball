package server

import (
	"bytes"
	"crypto/subtle"
	"io/fs"
	"net/http"
	"path"

	"github.com/noonat/eyeball"
)

// contentSecurityPolicy is sent with every surface response.
//
// img-src allows data: because app.css draws every icon and the disclosure
// caret as a data: mask, and a policy without it hides all of them, which a
// review pass confirmed in Chromium. No inline script exists to allow, and a
// review page framed somewhere else is a page somebody can click through.
const contentSecurityPolicy = "default-src 'self'; img-src 'self' data:; frame-ancestors 'none'"

// Surface is the handler for the reviewer's listeners: the surface's mux, with
// the headers every response carries and a page in place of the mux's own
// plain-text 404 and 405.
//
// An empty pattern from the mux means its fallback will answer, and only then
// is the request served through a writer that catches the fallback's status.
// The pattern alone cannot tell a 404 from a 405, and the writer alone would
// replace a handler's own 404 page.
//
// Every request passes Go's cross-origin check first. The tailnet listener has
// no token, so without it any page the phone opened could post a verdict as a
// plain form.
func (s *Server) Surface() http.Handler {
	return http.NewCrossOriginProtection().Handler(s.surfaceMux())
}

// LoopbackSurface is Surface for the loopback listener: a request that writes
// must also carry the run's token in tokenHeader. Reads need none.
//
// The token blocks another account on this machine, and a casual curl, from
// deciding. It does not stop a process running as the owner, which can write
// eyeball.db directly.
func (s *Server) LoopbackSurface(token string) http.Handler {
	inner := s.surfaceMux()
	want := []byte(token)
	return http.NewCrossOriginProtection().Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead &&
			subtle.ConstantTimeCompare([]byte(r.Header.Get(tokenHeader)), want) != 1 {
			s.refuseWithoutToken(w, r)
			return
		}
		inner.ServeHTTP(w, r)
	}))
}

// tokenHeader carries the loopback listener's token. web/ts names it too.
const tokenHeader = "X-Eyeball-Token"

// noToken is what a write on the loopback listener without the run's token is
// told.
const noToken = "This page cannot write to this eyeball. Open the address eyeball serve printed."

// refuseWithoutToken answers a write that lacks the token: plain text to htmx,
// which shows it beside the control, and a page to anything else.
func (s *Server) refuseWithoutToken(w http.ResponseWriter, r *http.Request) {
	setSurfaceHeaders(w.Header())
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(noToken))
		return
	}
	p := page{Title: "Not allowed", Back: "/", BackLabel: "Queue", Body: message{Heading: "Not allowed", Text: noToken}}
	s.renderPage(w, http.StatusForbidden, "error", p)
}

// surfaceMux is the mux with the headers every response carries and a page in
// place of the mux's own 404 and 405.
func (s *Server) surfaceMux() http.Handler {
	mux := s.SurfaceHandler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		setSurfaceHeaders(h)
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		caught := &statusCatcher{header: h}
		mux.ServeHTTP(caught, r)
		s.errorPage(w, caught.status)
	})
}

// statusCatcher records the status the mux's fallback writes and drops its
// plain-text body. It shares the real writer's header, so an Allow header the
// fallback sets on a 405 survives.
type statusCatcher struct {
	header http.Header
	status int
}

// Header is the real writer's header.
func (c *statusCatcher) Header() http.Header {
	return c.header
}

// WriteHeader records the status.
func (c *statusCatcher) WriteHeader(status int) {
	c.status = status
}

// Write drops the body.
func (c *statusCatcher) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return len(p), nil
}

// page is what the layout prints around one page template: the document's
// title, the topbar, and the data the page template reads as its dot.
type page struct {
	// Title is the document's title and the topbar's heading.
	Title string
	// Back is where the topbar's back link goes. It is empty on the queue,
	// whose topbar carries the wordmark instead.
	Back string
	// BackLabel is the back link's text.
	BackLabel string
	// Pill is the waiting count beside the wordmark, and empty for none.
	Pill string
	// Body is the page template's data.
	Body any
	// VAPID is the public key a device subscribes to push with, and empty
	// where it could not be read.
	VAPID string
}

// message is a page that says one thing.
type message struct {
	// Heading is what happened, and Text says more.
	Heading, Text string
	// Notice is a banner above it saying why, and nil for none.
	Notice *notice
}

// failurePage answers a surface request whose handler failed, as a page. The
// agent API's JSON refusal would reach a phone as text, and its fix speaks to
// an agent at a terminal.
func (s *Server) failurePage(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("surface request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	// htmx swaps no whole page and shows a plain answer beside the control
	// that failed, so a request it sent gets the sentence alone.
	if r.Header.Get("HX-Request") == "true" {
		plain(w, http.StatusInternalServerError, failedWrite)
		return
	}
	p := page{Title: "Something went wrong", Back: "/", BackLabel: "Queue", Body: message{Heading: "This page could not be read", Text: "The daemon's log says why."}}
	s.renderPage(w, http.StatusInternalServerError, "error", p)
}

// errorPage writes a page for a status the surface answers with on its own.
func (s *Server) errorPage(w http.ResponseWriter, status int) {
	p := page{Title: "Not found", Back: "/", BackLabel: "Queue", Body: message{Heading: "Nothing here", Text: "There is no page at this address."}}
	if status == http.StatusMethodNotAllowed {
		p.Title = "Not allowed"
		p.Body = message{Heading: "Not allowed", Text: "This address does not answer that kind of request."}
	} else {
		status = http.StatusNotFound
	}
	s.renderPage(w, status, "error", p)
}

// renderPage executes a page template inside the layout and writes it.
func (s *Server) renderPage(w http.ResponseWriter, status int, name string, data page) {
	data.VAPID = s.vapidPublic()
	// The clone's own copy of the page's tree, never s.pages's: executing a
	// template escapes its tree in place, so adding the shared tree let every
	// request write to it while others cloned it, which the race detector
	// reported for two pages rendered at once.
	t, err := s.pages.Clone()
	if err == nil {
		_, err = t.AddParseTree("page", t.Lookup(name).Tree)
	}
	var buf bytes.Buffer
	if err == nil {
		err = t.ExecuteTemplate(&buf, "layout", data)
	}
	if err != nil {
		s.log.Error("render page", "page", name, "err", err)
		http.Error(w, "the page could not be rendered", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// handleHealthz answers once the store does.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) error {
	if err := s.db.s.Ping(r.Context()); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
	return nil
}

// handleStylesheet serves app.css from the binary.
func (s *Server) handleStylesheet(w http.ResponseWriter, _ *http.Request) error {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write(eyeball.Stylesheet)
	return nil
}

// handleShell serves shell.css from the binary.
func (s *Server) handleShell(w http.ResponseWriter, _ *http.Request) error {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write(eyeball.Shell)
	return nil
}

// handleHtmx serves the vendored htmx from the binary.
func (s *Server) handleHtmx(w http.ResponseWriter, _ *http.Request) error {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = w.Write(eyeball.Htmx)
	return nil
}

// handleScript serves the client's bundle. A bundle that failed to build is a
// 500 and a log line, so a page still renders without its script.
func (s *Server) handleScript(w http.ResponseWriter, _ *http.Request) error {
	js, err := eyeball.Bundle()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = w.Write(js)
	return nil
}

// setSurfaceHeaders sets the headers every surface response carries.
func setSurfaceHeaders(h http.Header) {
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	// A document can link anywhere, and a browser following a link sends the
	// page's origin, the tailnet's machine name, under its default policy. One
	// header covers every link on every page.
	h.Set("Referrer-Policy", "no-referrer")
}

// handleWorker serves the service worker at the root, so its scope is the
// whole surface.
func (s *Server) handleWorker(w http.ResponseWriter, _ *http.Request) error {
	js, err := eyeball.Worker()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = w.Write(js)
	return nil
}

// handleIcon serves one of the installed page's icons.
func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) error {
	data, err := fs.ReadFile(eyeball.Icons, "web/static/icons/"+path.Base(r.PathValue("name")))
	if err != nil {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(data)
	return nil
}

// manifest is what makes the page installable: its name, where it opens, and
// its icons. The colors are app.css's light background and accent.
const manifest = `{
  "name": "eyeball",
  "short_name": "eyeball",
  "start_url": "/",
  "scope": "/",
  "display": "standalone",
  "background_color": "#fbfaf8",
  "theme_color": "#fbfaf8",
  "icons": [
    {"src": "/static/icons/icon-192.png", "sizes": "192x192", "type": "image/png"},
    {"src": "/static/icons/icon-512.png", "sizes": "512x512", "type": "image/png"}
  ]
}
`

// handleManifest serves the web app manifest.
func (s *Server) handleManifest(w http.ResponseWriter, _ *http.Request) error {
	w.Header().Set("Content-Type", "application/manifest+json")
	_, _ = w.Write([]byte(manifest))
	return nil
}
