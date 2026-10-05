package tailnet

import (
	"context"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"tailscale.com/client/local"
	"tailscale.com/ipn"
	"tailscale.com/logtail"
	"tailscale.com/tsnet"
)

// Hostname is the name a node asks the tailnet for. The tailnet may give a
// different one, so the surface's address is read from the node's certificate
// domains rather than built from this.
const Hostname = "eyeball"

// State is where a node is, in the terms the surface reports.
type State string

const (
	// StateStarting is a node that has not yet reached the tailnet.
	StateStarting State = "starting"
	// StateRunning is a node on the tailnet.
	StateRunning State = "running"
	// StateNeedsLogin is a node that needs the owner to log it in, which is
	// also what a node whose key expired looks like.
	StateNeedsLogin State = "needs_login"
	// StateNeedsApproval is a node waiting for the tailnet's admin to approve
	// it.
	StateNeedsApproval State = "needs_approval"
	// StateStopped is a node that has stopped.
	StateStopped State = "stopped"
)

// Status is what the surface needs to know about a running node.
type Status struct {
	// MagicDNS says the tailnet gives nodes names, which HTTPS needs.
	MagicDNS bool
	// CertDomains are the names a certificate can be issued for, empty when
	// the tailnet has HTTPS turned off.
	CertDomains []string
}

// Node is one tailnet node: everything the daemon and the login do with one.
//
// It is an interface so that every test runs against a fake that scripts a
// node's states. Nothing in the test suite starts a real node, because a real
// node contacts Tailscale.
type Node interface {
	// Start starts the node without waiting for it to reach the tailnet.
	Start() error
	// Watch reports the node's state as it changes, until ctx ends.
	Watch(ctx context.Context) (Watcher, error)
	// Status reads what the surface needs from a running node.
	Status(ctx context.Context) (Status, error)
	// ListenTLS listens on the tailnet with the node's certificate.
	ListenTLS(addr string) (net.Listener, error)
	// Close stops the node. It must not be called before Start returns, nor
	// after a Start that failed: tsnet's dereferences state an early failure
	// never set, and after a late one it would close again what Start's own
	// cleanup closed, which nothing here shows is safe. A Start that fails late
	// can leave parts of the node running until the process exits.
	Close() error
}

// Watcher yields a node's states.
type Watcher interface {
	// Next blocks for the next state, and returns an error once the watch's
	// context has ended.
	Next() (State, error)
}

// Constructor builds a node whose state lives in dir.
type Constructor func(dir string, logf Logf) Node

// Logf receives what a node has to say to a person, such as its login URL.
type Logf func(format string, args ...any)

// New builds a real node: tsnet, with its state in dir and its log upload off.
//
// tsnet uploads its own logs to Tailscale by default outside tests. Nothing
// asks for that, and it is a decision about sending a machine's logs to a third
// party, so it is turned off. logtail.Disable is process-wide and checked
// before any entry is sent.
func New(dir string, logf Logf) Node {
	logtail.Disable()
	return &tsNode{s: &tsnet.Server{
		Dir:      dir,
		Hostname: Hostname,
		UserLogf: func(format string, args ...any) { logf(format, args...) },
		Logf:     log.New(io.Discard, "", 0).Printf,
	}}
}

// tsNode is a Node backed by tsnet.
type tsNode struct {
	s *tsnet.Server
}

// Start starts tsnet.
func (n *tsNode) Start() error {
	return errors.Wrap(n.s.Start(), "start the tailnet node")
}

// Watch opens a watch on tsnet's state through its local client.
func (n *tsNode) Watch(ctx context.Context) (Watcher, error) {
	lc, err := n.s.LocalClient()
	if err != nil {
		return nil, errors.Wrap(err, "reach the tailnet node")
	}
	w, err := lc.WatchIPNBus(ctx, ipn.NotifyInitialState)
	if err != nil {
		return nil, errors.Wrap(err, "watch the tailnet node")
	}
	return &tsWatcher{w: w}, nil
}

// Status reads tsnet's status without its peers.
func (n *tsNode) Status(ctx context.Context) (Status, error) {
	lc, err := n.s.LocalClient()
	if err != nil {
		return Status{}, errors.Wrap(err, "reach the tailnet node")
	}
	st, err := lc.StatusWithoutPeers(ctx)
	if err != nil {
		return Status{}, errors.Wrap(err, "read the tailnet node's status")
	}
	out := Status{CertDomains: st.CertDomains}
	if st.CurrentTailnet != nil {
		out.MagicDNS = st.CurrentTailnet.MagicDNSEnabled
	}
	return out, nil
}

// ListenTLS listens with tsnet's certificate.
func (n *tsNode) ListenTLS(addr string) (net.Listener, error) {
	ln, err := n.s.ListenTLS("tcp", addr)
	return ln, errors.Wrapf(err, "listen on the tailnet at %s", addr)
}

// Close closes tsnet.
func (n *tsNode) Close() error {
	return errors.Wrap(n.s.Close(), "close the tailnet node")
}

// tsWatcher maps tsnet's notifications to states.
type tsWatcher struct {
	w *local.IPNBusWatcher
}

// Next skips notifications that carry no state.
func (t *tsWatcher) Next() (State, error) {
	for {
		n, err := t.w.Next()
		if err != nil {
			return "", errors.Wrap(err, "watch the tailnet node")
		}
		if n.State == nil {
			continue
		}
		switch *n.State {
		case ipn.Running:
			return StateRunning, nil
		case ipn.NeedsLogin:
			return StateNeedsLogin, nil
		case ipn.NeedsMachineAuth:
			return StateNeedsApproval, nil
		case ipn.Stopped:
			return StateStopped, nil
		default:
			return StateStarting, nil
		}
	}
}

// NodeDir is where an enrolled node's state lives in the state directory.
func NodeDir(stateDir string) string {
	return filepath.Join(stateDir, "node")
}

// Enrolled reports whether stateDir holds a node eyeball tailnet login
// enrolled. A first login enrolls into node.new and renames it only once the
// node is running with HTTPS available, so a node directory existing means
// the enrollment finished, and a leftover node.new means nothing to a daemon.
func Enrolled(stateDir string) bool {
	info, err := os.Stat(NodeDir(stateDir))
	return err == nil && info.IsDir()
}
