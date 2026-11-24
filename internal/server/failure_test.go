package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/kind"
	"github.com/noonat/eyeball/internal/view"
	. "github.com/onsi/gomega"
	"golang.org/x/net/html"
)

// serveAs sends one request with the Sec-Fetch-Dest a browser sends for what
// it is loading, and none where dest is empty.
func serveAs(h http.Handler, method, path, dest string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://eyeball"+path, nil)
	if dest != "" {
		req.Header.Set("Sec-Fetch-Dest", dest)
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

// loadPage is a request a browser makes to show a page, so a failure answers it
// with the shell.
func loadPage(h http.Handler, method, path string) *httptest.ResponseRecorder {
	return serveAs(h, method, path, "document")
}

// failureTag is the failure the shell carries, parsed back from the tag as the
// app reads it. It fails the test where the tag is absent.
func failureTag(g *WithT, body string) view.Failure {
	g.THelper()
	metas := findAll(parse(g, body), func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "meta" && attr(n, "name") == "eyeball-error"
	})
	g.Expect(metas).To(HaveLen(1), "the shell carries one eyeball-error tag")
	var f view.Failure
	g.Expect(json.Unmarshal([]byte(attr(metas[0], "content")), &f)).To(Succeed())
	return f
}

func TestServer_failureAnswers(t *testing.T) {
	g := NewWithT(t)
	saved := routes
	routes = append(append([]route{}, routes...), route{
		side:    sideSurface,
		pattern: "GET /test/fails",
		handle: func(*Server, http.ResponseWriter, *http.Request) error {
			return errors.New("git: the base commit is gone")
		},
	})
	t.Cleanup(func() { routes = saved })
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "big.png", strings.Repeat("x", kind.MaxSend+1))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"big.png"}, Note: "first"}).Review.ID
	surface := f.s.Surface()
	loopback := f.s.LoopbackSurface("RUN")

	failures := []struct {
		name    string
		handler http.Handler
		method  string
		path    string
		status  int
		want    view.Failure
	}{
		{
			name:    "an address with no route",
			handler: surface,
			method:  http.MethodGet,
			path:    "/no/such/page",
			status:  http.StatusNotFound,
			want:    failNotFound,
		},
		{
			name:    "a method the address does not answer",
			handler: surface,
			method:  http.MethodPost,
			path:    "/healthz",
			status:  http.StatusMethodNotAllowed,
			want:    failNotAllowed,
		},
		{
			name:    "a raw file past the send limit",
			handler: surface,
			method:  http.MethodGet,
			path:    "/reviews/" + itoa(id) + "/raw/1/big.png",
			status:  http.StatusRequestEntityTooLarge,
			want:    failTooLarge,
		},
		{
			name:    "a handler that fails",
			handler: surface,
			method:  http.MethodGet,
			path:    "/test/fails",
			status:  http.StatusInternalServerError,
			want:    failFailed,
		},
		{
			name:    "a write without the token",
			handler: loopback,
			method:  http.MethodPost,
			path:    "/healthz",
			status:  http.StatusForbidden,
			want:    failNoToken,
		},
	}
	dests := []struct {
		name string
		dest string
	}{
		{name: "an image", dest: "image"},
		{name: "a fetch", dest: "empty"},
		{name: "a frame", dest: "iframe"},
		{name: "no header", dest: ""},
	}
	for _, c := range failures {
		t.Run(c.name+" as a page", func(t *testing.T) {
			g := NewWithT(t)
			res := serveAs(c.handler, c.method, c.path, "document")
			g.Expect(res.Code).To(Equal(c.status))
			g.Expect(res.Header().Get("Content-Type")).To(Equal("text/html; charset=utf-8"))
			g.Expect(res.Header().Get("Content-Security-Policy")).To(Equal(contentSecurityPolicy))
			got := failureTag(g, res.Body.String())
			g.Expect(got.WaitingCount).To(HaveValue(BeEquivalentTo(1)))
			got.WaitingCount = nil
			g.Expect(got).To(Equal(c.want))
			g.Expect(res.Body.String()).To(ContainSubstring(`<div id="root"></div>`))
			g.Expect(res.Body.String()).To(ContainSubstring("<title>" + c.want.Title + " &middot; Eyeball</title>"))
		})
		for _, d := range dests {
			t.Run(c.name+" as "+d.name, func(t *testing.T) {
				g := NewWithT(t)
				res := serveAs(c.handler, c.method, c.path, d.dest)
				g.Expect(res.Code).To(Equal(c.status))
				g.Expect(res.Header().Get("Content-Type")).To(Equal("text/plain; charset=utf-8"))
				g.Expect(res.Body.String()).To(Equal(c.want.Text + "\n"))
			})
		}
	}
}

// The tag is an attribute, so a quote or a bracket in the failure's words
// cannot end it early or open an element.
func TestServer_failureTagEscapes(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	words := view.Failure{Title: `T"><b>`, Heading: "H&amp;", Text: `x'"</script><i>`}
	res := httptest.NewRecorder()
	f.s.fail(res, httptest.NewRequest(http.MethodGet, "http://eyeball/x", nil).WithContext(t.Context()), http.StatusTeapot, words)
	g.Expect(res.Code).To(Equal(http.StatusTeapot))
	g.Expect(res.Body.String()).To(Equal(words.Text + "\n"))

	req := httptest.NewRequest(http.MethodGet, "http://eyeball/x", nil)
	req.Header.Set("Sec-Fetch-Dest", "document")
	res = httptest.NewRecorder()
	f.s.fail(res, req, http.StatusTeapot, words)
	got := failureTag(g, res.Body.String())
	got.WaitingCount = nil
	g.Expect(got).To(Equal(words))
	g.Expect(res.Body.String()).NotTo(ContainSubstring("<b>"))
}

// A failure with no readable count carries a null, so the app draws no eye.
func TestServer_failureWithoutCount(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	g.Expect(f.store.Close()).To(Succeed())
	res := loadPage(f.s.Surface(), http.MethodGet, "/no/such/page")
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
	g.Expect(failureTag(g, res.Body.String()).WaitingCount).To(BeNil())
}
