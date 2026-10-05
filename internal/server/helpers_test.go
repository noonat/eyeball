package server

import (
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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/blob"
	"github.com/noonat/eyeball/internal/store"
	. "github.com/onsi/gomega"
)

// fixtureEnv keeps a fixture repository out of the developer's git
// configuration, so a global hook or template cannot decide what a test sees.
var fixtureEnv = []string{
	"GIT_CONFIG_GLOBAL=/dev/null",
	"GIT_CONFIG_SYSTEM=/dev/null",
	"GIT_AUTHOR_NAME=eyeball test",
	"GIT_AUTHOR_EMAIL=test@example.invalid",
	"GIT_COMMITTER_NAME=eyeball test",
	"GIT_COMMITTER_EMAIL=test@example.invalid",
}

// fixture is a server listening on a real unix socket, with a client for it.
type fixture struct {
	s      *Server
	store  *store.Store
	blobs  *blob.Store
	client *http.Client
	cancel context.CancelFunc
}

// newFixture starts a server on a socket under a short temporary directory.
// t.TempDir nests the test's name in the path, and a long name passes the 107
// bytes a socket path may hold.
func newFixture(g *WithT, t *testing.T) *fixture {
	g.THelper()
	state := t.TempDir()
	st, err := store.Open(t.Context(), filepath.Join(state, "eyeball.db"))
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = st.Close() })
	blobs, err := blob.Open(filepath.Join(state, "blobs"))
	g.Expect(err).NotTo(HaveOccurred())

	s := New(Config{
		Store:  st,
		Blobs:  blobs,
		Daemon: api.Daemon{PID: 42, Build: "test", StateDir: state},
		Home:   "/nonexistent-home",
	})

	sockDir, err := os.MkdirTemp("", "eb-")
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sock := filepath.Join(sockDir, "s")
	listener, err := net.Listen("unix", sock)
	g.Expect(err).NotTo(HaveOccurred())

	base, cancel := context.WithCancel(context.Background())
	srv := &http.Server{
		Handler:           s.AgentHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return base },
	}
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() {
		cancel()
		_ = srv.Close()
	})

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		},
	}}
	return &fixture{s: s, store: st, blobs: blobs, client: client, cancel: cancel}
}

// call sends one request and returns the status and body.
func (f *fixture) call(g *WithT, ctx context.Context, method, path string, body any) (int, []byte) {
	g.THelper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		g.Expect(err).NotTo(HaveOccurred())
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://eyeball"+path, reader)
	g.Expect(err).NotTo(HaveOccurred())
	resp, err := f.client.Do(req)
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	g.Expect(err).NotTo(HaveOccurred())
	return resp.StatusCode, out
}

// ok sends a request that must succeed and decodes its body into out.
func (f *fixture) ok(g *WithT, t *testing.T, method, path string, body, out any) {
	g.THelper()
	status, raw := f.call(g, t.Context(), method, path, body)
	g.Expect(status).To(Equal(http.StatusOK), string(raw))
	g.Expect(json.Unmarshal(raw, out)).To(Succeed())
}

// refused sends a request that must be refused and returns the refusal.
func (f *fixture) refused(g *WithT, t *testing.T, method, path string, body any) (int, api.Error) {
	g.THelper()
	status, raw := f.call(g, t.Context(), method, path, body)
	g.Expect(status).NotTo(Equal(http.StatusOK), string(raw))
	var e api.ErrorBody
	g.Expect(json.Unmarshal(raw, &e)).To(Succeed(), string(raw))
	return status, e.Error
}

// request opens a review or a round and fails the test if it cannot.
func (f *fixture) request(g *WithT, t *testing.T, body api.RequestBody) api.RequestResult {
	g.THelper()
	var out api.RequestResult
	f.ok(g, t, http.MethodPost, "/agent/requests", body, &out)
	return out
}

// decide gives a verdict on a review's latest round through the server's
// wrapper, which publishes exactly as the surface's handler will. Nothing on
// the socket can give a verdict.
func (f *fixture) decide(g *WithT, t *testing.T, reviewID int64, verdict store.Verdict, note string) {
	g.THelper()
	review, err := f.store.Review(t.Context(), reviewID)
	g.Expect(err).NotTo(HaveOccurred())
	round, err := f.store.Round(t.Context(), reviewID, review.LatestRound)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(f.s.db.Decide(t.Context(), round.ID, verdict, note)).To(Succeed())
}

// comment writes a file-level comment on a review's latest round through the
// wrapper.
func (f *fixture) comment(g *WithT, t *testing.T, reviewID int64, body string) store.Comment {
	g.THelper()
	review, err := f.store.Review(t.Context(), reviewID)
	g.Expect(err).NotTo(HaveOccurred())
	round, err := f.store.Round(t.Context(), reviewID, review.LatestRound)
	g.Expect(err).NotTo(HaveOccurred())
	c, err := f.s.db.AddComment(t.Context(), store.NewComment{RoundID: round.ID, Level: store.LevelFile, Path: "a.txt", Body: body})
	g.Expect(err).NotTo(HaveOccurred())
	return c
}

// wait waits on a review as the given caller and returns the result.
func (f *fixture) wait(g *WithT, t *testing.T, reviewID int64, c api.Caller) api.WaitResult {
	g.THelper()
	var out api.WaitResult
	f.ok(g, t, http.MethodGet, waitPath(reviewID, c), nil, &out)
	return out
}

// blockedWait starts a wait in the background and returns once the handler has
// finished its first read. The callers start it where nothing is ready yet, so
// from that point only a change signal can wake it. A test that wrote before the waiter got there
// would pass whether or not the write published anything.
func (f *fixture) blockedWait(g *WithT, t *testing.T, reviewID int64, c api.Caller) <-chan api.WaitResult {
	g.THelper()
	blocked := make(chan struct{})
	var once sync.Once
	hook := func() { once.Do(func() { close(blocked) }) }
	f.s.afterRead.Store(&hook)
	done := make(chan api.WaitResult, 1)
	go func() {
		g := NewWithT(t)
		done <- f.wait(g, t, reviewID, c)
	}()
	g.Eventually(blocked, 5*time.Second).Should(BeClosed())
	return done
}

// waitPath is the wait route for a review and a caller.
func waitPath(reviewID int64, c api.Caller) string {
	return "/agent/reviews/" + strconv.FormatInt(reviewID, 10) + "/wait?" + query(c)
}

// query encodes a caller as query parameters.
func query(c api.Caller) string {
	v := make([]string, 0, 2)
	v = append(v, "dir="+url.QueryEscape(c.Dir))
	if c.Home != "" {
		v = append(v, "home="+url.QueryEscape(c.Home))
	}
	if c.Agent != "" {
		v = append(v, "agent="+url.QueryEscape(c.Agent))
	}
	return strings.Join(v, "&")
}

// gitRepo builds a repository with one committed file and one edit to it, the
// smallest working copy with something to review.
func gitRepo(g *WithT, dir string) string {
	g.THelper()
	g.Expect(os.MkdirAll(dir, 0o700)).To(Succeed())
	git(g, dir, "init", "--quiet", "--template=", "-b", "main")
	write(g, dir, "a.txt", "one\n")
	write(g, dir, "keep.txt", "unchanged\n")
	git(g, dir, "add", "-A")
	tree := git(g, dir, "write-tree")
	commit := git(g, dir, "commit-tree", tree, "-m", "base")
	git(g, dir, "update-ref", "refs/heads/main", commit)
	write(g, dir, "a.txt", "one\ntwo\n")
	return dir
}

// write puts content at a path under dir.
func write(g *WithT, dir, path, content string) {
	g.THelper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	g.Expect(os.MkdirAll(filepath.Dir(full), 0o700)).To(Succeed())
	g.Expect(os.WriteFile(full, []byte(content), 0o600)).To(Succeed())
}

// git runs one git command in dir and returns its trimmed output.
func git(g *WithT, dir string, args ...string) string {
	g.THelper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), fixtureEnv...)
	out, err := cmd.CombinedOutput()
	g.Expect(err).NotTo(HaveOccurred(), string(out))
	return strings.TrimSpace(string(out))
}
