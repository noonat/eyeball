package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/tailnet"
	. "github.com/onsi/gomega"
)

// roleEnv makes the test binary act as one side of the detachment test when
// it is run as a subprocess, before any test starts.
const roleEnv = "EYEBALL_DAEMON_TEST_ROLE"

func init() {
	switch os.Getenv(roleEnv) {
	case "child":
		// The daemon stand-in: alive long enough for the test to see that its
		// parent's pipe closed while it ran.
		time.Sleep(5 * time.Second)
		os.Exit(0)
	case "daemon":
		// A daemon in a process of its own, for the signal test.
		p, err := Locate(os.Getenv, "/nonexistent")
		if err == nil {
			err = Run(context.Background(), Config{Paths: p, Build: "signal", Stderr: os.Stderr, NewNode: noNode})
		}
		if err != nil {
			_, _ = os.Stderr.WriteString(err.Error())
			os.Exit(1)
		}
		os.Exit(0)
	case "environment":
		// A started process that writes its environment to its log.
		for _, kv := range os.Environ() {
			_, _ = os.Stderr.WriteString(kv + "\n")
		}
		os.Exit(0)
	case "parent":
		// The agent's command stand-in: start a detached child, say its pid on
		// stdout, and exit. The test reads stdout to end-of-file.
		_ = os.Setenv(roleEnv, "child")
		p := Paths{Dir: os.Getenv("EYEBALL_DAEMON_TEST_DIR"), Log: filepath.Join(os.Getenv("EYEBALL_DAEMON_TEST_DIR"), "eyeball.log"), Socket: "s"}
		cmd, err := Start(p, os.Args[0], "-test.run=^$")
		if err != nil {
			_, _ = os.Stderr.WriteString(err.Error())
			os.Exit(1)
		}
		_, _ = os.Stdout.WriteString(strconv.Itoa(cmd.Process.Pid) + "\n")
		os.Exit(0)
	}
}

func Test_buildIsStable(t *testing.T) {
	g := NewWithT(t)
	first := Build()
	g.Expect(first).NotTo(BeEmpty())
	g.Expect(Build()).To(Equal(first))
}

func Test_daemonEnvDropsTailscaleVariables(t *testing.T) {
	g := NewWithT(t)
	got := daemonEnv([]string{"HOME=/h", "TS_AUTHKEY=k", "TSNET_FORCE_LOGIN=1", "TS_CONTROL_URL=u", "PATH=/bin"})
	g.Expect(got).To(Equal([]string{"HOME=/h", "PATH=/bin"}))
}

func TestCheck(t *testing.T) {
	g := NewWithT(t)
	base := shortDir(g, t)
	p, err := Locate(env(base), "/home/a")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(Check(p)).To(Succeed(), "a directory that does not exist yet")

	g.Expect(Prepare(p)).To(Succeed())
	info, err := os.Stat(p.Dir)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o700)))

	g.Expect(os.Chmod(p.Dir, 0o750)).To(Succeed())
	err = Check(p)
	g.Expect(errors.Is(err, ErrStateDirMode)).To(BeTrue())
	g.Expect(err.Error()).To(ContainSubstring("0750"))
	g.Expect(Prepare(p)).NotTo(Succeed(), "an opened directory is refused, not tightened")
	info, err = os.Stat(p.Dir)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o750)))
}

func TestCheck_socketPath(t *testing.T) {
	g := NewWithT(t)
	// /eyeball/eyeball.sock is 21 bytes, so a base of 86 is the longest that
	// fits in 107.
	fits := "/" + strings.Repeat("a", 85)
	p, err := Locate(env(fits), "/home/a")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(p.Socket).To(HaveLen(maxSocketPath))
	g.Expect(Check(p)).To(Succeed())

	p, err = Locate(env(fits+"a"), "/home/a")
	g.Expect(err).NotTo(HaveOccurred())
	err = Check(p)
	g.Expect(errors.Is(err, ErrSocketPath)).To(BeTrue())
	g.Expect(err.Error()).To(ContainSubstring("108 bytes"))
}

func TestHeld(t *testing.T) {
	g := NewWithT(t)
	p := testPaths(g, t)
	g.Expect(Held(p.Lock)).To(BeFalse(), "no lock file at all")

	d := startDaemon(g, t, p)
	g.Expect(Held(p.Lock)).To(BeTrue())
	d.cancel()
	g.Eventually(d.done, 5*time.Second).Should(BeClosed())
	g.Expect(Held(p.Lock)).To(BeFalse(), "a lock file left by a daemon that exited")
}

func TestLocate(t *testing.T) {
	cases := []struct {
		name string
		xdg  string
		home string
		want string
	}{
		{
			name: "XDG_STATE_HOME",
			xdg:  "/var/state",
			home: "/home/a",
			want: "/var/state/eyeball",
		},
		{
			name: "unset falls back to the home directory",
			home: "/home/a",
			want: "/home/a/.local/state/eyeball",
		},
		{
			name: "a relative value is ignored",
			xdg:  "state",
			home: "/home/a",
			want: "/home/a/.local/state/eyeball",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := Locate(env(c.xdg), c.home)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got.Dir).To(Equal(c.want))
			g.Expect(got.Socket).To(Equal(c.want + "/eyeball.sock"))
		})
	}
}

func TestRun(t *testing.T) {
	g := NewWithT(t)
	p := testPaths(g, t)
	d := startDaemon(g, t, p)

	var info api.Daemon
	status := get(g, t, p, "/agent/daemon", &info)
	g.Expect(status).To(Equal(http.StatusOK))
	g.Expect(info.PID).To(Equal(os.Getpid()))
	g.Expect(info.Build).To(Equal("test-build"))
	holder, err := ReadHolder(p.Lock)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(holder.PID).To(Equal(os.Getpid()))

	// The stop route answers before the daemon goes.
	var stopped api.StopResult
	g.Expect(post(g, t, p, "/agent/stop", &stopped)).To(Equal(http.StatusOK))
	g.Expect(stopped.Daemon.PID).To(Equal(os.Getpid()))
	g.Eventually(d.done, 5*time.Second).Should(BeClosed())
	g.Expect(d.err).NotTo(HaveOccurred())
	_, err = os.Stat(p.Socket)
	g.Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue(), "the socket is unlinked")
	relock, err := takeLock(p.Lock, 1, time.Now())
	g.Expect(err).NotTo(HaveOccurred(), "the lock was released")
	g.Expect(relock.release()).To(Succeed())
}

// The whole loop through a real daemon: an agent asks over the socket, the
// reviewer comments and decides through the address serve printed, and the
// agent's wait returns both.
func TestRun_reviewLoopOnLoopback(t *testing.T) {
	g := NewWithT(t)
	p := testPaths(g, t)
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	g.Expect(err).NotTo(HaveOccurred())
	addr := probe.Addr().String()
	g.Expect(probe.Close()).To(Succeed())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var stderr syncBuffer
	go func() {
		done <- Run(ctx, Config{Paths: p, Build: "b", Stderr: &stderr, NewNode: noNode, SurfaceAddr: addr})
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	printed := regexp.MustCompile(`#token=([A-Z2-7]{26})`)
	g.Eventually(stderr.String, 5*time.Second).Should(MatchRegexp(printed.String()))
	token := printed.FindStringSubmatch(stderr.String())[1]

	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	var opened api.RequestResult
	body := api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "look"}
	g.Expect(send(g, t, p, http.MethodPost, "/agent/requests", body, &opened)).To(Equal(http.StatusOK))
	review := strconv.FormatInt(opened.Review.ID, 10)

	write := func(path string, form url.Values) int {
		req, err := http.NewRequest(http.MethodPost, "http://"+addr+path, strings.NewReader(form.Encode()))
		g.Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		req.Header.Set("X-Eyeball-Token", token)
		res, err := http.DefaultClient.Do(req)
		g.Expect(err).NotTo(HaveOccurred())
		_ = res.Body.Close()
		return res.StatusCode
	}
	g.Expect(write("/reviews/"+review+"/comments", url.Values{"round": {"1"}, "level": {"line"}, "path": {"a.txt"}, "start": {"2"}, "body": {"Why two?"}})).To(Equal(http.StatusOK))
	g.Expect(write("/reviews/"+review+"/decision", url.Values{"round": {"1"}, "verdict": {"changes"}, "note": {"Try again."}})).To(Equal(http.StatusOK))

	var waited api.WaitResult
	g.Expect(get(g, t, p, "/agent/reviews/"+review+"/wait?dir="+repo, &waited)).To(Equal(http.StatusOK))
	g.Expect(waited.Kind).To(Equal(api.KindVerdict))
	g.Expect(waited.Note).To(Equal("Try again."))
	g.Expect(waited.Comments).To(HaveLen(1))
	g.Expect(waited.Comments[0].Body).To(Equal("Why two?"))
	g.Expect(waited.Comments[0].Quoted).To(Equal("two"))
}

func TestRun_shutdownLetsWorkFinish(t *testing.T) {
	g := NewWithT(t)
	p := testPaths(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	// A git that sleeps before every call, so a request is still
	// capturing when the daemon is told to stop.
	slow := t.TempDir()
	real, err := exec.LookPath("git")
	g.Expect(err).NotTo(HaveOccurred())
	script := "#!/bin/sh\nsleep 0.1\nexec " + real + " \"$@\"\n"
	g.Expect(os.WriteFile(filepath.Join(slow, "git"), []byte(script), 0o700)).To(Succeed())
	t.Setenv("PATH", slow+string(os.PathListSeparator)+os.Getenv("PATH"))
	d := startDaemon(g, t, p)

	type answer struct {
		status int
		body   string
		err    error
	}
	answered := make(chan answer, 1)
	go func() {
		body := `{"dir": "` + repo + `", "paths": ["a.txt"], "note": "look"}`
		status, raw, err := rawPost(t.Context(), p, "/agent/requests", body)
		answered <- answer{status, raw, err}
	}()
	time.Sleep(250 * time.Millisecond)
	d.cancel()

	// While the capture finishes, the lock file says the daemon is stopping,
	// which is what tells a command not to call it stuck.
	g.Eventually(func() bool {
		holder, err := ReadHolder(p.Lock)
		return err == nil && holder.Stopping
	}, 2*time.Second, 10*time.Millisecond).Should(BeTrue())

	var got answer
	g.Eventually(answered, 10*time.Second).Should(Receive(&got))
	g.Expect(got.err).NotTo(HaveOccurred())
	g.Expect(got.status).To(Equal(http.StatusOK), "a capture in progress was aborted by the shutdown: "+got.body)
	g.Eventually(d.done, 10*time.Second).Should(BeClosed())
}

func TestRun_signal(t *testing.T) {
	signals := []syscall.Signal{syscall.SIGTERM, syscall.SIGINT}
	for _, sig := range signals {
		t.Run(sig.String(), func(t *testing.T) {
			g := NewWithT(t)
			base := shortDir(g, t)
			p, err := Locate(env(base), "/home/a")
			g.Expect(err).NotTo(HaveOccurred())
			cmd := exec.Command(os.Args[0], "-test.run=^$")
			cmd.Env = append(os.Environ(), roleEnv+"=daemon", "XDG_STATE_HOME="+base)
			var stderr syncBuffer
			cmd.Stderr = &stderr
			g.Expect(cmd.Start()).To(Succeed())
			t.Cleanup(func() { _ = cmd.Process.Kill() })
			g.Eventually(func() error {
				conn, err := net.Dial("unix", p.Socket)
				if err == nil {
					_ = conn.Close()
				}
				return err
			}, 5*time.Second, 20*time.Millisecond).Should(Succeed(), stderr.String)

			g.Expect(cmd.Process.Signal(sig)).To(Succeed())
			g.Expect(cmd.Wait()).To(Succeed(), "exits cleanly")
			_, err = os.Stat(p.Socket)
			g.Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue(), "the socket is unlinked")
			relock, err := takeLock(p.Lock, 1, time.Now())
			g.Expect(err).NotTo(HaveOccurred(), "the lock was released")
			g.Expect(relock.release()).To(Succeed())
		})
	}
}

func TestRun_staleSocket(t *testing.T) {
	g := NewWithT(t)
	p := testPaths(g, t)
	g.Expect(Prepare(p)).To(Succeed())

	// A crashed daemon leaves its socket file behind. A normally closed Go
	// listener removes it, so this one is told not to.
	stale, err := net.Listen("unix", p.Socket)
	g.Expect(err).NotTo(HaveOccurred())
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	g.Expect(stale.Close()).To(Succeed())
	_, err = os.Stat(p.Socket)
	g.Expect(err).NotTo(HaveOccurred(), "the stale socket is there")

	startDaemon(g, t, p)
	g.Expect(get(g, t, p, "/agent/daemon", &api.Daemon{})).To(Equal(http.StatusOK))
}

// A page's live stream ends when the daemon stops, so a stop with a page open
// does not wait out the shutdown grace.
func TestRun_stopWithAStreamOpen(t *testing.T) {
	g := NewWithT(t)
	p := testPaths(g, t)
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	g.Expect(err).NotTo(HaveOccurred())
	addr := probe.Addr().String()
	g.Expect(probe.Close()).To(Succeed())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Paths: p, Build: "b", Stderr: io.Discard, NewNode: noNode, SurfaceAddr: addr})
	}()
	g.Eventually(func() error { return dialSocket(p) }, 5*time.Second, 20*time.Millisecond).Should(Succeed())
	res, err := http.Get("http://" + addr + "/events")
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { _ = res.Body.Close() }()
	g.Expect(res.StatusCode).To(Equal(http.StatusOK))

	began := time.Now()
	cancel()
	g.Eventually(done, 5*time.Second).Should(Receive(BeNil()))
	g.Expect(time.Since(began)).To(BeNumerically("<", time.Second))
}

func TestRun_surfaceAddrInUse(t *testing.T) {
	g := NewWithT(t)
	p := testPaths(g, t)
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { _ = taken.Close() }()
	err = Run(t.Context(), Config{Paths: p, Build: "b", Stderr: io.Discard, NewNode: noNode, SurfaceAddr: taken.Addr().String()})
	g.Expect(err).To(MatchError(ContainSubstring("listen on " + taken.Addr().String())))
}

func TestRun_surfaceAddrRefused(t *testing.T) {
	cases := []struct {
		name string
		addr string
		want string
	}{
		{
			name: "every interface",
			addr: "0.0.0.0:8111",
			want: "not a loopback address",
		},
		{
			name: "an empty host",
			addr: ":8111",
			want: "not a loopback address",
		},
		{
			name: "a name",
			addr: "localhost:8111",
			want: "not a loopback address",
		},
		{
			name: "no port",
			addr: "127.0.0.1",
			want: "not a host and a port",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			p := testPaths(g, t)
			err := Run(t.Context(), Config{Paths: p, Build: "b", Stderr: io.Discard, NewNode: noNode, SurfaceAddr: c.addr})
			g.Expect(err).To(MatchError(ContainSubstring(c.want)))
			_, statErr := os.Stat(p.Socket)
			g.Expect(errors.Is(statErr, os.ErrNotExist)).To(BeTrue(), "a daemon that failed startup left its socket")
			relock, err := takeLock(p.Lock, 1, time.Now())
			g.Expect(err).NotTo(HaveOccurred(), "a daemon that failed startup kept its lock")
			g.Expect(relock.release()).To(Succeed())
		})
	}
}

func TestRun_surfaceOnLoopback(t *testing.T) {
	g := NewWithT(t)
	p := testPaths(g, t)
	// A free port, found by binding and releasing one.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	g.Expect(err).NotTo(HaveOccurred())
	addr := probe.Addr().String()
	g.Expect(probe.Close()).To(Succeed())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var stderr syncBuffer
	go func() {
		done <- Run(ctx, Config{Paths: p, Build: "b", Stderr: &stderr, NewNode: noNode, SurfaceAddr: addr})
	}()
	g.Eventually(func() error { return dialSocket(p) }, 5*time.Second, 20*time.Millisecond).Should(Succeed())

	res, err := http.Get("http://" + addr + "/healthz")
	g.Expect(err).NotTo(HaveOccurred())
	_ = res.Body.Close()
	g.Expect(res.StatusCode).To(Equal(http.StatusOK))
	g.Expect(res.Header.Get("Cache-Control")).To(Equal("no-store"))

	// The address is printed once with the run's token in its fragment, and
	// the token appears on no other line, the log's included.
	printed := regexp.MustCompile(`(?m)^eyeball: the surface is at http://` + regexp.QuoteMeta(addr) + `/#token=([A-Z2-7]{26})$`)
	g.Eventually(stderr.String).Should(MatchRegexp(printed.String()))
	token := printed.FindStringSubmatch(stderr.String())[1]
	g.Expect(strings.Count(stderr.String(), token)).To(Equal(1))
	var info api.Daemon
	get(g, t, p, "/agent/daemon", &info)
	raw, err := json.Marshal(info)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(raw)).NotTo(ContainSubstring(token))

	// A write needs the token. POST to /healthz is a 405 once past the check,
	// which tells it apart from the check's 403.
	writes := []struct {
		header string
		status int
	}{
		{header: "", status: http.StatusForbidden},
		{header: "WRONG", status: http.StatusForbidden},
		{header: token, status: http.StatusMethodNotAllowed},
	}
	for _, c := range writes {
		req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/healthz", nil)
		g.Expect(err).NotTo(HaveOccurred())
		if c.header != "" {
			req.Header.Set("X-Eyeball-Token", c.header)
		}
		res, err := http.DefaultClient.Do(req)
		g.Expect(err).NotTo(HaveOccurred())
		_ = res.Body.Close()
		g.Expect(res.StatusCode).To(Equal(c.status), c.header)
	}

	cancel()
	var runErr error
	g.Eventually(done, 10*time.Second).Should(Receive(&runErr))
	g.Expect(runErr).NotTo(HaveOccurred())
	_, err = http.Get("http://" + addr + "/healthz")
	g.Expect(err).To(HaveOccurred(), "the loopback listener outlived the daemon")
}

func TestRun_tailnetCloseOverrun(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the close budget")
	}
	g := NewWithT(t)
	p := testPaths(g, t)
	g.Expect(Prepare(p)).To(Succeed())
	g.Expect(os.Mkdir(tailnet.NodeDir(p.Dir), 0o700)).To(Succeed())

	// A node whose close never returns. The daemon must not release its lock,
	// or a successor could start a second node on the same directory while
	// this one is still closing.
	node := &fakeNode{closeGate: make(chan struct{})}
	t.Cleanup(func() { close(node.closeGate) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Paths: p, Build: "b", Stderr: io.Discard, NewNode: node.construct})
	}()
	g.Eventually(func() error { return dialSocket(p) }, 5*time.Second, 20*time.Millisecond).Should(Succeed())
	g.Eventually(func() bool { return node.listening() }, 5*time.Second).Should(BeTrue())

	cancel()
	var runErr error
	g.Eventually(done, tailnet.CloseBudget+5*time.Second).Should(Receive(&runErr))
	g.Expect(errors.Is(runErr, tailnet.ErrCloseOverrun)).To(BeTrue())
	g.Expect(Held(p.Lock)).To(BeTrue(), "the lock was released while the node was still closing")
	g.Expect(keptUntilExit).NotTo(BeNil(), "the lock's file is not held where the collector cannot close it")
	keptUntilExit = nil
}

func TestRun_tailnetForAnEnrolledNode(t *testing.T) {
	g := NewWithT(t)
	p := testPaths(g, t)
	g.Expect(Prepare(p)).To(Succeed())
	g.Expect(os.Mkdir(tailnet.NodeDir(p.Dir), 0o700)).To(Succeed())

	node := &fakeNode{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Paths: p, Build: "b", Stderr: io.Discard, NewNode: node.construct})
	}()
	g.Eventually(func() error { return dialSocket(p) }, 5*time.Second, 20*time.Millisecond).Should(Succeed())

	g.Eventually(func() api.Surface {
		var info api.Daemon
		get(g, t, p, "/agent/daemon", &info)
		return info.Surface
	}, 5*time.Second, 20*time.Millisecond).Should(Equal(api.Surface{State: "serving", URL: "https://eyeball.example.ts.net"}))
	g.Expect(node.dir).To(Equal(tailnet.NodeDir(p.Dir)))

	// The surface answers on the node's listener.
	res, err := http.Get("http://" + node.addr() + "/healthz")
	g.Expect(err).NotTo(HaveOccurred())
	_ = res.Body.Close()
	g.Expect(res.StatusCode).To(Equal(http.StatusOK))
	g.Expect(res.Header.Get("Content-Security-Policy")).NotTo(BeEmpty())

	cancel()
	g.Eventually(done, 10*time.Second).Should(Receive(BeNil()))
	g.Expect(node.closed.Load()).To(BeTrue(), "the node was closed on shutdown")
}

func TestRun_tailnetStopsWithTheSocket(t *testing.T) {
	g := NewWithT(t)
	p := testPaths(g, t)
	g.Expect(Prepare(p)).To(Succeed())
	g.Expect(os.Mkdir(tailnet.NodeDir(p.Dir), 0o700)).To(Succeed())
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	// A git that sleeps before every call keeps a capture running, and with
	// it the socket's shutdown, after the daemon is told to stop.
	slow := t.TempDir()
	real, err := exec.LookPath("git")
	g.Expect(err).NotTo(HaveOccurred())
	script := "#!/bin/sh\nsleep 0.1\nexec " + real + " \"$@\"\n"
	g.Expect(os.WriteFile(filepath.Join(slow, "git"), []byte(script), 0o700)).To(Succeed())
	t.Setenv("PATH", slow+string(os.PathListSeparator)+os.Getenv("PATH"))

	node := &fakeNode{}
	ctx, cancel := context.WithCancel(context.Background())
	var runErr error
	ran := make(chan struct{})
	go func() {
		runErr = Run(ctx, Config{Paths: p, Build: "b", Stderr: io.Discard, NewNode: node.construct})
		close(ran)
	}()
	t.Cleanup(func() { cancel(); <-ran })
	g.Eventually(func() error { return dialSocket(p) }, 5*time.Second, 20*time.Millisecond).Should(Succeed())
	g.Eventually(func() string {
		var info api.Daemon
		get(g, t, p, "/agent/daemon", &info)
		return info.Surface.State
	}, 5*time.Second, 20*time.Millisecond).Should(Equal("serving"))

	answered := make(chan int, 1)
	go func() {
		body := `{"dir": "` + repo + `", "paths": ["a.txt"], "note": "look"}`
		status, _, _ := rawPost(context.Background(), p, "/agent/requests", body)
		answered <- status
	}()
	time.Sleep(250 * time.Millisecond)
	cancel()

	// While the socket's capture finishes, the tailnet's server has already
	// stopped taking connections. It shuts down with the socket's server,
	// rather than serving on until the node closes.
	g.Eventually(func() bool {
		holder, err := ReadHolder(p.Lock)
		return err == nil && holder.Stopping
	}, 2*time.Second, 10*time.Millisecond).Should(BeTrue())
	g.Eventually(func() error {
		conn, err := net.Dial("tcp", node.addr())
		if err == nil {
			_ = conn.Close()
		}
		return err
	}, time.Second, 10*time.Millisecond).Should(HaveOccurred())
	g.Expect(answered).NotTo(Receive(), "the capture finished before the tailnet's listener was checked")
	g.Eventually(answered, 10*time.Second).Should(Receive(Equal(http.StatusOK)))
	g.Eventually(ran, 10*time.Second).Should(BeClosed())
	g.Expect(runErr).NotTo(HaveOccurred())
}

func TestRun_twoDaemons(t *testing.T) {
	g := NewWithT(t)
	p := testPaths(g, t)
	first := startDaemon(g, t, p)

	var stderr bytes.Buffer
	err := Run(t.Context(), Config{Paths: p, Build: "second", Stderr: &stderr, NewNode: noNode})
	g.Expect(err).NotTo(HaveOccurred(), "the loser exits cleanly")
	g.Expect(stderr.String()).To(ContainSubstring("already running"))
	g.Expect(stderr.String()).To(ContainSubstring("pid " + strconv.Itoa(os.Getpid())))

	var info api.Daemon
	get(g, t, p, "/agent/daemon", &info)
	g.Expect(info.Build).To(Equal("test-build"), "the first daemon still answers")
	g.Expect(first.done).NotTo(BeClosed())
}

func TestRun_waiterOnStop(t *testing.T) {
	g := NewWithT(t)
	p := testPaths(g, t)
	d := startDaemon(g, t, p)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))

	var opened api.RequestResult
	body := api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "look"}
	g.Expect(send(g, t, p, http.MethodPost, "/agent/requests", body, &opened)).To(Equal(http.StatusOK))

	type answer struct {
		status int
		body   string
	}
	// The goroutine reports through the channel rather than asserting, so a
	// failure there arrives as that failure rather than as a timeout.
	waited := make(chan answer, 1)
	go func() {
		path := "/agent/reviews/" + strconv.FormatInt(opened.Review.ID, 10) + "/wait?dir=" + repo
		status, body, err := rawSend(t.Context(), p, http.MethodGet, path)
		if err != nil {
			body = "request failed: " + err.Error()
		}
		waited <- answer{status, body}
	}()
	// The wait is blocked once it has subscribed, which is all that can be
	// observed from here; give it a moment past the connection.
	time.Sleep(300 * time.Millisecond)
	d.cancel()

	var got answer
	g.Eventually(waited, 5*time.Second).Should(Receive(&got))
	g.Expect(got.status).To(Equal(http.StatusServiceUnavailable))
	g.Expect(got.body).To(ContainSubstring(string(api.CodeDaemonStopping)))
	g.Eventually(d.done, 5*time.Second).Should(BeClosed())
	g.Expect(d.err).NotTo(HaveOccurred())
}

func TestStart(t *testing.T) {
	g := NewWithT(t)
	dir := shortDir(g, t)
	parent := exec.Command(os.Args[0], "-test.run=^$")
	parent.Env = append(os.Environ(), roleEnv+"=parent", "EYEBALL_DAEMON_TEST_DIR="+dir)
	stdout, err := parent.StdoutPipe()
	g.Expect(err).NotTo(HaveOccurred())
	began := time.Now()
	g.Expect(parent.Start()).To(Succeed())

	// Reading to end-of-file is what an agent's harness does with a
	// command's output. A child holding the pipe would keep it open for the
	// five seconds it sleeps.
	out, err := io.ReadAll(stdout)
	g.Expect(err).NotTo(HaveOccurred())
	elapsed := time.Since(began)
	g.Expect(parent.Wait()).To(Succeed())
	g.Expect(elapsed).To(BeNumerically("<", 3*time.Second), "the pipe stayed open while the child ran")

	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	g.Expect(err).NotTo(HaveOccurred(), string(out))
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	g.Expect(syscall.Kill(pid, 0)).To(Succeed(), "the child is still running")
	g.Expect(sessionOf(g, pid)).To(Equal(pid), "the child leads its own session")
	cwd, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "cwd"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cwd).To(Equal(dir), "the child runs in the state directory")
}

func TestStart_dropsTailscaleVariables(t *testing.T) {
	g := NewWithT(t)
	p := testPaths(g, t)
	t.Setenv("TS_AUTHKEY", "not-a-real-key")
	t.Setenv("TSNET_FORCE_LOGIN", "1")
	t.Setenv(roleEnv, "environment")
	cmd, err := Start(p, os.Args[0], "-test.run=^$")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cmd.Wait()).To(Succeed())
	logged, err := os.ReadFile(p.Log)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(logged)).To(ContainSubstring(roleEnv+"=environment"), "the started process wrote its environment")
	g.Expect(string(logged)).NotTo(ContainSubstring("TS_AUTHKEY"))
	g.Expect(string(logged)).NotTo(ContainSubstring("TSNET_FORCE_LOGIN"))
}

func TestTakeLoginLock(t *testing.T) {
	g := NewWithT(t)
	p := testPaths(g, t)
	held, err := TakeLoginLock(p)
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { _ = held.Release() }()

	holder, err := ReadHolder(p.Lock)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(holder).To(Equal(Holder{PID: os.Getpid(), Started: holder.Started, Login: true}))

	// A daemon started during the login loses the lock and exits, so no
	// second tsnet server opens the node's directory.
	var stderr bytes.Buffer
	g.Expect(Run(t.Context(), Config{Paths: p, Build: "b", Stderr: &stderr, NewNode: noNode})).To(Succeed())
	g.Expect(stderr.String()).To(ContainSubstring("already running"))
	_, err = TakeLoginLock(p)
	g.Expect(errors.Is(err, ErrRunning)).To(BeTrue(), "a second login took the lock")

	g.Expect(held.Release()).To(Succeed())
	g.Expect(Held(p.Lock)).To(BeFalse())
}

// noNode is the node constructor every daemon test runs with. Calling it would
// mean a test started a tailnet node, which contacts Tailscale.
func noNode(string, tailnet.Logf) tailnet.Node {
	panic("a daemon test started a tailnet node")
}

// startDaemon runs a daemon on p and waits until its socket answers.
func startDaemon(g *WithT, t *testing.T, p Paths) *daemon {
	g.THelper()
	ctx, cancel := context.WithCancel(context.Background())
	d := &daemon{cancel: cancel, done: make(chan struct{})}
	var logged syncBuffer
	go func() {
		d.err = Run(ctx, Config{Paths: p, Build: "test-build", Home: "/nonexistent", Stderr: &logged, NewNode: noNode})
		close(d.done)
	}()
	t.Cleanup(func() {
		cancel()
		<-d.done
	})
	g.Eventually(func() error {
		conn, err := net.Dial("unix", p.Socket)
		if err == nil {
			_ = conn.Close()
		}
		return err
	}, 5*time.Second, 20*time.Millisecond).Should(Succeed(), logged.String)
	return d
}

// Write appends to the buffer.
func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String reads the buffer.
func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// testPaths are the daemon's paths under a short temporary directory, since
// t.TempDir nests the test's name and a long name passes the socket limit.
func testPaths(g *WithT, t *testing.T) Paths {
	g.THelper()
	p, err := Locate(env(shortDir(g, t)), "/home/a")
	g.Expect(err).NotTo(HaveOccurred())
	return p
}

// shortDir is a temporary directory with a short path.
func shortDir(g *WithT, t *testing.T) string {
	g.THelper()
	dir, err := os.MkdirTemp("", "ebd-")
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// env is a getenv that knows only XDG_STATE_HOME.
func env(xdg string) func(string) string {
	return func(name string) string {
		if name == "XDG_STATE_HOME" {
			return xdg
		}
		return ""
	}
}

// get sends a GET to the daemon and decodes the answer.
func get(g *WithT, t *testing.T, p Paths, path string, out any) int {
	g.THelper()
	return send(g, t, p, http.MethodGet, path, nil, out)
}

// post sends a POST with an empty JSON body.
func post(g *WithT, t *testing.T, p Paths, path string, out any) int {
	g.THelper()
	return send(g, t, p, http.MethodPost, path, struct{}{}, out)
}

// send makes one request over the daemon's socket.
func send(g *WithT, t *testing.T, p Paths, method, path string, body, out any) int {
	g.THelper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		g.Expect(err).NotTo(HaveOccurred())
		reader = bytes.NewReader(raw)
	}
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", p.Socket)
		},
	}}
	req, err := http.NewRequestWithContext(t.Context(), method, "http://eyeball"+path, reader)
	g.Expect(err).NotTo(HaveOccurred())
	resp, err := client.Do(req)
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(bufio.NewReader(resp.Body))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(json.Unmarshal(raw, out)).To(Succeed(), string(raw))
	return resp.StatusCode
}

// rawSend makes one request over the daemon's socket and returns what came
// back, for a goroutine that cannot assert.
func rawSend(ctx context.Context, p Paths, method, path string) (int, string, error) {
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", p.Socket)
		},
	}}
	req, err := http.NewRequestWithContext(ctx, method, "http://eyeball"+path, nil)
	if err != nil {
		return 0, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw), err
}

// rawPost sends a JSON body over the daemon's socket and returns what came
// back, for a goroutine that cannot assert.
func rawPost(ctx context.Context, p Paths, path, body string) (int, string, error) {
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", p.Socket)
		},
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://eyeball"+path, strings.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw), err
}

// dialSocket reports whether the daemon's socket answers.
func dialSocket(p Paths) error {
	conn, err := net.Dial("unix", p.Socket)
	if err == nil {
		_ = conn.Close()
	}
	return err
}

// listening reports whether the surface has listened on the fake.
func (f *fakeNode) listening() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ln != nil
}

// construct is the fake's constructor.
func (f *fakeNode) construct(dir string, _ tailnet.Logf) tailnet.Node {
	f.mu.Lock()
	f.dir = dir
	f.mu.Unlock()
	return f
}

// addr is where the fake listens.
func (f *fakeNode) addr() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ln.Addr().String()
}

// Start does nothing.
func (f *fakeNode) Start() error {
	return nil
}

// Watch reports running, then waits for the watch to end.
func (f *fakeNode) Watch(ctx context.Context) (tailnet.Watcher, error) {
	return &fakeWatch{ctx: ctx}, nil
}

// Status reports a tailnet with HTTPS.
func (f *fakeNode) Status(context.Context) (tailnet.Status, error) {
	return tailnet.Status{MagicDNS: true, CertDomains: []string{"eyeball.example.ts.net"}}, nil
}

// ListenTLS listens on loopback, without TLS.
func (f *fakeNode) ListenTLS(string) (net.Listener, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	f.mu.Lock()
	f.ln = ln
	f.mu.Unlock()
	return ln, err
}

// Close records that it ran, and blocks on closeGate when one is set.
func (f *fakeNode) Close() error {
	f.closed.Store(true)
	if f.closeGate != nil {
		<-f.closeGate
	}
	return nil
}

// Next reports running, then blocks until the watch ends.
func (w *fakeWatch) Next() (tailnet.State, error) {
	if !w.sent {
		w.sent = true
		return tailnet.StateRunning, nil
	}
	<-w.ctx.Done()
	return "", w.ctx.Err()
}

// sessionOf reads a process's session id from /proc, field 6 of its stat line.
// The command name in field 2 is in parentheses and may hold spaces, so the
// fields are counted from after its closing parenthesis.
func sessionOf(g *WithT, pid int) int {
	g.THelper()
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	g.Expect(err).NotTo(HaveOccurred())
	stat := string(raw)
	fields := strings.Fields(stat[strings.LastIndexByte(stat, ')')+1:])
	g.Expect(len(fields)).To(BeNumerically(">", 3))
	sid, err := strconv.Atoi(fields[3])
	g.Expect(err).NotTo(HaveOccurred())
	return sid
}

// gitRepo builds a repository with one committed file and an edit to it.
func gitRepo(g *WithT, dir string) string {
	g.THelper()
	g.Expect(os.MkdirAll(dir, 0o700)).To(Succeed())
	run := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(
			os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=t",
			"GIT_AUTHOR_EMAIL=t@example.invalid",
			"GIT_COMMITTER_NAME=t",
			"GIT_COMMITTER_EMAIL=t@example.invalid",
		)
		out, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred(), string(out))
		return strings.TrimSpace(string(out))
	}
	run("init", "--quiet", "--template=", "-b", "main")
	g.Expect(os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o600)).To(Succeed())
	run("add", "-A")
	commit := run("commit-tree", run("write-tree"), "-m", "base")
	run("update-ref", "refs/heads/main", commit)
	g.Expect(os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\ntwo\n"), 0o600)).To(Succeed())
	return dir
}

// daemon is a daemon running in this process for one test.
type daemon struct {
	cancel context.CancelFunc
	// done closes when Run returns, and err is what it returned. A closed
	// channel can be waited on by the test and by its cleanup alike, where a
	// value sent once is taken by whichever reads first.
	done chan struct{}
	err  error
}

// syncBuffer is a buffer the daemon's logger writes from one goroutine while
// a failing test reads it from another.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// fakeNode is a tailnet node that reaches running at once and listens on
// loopback, so no test starts tsnet.
type fakeNode struct {
	mu        sync.Mutex
	dir       string
	ln        net.Listener
	closed    atomic.Bool
	closeGate chan struct{}
}

// fakeWatch reports running once.
type fakeWatch struct {
	ctx  context.Context
	sent bool
}
