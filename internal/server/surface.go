package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

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
	// A write under /api is the app's, which reads JSON and shows the sentence
	// beside its control.
	if strings.HasPrefix(r.URL.Path, "/api/") {
		refuseJSON(w, http.StatusForbidden, noToken)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(noToken))
		return
	}
	s.fail(w, r, http.StatusForbidden, failNoToken)
}

// surfaceMux is the mux with the headers every response carries and a page in
// place of the mux's own 404 and 405.
func (s *Server) surfaceMux() http.Handler {
	mux := s.SurfaceHandler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		setSurfaceHeaders(h)
		if s.refuseOtherBuild(w, r) {
			return
		}
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		caught := &statusCatcher{header: h}
		mux.ServeHTTP(caught, r)
		s.errorPage(w, r, caught.status)
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
	// Eye is the waiting count the top bar's eye shows, on every page. Nil
	// draws no eye, which is what a page gets when the count cannot be read: a
	// shut eye would say nothing waits.
	Eye *eye
	// Body is the page template's data.
	Body any
	// VAPID is the public key a device subscribes to push with, and empty
	// where it could not be read.
	VAPID string
	// Build is the daemon's build, which the stream compares with the build
	// running when it connects, so a page served by an older daemon says so.
	Build string
	// Failure is the failure the shell carries to the app as JSON, and empty
	// for every shell that carries none. The template escapes it as an
	// attribute.
	Failure string
	// Token is the build's short hash, which the head writes into the address
	// of each script and stylesheet it links.
	Token string
}

// eye is the top bar's count of reviews waiting on the reviewer. Zero shuts it.
type eye struct {
	// Count is how many reviews are in the queue's Waiting section.
	Count int
}

// eyeTimeout bounds the count's query on a page that did not read it already.
// A failure page is often the store failing, and its answer should not wait
// on the store a second time.
const eyeTimeout = time.Second

// failurePage answers a surface request whose handler failed, as a page. The
// agent API's JSON refusal would reach a phone as text, and its fix speaks to
// an agent at a terminal.
func (s *Server) failurePage(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("surface request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	// A write under /api is answered by the app, which reads JSON and shows
	// the sentence beside its control. A page would reach it as text it cannot
	// parse, and the control would show nothing.
	if r.Method != http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/") {
		refuseJSON(w, http.StatusInternalServerError, failedWrite)
		return
	}
	s.fail(w, r, http.StatusInternalServerError, failFailed)
}

// errorPage answers a status the surface answers with on its own: a 405, and
// a 404 for anything else.
func (s *Server) errorPage(w http.ResponseWriter, r *http.Request, status int) {
	if status == http.StatusMethodNotAllowed {
		s.fail(w, r, status, failNotAllowed)
		return
	}
	s.fail(w, r, http.StatusNotFound, failNotFound)
}

// waitingCount is the number of reviews waiting, for the top bar's eye, and nil
// where it cannot be read: a zero would say nothing waits. what names the page
// or view in the log. The count is a read of the local store, so it gets a
// context of its own with a bound, since the caller's may be the request of a
// failure and its answer should not wait on the store a second time.
func (s *Server) waitingCount(what string) *int32 {
	ctx, cancel := context.WithTimeout(context.Background(), eyeTimeout)
	defer cancel()
	n, err := s.waiting(ctx)
	if err != nil {
		s.log.Error("count waiting", "page", what, "err", err)
		return nil
	}
	count := int32(n)
	return &count
}

// handleApp answers every address the React app draws with the shell: the head
// every page writes, the React bundle and the element it draws into. The app
// fetches the address's view from /api and opens its own stream, so the shell
// holds nothing that changes with the store. A stamp printed here would be a
// second read of the store beside the view's own, which is the gap the view's
// stamp closes. The shell cannot say whether an address exists, so a missing
// one is the app's 404 screen, from the view's 404.
func (s *Server) handleApp(w http.ResponseWriter, r *http.Request) error {
	title := "Eyeball"
	if r.URL.Path == "/" {
		title = "Queue"
	}
	return s.writeShell(w, http.StatusOK, page{Title: title})
}

// handleReact serves the React app's bundle, which loads beside the template
// pages' bundle until the last screen moves.
func (s *Server) handleReact(w http.ResponseWriter, _ *http.Request) error {
	app, err := eyeball.App()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = w.Write(app.Script)
	return nil
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

// handleIcon serves one of the installed page's icons.
func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) error {
	data, err := fs.ReadFile(eyeball.Icons, "web/static/icons/"+path.Base(r.PathValue("name")))
	if err != nil {
		s.errorPage(w, r, http.StatusNotFound)
		return nil
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(data)
	return nil
}

// fontTags is a strong ETag for each font file app.css can name, keyed by its
// path under docs/design/fonts, hashed once from the embedded bytes. Only a
// .woff2 has one, so a license or any other path is not served.
var fontTags = sync.OnceValue(func() map[string]string {
	tags := map[string]string{}
	_ = fs.WalkDir(eyeball.Fonts, fontRoot, func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(name) != ".woff2" {
			return err
		}
		data, err := fs.ReadFile(eyeball.Fonts, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		tags[strings.TrimPrefix(name, fontRoot+"/")] = `"` + hex.EncodeToString(sum[:16]) + `"`
		return nil
	})
	return tags
})

// fontRoot is where the fonts sit in the embedded files.
const fontRoot = "docs/design/fonts"

// handleFont serves a font app.css names.
//
// It is the one surface response a browser may keep. The fonts are 215 KB, and
// under the surface's no-store they would be sent with every page. no-cache
// keeps a copy and asks each time, which costs a request and a 304 rather than
// the bytes. A name that changes with the content would let the browser skip
// the request too, at the cost of changing how every static file is addressed.
func (s *Server) handleFont(w http.ResponseWriter, r *http.Request) error {
	name := r.PathValue("path")
	tag, ok := fontTags()[name]
	if !ok {
		s.errorPage(w, r, http.StatusNotFound)
		return nil
	}
	data, err := fs.ReadFile(eyeball.Fonts, fontRoot+"/"+name)
	if err != nil {
		return err
	}
	h := w.Header()
	h.Set("Content-Type", "font/woff2")
	h.Set("Cache-Control", "no-cache")
	h.Set("ETag", tag)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	return nil
}

// manifest is what makes the page installable: its name, where it opens, and
// its icons. Both colors are app.css's light paper, which the installed page
// shows before its stylesheet arrives.
const manifest = `{
  "name": "Eyeball",
  "short_name": "Eyeball",
  "start_url": "/",
  "scope": "/",
  "display": "standalone",
  "background_color": "#f3f4f1",
  "theme_color": "#f3f4f1",
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
