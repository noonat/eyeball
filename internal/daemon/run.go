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
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/blob"
	"github.com/noonat/eyeball/internal/server"
	"github.com/noonat/eyeball/internal/store"
)

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
}

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

	stopped := make(chan struct{})
	var once sync.Once
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
	if err := hs.Shutdown(shutdownCtx); err != nil {
		_ = hs.Close()
	}
	cancel()
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Wrap(serveErr, "serve the socket")
}
