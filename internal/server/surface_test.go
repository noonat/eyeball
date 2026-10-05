package server

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/noonat/eyeball"
	. "github.com/onsi/gomega"
	"golang.org/x/net/html"
)

func Test_policyAllowsEveryURLInTheStylesheet(t *testing.T) {
	g := NewWithT(t)
	// Every url( in app.css has a scheme the policy's img-src allows. A
	// policy without data: hides every icon, which looks like a spacing bug.
	urls := regexp.MustCompile(`url\(\s*["']?([a-z]+):`).FindAllStringSubmatch(string(eyeball.Stylesheet), -1)
	g.Expect(urls).NotTo(BeEmpty(), "the stylesheet's masks were found")
	imgSrc := ""
	for _, directive := range strings.Split(contentSecurityPolicy, ";") {
		fields := strings.Fields(directive)
		if len(fields) > 0 && fields[0] == "img-src" {
			imgSrc = directive
		}
	}
	g.Expect(imgSrc).NotTo(BeEmpty())
	for _, m := range urls {
		g.Expect(imgSrc).To(ContainSubstring(m[1]+":"), "img-src does not allow "+m[1]+":")
	}
}

func TestServer_Surface(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	surface := f.s.Surface()

	res := serveSurface(surface, http.MethodGet, "/healthz")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Body.String()).To(Equal("ok\n"))
	g.Expect(res.Header().Get("Content-Security-Policy")).To(Equal(contentSecurityPolicy))
	g.Expect(res.Header().Get("Cache-Control")).To(Equal("no-store"))

	res = serveSurface(surface, http.MethodGet, "/static/app.css")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Header().Get("Content-Type")).To(HavePrefix("text/css"))
	g.Expect(res.Body.Bytes()).To(Equal(eyeball.Stylesheet))

	res = serveSurface(surface, http.MethodGet, "/static/shell.css")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Header().Get("Content-Type")).To(HavePrefix("text/css"))
	g.Expect(res.Body.Bytes()).To(Equal(eyeball.Shell))
}

func TestServer_Surface_errorPages(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	surface := f.s.Surface()

	// The mux's own fallback answers in plain text. The surface answers with
	// a page, and keeps the headers every response carries.
	res := serveSurface(surface, http.MethodGet, "/no/such/page")
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
	g.Expect(res.Header().Get("Content-Type")).To(HavePrefix("text/html"))
	g.Expect(res.Header().Get("Content-Security-Policy")).To(Equal(contentSecurityPolicy))
	g.Expect(pageText(g, res.Body.String())).To(ContainSubstring("There is no page at this address."))
	// The layout carries the topbar, the light theme and the stylesheet.
	g.Expect(res.Body.String()).To(ContainSubstring(`<html lang="en" data-theme="light">`))
	g.Expect(res.Body.String()).To(ContainSubstring(`<link rel="stylesheet" href="/static/app.css">`))
	g.Expect(res.Body.String()).To(ContainSubstring(`<link rel="stylesheet" href="/static/shell.css">`))
	g.Expect(res.Body.String()).To(ContainSubstring(`<div class="topbar"><a class="back" href="/">`))
	g.Expect(res.Body.String()).To(ContainSubstring(`<h1>Not found</h1>`))

	res = serveSurface(surface, http.MethodPost, "/healthz")
	g.Expect(res.Code).To(Equal(http.StatusMethodNotAllowed))
	g.Expect(res.Header().Get("Allow")).To(ContainSubstring(http.MethodGet), "a 405 keeps its Allow header")
	g.Expect(pageText(g, res.Body.String())).To(ContainSubstring("does not answer that kind of request"))

	// The agent API is not on the surface, so its routes are the surface's
	// 404 page rather than anything an agent route would answer.
	res = serveSurface(surface, http.MethodGet, "/agent/daemon")
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
}

func TestServer_Surface_healthzTouchesTheStore(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	g.Expect(f.store.Close()).To(Succeed())
	res := serveSurface(f.s.Surface(), http.MethodGet, "/healthz")
	g.Expect(res.Code).To(Equal(http.StatusInternalServerError))
}

func TestServer_Surface_keepsAHandlersOwn404(t *testing.T) {
	g := NewWithT(t)
	// No surface route answers 404 of its own yet, so one is added for the
	// test. The table is restored afterward.
	saved := routes
	routes = append(append([]route{}, routes...), route{
		side:    sideSurface,
		pattern: "GET /test/own-404",
		handle: func(_ *Server, w http.ResponseWriter, _ *http.Request) error {
			w.WriteHeader(http.StatusNotFound)
			_, err := w.Write([]byte("the handler's own page"))
			return err
		},
	})
	t.Cleanup(func() { routes = saved })
	f := newFixture(g, t)

	res := serveSurface(f.s.Surface(), http.MethodGet, "/test/own-404")
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
	g.Expect(res.Body.String()).To(Equal("the handler's own page"))
	g.Expect(res.Header().Get("Content-Security-Policy")).To(Equal(contentSecurityPolicy))
}

// serveSurface sends one request to a surface handler.
func serveSurface(h http.Handler, method, path string) *httptest.ResponseRecorder {
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(method, "http://eyeball"+path, nil))
	return res
}

// pageText is the text a parsed page shows, so an assertion reads what a person
// reads rather than matching markup.
func pageText(g *WithT, body string) string {
	g.THelper()
	doc, err := html.Parse(strings.NewReader(body))
	g.Expect(err).NotTo(HaveOccurred())
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return strings.Join(strings.Fields(b.String()), " ")
}
