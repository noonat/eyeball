package server

import (
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"

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
}

// Server holds the handlers for both listeners.
type Server struct {
	db     *db
	blobs  *blob.Store
	hub    *hub
	daemon api.Daemon
	home   string
	log    *slog.Logger
	stop   func()

	// stopping closes when the daemon begins to shut down, which ends every
	// wait at once. Requests doing work are left to finish.
	stopping     chan struct{}
	stoppingOnce sync.Once

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
	return &Server{
		stopping: make(chan struct{}),
		db:       &db{s: c.Store, hub: h},
		blobs:    c.Blobs,
		hub:      h,
		daemon:   c.Daemon,
		home:     home,
		log:      log,
		stop:     stop,
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
		mux.HandleFunc(rt.pattern, func(w http.ResponseWriter, r *http.Request) {
			if err := handle(s, w, r); err != nil {
				s.answerError(w, r, err)
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
