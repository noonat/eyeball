package tailnet

import (
	"context"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
)

// The reasons a login's checks fail. Each is a setting of the tailnet's, which
// the owner changes in its admin console before logging in again.
var (
	// ErrNoMagicDNS is a tailnet with MagicDNS off, so the node has no name
	// for a certificate.
	ErrNoMagicDNS = errors.New("MagicDNS is off on the tailnet, so the surface cannot have an HTTPS name")
	// ErrNoHTTPS is a tailnet with HTTPS certificates off.
	ErrNoHTTPS = errors.New("HTTPS certificates are off on the tailnet")
)

// Enrollment is what a login did.
type Enrollment struct {
	// Dir is where the node's state lives.
	Dir string
	// URL is the surface's address once a daemon serves it.
	URL string
	// First says this login enrolled the node, rather than logging an
	// enrolled one in again.
	First bool
}

// NewNodeDir is where a first enrollment keeps its node until it has passed
// every check.
func NewNodeDir(stateDir string) string {
	return filepath.Join(stateDir, "node.new")
}

// Login logs a node in and checks that the tailnet can serve the surface. The
// caller holds the daemon's lock for the whole call.
//
// With state/node present it logs that node in again in place. A fresh
// directory would be a second machine with a new name, and so a new HTTPS
// origin for the installed page. Otherwise it enrolls into state/node.new, and
// renames that to state/node only once the node runs with MagicDNS and
// certificate domains, so an interrupted or failed first login never leaves a
// directory the daemon reads as enrolled. A node.new such a login left is used
// again, so logging in again after fixing the tailnet's settings does not make
// another machine.
//
// note receives what the person needs to read while the login waits, such as
// tsnet's login URL. Login returns when the checks pass or fail, or ctx ends.
func Login(ctx context.Context, stateDir string, newNode Constructor, note Logf) (Enrollment, error) {
	first := !Enrolled(stateDir)
	dir := NodeDir(stateDir)
	if first {
		dir = NewNodeDir(stateDir)
	}
	node := newNode(dir, note)
	// A failed Start is not followed by Close, which would dereference state
	// a Start that failed early never set. One that failed late can leave
	// parts of the node running, and the login exits after it.
	if err := node.Start(); err != nil {
		return Enrollment{}, err
	}
	url, err := awaitServable(ctx, node, note)
	closeErr := node.Close()
	if err != nil {
		return Enrollment{}, err
	}
	if closeErr != nil {
		return Enrollment{}, closeErr
	}
	if first {
		if err := os.Rename(dir, NodeDir(stateDir)); err != nil {
			return Enrollment{}, errors.Wrap(err, "keep the enrolled node")
		}
	}
	return Enrollment{Dir: NodeDir(stateDir), URL: url, First: first}, nil
}

// awaitServable waits for a node to run, then checks that it can serve HTTPS,
// and returns the surface's URL. A node needing a login waits for one: tsnet
// prints the URL to open through note.
func awaitServable(ctx context.Context, node Node, note Logf) (string, error) {
	watch, err := node.Watch(ctx)
	if err != nil {
		return "", err
	}
	approval := false
	for {
		state, err := watch.Next()
		if err != nil {
			if ctx.Err() != nil {
				return "", errors.WithStack(ctx.Err())
			}
			return "", err
		}
		switch state {
		case StateNeedsApproval:
			if !approval {
				note("the node is waiting for approval in the tailnet's admin console")
				approval = true
			}
		case StateRunning:
			status, err := node.Status(ctx)
			switch {
			case err != nil:
				return "", err
			case !status.MagicDNS:
				return "", errors.WithStack(ErrNoMagicDNS)
			case len(status.CertDomains) == 0:
				return "", errors.WithStack(ErrNoHTTPS)
			}
			return "https://" + status.CertDomains[0], nil
		}
	}
}
