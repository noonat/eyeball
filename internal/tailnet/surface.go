package tailnet

import (
	"context"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
)

// CloseBudget bounds how long closing a node may take. tsnet's own close is
// unbounded past two of its goroutines, so the budget is the daemon's: a close
// that overruns it makes the daemon exit, which releases its lock and the
// node's sockets together.
const CloseBudget = 10 * time.Second

// ErrCloseOverrun reports a node that did not close within CloseBudget.
var ErrCloseOverrun = errors.New("the tailnet node did not close in time")

// Report is the surface's state on the tailnet, as eyeball status shows it.
type Report struct {
	// State is off, starting, serving or failed.
	State string
	// URL is the surface's address while it is serving.
	URL string
	// Reason says what failed, and names what the owner does about it. It
	// never names a command for an agent to run.
	Reason string
}

// Surface states a Report holds.
const (
	// ReportOff is a daemon with no enrolled node.
	ReportOff = "off"
	// ReportStarting is a node not yet serving.
	ReportStarting = "starting"
	// ReportServing is a node serving the surface.
	ReportServing = "serving"
	// ReportFailed is a node that cannot serve, with the reason.
	ReportFailed = "failed"
)

// The reasons a failed surface gives. Each says what the owner does, because
// an agent reads them, and an agent told to run a command runs it.
const (
	reasonLogin    = "the tailnet node needs a login: the owner runs eyeball tailnet login on this machine"
	reasonApproval = "the tailnet node is waiting for approval: the owner approves it in the tailnet's admin console"
	reasonMagicDNS = "MagicDNS is off on the tailnet, so the surface cannot have an HTTPS name: the owner turns it on in the admin console"
	reasonHTTPS    = "HTTPS certificates are off on the tailnet: the owner turns them on in the admin console"
	reasonStopped  = "the tailnet node stopped: the owner restarts the daemon"
)

// reasonFailed is the reason for a node that failed for a reason of its own,
// which is logged in full where the owner reads it: eyeball.log for a started
// daemon, the terminal for one a person runs with eyeball serve.
func reasonFailed(err error, logf Logf) string {
	logf("the tailnet node failed: %v", err)
	return "the tailnet node failed (" + err.Error() + "): the owner reads the daemon's log for the rest"
}

// Surface owns one node for the daemon's life: it starts it, watches it,
// serves the surface on it, and closes it at most once.
//
// One goroutine starts and watches the node, and closes a node that failed.
// Shutdown marks the node stopping so a start that has not begun never will,
// and cancels the watch so a wait for the running state ends at once. Stop
// then waits for tsnet's own Start to return if one is in progress, and closes
// the node itself, through the same sync.Once the failed path uses, so that it
// can give up on a close that overruns its budget. tsnet's Close must not run
// before or during Start. Stop never waits for the running state, which an
// offline node never reaches.
type Surface struct {
	report atomic.Pointer[Report]

	mu       sync.Mutex
	stopping bool
	starting bool
	started  chan struct{}
	node     Node
	server   *http.Server

	cancel    context.CancelFunc
	closeOnce sync.Once
	closeErr  error
	done      chan struct{}
}

// Off is the report of a daemon with no enrolled node.
func Off() Report {
	return Report{State: ReportOff}
}

// StartSurface starts a node with newNode and serves handler on it once the
// node is running with MagicDNS and certificate domains, in a goroutine. It
// returns at once.
func StartSurface(dir string, newNode Constructor, handler http.Handler, logf Logf) *Surface {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Surface{started: make(chan struct{}), cancel: cancel, done: make(chan struct{})}
	s.set(Report{State: ReportStarting})
	go s.run(ctx, dir, newNode, handler, logf)
	return s
}

// Report is the surface's current state.
func (s *Surface) Report() Report {
	return *s.report.Load()
}

// set records the surface's state.
func (s *Surface) set(r Report) {
	s.report.Store(&r)
}

// fail records a failed state and closes the node.
func (s *Surface) fail(reason string) {
	s.set(Report{State: ReportFailed, Reason: reason})
	_ = s.close()
}

// close closes the node at most once, on whichever path reaches it first.
func (s *Surface) close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		node, server := s.node, s.server
		s.mu.Unlock()
		if server != nil {
			_ = server.Close()
		}
		if node != nil {
			s.closeErr = node.Close()
		}
	})
	return s.closeErr
}

// run is the goroutine that owns the node.
func (s *Surface) run(ctx context.Context, dir string, newNode Constructor, handler http.Handler, logf Logf) {
	defer close(s.done)
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		close(s.started)
		return
	}
	s.starting = true
	s.mu.Unlock()

	node := newNode(dir, logf)
	err := node.Start()
	s.mu.Lock()
	s.node = node
	s.starting = false
	stopping := s.stopping
	s.mu.Unlock()
	close(s.started)
	if err != nil {
		s.fail(reasonFailed(err, logf))
		return
	}
	if stopping {
		return
	}

	watch, err := node.Watch(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.fail(reasonFailed(err, logf))
		}
		return
	}
	for {
		state, err := watch.Next()
		if err != nil {
			if ctx.Err() == nil {
				s.fail(reasonFailed(err, logf))
			}
			return
		}
		switch state {
		case StateNeedsLogin:
			s.fail(reasonLogin)
			return
		case StateNeedsApproval:
			s.fail(reasonApproval)
			return
		case StateStopped:
			s.fail(reasonStopped)
			return
		case StateRunning:
			if !s.serve(ctx, node, handler, logf) {
				return
			}
		}
	}
}

// serve checks a running node and serves the surface on it, once. It reports
// false when the surface failed and the node is closed, or shutdown began.
func (s *Surface) serve(ctx context.Context, node Node, handler http.Handler, logf Logf) bool {
	s.mu.Lock()
	serving := s.server != nil
	s.mu.Unlock()
	if serving {
		return true
	}
	status, err := node.Status(ctx)
	switch {
	case err != nil && ctx.Err() != nil:
		// Shutdown canceled the read. Stop closes the node.
		return false
	case err != nil:
		s.fail(reasonFailed(err, logf))
		return false
	case !status.MagicDNS:
		s.fail(reasonMagicDNS)
		return false
	case len(status.CertDomains) == 0:
		s.fail(reasonHTTPS)
		return false
	}
	ln, err := node.ListenTLS(":443")
	if err != nil {
		s.fail(reasonFailed(err, logf))
		return false
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	// Checked under the same lock Shutdown takes, so a server is either seen
	// by Shutdown and drained, or never serves, and the report cannot turn
	// back to serving once shutdown has begun.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping {
		_ = ln.Close()
		return false
	}
	s.server = server
	go func(ln net.Listener) { _ = server.Serve(ln) }(ln)
	s.set(Report{State: ReportServing, URL: "https://" + status.CertDomains[0]})
	return true
}

// Shutdown stops the surface's server within ctx, which carries the daemon's
// shutdown grace, so the tailnet's requests drain with the socket's and the
// loopback's rather than out of the close budget. It also marks the node
// stopping and ends the wait for the running state.
func (s *Surface) Shutdown(ctx context.Context) {
	s.mu.Lock()
	s.stopping = true
	server := s.server
	s.mu.Unlock()
	s.cancel()
	if server != nil {
		if err := server.Shutdown(ctx); err != nil {
			_ = server.Close()
		}
	}
}

// Stop closes the node within ctx, which carries the daemon's close budget,
// and returns ErrCloseOverrun when ctx ends first. A server Shutdown has not
// drained is closed outright.
func (s *Surface) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.stopping = true
	s.mu.Unlock()
	s.cancel()

	// A start in progress is waited for, because tsnet's Close must not run
	// during Start. The wait for the running state is not waited for: the
	// cancel above has already ended it.
	select {
	case <-s.started:
	case <-ctx.Done():
		return errors.WithStack(ErrCloseOverrun)
	}
	closed := make(chan error, 1)
	go func() { closed <- s.close() }()
	select {
	case err := <-closed:
		s.set(Report{State: ReportOff})
		return err
	case <-ctx.Done():
		return errors.WithStack(ErrCloseOverrun)
	}
}
