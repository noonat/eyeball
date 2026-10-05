package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/daemon"
	"github.com/noonat/eyeball/internal/tailnet"
	. "github.com/onsi/gomega"
)

func TestRun_loginInProgress(t *testing.T) {
	g := NewWithT(t)
	paths := statePaths(g, t)
	g.Expect(daemon.Prepare(paths)).To(Succeed())
	holdLock(g, t, paths, "4321\n2026-10-05T00:00:00Z\nlogin\n")

	env := testEnv(paths, t.TempDir())
	env.Getenv = func(name string) string {
		if name == "XDG_STATE_HOME" {
			return filepath.Dir(paths.Dir)
		}
		return ""
	}
	t.Setenv(roleEnv, "die")
	env.Exe = os.Args[0]

	// A login holds the lock and has no socket, which is what a stuck daemon
	// looks like too. The command says to wait for the login, and never to
	// kill it.
	commands := []string{"status", "stop"}
	for _, command := range commands {
		res := runEnv(t, env, "--json", command)
		g.Expect(res.code).To(Equal(1), command)
		var refusal api.ErrorBody
		g.Expect(json.Unmarshal([]byte(res.stdout), &refusal)).To(Succeed(), res.stdout)
		g.Expect(refusal.Error.Code).To(Equal(api.CodeDaemonUnavailable))
		g.Expect(refusal.Error.Message).To(ContainSubstring("tailnet login"))
		g.Expect(refusal.Error.Fix).To(ContainSubstring("once the login finishes"))
		g.Expect(refusal.Error.Fix).NotTo(ContainSubstring("kill"))
	}
	_, statErr := os.Stat(paths.Log)
	g.Expect(os.IsNotExist(statErr)).To(BeTrue(), "a daemon was started against a login's lock")
}

func TestRun_serveUsesTheInjectedNode(t *testing.T) {
	g := NewWithT(t)
	paths := statePaths(g, t)
	g.Expect(daemon.Prepare(paths)).To(Succeed())
	g.Expect(os.Mkdir(tailnet.NodeDir(paths.Dir), 0o700)).To(Succeed())
	built := make(chan string, 1)
	env := testEnv(paths, t.TempDir())
	env.Stdout, env.Stderr = io.Discard, io.Discard
	// The daemon serve runs builds its node with the environment's
	// constructor, which is how a test of serve keeps tsnet out.
	env.NewNode = func(dir string, logf tailnet.Logf) tailnet.Node {
		built <- dir
		node := &loginNode{paths: paths, never: true}
		return node.construct(dir, logf)
	}
	ctx, cancel := context.WithCancel(t.Context())
	var runErr error
	ran := make(chan struct{})
	go func() {
		runErr = Run(ctx, []string{"eyeball", "serve"}, env)
		close(ran)
	}()
	t.Cleanup(func() { cancel(); <-ran })

	g.Eventually(built, 5*time.Second).Should(Receive(Equal(tailnet.NodeDir(paths.Dir))))
	cancel()
	g.Eventually(ran, 10*time.Second).Should(BeClosed())
	g.Expect(runErr).NotTo(HaveOccurred())
}

func TestRun_tailnetLogin(t *testing.T) {
	g := NewWithT(t)
	paths := statePaths(g, t)
	node := &loginNode{paths: paths}
	env := testEnv(paths, t.TempDir())
	env.Terminal = true
	env.NewNode = node.construct

	res := runEnv(t, env, "tailnet", "login")
	g.Expect(res.code).To(BeZero(), res.stderr)
	g.Expect(res.stderr).To(ContainSubstring("https://login.example.com/a/1234"), "the login URL did not reach the person")
	g.Expect(res.stdout).To(ContainSubstring("enrolled"))
	g.Expect(res.stdout).To(ContainSubstring("https://eyeball.example.ts.net"))
	g.Expect(tailnet.Enrolled(paths.Dir)).To(BeTrue())
	// The lock was held, and marked as a login, for as long as the node ran.
	g.Expect(node.heldDuringStart).To(BeTrue())
	g.Expect(node.markedLogin).To(BeTrue())
	g.Expect(node.closed).To(BeTrue())
	g.Expect(daemon.Held(paths.Lock)).To(BeFalse(), "the lock outlived the login")

	// Logging in again uses the enrolled node.
	node = &loginNode{paths: paths}
	env.NewNode = node.construct
	res = runEnv(t, env, "--json", "tailnet", "login")
	g.Expect(res.code).To(BeZero(), res.stderr)
	var got api.LoginResult
	g.Expect(json.Unmarshal([]byte(res.stdout), &got)).To(Succeed(), res.stdout)
	g.Expect(got).To(Equal(api.LoginResult{Dir: tailnet.NodeDir(paths.Dir), URL: "https://eyeball.example.ts.net", First: false}))
	g.Expect(node.dir).To(Equal(tailnet.NodeDir(paths.Dir)))
}

func TestRun_tailnetLoginFails(t *testing.T) {
	cases := []struct {
		name   string
		status tailnet.Status
		cut    bool
		want   string
	}{
		{
			name:   "a tailnet without MagicDNS",
			status: tailnet.Status{CertDomains: []string{"x"}},
			want:   "turn MagicDNS on",
		},
		{
			name: "a login the owner interrupted",
			cut:  true,
			want: "again to finish it",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			paths := statePaths(g, t)
			node := &loginNode{paths: paths, status: &c.status, never: c.cut}
			env := testEnv(paths, t.TempDir())
			env.Terminal = true
			env.NewNode = node.construct
			var stdout, stderr bytes.Buffer
			env.Stdout, env.Stderr = &stdout, &stderr
			// Canceled, as ^C cancels the login's context.
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			time.AfterFunc(200*time.Millisecond, cancel)

			err := Run(ctx, []string{"eyeball", "--json", "tailnet", "login"}, env)
			g.Expect(ExitCode(err)).To(Equal(1))
			var refusal api.ErrorBody
			g.Expect(json.Unmarshal(stdout.Bytes(), &refusal)).To(Succeed(), stdout.String())
			g.Expect(refusal.Error.Code).To(Equal(api.CodeLoginFailed))
			g.Expect(refusal.Error.Fix).To(ContainSubstring(c.want))
			g.Expect(tailnet.Enrolled(paths.Dir)).To(BeFalse())
			g.Expect(daemon.Held(paths.Lock)).To(BeFalse())
		})
	}
}

func TestRun_tailnetLoginNeedsATerminal(t *testing.T) {
	g := NewWithT(t)
	paths := statePaths(g, t)
	env := testEnv(paths, t.TempDir())
	base := env.Getenv
	// An auth key in the environment would let tsnet log in with nobody
	// there, so it changes nothing. tsnet reads the process's environment and
	// the CLI reads Env's, so it is set in both.
	t.Setenv("TS_AUTHKEY", "not-a-real-key")
	env.Getenv = func(name string) string {
		if name == "TS_AUTHKEY" {
			return "not-a-real-key"
		}
		return base(name)
	}
	env.Terminal = false
	env.NewNode = func(string, tailnet.Logf) tailnet.Node {
		panic("a login with no terminal started a node")
	}

	res := runEnv(t, env, "--json", "tailnet", "login")
	g.Expect(res.code).To(Equal(1))
	var refusal api.ErrorBody
	g.Expect(json.Unmarshal([]byte(res.stdout), &refusal)).To(Succeed(), res.stdout)
	g.Expect(refusal.Error.Code).To(Equal(api.CodeOwnerOnly))
	// Refused before anything else: not even the lock was taken.
	_, statErr := os.Stat(paths.Lock)
	g.Expect(os.IsNotExist(statErr)).To(BeTrue(), "the login took the lock before checking for a terminal")
}

func TestRun_tailnetLoginRefusedWhileHeld(t *testing.T) {
	cases := []struct {
		name string
		lock string
		want string
	}{
		{
			name: "a daemon",
			lock: "4321\n2026-10-05T00:00:00Z\n",
			want: "run eyeball stop",
		},
		{
			name: "another login",
			lock: "4321\n2026-10-05T00:00:00Z\nlogin\n",
			want: "finish that login",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			paths := statePaths(g, t)
			g.Expect(daemon.Prepare(paths)).To(Succeed())
			holdLock(g, t, paths, c.lock)
			env := testEnv(paths, t.TempDir())
			env.Terminal = true
			env.NewNode = func(string, tailnet.Logf) tailnet.Node {
				panic("a login started a node while the lock was held")
			}

			res := runEnv(t, env, "--json", "tailnet", "login")
			g.Expect(res.code).To(Equal(1))
			var refusal api.ErrorBody
			g.Expect(json.Unmarshal([]byte(res.stdout), &refusal)).To(Succeed(), res.stdout)
			g.Expect(refusal.Error.Code).To(Equal(api.CodeDaemonRunning))
			g.Expect(refusal.Error.Fix).To(ContainSubstring(c.want))
		})
	}
}

// holdLock writes content to the lock file and holds its flock until the test
// ends, as a daemon or a login in another process would.
func holdLock(g *WithT, t *testing.T, paths daemon.Paths, content string) {
	g.THelper()
	g.Expect(os.WriteFile(paths.Lock, []byte(content), 0o600)).To(Succeed())
	lock, err := os.OpenFile(paths.Lock, os.O_RDWR, 0)
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = lock.Close() })
	g.Expect(syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)).To(Succeed())
}

// loginNode is a tailnet node that runs at once on a tailnet with HTTPS, and
// records what it saw of the lock while it ran.
type loginNode struct {
	paths daemon.Paths
	// status replaces the healthy tailnet's status when set.
	status *tailnet.Status
	// never keeps the node from running, as one waiting on a login does.
	never           bool
	dir             string
	heldDuringStart bool
	markedLogin     bool
	closed          bool
	logf            tailnet.Logf
}

// construct is the node's constructor.
func (n *loginNode) construct(dir string, logf tailnet.Logf) tailnet.Node {
	n.dir = dir
	n.logf = logf
	return n
}

// Start creates the node's directory, as tsnet does, says a login URL as
// tsnet does through UserLogf, and looks at the lock.
func (n *loginNode) Start() error {
	n.logf("To authenticate, visit:\n\n\t%s", "https://login.example.com/a/1234")
	n.heldDuringStart = daemon.Held(n.paths.Lock)
	holder, err := daemon.ReadHolder(n.paths.Lock)
	n.markedLogin = err == nil && holder.Login
	return os.MkdirAll(n.dir, 0o700)
}

// Watch reports running at once, unless the node never runs.
func (n *loginNode) Watch(ctx context.Context) (tailnet.Watcher, error) {
	return &runningWatch{ctx: ctx, sent: n.never}, nil
}

// Status reports a tailnet with MagicDNS and HTTPS, or the status set.
func (n *loginNode) Status(context.Context) (tailnet.Status, error) {
	if n.status != nil {
		return *n.status, nil
	}
	return tailnet.Status{MagicDNS: true, CertDomains: []string{"eyeball.example.ts.net"}}, nil
}

// ListenTLS is not used by a login.
func (n *loginNode) ListenTLS(string) (net.Listener, error) {
	return nil, errors.New("a login does not listen")
}

// Close records that it ran.
func (n *loginNode) Close() error {
	n.closed = true
	return nil
}

// runningWatch reports running, then waits for the watch to end.
type runningWatch struct {
	ctx  context.Context
	sent bool
}

// Next reports running once.
func (w *runningWatch) Next() (tailnet.State, error) {
	if !w.sent {
		w.sent = true
		return tailnet.StateRunning, nil
	}
	<-w.ctx.Done()
	return "", w.ctx.Err()
}
