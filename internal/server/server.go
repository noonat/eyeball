package server

import (
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/noonat/eyeball"
	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/blob"
	"github.com/noonat/eyeball/internal/store"
)

// Config is what a server is built from.
type Config struct {
	// Store is the database.
	Store *store.Store
	// Blobs holds captured file content.
	Blobs *blob.Store
	// Daemon describes the process the server runs in.
	Daemon api.Daemon
	// Home is the user's home directory, which an agent's default name is
	// written relative to.
	Home string
	// Log receives what the daemon has to say, including every failure an
	// agent was answered with a generic message for.
	Log *slog.Logger
	// Stop asks the daemon to exit. It must return at once: the stop handler
	// calls it after answering and before the shutdown it starts has begun.
	Stop func()
	// Surface reports the surface's state on the tailnet, and is nil for a
	// daemon with no tailnet node.
	Surface func() api.Surface
	// Now is the clock the surface measures ages against. Nil is the system
	// clock.
	Now func() time.Time
}

// Server holds the handlers for both listeners.
type Server struct {
	db      *db
	surface func() api.Surface
	pages   *template.Template
	blobs   *blob.Store
	hub     *hub
	daemon  api.Daemon
	home    string
	log     *slog.Logger
	stop    func()
	now     func() time.Time

	// stopping closes when the daemon begins to shut down, which ends every
	// wait at once. Requests doing work are left to finish.
	stopping     chan struct{}
	stoppingOnce sync.Once

	// pushes counts the notifications being sent, which shutdown waits for
	// within its grace.
	pushes sync.WaitGroup

	// vapidMu guards vapidKey, the public key read from the store on first
	// use and kept, since it never changes once made.
	vapidMu  sync.Mutex
	vapidKey string

	// afterRead runs in a wait between reading the review and blocking on
	// the hub. Tests set it to land a verdict in exactly the gap a waiter
	// that subscribed late would miss. It is atomic because a test sets it
	// while a wait from an earlier step may still be running.
	afterRead atomic.Pointer[func()]
}

// New builds a server.
func New(c Config) *Server {
	h := newHub()
	home := c.Home
	if resolved, err := filepath.EvalSymlinks(home); err == nil && home != "" {
		home = resolved
	}
	log := c.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	stop := c.Stop
	if stop == nil {
		stop = func() {}
	}
	surface := c.Surface
	if surface == nil {
		surface = func() api.Surface { return api.Surface{State: "off"} }
	}
	now := c.Now
	if now == nil {
		now = time.Now
	}
	return &Server{
		stopping: make(chan struct{}),
		surface:  surface,
		pages:    template.Must(template.ParseFS(eyeball.Templates, "web/templates/*.html")),
		db:       &db{s: c.Store, hub: h},
		blobs:    c.Blobs,
		hub:      h,
		daemon:   c.Daemon,
		home:     home,
		log:      log,
		stop:     stop,
		now:      now,
	}
}

// BeginShutdown ends every wait with daemon_stopping and leaves other requests
// running. The daemon calls it before http.Server.Shutdown, which waits for
// active requests, and a wait would otherwise hold Shutdown until its grace
// ran out. Canceling every request instead aborted a capture in progress.
func (s *Server) BeginShutdown() {
	s.stoppingOnce.Do(func() { close(s.stopping) })
}

// side is which listener a route belongs to.
type side int

const (
	// sideAgent is the unix socket every agent's CLI talks to.
	sideAgent side = iota
	// sideSurface is the reviewer's listener on the tailnet.
	sideSurface
)

// route is one pattern, the listener it belongs to, and its handler.
type route struct {
	side    side
	pattern string
	handle  func(s *Server, w http.ResponseWriter, r *http.Request) error
}

// routes is every route the daemon serves, declared once.
//
// Each listener's mux is built from this table and from nothing else, and a
// test walks it to prove each route matches on its own listener only. That is
// what keeps a verdict off the socket, where an agent could approve its own
// review, and the agent API off the tailnet. A route added anywhere else is a
// route that test cannot see.
var routes = []route{
	{
		side:    sideAgent,
		pattern: "GET /agent/daemon",
		handle:  (*Server).handleDaemon,
	},
	{
		side:    sideAgent,
		pattern: "POST /agent/stop",
		handle:  (*Server).handleStop,
	},
	{
		side:    sideAgent,
		pattern: "POST /agent/requests",
		handle:  (*Server).handleRequest,
	},
	{
		side:    sideAgent,
		pattern: "GET /agent/reviews/{review}",
		handle:  (*Server).handleShow,
	},
	{
		side:    sideAgent,
		pattern: "GET /agent/reviews/{review}/wait",
		handle:  (*Server).handleWait,
	},
	{
		side:    sideAgent,
		pattern: "POST /agent/reviews/{review}/receipt",
		handle:  (*Server).handleReceipt,
	},
	{
		side:    sideAgent,
		pattern: "POST /agent/reviews/{review}/abandon",
		handle:  (*Server).handleAbandon,
	},
	{
		side:    sideAgent,
		pattern: "POST /agent/comments/{comment}/reply",
		handle:  (*Server).handleReply,
	},
	{
		side:    sideAgent,
		pattern: "GET /agent/status",
		handle:  (*Server).handleStatus,
	},
	{
		side:    sideSurface,
		pattern: "GET /{$}",
		handle:  (*Server).handleQueue,
	},
	{
		side:    sideSurface,
		pattern: "GET /reviews/{review}",
		handle:  (*Server).handleReviewPage,
	},
	{
		side:    sideSurface,
		pattern: "GET /reviews/{review}/rounds/{round}",
		handle:  (*Server).handleRoundPage,
	},
	{
		side:    sideSurface,
		pattern: "GET /reviews/{review}/files/{path...}",
		handle:  (*Server).handleFilePage,
	},
	{
		side:    sideSurface,
		pattern: "GET /reviews/{review}/rounds/{round}/files/{path...}",
		handle:  (*Server).handleRoundFilePage,
	},
	{
		side:    sideSurface,
		pattern: "GET /reviews/{review}/raw/{round}/{path...}",
		handle:  (*Server).handleRaw,
	},
	{
		side:    sideSurface,
		pattern: "GET /projects/{project}",
		handle:  (*Server).handleProjectPage,
	},
	{
		side:    sideSurface,
		pattern: "GET /projects/{project}/files/{path...}",
		handle:  (*Server).handleProjectFilePage,
	},
	{
		side:    sideSurface,
		pattern: "GET /healthz",
		handle:  (*Server).handleHealthz,
	},
	{
		side:    sideSurface,
		pattern: "GET /static/app.css",
		handle:  (*Server).handleStylesheet,
	},
	{
		side:    sideSurface,
		pattern: "GET /static/shell.css",
		handle:  (*Server).handleShell,
	},
	{
		side:    sideSurface,
		pattern: "POST /reviews/{review}/comments",
		handle:  (*Server).handleAddComment,
	},
	{
		side:    sideSurface,
		pattern: "PUT /reviews/{review}/comments/{comment}",
		handle:  (*Server).handleEditComment,
	},
	{
		side:    sideSurface,
		pattern: "DELETE /reviews/{review}/comments/{comment}",
		handle:  (*Server).handleDeleteComment,
	},
	{
		side:    sideSurface,
		pattern: "GET /reviews/{review}/comments/{comment}",
		handle:  (*Server).handleComment,
	},
	{
		side:    sideSurface,
		pattern: "POST /reviews/{review}/decision",
		handle:  (*Server).handleDecide,
	},
	{
		side:    sideSurface,
		pattern: "GET /reviews/{review}/composer",
		handle:  (*Server).handleComposer,
	},
	{
		side:    sideSurface,
		pattern: "GET /reviews/{review}/rounds/{round}/cards/{path...}",
		handle:  (*Server).handleCardBody,
	},
	{
		side:    sideSurface,
		pattern: "GET /events",
		handle:  (*Server).handleEvents,
	},
	{
		side:    sideSurface,
		pattern: "POST /subscriptions",
		handle:  (*Server).handleSubscribe,
	},
	{
		side:    sideSurface,
		pattern: "GET /manifest.webmanifest",
		handle:  (*Server).handleManifest,
	},
	{
		side:    sideSurface,
		pattern: "GET /sw.js",
		handle:  (*Server).handleWorker,
	},
	{
		side:    sideSurface,
		pattern: "GET /static/icons/{name}",
		handle:  (*Server).handleIcon,
	},
	{
		side:    sideSurface,
		pattern: "GET /static/htmx.js",
		handle:  (*Server).handleHtmx,
	},
	{
		side:    sideSurface,
		pattern: "GET /static/app.js",
		handle:  (*Server).handleScript,
	},
}

// AgentHandler is the handler for the unix socket.
func (s *Server) AgentHandler() *http.ServeMux {
	return s.mux(sideAgent)
}

// SurfaceHandler is the handler for the reviewer's listener.
func (s *Server) SurfaceHandler() *http.ServeMux {
	return s.mux(sideSurface)
}

// mux builds the handler for one listener from the route table.
func (s *Server) mux(which side) *http.ServeMux {
	mux := http.NewServeMux()
	for _, rt := range routes {
		if rt.side != which {
			continue
		}
		handle := rt.handle
		answer := s.answerError
		if which == sideSurface {
			answer = s.failurePage
		}
		mux.HandleFunc(rt.pattern, func(w http.ResponseWriter, r *http.Request) {
			if err := handle(s, w, r); err != nil {
				answer(w, r, err)
			}
		})
	}
	return mux
}

// answerError writes a handler's error as a refusal, logging anything that was
// a failure rather than a refusal.
func (s *Server) answerError(w http.ResponseWriter, r *http.Request, err error) {
	status, body, failed := toRefusal(err)
	if failed {
		s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	}
	writeJSON(w, status, api.ErrorBody{Error: body})
}
