package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/blob"
	"github.com/noonat/eyeball/internal/daemon"
	"github.com/noonat/eyeball/internal/server"
	"github.com/noonat/eyeball/internal/store"
	. "github.com/onsi/gomega"
)

// update rewrites the golden files from what the commands print now. The
// diff of testdata is then the review of the change.
var update = flag.Bool("update", false, "rewrite the golden files")

// roleEnv makes the test binary stand in for a daemon that dies at once, when
// the dialer starts it as a subprocess.
const roleEnv = "EYEBALL_CLI_TEST_ROLE"

func init() {
	if os.Getenv(roleEnv) == "die" {
		_, _ = os.Stderr.WriteString("the stand-in daemon exited\n")
		os.Exit(1)
	}
}

func TestRun(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)

	// Flags after the paths, and a root flag after the subcommand.
	res := f.run(t, "request", "a.txt", "-m", "Add a line", "--json", "--agent", "bob")
	g.Expect(res.code).To(BeZero(), res.stderr)
	var opened api.RequestResult
	g.Expect(json.Unmarshal([]byte(res.stdout), &opened)).To(Succeed(), res.stdout)
	g.Expect(opened.Review.Agent).To(Equal("bob"))
	g.Expect(opened.Review.Paths).To(Equal([]string{"a.txt"}))
	g.Expect(opened.Next).To(ContainSubstring("eyeball wait"))
}

func TestRun_blankNote(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	f.stop()

	// Refused before anything is sent: with no daemon and autostart off, a
	// command that tried to send would say so instead.
	res := f.run(t, "request", "a.txt", "-m", " ", "-m", "\n")
	g.Expect(res.code).To(Equal(1))
	g.Expect(res.stderr).To(ContainSubstring("needs a note"))
	g.Expect(res.stderr).To(ContainSubstring("fix: "))
}

func TestRun_buildMismatch(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)

	res := f.runWith(t, map[string]string{}, "other-build", "status")
	g.Expect(res.code).To(BeZero(), res.stderr)
	g.Expect(res.stderr).To(ContainSubstring("eyeball stop"))

	res = f.run(t, "status")
	g.Expect(res.stderr).To(BeEmpty(), "the same build is not warned about")
}

func TestRun_dashedPath(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	write(g, f.repo, "-weird.txt", "x\n")

	res := f.run(t, "--json", "request", "-m", "odd name", "--", "-weird.txt")
	g.Expect(res.code).To(BeZero(), res.stderr)
	g.Expect(res.stdout).To(ContainSubstring(`"-weird.txt"`))
}

func TestRun_format(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	write(g, f.repo, "big.txt", strings.Repeat("line\n", 2000))

	res := f.run(t, "request", "a.txt", "big.txt", "-m", "x", "--format", "{{if eq .review.id 1}}first{{end}} {{.size.added}} {{if gt .size.added 1000}}big{{end}}")
	g.Expect(res.code).To(BeZero(), res.stderr)
	g.Expect(res.stdout).To(Equal("first 2001 big\n"))
}

func TestRun_formatParsedBeforeSending(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)

	res := f.run(t, "request", "a.txt", "-m", "x", "--format", "{{.review.id")
	g.Expect(res.code).To(Equal(1))
	g.Expect(res.stderr).To(ContainSubstring("does not parse"))
	_, err := f.store.Review(t.Context(), 1)
	g.Expect(err).To(HaveOccurred(), "the request was sent before the template was checked")
}

func TestRun_humanOutput(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	steps := []struct {
		name string
		args []string
		pre  func()
	}{
		{
			name: "request",
			args: []string{"request", "a.txt", "-m", "Add a second line\n\nIt was missing."},
		},
		{
			name: "status",
			args: []string{"status"},
		},
		{
			name: "wait",
			args: []string{"wait", "1"},
			pre: func() {
				f.comment(g, t, 1, "Say why.")
				f.decide(g, t, 1, store.VerdictChanges, "Close.")
			},
		},
		{
			name: "reply",
			args: []string{"reply", "1", "-m", "Added a comment saying why."},
		},
		{
			name: "show",
			args: []string{"show", "1"},
		},
		{
			name: "abandon",
			args: []string{"abandon", "1"},
		},
	}
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			g := NewWithT(t)
			if s.pre != nil {
				s.pre()
			}
			res := f.run(t, s.args...)
			g.Expect(res.code).To(BeZero(), res.stderr)
			got := f.normalize(res.stdout)
			golden := filepath.Join("testdata", s.name+".txt")
			if *update {
				g.Expect(os.WriteFile(golden, []byte(got), 0o644)).To(Succeed())
			}
			want, err := os.ReadFile(golden)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(string(want)))
		})
	}
}

func TestRun_noStart(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	f.stop()

	res := f.run(t, "status")
	g.Expect(res.code).To(Equal(1))
	g.Expect(res.stderr).To(ContainSubstring("nothing answers on " + f.paths.Socket))
	_, err := os.Stat(f.paths.Log)
	g.Expect(os.IsNotExist(err)).To(BeTrue(), "EYEBALL_NO_START started a daemon")
}

func TestRun_olderDaemon(t *testing.T) {
	g := NewWithT(t)
	paths := statePaths(g, t)
	g.Expect(daemon.Prepare(paths)).To(Succeed())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /agent/daemon", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"build": "test"}`))
	})
	serve(g, t, paths, mux)

	// A route the running daemon predates: its mux answers 404 in plain text.
	res := runIn(t, paths, t.TempDir(), "status")
	g.Expect(res.code).To(Equal(1))
	g.Expect(res.stderr).To(ContainSubstring("older build"))
	g.Expect(res.stderr).To(ContainSubstring("eyeball stop"))
}

func TestRun_refusal(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)

	res := f.run(t, "request", "nothing-here.txt", "-m", "typo")
	g.Expect(res.code).To(Equal(1))
	g.Expect(res.stdout).To(BeEmpty())
	g.Expect(res.stderr).To(ContainSubstring("eyeball: nothing-here.txt is not in the project"))
	g.Expect(res.stderr).To(ContainSubstring("fix: "))

	res = f.run(t, "--json", "request", "nothing-here.txt", "-m", "typo")
	g.Expect(res.code).To(Equal(1))
	var refusal api.ErrorBody
	g.Expect(json.Unmarshal([]byte(res.stdout), &refusal)).To(Succeed(), res.stdout)
	g.Expect(refusal.Error.Code).To(Equal(api.CodePathUnmatched))
}

func TestRun_refusalBodyAsReceived(t *testing.T) {
	g := NewWithT(t)
	paths := statePaths(g, t)
	g.Expect(daemon.Prepare(paths)).To(Succeed())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /agent/daemon", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"build": "test"}`))
	})
	refusal := `{"error": {"code": "not_found", "message": "no", "fix": "yes"}, "from_a_newer_daemon": true}`
	mux.HandleFunc("GET /agent/status", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(refusal))
	})
	serve(g, t, paths, mux)

	res := runIn(t, paths, t.TempDir(), "--json", "status")
	g.Expect(res.code).To(Equal(1))
	g.Expect(res.stdout).To(Equal(refusal+"\n"), "the body is printed as it arrived, fields this build does not know included")
}

func TestRun_repeatedMessage(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)

	res := f.run(t, "request", "a.txt", "-m", "summary, with a comma", "-m", "detail")
	g.Expect(res.code).To(BeZero(), res.stderr)
	round, err := f.store.Round(t.Context(), 1, 1)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(round.Note).To(Equal("summary, with a comma\n\ndetail"))
}

func TestRun_socketPathTooLong(t *testing.T) {
	g := NewWithT(t)
	long := filepath.Join(statePaths(g, t).Dir, strings.Repeat("d", 100))
	paths, err := daemon.Locate(func(string) string { return long }, "/nonexistent")
	g.Expect(err).NotTo(HaveOccurred())

	res := runIn(t, paths, t.TempDir(), "status")
	g.Expect(res.code).To(Equal(1))
	g.Expect(res.stderr).To(ContainSubstring("at most 107"), "the dial's own error names nothing")
}

func TestRun_startChecksFirst(t *testing.T) {
	g := NewWithT(t)
	paths := statePaths(g, t)
	g.Expect(os.MkdirAll(paths.Dir, 0o755)).To(Succeed())
	g.Expect(os.Chmod(paths.Dir, 0o755)).To(Succeed())
	env := testEnv(paths, t.TempDir())
	env.Getenv = func(name string) string {
		if name == "XDG_STATE_HOME" {
			return filepath.Dir(paths.Dir)
		}
		return ""
	}
	env.Exe = "/nonexistent/eyeball"
	var stdout, stderr bytes.Buffer
	env.Stdout, env.Stderr = &stdout, &stderr

	err := Run(t.Context(), []string{"eyeball", "status"}, env)
	g.Expect(ExitCode(err)).To(Equal(1))
	g.Expect(stderr.String()).To(ContainSubstring("0755"), "the daemon's refusal is made here, where it is read")
	_, statErr := os.Stat(paths.Log)
	g.Expect(os.IsNotExist(statErr)).To(BeTrue(), "nothing was started")
}

func TestRun_startRestartsAndGivesUp(t *testing.T) {
	g := NewWithT(t)
	paths := statePaths(g, t)
	env := testEnv(paths, t.TempDir())
	env.Getenv = func(name string) string {
		if name == "XDG_STATE_HOME" {
			return filepath.Dir(paths.Dir)
		}
		return ""
	}
	// The started "daemon" is this test binary, told to die at once.
	t.Setenv(roleEnv, "die")
	env.Exe = os.Args[0]
	var stdout, stderr bytes.Buffer
	env.Stdout, env.Stderr = &stdout, &stderr

	err := Run(t.Context(), []string{"eyeball", "status"}, env)
	g.Expect(ExitCode(err)).To(Equal(1))
	g.Expect(stderr.String()).To(ContainSubstring("each exited without answering"))
	g.Expect(stderr.String()).To(ContainSubstring("the stand-in daemon exited"), "the log's tail is printed")
	logged, readErr := os.ReadFile(paths.Log)
	g.Expect(readErr).NotTo(HaveOccurred())
	starts := strings.Count(string(logged), "the stand-in daemon exited")
	g.Expect(starts).To(BeNumerically(">", 1), "an early exit is started again")
	g.Expect(starts).To(BeNumerically("<=", maxStarts), "a daemon that cannot start is not started without limit")
}

func TestRun_stopWithNoDaemon(t *testing.T) {
	g := NewWithT(t)
	paths := statePaths(g, t)

	// Autostart is left on: stop must not start a daemon in order to stop it.
	env := testEnv(paths, t.TempDir())
	env.Getenv = func(name string) string {
		if name == "XDG_STATE_HOME" {
			return filepath.Dir(paths.Dir)
		}
		return ""
	}
	var stdout, stderr bytes.Buffer
	env.Stdout, env.Stderr = &stdout, &stderr
	err := Run(t.Context(), []string{"eyeball", "stop"}, env)
	g.Expect(err).NotTo(HaveOccurred(), stderr.String())
	g.Expect(stdout.String()).To(ContainSubstring("no daemon is running"))

	// The same shape as a stop that found a daemon, with stopped false.
	stdout.Reset()
	err = Run(t.Context(), []string{"eyeball", "--json", "stop"}, env)
	g.Expect(err).NotTo(HaveOccurred(), stderr.String())
	var got api.StopResult
	g.Expect(json.Unmarshal(stdout.Bytes(), &got)).To(Succeed(), stdout.String())
	g.Expect(stdout.String()).To(ContainSubstring(`"stopped": false`))
	g.Expect(got.Daemon).To(BeNil())
	_, statErr := os.Stat(paths.Log)
	g.Expect(os.IsNotExist(statErr)).To(BeTrue(), "stop tried to start a daemon")
}

func TestRun_stoppingDaemonIsNotStuck(t *testing.T) {
	g := NewWithT(t)
	paths := statePaths(g, t)
	g.Expect(daemon.Prepare(paths)).To(Succeed())
	// A daemon that has begun to stop: no socket, the lock held, and marked
	// stopping. It releases the lock two seconds later, past stuckGrace.
	g.Expect(os.WriteFile(paths.Lock, []byte("4321\n2026-10-05T00:00:00Z\nstopping\n"), 0o600)).To(Succeed())
	lock, err := os.OpenFile(paths.Lock, os.O_RDWR, 0)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)).To(Succeed())
	time.AfterFunc(2*time.Second, func() { _ = lock.Close() })
	t.Cleanup(func() { _ = lock.Close() })

	env := testEnv(paths, t.TempDir())
	env.Getenv = func(name string) string {
		if name == "XDG_STATE_HOME" {
			return filepath.Dir(paths.Dir)
		}
		return ""
	}
	t.Setenv(roleEnv, "die")
	env.Exe = os.Args[0]
	var stdout, stderr bytes.Buffer
	env.Stdout, env.Stderr = &stdout, &stderr

	// The started stand-in dies, so the command fails, but only after the
	// stopping daemon let go of the lock and a start was tried.
	err = Run(t.Context(), []string{"eyeball", "status"}, env)
	g.Expect(ExitCode(err)).To(Equal(1))
	g.Expect(stderr.String()).NotTo(ContainSubstring("kill 4321"), "a stopping daemon was called stuck")
	_, statErr := os.Stat(paths.Log)
	g.Expect(statErr).NotTo(HaveOccurred(), "no start was tried once the lock was free")
}

func TestRun_stuckDaemon(t *testing.T) {
	g := NewWithT(t)
	paths := statePaths(g, t)
	g.Expect(daemon.Prepare(paths)).To(Succeed())
	// A daemon that holds the lock and has lost its socket. A flock belongs to
	// an open file, so holding it on a descriptor here is what Held sees from
	// any other descriptor, in this process or another.
	g.Expect(os.WriteFile(paths.Lock, []byte("4321\n2026-10-05T00:00:00Z\n"), 0o600)).To(Succeed())
	lock, err := os.OpenFile(paths.Lock, os.O_RDWR, 0)
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = lock.Close() })
	g.Expect(syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)).To(Succeed())
	g.Expect(daemon.Held(paths.Lock)).To(BeTrue())

	env := testEnv(paths, t.TempDir())
	env.Getenv = func(name string) string {
		if name == "XDG_STATE_HOME" {
			return filepath.Dir(paths.Dir)
		}
		return ""
	}
	t.Setenv(roleEnv, "die")
	env.Exe = os.Args[0]
	var stdout, stderr bytes.Buffer
	env.Stdout, env.Stderr = &stdout, &stderr

	err = Run(t.Context(), []string{"eyeball", "status"}, env)
	g.Expect(ExitCode(err)).To(Equal(1))
	g.Expect(stderr.String()).To(ContainSubstring("pid 4321 holds the lock"))
	g.Expect(stderr.String()).To(ContainSubstring("kill 4321"))
	_, statErr := os.Stat(paths.Log)
	g.Expect(os.IsNotExist(statErr)).To(BeTrue(), "a start was tried against a stuck daemon")

	stdout.Reset()
	stderr.Reset()
	err = Run(t.Context(), []string{"eyeball", "stop"}, env)
	g.Expect(ExitCode(err)).To(Equal(1), "stop reported nothing to stop while a daemon held the lock")
	g.Expect(stderr.String()).To(ContainSubstring("kill 4321"))
}

func TestRun_usage(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)

	// urfave builds an exit status of 3 for an unknown command, which would
	// read as wait's late comments.
	res := f.run(t, "wiat", "5")
	g.Expect(res.code).To(Equal(1))
	g.Expect(res.stderr).To(ContainSubstring("no command 'wiat'"))
	g.Expect(res.stdout).To(BeEmpty())

	res = f.run(t, "--json", "request", "a.txt", "--nope")
	g.Expect(res.code).To(Equal(1))
	var refusal api.ErrorBody
	g.Expect(json.Unmarshal([]byte(res.stdout), &refusal)).To(Succeed(), "stdout holds only the refusal: "+res.stdout)
	g.Expect(refusal.Error.Code).To(Equal(api.CodeUsage))

	res = f.run(t)
	g.Expect(res.code).To(BeZero())
	g.Expect(res.stdout).To(ContainSubstring("COMMANDS:"))
}

func TestRun_wait(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	f.run(t, "request", "a.txt", "-m", "look")
	c := f.comment(g, t, 1, "a point")
	f.decide(g, t, 1, store.VerdictChanges, "more")

	res := f.run(t, "wait", "1")
	g.Expect(res.code).To(BeZero(), res.stderr)
	g.Expect(res.stdout).To(ContainSubstring("changes requested"))
	g.Expect(res.stdout).To(ContainSubstring("a point"))
	g.Expect(f.delivered(g, t, 1, c.ID)).To(BeTrue(), "the receipt went out after printing")
	rows, err := f.store.ReviewsIn(t.Context(), 1)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(rows[0].VerdictUndelivered).To(BeFalse(), "the receipt stamped the verdict as received")
}

func TestRun_waitDroppedConnection(t *testing.T) {
	g := NewWithT(t)
	paths := statePaths(g, t)
	g.Expect(daemon.Prepare(paths)).To(Succeed())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /agent/daemon", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"build": "test"}`))
	})
	hangUp := func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}
	mux.HandleFunc("GET /agent/reviews/{review}/wait", hangUp)
	mux.HandleFunc("GET /agent/reviews/{review}", hangUp)
	serve(g, t, paths, mux)

	res := runIn(t, paths, t.TempDir(), "wait", "7")
	g.Expect(res.code).To(Equal(1))
	g.Expect(res.stderr).To(ContainSubstring("connection to the daemon was lost"))
	g.Expect(res.stderr).To(ContainSubstring("fix: run eyeball wait 7 again"))

	// Any other command reports the same code, not a failure inside the
	// daemon.
	res = runIn(t, paths, t.TempDir(), "--json", "show", "7")
	g.Expect(res.code).To(Equal(1))
	g.Expect(res.stdout).To(ContainSubstring(string(api.CodeConnectionLost)))
}

func TestRun_waitEmptyBody(t *testing.T) {
	g := NewWithT(t)
	paths := statePaths(g, t)
	g.Expect(daemon.Prepare(paths)).To(Succeed())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /agent/daemon", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"build": "test"}`))
	})
	// A handler that returned without writing: a 200 with nothing in it,
	// which must not read as a verdict with no comments.
	mux.HandleFunc("GET /agent/reviews/{review}/wait", func(http.ResponseWriter, *http.Request) {})
	serve(g, t, paths, mux)

	res := runIn(t, paths, t.TempDir(), "wait", "7")
	g.Expect(res.code).To(Equal(1))
	g.Expect(res.stderr).To(ContainSubstring("not JSON"))
	g.Expect(res.stdout).To(BeEmpty())
}

func TestRun_waitFormatPostsNoReceipt(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	f.run(t, "request", "a.txt", "-m", "look")
	c := f.comment(g, t, 1, "a point")
	f.decide(g, t, 1, store.VerdictChanges, "more")

	res := f.run(t, "wait", "1", "--format", "{{.verdict}}")
	g.Expect(res.code).To(BeZero(), res.stderr)
	g.Expect(res.stdout).To(Equal("changes\n"))
	g.Expect(f.delivered(g, t, 1, c.ID)).To(BeFalse(), "a template that printed no comment acknowledged one")
}

func TestRun_waitLateComments(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	f.run(t, "request", "a.txt", "-m", "look")
	f.decide(g, t, 1, store.VerdictChanges, "more")
	write(g, f.repo, "a.txt", "three\n")
	f.run(t, "request", "--review", "1", "-m", "again")
	round, err := f.store.Round(t.Context(), 1, 1)
	g.Expect(err).NotTo(HaveOccurred())
	_, err = f.store.AddComment(t.Context(), store.NewComment{RoundID: round.ID, Level: store.LevelReview, Body: "one more thing"})
	g.Expect(err).NotTo(HaveOccurred())

	res := f.run(t, "wait", "1")
	g.Expect(res.code).To(Equal(exitLateComments), res.stderr)
	g.Expect(res.stdout).To(ContainSubstring("one more thing"))
}

func TestRun_waitTimeout(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	f.run(t, "request", "a.txt", "-m", "look")

	res := f.run(t, "--json", "wait", "1", "--timeout", "200ms")
	g.Expect(res.code).To(Equal(exitStillWaiting), res.stderr)
	var got api.WaitResult
	g.Expect(json.Unmarshal([]byte(res.stdout), &got)).To(Succeed(), res.stdout)
	g.Expect(got.Kind).To(Equal(api.KindStillWaiting))
	g.Expect(got.Review.ID).To(Equal(int64(1)))
	g.Expect(got.Review.Title).To(Equal("look"), "the review's fields are filled in from show")
	g.Expect(res.stdout).To(ContainSubstring(`"decided_at": null`), "a template naming it does not fail")
}

func TestRun_waitWriteFails(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	f.run(t, "request", "a.txt", "-m", "look")
	c := f.comment(g, t, 1, "a point")
	f.decide(g, t, 1, store.VerdictChanges, "more")

	env := testEnv(f.paths, f.repo)
	var stderr bytes.Buffer
	env.Stdout, env.Stderr = failingWriter{}, &stderr
	err := Run(t.Context(), []string{"eyeball", "wait", "1"}, env)
	g.Expect(ExitCode(err)).To(Equal(1))
	g.Expect(f.delivered(g, t, 1, c.ID)).To(BeFalse(), "a comment that never reached the screen was acknowledged")
}

// failingWriter fails every write, as a full disk does to a redirected stdout.
type failingWriter struct{}

// Write fails.
func (failingWriter) Write([]byte) (int, error) {
	return 0, syscall.ENOSPC
}

// fixture is an in-process daemon on a short socket path with a repository
// to review, and a way to run the command line against both.
type fixture struct {
	paths daemon.Paths
	repo  string
	store *store.Store
	srv   *server.Server
	stop  func()
}

// result is what one run of the command line printed and exited with.
type result struct {
	stdout string
	stderr string
	code   int
}

// newFixture starts a server, built here so the test holds the store a
// verdict is written to. Nothing on the socket gives a verdict.
func newFixture(g *WithT, t *testing.T) *fixture {
	g.THelper()
	paths := statePaths(g, t)
	g.Expect(daemon.Prepare(paths)).To(Succeed())
	st, err := store.Open(t.Context(), paths.DB)
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = st.Close() })
	blobs, err := blob.Open(paths.Blobs)
	g.Expect(err).NotTo(HaveOccurred())
	srv := server.New(server.Config{
		Store:  st,
		Blobs:  blobs,
		Daemon: api.Daemon{PID: 4242, Build: "test", StateDir: paths.Dir, Started: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)},
		Home:   "/nonexistent-home",
	})
	stop := serve(g, t, paths, srv.AgentHandler())
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	return &fixture{paths: paths, repo: repo, store: st, srv: srv, stop: stop}
}

// serve listens on the fixture's socket with handler, returning a function
// that stops it.
func serve(g *WithT, t *testing.T, paths daemon.Paths, handler http.Handler) func() {
	g.THelper()
	listener, err := net.Listen("unix", paths.Socket)
	g.Expect(err).NotTo(HaveOccurred())
	base, cancel := context.WithCancel(context.Background())
	hs := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return base },
	}
	go func() { _ = hs.Serve(listener) }()
	stop := func() {
		cancel()
		_ = hs.Close()
	}
	t.Cleanup(stop)
	return stop
}

// run runs the command line in the repository, as the test build.
func (f *fixture) run(t *testing.T, args ...string) result {
	return f.runWith(t, map[string]string{}, "test", args...)
}

// runWith runs the command line with extra environment and a build.
func (f *fixture) runWith(t *testing.T, extra map[string]string, build string, args ...string) result {
	env := testEnv(f.paths, f.repo)
	env.Build = build
	base := env.Getenv
	env.Getenv = func(name string) string {
		if v, ok := extra[name]; ok {
			return v
		}
		return base(name)
	}
	return runEnv(t, env, args...)
}

// runIn runs the command line in dir against the daemon at paths.
func runIn(t *testing.T, paths daemon.Paths, dir string, args ...string) result {
	return runEnv(t, testEnv(paths, dir), args...)
}

// runEnv runs the command line with env and captures what it printed.
func runEnv(t *testing.T, env Env, args ...string) result {
	var stdout, stderr bytes.Buffer
	env.Stdout, env.Stderr = &stdout, &stderr
	err := Run(t.Context(), append([]string{"eyeball"}, args...), env)
	code := 0
	if err != nil {
		code = ExitCode(err)
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// testEnv is an environment pointing at paths' state directory, with autostart
// off.
func testEnv(paths daemon.Paths, dir string) Env {
	state := filepath.Dir(paths.Dir)
	return Env{
		Dir: dir,
		Getenv: func(name string) string {
			switch name {
			case "XDG_STATE_HOME":
				return state
			case "EYEBALL_NO_START":
				return "1"
			}
			return ""
		},
		Home:  "/nonexistent-home",
		Exe:   "/nonexistent/eyeball",
		Build: "test",
	}
}

// statePaths are the daemon's paths under a short temporary directory, since
// t.TempDir nests the test's name and a long name passes the socket limit.
func statePaths(g *WithT, t *testing.T) daemon.Paths {
	g.THelper()
	dir, err := os.MkdirTemp("", "ebc-")
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	paths, err := daemon.Locate(func(string) string { return dir }, "/nonexistent")
	g.Expect(err).NotTo(HaveOccurred())
	return paths
}

// comment writes a file-level comment on a review's latest round.
func (f *fixture) comment(g *WithT, t *testing.T, reviewID int64, body string) store.Comment {
	g.THelper()
	review, err := f.store.Review(t.Context(), reviewID)
	g.Expect(err).NotTo(HaveOccurred())
	round, err := f.store.Round(t.Context(), reviewID, review.LatestRound)
	g.Expect(err).NotTo(HaveOccurred())
	c, err := f.store.AddComment(t.Context(), store.NewComment{RoundID: round.ID, Level: store.LevelLine, Path: "a.txt", StartLine: 2, EndLine: 2, Quoted: "two", Body: body})
	g.Expect(err).NotTo(HaveOccurred())
	return c
}

// decide gives a verdict on a review's latest round, straight into the store.
// The wait that reads it starts afterward, so nothing has to be woken.
func (f *fixture) decide(g *WithT, t *testing.T, reviewID int64, verdict store.Verdict, note string) {
	g.THelper()
	review, err := f.store.Review(t.Context(), reviewID)
	g.Expect(err).NotTo(HaveOccurred())
	round, err := f.store.Round(t.Context(), reviewID, review.LatestRound)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(f.store.Decide(t.Context(), round.ID, verdict, note)).To(Succeed())
}

// delivered reports whether a comment carries a receipt.
func (f *fixture) delivered(g *WithT, t *testing.T, reviewID, commentID int64) bool {
	g.THelper()
	released, err := f.store.Released(t.Context(), reviewID)
	g.Expect(err).NotTo(HaveOccurred())
	for _, c := range released {
		if c.ID == commentID {
			return !c.Delivered.IsZero()
		}
	}
	return false
}

// rfc3339 matches a timestamp in the layout the human output uses.
var rfc3339 = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?(Z|[+-]\d\d:\d\d)`)

// digest matches the twelve-character digest prefix show prints.
var digest = regexp.MustCompile(`\b[0-9a-f]{12}\b`)

// normalize replaces what changes between runs with placeholders, so a golden
// file holds only what the output says.
func (f *fixture) normalize(s string) string {
	s = strings.ReplaceAll(s, f.repo, "<repo>")
	s = strings.ReplaceAll(s, filepath.Base(f.repo), "<repo-name>")
	s = rfc3339.ReplaceAllString(s, "<time>")
	return digest.ReplaceAllString(s, "<digest>")
}

// write puts content at a path under dir.
func write(g *WithT, dir, path, content string) {
	g.THelper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	g.Expect(os.MkdirAll(filepath.Dir(full), 0o700)).To(Succeed())
	g.Expect(os.WriteFile(full, []byte(content), 0o600)).To(Succeed())
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
	write(g, dir, "a.txt", "one\n")
	run("add", "-A")
	commit := run("commit-tree", run("write-tree"), "-m", "base")
	run("update-ref", "refs/heads/main", commit)
	write(g, dir, "a.txt", "one\ntwo\n")
	return dir
}
