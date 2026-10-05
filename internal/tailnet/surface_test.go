package tailnet

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestEnrolled(t *testing.T) {
	cases := []struct {
		name string
		make func(dir string) error
		want bool
	}{
		{
			name: "no node directory",
			make: func(string) error { return nil },
			want: false,
		},
		{
			name: "a first login that never finished",
			make: func(dir string) error { return os.Mkdir(filepath.Join(dir, "node.new"), 0o700) },
			want: false,
		},
		{
			name: "an enrolled node",
			make: func(dir string) error { return os.Mkdir(NodeDir(dir), 0o700) },
			want: true,
		},
		{
			name: "a file where the directory would be",
			make: func(dir string) error { return os.WriteFile(NodeDir(dir), nil, 0o600) },
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			dir := t.TempDir()
			g.Expect(c.make(dir)).To(Succeed())
			g.Expect(Enrolled(dir)).To(Equal(c.want))
		})
	}
}

func TestStartSurface(t *testing.T) {
	g := NewWithT(t)
	node := newFake(Status{MagicDNS: true, CertDomains: []string{"eyeball.example.ts.net"}}, StateStarting, StateRunning)
	s := StartSurface("dir", node.construct, http.NotFoundHandler(), discard)

	g.Eventually(s.Report, 5*time.Second).Should(Equal(Report{State: ReportServing, URL: "https://eyeball.example.ts.net"}))
	g.Expect(node.listened.Load()).To(BeTrue())
	g.Expect(s.Stop(stopCtx(t))).To(Succeed())
	g.Expect(node.closes.Load()).To(Equal(int32(1)))
	g.Expect(s.Report().State).To(Equal(ReportOff))
}

func TestStartSurface_aStartThatFails(t *testing.T) {
	g := NewWithT(t)
	node := newFake(Status{}, StateRunning)
	node.startErr = errors.New("/state/node is not a directory")
	s := StartSurface("dir", node.construct, http.NotFoundHandler(), discard)
	g.Eventually(func() string { return s.Report().State }, 5*time.Second).Should(Equal(ReportFailed))
	g.Expect(s.Report().Reason).To(ContainSubstring("is not a directory"))
	g.Expect(s.Stop(stopCtx(t))).To(Succeed())
	// tsnet's Close after a Start that failed early dereferences nil.
	g.Expect(node.closes.Load()).To(BeZero(), "a node whose Start failed was closed")
}

func TestStartSurface_fails(t *testing.T) {
	healthy := Status{MagicDNS: true, CertDomains: []string{"eyeball.example.ts.net"}}
	cases := []struct {
		name     string
		status   Status
		states   []State
		watchEnd error
		reason   string
	}{
		{
			name:   "a node needing a login",
			status: healthy,
			states: []State{StateStarting, StateNeedsLogin},
			reason: "the owner runs eyeball tailnet login",
		},
		{
			name:   "a node waiting for approval",
			status: healthy,
			states: []State{StateNeedsApproval},
			reason: "the owner approves it",
		},
		{
			name:   "MagicDNS off",
			status: Status{CertDomains: []string{"x"}},
			states: []State{StateRunning},
			reason: "MagicDNS is off",
		},
		{
			name:   "HTTPS off",
			status: Status{MagicDNS: true},
			states: []State{StateRunning},
			reason: "HTTPS certificates are off",
		},
		{
			name:   "a node that stopped",
			status: healthy,
			states: []State{StateStarting, StateStopped},
			reason: "the owner restarts the daemon",
		},
		{
			name:     "a watch that ended on its own",
			status:   healthy,
			states:   []State{StateStarting},
			watchEnd: errors.New("the bus went away"),
			reason:   "the bus went away",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			node := newFake(c.status, c.states...)
			node.watchEnd = c.watchEnd
			s := StartSurface("dir", node.construct, http.NotFoundHandler(), discard)
			g.Eventually(func() string { return s.Report().State }, 5*time.Second).Should(Equal(ReportFailed))
			g.Expect(s.Report().Reason).To(ContainSubstring(c.reason))
			// A failed surface never tells an agent to run a command: its
			// reason says what the owner does.
			g.Expect(s.Report().Reason).NotTo(ContainSubstring("eyeball stop"))
			g.Expect(node.listened.Load()).To(BeFalse())
			g.Eventually(node.closes.Load, 5*time.Second).Should(Equal(int32(1)), "a failed node is closed")
			g.Expect(s.Stop(stopCtx(t))).To(Succeed())
			g.Expect(node.closes.Load()).To(Equal(int32(1)), "closed at most once")
		})
	}
}

func TestSurface_Shutdown_drainsRequests(t *testing.T) {
	g := NewWithT(t)
	node := newFake(Status{MagicDNS: true, CertDomains: []string{"x"}}, StateRunning)
	release := make(chan struct{})
	inFlight := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(inFlight)
		<-release
		w.WriteHeader(http.StatusOK)
	})
	s := StartSurface("dir", node.construct, handler, discard)
	g.Eventually(func() string { return s.Report().State }, 5*time.Second).Should(Equal(ReportServing))

	answered := make(chan int, 1)
	go func() {
		res, err := http.Get("http://" + node.addr() + "/")
		if err != nil {
			answered <- 0
			return
		}
		_ = res.Body.Close()
		answered <- res.StatusCode
	}()
	<-inFlight

	// Shutdown waits for the request under the shutdown grace, and leaves the
	// node to Stop.
	shut := make(chan struct{})
	go func() { s.Shutdown(stopCtx(t)); close(shut) }()
	g.Consistently(shut, 200*time.Millisecond).ShouldNot(BeClosed())
	close(release)
	g.Eventually(shut, 5*time.Second).Should(BeClosed())
	g.Expect(answered).To(Receive(Equal(http.StatusOK)), "the request in flight was answered")
	g.Expect(node.closes.Load()).To(BeZero(), "Shutdown closed the node")

	g.Expect(s.Stop(stopCtx(t))).To(Succeed())
	g.Expect(node.closes.Load()).To(Equal(int32(1)))
}

func TestSurface_Shutdown_duringServe(t *testing.T) {
	g := NewWithT(t)
	node := newFake(Status{MagicDNS: true, CertDomains: []string{"x"}}, StateRunning)
	node.statusGate = make(chan struct{})
	s := StartSurface("dir", node.construct, http.NotFoundHandler(), discard)
	g.Eventually(node.statusCalled.Load, 5*time.Second).Should(BeTrue())

	// Shutdown lands between the node's status check and the server being
	// recorded. The server never serves, and the report never says serving.
	s.Shutdown(stopCtx(t))
	close(node.statusGate)
	g.Eventually(s.done, 5*time.Second).Should(BeClosed())
	g.Expect(node.listened.Load()).To(BeTrue())
	g.Expect(s.Report().State).NotTo(Equal(ReportServing))
	_, err := net.Dial("tcp", node.addr())
	g.Expect(err).To(HaveOccurred(), "the listener was left open")
	g.Expect(s.Stop(stopCtx(t))).To(Succeed())
	g.Expect(node.closes.Load()).To(Equal(int32(1)))
}

func TestSurface_Shutdown_duringStatus(t *testing.T) {
	g := NewWithT(t)
	node := newFake(Status{MagicDNS: true, CertDomains: []string{"x"}}, StateRunning)
	node.statusGate = make(chan struct{})
	defer close(node.statusGate)
	node.statusHonorsCtx = true
	s := StartSurface("dir", node.construct, http.NotFoundHandler(), discard)
	g.Eventually(node.statusCalled.Load, 5*time.Second).Should(BeTrue())

	// Shutdown cancels the status read. That is not the node failing: the
	// surface is not reported failed, and Stop is what closes the node.
	s.Shutdown(stopCtx(t))
	g.Eventually(s.done, 5*time.Second).Should(BeClosed())
	g.Expect(s.Report().State).NotTo(Equal(ReportFailed))
	g.Expect(node.closes.Load()).To(BeZero())
	g.Expect(s.Stop(stopCtx(t))).To(Succeed())
	g.Expect(node.closes.Load()).To(Equal(int32(1)))
}

func TestSurface_Stop_beforeStart(t *testing.T) {
	g := NewWithT(t)
	node := newFake(Status{}, StateStarting)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// The owning goroutine runs only after shutdown has marked the surface
	// stopping. It never starts the node, and nothing closes one: tsnet's Close
	// dereferences state its Start sets.
	s := &Surface{started: make(chan struct{}), cancel: func() {}, done: make(chan struct{})}
	s.stopping = true
	s.run(ctx, "dir", node.construct, http.NotFoundHandler(), discard)
	g.Expect(node.startCalled.Load()).To(BeFalse())
	g.Expect(node.closes.Load()).To(BeZero())
	g.Expect(s.Stop(stopCtx(t))).To(Succeed(), "a stop after an unstarted run has nothing to wait for")
}

func TestSurface_Stop_duringStart(t *testing.T) {
	g := NewWithT(t)
	node := newFake(Status{}, StateStarting)
	node.startGate = make(chan struct{})
	s := StartSurface("dir", node.construct, http.NotFoundHandler(), discard)
	g.Eventually(node.startCalled.Load, 5*time.Second).Should(BeTrue())

	// Shutdown arrives while tsnet's Start is still running. Close must not
	// run until Start has returned.
	stopped := make(chan error, 1)
	go func() { stopped <- s.Stop(stopCtx(t)) }()
	g.Consistently(stopped, 200*time.Millisecond).ShouldNot(Receive())
	g.Expect(node.closes.Load()).To(BeZero(), "closed during Start")
	close(node.startGate)
	g.Eventually(stopped, 5*time.Second).Should(Receive(BeNil()))
	g.Expect(node.closes.Load()).To(Equal(int32(1)))
	g.Expect(node.closedDuringStart.Load()).To(BeFalse())
}

func TestSurface_Stop_endsAWaitForRunning(t *testing.T) {
	g := NewWithT(t)
	// A node that never reaches running, as an offline one does not.
	node := newFake(Status{}, StateStarting)
	s := StartSurface("dir", node.construct, http.NotFoundHandler(), discard)
	g.Eventually(node.watching.Load, 5*time.Second).Should(BeTrue())
	g.Expect(s.Stop(stopCtx(t))).To(Succeed(), "shutdown waited for a running state that never came")
	g.Expect(node.closes.Load()).To(Equal(int32(1)))
	// The owning goroutine has ended too. A watch left open would keep it, and
	// on a real node would hold up tsnet's own shutdown.
	g.Eventually(s.done, 5*time.Second).Should(BeClosed())
}

func TestSurface_Stop_overrun(t *testing.T) {
	g := NewWithT(t)
	node := newFake(Status{MagicDNS: true, CertDomains: []string{"x"}}, StateRunning)
	node.closeGate = make(chan struct{})
	defer close(node.closeGate)
	s := StartSurface("dir", node.construct, http.NotFoundHandler(), discard)
	g.Eventually(func() string { return s.Report().State }, 5*time.Second).Should(Equal(ReportServing))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := s.Stop(ctx)
	g.Expect(errors.Is(err, ErrCloseOverrun)).To(BeTrue())
}

// fake is a node that plays a script of states, so no test starts tsnet.
type fake struct {
	status Status
	states []State

	startGate chan struct{}
	closeGate chan struct{}
	// startErr makes Start fail, as tsnet's does when its directory cannot be
	// made, before it has set what its Close dereferences.
	startErr   error
	statusGate chan struct{}
	watchEnd   error
	// statusHonorsCtx makes a Status blocked on statusGate return when its
	// context ends, as a real node's does.
	statusHonorsCtx bool
	// mkdir makes Start create the node's directory, as tsnet does.
	mkdir bool
	dir   string
	// loginURL is said through the constructor's Logf on Start, as tsnet says
	// a login URL through UserLogf.
	loginURL string
	logf     Logf
	ln       atomic.Pointer[net.Listener]

	startCalled       atomic.Bool
	starting          atomic.Bool
	watching          atomic.Bool
	listened          atomic.Bool
	statusCalled      atomic.Bool
	closes            atomic.Int32
	closedDuringStart atomic.Bool
}

// newFake builds a fake that reports status and walks states in order, then
// waits for its watch to end.
func newFake(status Status, states ...State) *fake {
	return &fake{status: status, states: states}
}

// construct is the fake's Constructor.
func (f *fake) construct(dir string, logf Logf) Node {
	f.dir = dir
	f.logf = logf
	return f
}

// Start blocks on startGate when one is set.
func (f *fake) Start() error {
	f.startCalled.Store(true)
	if f.startErr != nil {
		return f.startErr
	}
	if f.mkdir {
		if err := os.MkdirAll(f.dir, 0o700); err != nil {
			return err
		}
	}
	if f.loginURL != "" {
		f.logf("To authenticate, visit:\n\n\t%s", f.loginURL)
	}
	f.starting.Store(true)
	defer f.starting.Store(false)
	if f.startGate != nil {
		<-f.startGate
	}
	return nil
}

// Watch plays the script, then blocks until ctx ends.
func (f *fake) Watch(ctx context.Context) (Watcher, error) {
	f.watching.Store(true)
	return &fakeWatcher{ctx: ctx, states: f.states, end: f.watchEnd}, nil
}

// Status returns the scripted status, blocking on statusGate when one is set.
func (f *fake) Status(ctx context.Context) (Status, error) {
	f.statusCalled.Store(true)
	if f.statusGate == nil {
		return f.status, nil
	}
	if !f.statusHonorsCtx {
		<-f.statusGate
		return f.status, nil
	}
	select {
	case <-f.statusGate:
		return f.status, nil
	case <-ctx.Done():
		return Status{}, ctx.Err()
	}
}

// ListenTLS listens on loopback in place of the tailnet.
func (f *fake) ListenTLS(string) (net.Listener, error) {
	f.listened.Store(true)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err == nil {
		f.ln.Store(&ln)
	}
	return ln, err
}

// addr is where the fake listens.
func (f *fake) addr() string {
	return (*f.ln.Load()).Addr().String()
}

// Close counts closes and blocks on closeGate when one is set.
func (f *fake) Close() error {
	if f.starting.Load() {
		f.closedDuringStart.Store(true)
	}
	f.closes.Add(1)
	if f.closeGate != nil {
		<-f.closeGate
	}
	return nil
}

// fakeWatcher yields scripted states.
type fakeWatcher struct {
	ctx    context.Context
	states []State
	end    error
}

// Next yields the next state, then end if one is set, or blocks until the
// watch ends.
func (w *fakeWatcher) Next() (State, error) {
	if len(w.states) > 0 {
		s := w.states[0]
		w.states = w.states[1:]
		return s, nil
	}
	if w.end != nil {
		return "", w.end
	}
	<-w.ctx.Done()
	return "", w.ctx.Err()
}

// discard drops a node's messages.
func discard(string, ...any) {
}

// stopCtx is the close budget a test gives Stop.
func stopCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
