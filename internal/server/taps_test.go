package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/noonat/eyeball"
)

// logBuffer collects what a server logs, for a test to read.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends to the buffer under its lock, since requests log from their
// own goroutines.
func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// String is everything logged so far.
func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// logInto points the server's log at a buffer.
func (f *fixture) logInto() *logBuffer {
	out := &logBuffer{}
	f.s.log = slog.New(slog.NewTextHandler(out, nil))
	return out
}

// postTap sends one beacon to a surface handler.
func postTap(h http.Handler, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "http://eyeball/taps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Eyeball-Token", token)
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

// A beacon is written to the log as one info line, beside the stamp of the
// worker the daemon serves, and answered 204.
func TestServer_handleTap(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	out := f.logInto()
	stamp, err := eyeball.WorkerStamp()
	g.Expect(err).NotTo(HaveOccurred())

	res := postTap(f.s.Surface(), "{\n  \"step\": \"start\",\n  \"stamp\": \"abc\"\n}\n", "")
	g.Expect(res.Code).To(Equal(http.StatusNoContent))
	g.Expect(res.Body.String()).To(BeEmpty())
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	g.Expect(lines).To(HaveLen(1), "one line per beacon, whatever the body's own lines")
	g.Expect(lines[0]).To(ContainSubstring("level=INFO"))
	g.Expect(lines[0]).To(ContainSubstring("msg=tap"))
	g.Expect(lines[0]).To(ContainSubstring(`beacon="{\"step\":\"start\",\"stamp\":\"abc\"}"`))
	g.Expect(lines[0]).To(HaveSuffix("served=" + stamp))
}

// A beacon is a write, and on loopback a write needs the token, which a
// worker has no way to read. Nothing is logged without it.
func TestServer_tapLoopback(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	out := f.logInto()
	g.Expect(postTap(f.s.LoopbackSurface("RUN"), `{}`, "").Code).To(Equal(http.StatusForbidden))
	g.Expect(out.String()).To(BeEmpty())
	g.Expect(postTap(f.s.LoopbackSurface("RUN"), `{}`, "RUN").Code).To(Equal(http.StatusNoContent))
	g.Expect(out.String()).To(ContainSubstring("msg=tap"))
}

// A body that is too big or is not JSON is refused with 400 and logs nothing,
// and one at the limit is taken.
func TestServer_tapRefusals(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	out := f.logInto()
	pad := func(n int) string {
		return `{"p":"` + strings.Repeat("x", n-len(`{"p":""}`)) + `"}`
	}

	cases := []struct {
		name string
		body string
		want int
	}{
		{name: "not json", body: "tapped", want: http.StatusBadRequest},
		{name: "empty", body: "", want: http.StatusBadRequest},
		{name: "cut off", body: `{"step":`, want: http.StatusBadRequest},
		{name: "two values", body: `{} {}`, want: http.StatusBadRequest},
		{name: "one byte over", body: pad(8193), want: http.StatusBadRequest},
		{name: "at the limit", body: pad(8192), want: http.StatusNoContent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			before := out.String()
			g.Expect(postTap(f.s.Surface(), c.body, "").Code).To(Equal(c.want))
			logged := strings.TrimPrefix(out.String(), before) != ""
			g.Expect(logged).To(Equal(c.want == http.StatusNoContent))
		})
	}
}
