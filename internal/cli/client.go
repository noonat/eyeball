package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/daemon"
)

// startTimeout is how long a command waits for a daemon it started to answer.
// A guess, measured against nothing yet: opening the database and applying
// migrations is the whole of startup today.
const startTimeout = 5 * time.Second

// errNoDaemon reports that no daemon answered and none was started.
var errNoDaemon = errors.New("no daemon is running")

// errDropped reports a connection the daemon closed before answering.
var errDropped = errors.New("the connection to the daemon was lost")

// errOldDaemon reports a route the running daemon does not have, which means
// it is an older build than this command.
var errOldDaemon = errors.New("the running daemon does not know this request")

// maxStarts bounds how many daemons one command starts. A daemon that cannot
// start exits at once, and starting another on every exit spawned about 1,800
// processes in five seconds before this bound existed.
const maxStarts = 3

// stuckGrace is how long a lock held by a daemon that does not answer, and has
// not marked itself stopping, is given before the daemon is called stuck. A
// daemon that has marked itself stopping is given its whole shutdown grace,
// because it removes its socket first and may then finish a capture.
const stuckGrace = time.Second

// stuckError reports a daemon that holds the lock and does not answer on the
// socket: alive, but unreachable, typically because its socket file was
// removed. Starting another cannot help, since the new one loses the lock.
type stuckError struct {
	holder daemon.Holder
}

// Error names the daemon.
func (e *stuckError) Error() string {
	return fmt.Sprintf("daemon pid %d holds the lock and does not answer on its socket", e.holder.PID)
}

// client talks to the daemon over its socket.
type client struct {
	env     Env
	paths   daemon.Paths
	http    *http.Client
	started bool
}

// newClient builds a client for the daemon at paths.
func newClient(env Env, paths daemon.Paths) *client {
	return &client{
		env:   env,
		paths: paths,
		http: &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", paths.Socket)
			},
		}},
	}
}

// dial reports whether a daemon answers on the socket.
func (c *client) dial(ctx context.Context) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.paths.Socket)
	if err != nil {
		return err
	}
	return conn.Close()
}

// absent reports whether a dial failed because nothing is listening, which is
// the only failure that starting a daemon can fix.
func absent(err error) bool {
	return errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED)
}

// connect makes sure a daemon answers, starting one when allowed and needed.
//
// A started daemon that exits before its socket answers is started again, once
// per exit. That settles a command that raced an eyeball stop: the first child
// finds the dying daemon still holding the lock and exits, and the next one
// gets it.
func (c *client) connect(ctx context.Context, start bool) error {
	// Checked before the first dial: a socket path past 107 bytes fails the
	// dial itself with "invalid argument", which names nothing and is not the
	// absence that starting a daemon could fix.
	if err := daemon.Check(c.paths); err != nil {
		return err
	}
	err := c.dial(ctx)
	if err == nil || !absent(err) {
		return err
	}
	if err := c.stuck(ctx); err != nil {
		return err
	}
	if !start || c.env.Getenv("EYEBALL_NO_START") != "" {
		return errors.Wrapf(errNoDaemon, "nothing answers on %s", c.paths.Socket)
	}
	exited, err := c.spawn()
	if err != nil {
		return err
	}
	starts := 1
	deadline := time.Now().Add(startTimeout)
	for time.Now().Before(deadline) {
		if err := c.dial(ctx); err == nil {
			c.started = true
			return nil
		}
		select {
		case <-exited:
			// A child that lost the lock to a daemon in the middle of
			// exiting is the one case another start fixes. A live daemon
			// that holds the lock and does not answer is not, and one that
			// cannot start at all will fail every time.
			if err := c.stuck(ctx); err != nil {
				return err
			}
			if starts >= maxStarts {
				return errors.Newf("%d daemons were started and each exited without answering; the end of %s says:\n%s", starts, c.paths.Log, logTail(c.paths.Log, 10))
			}
			starts++
			if exited, err = c.spawn(); err != nil {
				return err
			}
		case <-ctx.Done():
			return errors.WithStack(ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	return errors.Newf("a daemon was started and did not answer within %s; the end of %s says:\n%s", startTimeout, c.paths.Log, logTail(c.paths.Log, 10))
}

// stuck reports a daemon that holds the lock and has not answered within its
// grace: stuckGrace, or the whole shutdown grace for one marked stopping.
func (c *client) stuck(ctx context.Context) error {
	began := time.Now()
	for daemon.Held(c.paths.Lock) {
		if c.dial(ctx) == nil {
			return nil
		}
		holder, err := daemon.ReadHolder(c.paths.Lock)
		grace := stuckGrace
		if err == nil && holder.Stopping {
			grace = daemon.StoppingGrace + time.Second
		}
		if time.Since(began) > grace {
			if err != nil {
				return errors.Wrap(err, "a daemon holds the lock and does not answer")
			}
			return &stuckError{holder: holder}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

// spawn starts a detached daemon and returns a channel that closes when it
// exits.
func (c *client) spawn() (<-chan struct{}, error) {
	cmd, err := daemon.Start(c.paths, c.env.Exe, "serve")
	if err != nil {
		return nil, err
	}
	exited := make(chan struct{})
	go func(cmd *exec.Cmd) {
		_ = cmd.Wait()
		close(exited)
	}(cmd)
	return exited, nil
}

// logTail is the last n lines of the daemon's log, or a note saying there is
// none.
func logTail(path string, n int) string {
	raw, err := os.ReadFile(path)
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		return "(nothing)"
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// call sends one request and returns the status and the body as it arrived.
func (c *client) call(ctx context.Context, method, path string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, errors.Wrap(err, "encode the request")
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://eyeball"+path, reader)
	if err != nil {
		return 0, nil, errors.Wrap(err, "build the request")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, errors.WithStack(ctx.Err())
		}
		return 0, nil, errors.Wrap(errDropped, err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, errors.WithStack(ctx.Err())
		}
		return 0, nil, errors.Wrap(errDropped, err.Error())
	}
	// A body that is not JSON is not an answer. A wait cut short with nothing
	// written would otherwise read as a verdict with no comments. A plain-text
	// 404 or 405 is the mux of a daemon that predates the route.
	if !json.Valid(raw) && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed) {
		return 0, nil, errors.Wrapf(errOldDaemon, "%s %s answered %d", method, path, resp.StatusCode)
	}
	if !json.Valid(raw) {
		return 0, nil, errors.Wrapf(errDropped, "the daemon answered %d with a body that is not JSON", resp.StatusCode)
	}
	return resp.StatusCode, raw, nil
}
