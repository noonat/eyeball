package daemon

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/blob"
	"github.com/noonat/eyeball/internal/server"
	"github.com/noonat/eyeball/internal/store"
	"github.com/noonat/eyeball/internal/tailnet"
)

// requireLoopback refuses any surface address but a literal loopback IP and a
// port. 0.0.0.0 or an empty host binds every interface, putting a surface with
// no authentication on every network the machine joins, and a name would need
// resolving.
func requireLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.Wrapf(err, "--surface-addr %q is not a host and a port", addr)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.Newf("--surface-addr %q is not a loopback address; use 127.0.0.1 or [::1] with a port", addr)
	}
	return nil
}

// ShutdownGrace bounds how long the daemon waits for requests to finish once
// it has begun to stop. Waits answer at once, so this covers a request in the
// middle of a capture. One longer than this has its connection cut, which the
// client reports as connection_lost. A command that finds a stopping daemon
// waits this long for its lock.
const ShutdownGrace = 5 * time.Second

// Config is what a daemon runs with.
type Config struct {
	// Paths are where the daemon keeps everything.
	Paths Paths
	// Build identifies the binary, as Build returns it, and is a field so a
	// test can set one.
	Build string
	// Home is the user's home directory, for agents' default names.
	Home string
	// Stderr receives the daemon's log, and the line saying another daemon
	// holds the lock.
	Stderr io.Writer
	// SurfaceAddr is a literal loopback address to serve the surface on, for a
	// person debugging, and empty for no loopback listener.
	SurfaceAddr string
	// NewNode builds the tailnet node for an enrolled state directory, and is
	// nil for the real one. Every test sets it, so no test can start a node
	// that contacts Tailscale.
	NewNode tailnet.Constructor
}

// StoppingGrace is how long a stopping daemon may hold its lock: the grace its
// requests get, then the budget for closing its tailnet node.
const StoppingGrace = ShutdownGrace + tailnet.CloseBudget

// keptUntilExit holds the lock of a daemon whose tailnet node did not close in
// time. Run returns without releasing it, so the process's exit releases it
// together with the node's sockets, and a successor cannot start a second node
// on the same directory. It is a package variable because an os.File the
// collector finds unreachable has its descriptor closed by a finalizer, which
// would release the lock early.
var keptUntilExit *lock

// Run is the daemon. It returns when the daemon has stopped: when ctx ends,
// when an agent asks it to stop, or at once and without error when another
// daemon already holds the lock, which the client that started this one
// connects to instead.
//
// Shutdown runs in one order and the lock is released last. A closing Go
// unix listener unlinks whatever is at its path, so a daemon that released the
// lock first and closed its listener second would delete the socket its
// successor had just bound, leaving a live daemon nobody could reach.
func Run(ctx context.Context, c Config) error {
	// SIGINT and SIGTERM take the same path as eyeball stop, so a daemon
	// killed politely still releases its lock last and leaves no socket.
	ctx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	if err := Prepare(c.Paths); err != nil {
		return err
	}
	started := time.Now().UTC()
	held, err := takeLock(c.Paths.Lock, os.Getpid(), started)
	if errors.Is(err, ErrRunning) {
		_, _ = fmt.Fprintf(c.Stderr, "eyeball: %v\n", err)
		return nil
	}
	if err != nil {
		return err
	}
	runErr := serve(ctx, c, started, held)
	if errors.Is(runErr, tailnet.ErrCloseOverrun) {
		keptUntilExit = held
		return runErr
	}
	if err := held.release(); err != nil && runErr == nil {
		runErr = err
	}
	return runErr
}

// serve runs everything that happens while the lock is held.
func serve(ctx context.Context, c Config, started time.Time, held *lock) error {
	st, err := store.Open(ctx, c.Paths.DB)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	blobs, err := blob.Open(c.Paths.Blobs)
	if err != nil {
		return err
	}
	if err := os.Remove(c.Paths.Socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Wrapf(err, "remove the stale socket %s", c.Paths.Socket)
	}
	listener, err := net.Listen("unix", c.Paths.Socket)
	if err != nil {
		return errors.Wrapf(err, "listen on %s", c.Paths.Socket)
	}

	// The loopback listener is bound now, synchronously, so a bad address or a
	// port in use fails startup where a person running serve reads it.
	var loopback net.Listener
	if c.SurfaceAddr != "" {
		if err := requireLoopback(c.SurfaceAddr); err != nil {
			_ = listener.Close()
			return err
		}
		if loopback, err = net.Listen("tcp", c.SurfaceAddr); err != nil {
			_ = listener.Close()
			return errors.Wrapf(err, "listen on %s", c.SurfaceAddr)
		}
	}

	stopped := make(chan struct{})
	var once sync.Once
	var surface atomic.Pointer[tailnet.Surface]
	log := slog.New(slog.NewTextHandler(c.Stderr, nil))
	srv := server.New(server.Config{
		Store: st,
		Blobs: blobs,
		Daemon: api.Daemon{
			PID:      os.Getpid(),
			Started:  started,
			Build:    c.Build,
			StateDir: c.Paths.Dir,
		},
		Home: c.Home,
		Log:  log,
		Stop: func() { once.Do(func() { close(stopped) }) },
		Surface: func() api.Surface {
			r := tailnet.Off()
			if s := surface.Load(); s != nil {
				r = s.Report()
			}
			return api.Surface{State: r.State, URL: r.URL, Reason: r.Reason}
		},
	})

	// No WriteTimeout: it would cut every long poll at that duration.
	// ReadHeaderTimeout only bounds a client that never finishes sending.
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	hs := &http.Server{
		Handler:           srv.AgentHandler(),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return base },
	}
	served := make(chan error, 1)
	go func() { served <- hs.Serve(listener) }()
	log.Info("listening", "socket", c.Paths.Socket, "pid", os.Getpid(), "build", c.Build)

	var local *http.Server
	if loopback != nil {
		local = &http.Server{Handler: srv.Surface(), ReadHeaderTimeout: 10 * time.Second}
		go func() { _ = local.Serve(loopback) }()
		log.Info("serving the surface on loopback", "addr", loopback.Addr().String())
	}
	// The tailnet starts only for a node the owner enrolled, and in a
	// goroutine, so a node that is offline or needs a login never delays the
	// socket.
	if tailnet.Enrolled(c.Paths.Dir) {
		newNode := c.NewNode
		if newNode == nil {
			newNode = tailnet.New
		}
		logf := func(format string, args ...any) { log.Info(fmt.Sprintf(format, args...)) }
		surface.Store(tailnet.StartSurface(tailnet.NodeDir(c.Paths.Dir), newNode, srv.Surface(), logf))
	}

	var serveErr error
	select {
	case <-ctx.Done():
	case <-stopped:
	case serveErr = <-served:
	}
	log.Info("stopping")

	// End the waits first. Shutdown waits for active requests, and a waiting
	// agent's request does not finish on its own. A capture does, so requests
	// doing work are left to finish within the grace.
	srv.BeginShutdown()
	if err := held.markStopping(); err != nil {
		log.Warn("mark stopping", "err", err)
	}
	shutdownCtx, done := context.WithTimeout(context.Background(), ShutdownGrace)
	defer done()
	var wg sync.WaitGroup
	if s := surface.Load(); s != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Shutdown(shutdownCtx)
		}()
	}
	servers := []*http.Server{hs, local}
	for _, s := range servers {
		if s == nil {
			continue
		}
		wg.Add(1)
		go func(s *http.Server) {
			defer wg.Done()
			if err := s.Shutdown(shutdownCtx); err != nil {
				_ = s.Close()
			}
		}(s)
	}
	wg.Wait()
	cancel()
	if s := surface.Load(); s != nil {
		closeCtx, closeDone := context.WithTimeout(context.Background(), tailnet.CloseBudget)
		defer closeDone()
		if err := s.Stop(closeCtx); err != nil {
			log.Error("close the tailnet node", "err", err)
			if errors.Is(err, tailnet.ErrCloseOverrun) {
				return err
			}
		}
	}
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Wrap(serveErr, "serve the socket")
}
