package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/api"
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
			err = Run(context.Background(), Config{Paths: p, Build: "signal", Stderr: os.Stderr})
		}
		if err != nil {
			_, _ = os.Stderr.WriteString(err.Error())
			os.Exit(1)
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
			var stderr bytes.Buffer
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

func TestRun_twoDaemons(t *testing.T) {
	g := NewWithT(t)
	p := testPaths(g, t)
	first := startDaemon(g, t, p)

	var stderr bytes.Buffer
	err := Run(t.Context(), Config{Paths: p, Build: "second", Stderr: &stderr})
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

// daemon is a daemon running in this process for one test.
type daemon struct {
	cancel context.CancelFunc
	// done closes when Run returns, and err is what it returned. A closed
	// channel can be waited on by the test and by its cleanup alike, where a
	// value sent once is taken by whichever reads first.
	done chan struct{}
	err  error
}

// startDaemon runs a daemon on p and waits until its socket answers.
func startDaemon(g *WithT, t *testing.T, p Paths) *daemon {
	g.THelper()
	ctx, cancel := context.WithCancel(context.Background())
	d := &daemon{cancel: cancel, done: make(chan struct{})}
	var logged syncBuffer
	go func() {
		d.err = Run(ctx, Config{Paths: p, Build: "test-build", Home: "/nonexistent", Stderr: &logged})
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

// syncBuffer is a buffer the daemon's logger writes from one goroutine while
// a failing test reads it from another.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
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
