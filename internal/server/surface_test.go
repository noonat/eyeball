package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/noonat/eyeball"
	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/store"
	"github.com/noonat/eyeball/internal/view"
	. "github.com/onsi/gomega"
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

// On loopback a write needs the run's token; on the tailnet the header means
// nothing.
func TestServer_LoopbackSurface(t *testing.T) {
	cases := []struct {
		name    string
		token   string
		status  int
		text    string
		tailnet int
	}{
		{
			name:    "no header",
			status:  http.StatusForbidden,
			text:    noToken,
			tailnet: http.StatusMethodNotAllowed,
		},
		{
			name:    "a wrong token",
			token:   "WRONG",
			status:  http.StatusForbidden,
			text:    noToken,
			tailnet: http.StatusMethodNotAllowed,
		},
		{
			name:    "an earlier run's token",
			token:   "EARLIER",
			status:  http.StatusForbidden,
			text:    noToken,
			tailnet: http.StatusMethodNotAllowed,
		},
		{
			name:    "the run's token",
			token:   "RUN",
			status:  http.StatusMethodNotAllowed,
			tailnet: http.StatusMethodNotAllowed,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			f := newFixture(g, t)
			req := func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "http://eyeball/healthz", nil)
				if c.token != "" {
					r.Header.Set("X-Eyeball-Token", c.token)
				}
				return r
			}
			res := httptest.NewRecorder()
			f.s.LoopbackSurface("RUN").ServeHTTP(res, req())
			g.Expect(res.Code).To(Equal(c.status))
			g.Expect(res.Header().Get("Content-Security-Policy")).To(Equal(contentSecurityPolicy))
			if c.text != "" {
				g.Expect(res.Header().Get("Content-Type")).To(HavePrefix("text/plain"))
				g.Expect(res.Body.String()).To(Equal(c.text + "\n"))
			}
			res = httptest.NewRecorder()
			f.s.Surface().ServeHTTP(res, req())
			g.Expect(res.Code).To(Equal(c.tailnet))
		})
	}

	g := NewWithT(t)
	f := newFixture(g, t)
	res := httptest.NewRecorder()
	f.s.LoopbackSurface("RUN").ServeHTTP(res, httptest.NewRequest(http.MethodGet, "http://eyeball/healthz", nil))
	g.Expect(res.Code).To(Equal(http.StatusOK), "a read needs no token")
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
	g.Expect(res.Header().Get("Referrer-Policy")).To(Equal("no-referrer"))

	res = serveSurface(surface, http.MethodGet, "/static/app.css")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Header().Get("Content-Type")).To(HavePrefix("text/css"))
	g.Expect(res.Body.Bytes()).To(Equal(eyeball.Stylesheet))

	res = serveSurface(surface, http.MethodGet, "/static/shell.css")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Header().Get("Content-Type")).To(HavePrefix("text/css"))
	g.Expect(res.Body.Bytes()).To(Equal(eyeball.Shell))

	app, err := eyeball.App()
	g.Expect(err).NotTo(HaveOccurred())
	res = serveSurface(surface, http.MethodGet, "/static/app.js")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Header().Get("Content-Type")).To(HavePrefix("text/javascript"))
	g.Expect(res.Body.Bytes()).To(Equal(app.Script))

	// The bundle moved from react.js, and no alias is kept: a worker that
	// still holds the old address holds it by that exact address.
	res = serveSurface(surface, http.MethodGet, "/static/react.js")
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
}

func TestServer_Surface_concurrentPages(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	surface := f.s.Surface()
	// Pages rendered at once share the parsed templates. Run under -race,
	// this is what caught a page writing to the shared trees.
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 5 {
				_ = serveSurface(surface, http.MethodGet, "/")
				_ = serveSurface(surface, http.MethodGet, "/no/such/page")
			}
		})
	}
	wg.Wait()
	g.Expect(serveSurface(surface, http.MethodGet, "/").Code).To(Equal(http.StatusOK))
}

// Every case of the cross-origin check the spec's probe ran. POST /healthz is a
// 405 once past the check, which tells a request let through from one the check
// refused with 403.
func TestServer_Surface_crossOrigin(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		headers map[string]string
		status  int
	}{
		{
			name:    "same origin",
			method:  http.MethodPost,
			headers: map[string]string{"Sec-Fetch-Site": "same-origin"},
			status:  http.StatusMethodNotAllowed,
		},
		{
			name:    "typed by the user",
			method:  http.MethodPost,
			headers: map[string]string{"Sec-Fetch-Site": "none"},
			status:  http.StatusMethodNotAllowed,
		},
		{
			name:   "no headers, as curl sends",
			method: http.MethodPost,
			status: http.StatusMethodNotAllowed,
		},
		{
			name:    "an origin matching the host",
			method:  http.MethodPost,
			headers: map[string]string{"Origin": "http://eyeball"},
			status:  http.StatusMethodNotAllowed,
		},
		{
			name:    "same site, another port",
			method:  http.MethodPost,
			headers: map[string]string{"Sec-Fetch-Site": "same-site"},
			status:  http.StatusForbidden,
		},
		{
			name:    "cross site",
			method:  http.MethodPost,
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
			status:  http.StatusForbidden,
		},
		{
			name:    "a sandboxed frame",
			method:  http.MethodPost,
			headers: map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "null"},
			status:  http.StatusForbidden,
		},
		{
			name:    "an origin not matching the host",
			method:  http.MethodPost,
			headers: map[string]string{"Origin": "http://other:8112"},
			status:  http.StatusForbidden,
		},
		{
			name:    "a cross-site delete",
			method:  http.MethodDelete,
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
			status:  http.StatusForbidden,
		},
		{
			name:    "a cross-site read",
			method:  http.MethodGet,
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
			status:  http.StatusOK,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			f := newFixture(g, t)
			handlers := []http.Handler{f.s.Surface(), f.s.LoopbackSurface("T")}
			for _, h := range handlers {
				req := httptest.NewRequest(c.method, "http://eyeball/healthz", nil)
				req.Header.Set("X-Eyeball-Token", "T")
				for k, v := range c.headers {
					req.Header.Set(k, v)
				}
				res := httptest.NewRecorder()
				h.ServeHTTP(res, req)
				g.Expect(res.Code).To(Equal(c.status))
			}
		})
	}
}

func TestServer_Surface_errorPages(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	surface := f.s.Surface()

	// The mux's own fallback answers in plain text. The surface answers a page
	// load with the shell, and keeps the headers every response carries.
	res := loadPage(surface, http.MethodGet, "/no/such/page")
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
	g.Expect(res.Header().Get("Content-Type")).To(HavePrefix("text/html"))
	g.Expect(res.Header().Get("Content-Security-Policy")).To(Equal(contentSecurityPolicy))
	g.Expect(res.Header().Get("Referrer-Policy")).To(Equal("no-referrer"))
	g.Expect(failureTag(g, res.Body.String()).Text).To(Equal("There is no page at this address."))
	// The shell carries the theme, the stylesheets and the bundle the failure
	// is drawn by.
	g.Expect(res.Body.String()).To(ContainSubstring(`<html lang="en" data-theme="light">`))
	g.Expect(res.Body.String()).To(ContainSubstring(`<link rel="stylesheet" href="/static/app.css?b=` + f.s.buildToken() + `">`))
	g.Expect(res.Body.String()).To(ContainSubstring(`<link rel="stylesheet" href="/static/shell.css?b=` + f.s.buildToken() + `">`))
	g.Expect(res.Body.String()).To(ContainSubstring(`<div id="root"></div>`))

	res = loadPage(surface, http.MethodPost, "/healthz")
	g.Expect(res.Code).To(Equal(http.StatusMethodNotAllowed))
	g.Expect(res.Header().Get("Allow")).To(ContainSubstring(http.MethodGet), "a 405 keeps its Allow header")
	g.Expect(failureTag(g, res.Body.String()).Text).To(ContainSubstring("does not answer that kind of request"))

	// The agent API is not on the surface, so its routes are the surface's
	// 404 page rather than anything an agent route would answer.
	res = serveSurface(surface, http.MethodGet, "/agent/daemon")
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
}

// The top bar's eye counts what waits on every page, not only the queue, and
// draws nothing where the count cannot be read, because a shut eye would say
// that nothing waits.
func TestServer_Surface_eye(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "look"}).Review.ID

	// The review's view carries the count its top bar's eye draws.
	round := f.roundAt(g, "/reviews/"+itoa(id))
	g.Expect(round.WaitingCount).To(HaveValue(BeEquivalentTo(1)))
	res := loadPage(f.s.Surface(), http.MethodGet, "/nope")
	g.Expect(failureTag(g, res.Body.String()).WaitingCount).To(HaveValue(BeEquivalentTo(1)))

	g.Expect(f.store.Close()).To(Succeed())
	res = getView(f, "/api/reviews/abc", nil)
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
	var missing view.NotFound
	g.Expect(json.Unmarshal(res.Body.Bytes(), &missing)).To(Succeed())
	g.Expect(missing.WaitingCount).To(BeNil(), "no count, so no eye")
	res = loadPage(f.s.Surface(), http.MethodGet, "/nope")
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
	g.Expect(failureTag(g, res.Body.String()).WaitingCount).To(BeNil(), "no count, so no eye")
}

func TestServer_Surface_failureIsAPage(t *testing.T) {
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

	res := loadPage(f.s.Surface(), http.MethodGet, "/test/fails")
	g.Expect(res.Code).To(Equal(http.StatusInternalServerError))
	g.Expect(res.Header().Get("Content-Type")).To(HavePrefix("text/html"))
	g.Expect(failureTag(g, res.Body.String()).Heading).To(Equal("This page could not be read"))
	g.Expect(res.Body.String()).NotTo(ContainSubstring("base commit is gone"), "the error itself goes to the log")
}

// Every font app.css names is served from where its relative path resolves
// under /static/app.css, with a cache the surface's own no-store would
// otherwise forbid.
func TestServer_Surface_fonts(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	surface := f.s.Surface()

	urls := regexp.MustCompile(`url\("(fonts/[^"]+)"\)`).FindAllStringSubmatch(string(eyeball.Stylesheet), -1)
	g.Expect(urls).To(HaveLen(6), "app.css names six font files")
	for _, m := range urls {
		target := "/static/" + m[1]
		res := serveSurface(surface, http.MethodGet, target)
		g.Expect(res.Code).To(Equal(http.StatusOK), target)
		g.Expect(res.Header().Get("Content-Type")).To(Equal("font/woff2"))
		g.Expect(res.Header().Get("Cache-Control")).To(Equal("no-cache"))
		tag := res.Header().Get("ETag")
		g.Expect(tag).To(MatchRegexp(`^"[0-9a-f]{32}"$`), "a strong ETag")
		g.Expect(res.Body.Bytes()[:4]).To(Equal([]byte("wOF2")))

		again := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://eyeball"+target, nil)
		req.Header.Set("If-None-Match", tag)
		surface.ServeHTTP(again, req)
		g.Expect(again.Code).To(Equal(http.StatusNotModified), target)
		g.Expect(again.Body.Len()).To(BeZero())
	}

	missing := []string{
		"/static/fonts/literata/OFL.txt",
		"/static/fonts/literata/nope.woff2",
		"/static/fonts/literata",
		// The mux cleans a literal .. with a redirect before any handler runs,
		// so the escaped form is the one that reaches this route.
		"/static/fonts/..%2fapp.css",
	}
	for _, target := range missing {
		g.Expect(serveSurface(surface, http.MethodGet, target).Code).To(Equal(http.StatusNotFound), target)
	}
}

func TestServer_Surface_healthzTouchesTheStore(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	g.Expect(f.store.Close()).To(Succeed())
	res := serveSurface(f.s.Surface(), http.MethodGet, "/healthz")
	g.Expect(res.Code).To(Equal(http.StatusInternalServerError))
}

// The page is installable: the layout links a manifest naming icons that are
// served, and the service worker is served at the root, so its scope is the
// whole surface.
func TestServer_Surface_installable(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	surface := f.s.Surface()
	doc := f.page(g, "/", http.StatusOK)
	links := map[string]string{}
	for _, l := range findAll(doc, isElement("link")) {
		links[attr(l, "rel")] = attr(l, "href")
	}
	g.Expect(links).To(HaveKeyWithValue("manifest", "/manifest.webmanifest"))
	g.Expect(links).To(HaveKeyWithValue("apple-touch-icon", "/static/icons/icon-180.png"))

	res := serveSurface(surface, http.MethodGet, "/manifest.webmanifest")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Header().Get("Content-Type")).To(Equal("application/manifest+json"))
	var m struct {
		Name    string `json:"name"`
		Start   string `json:"start_url"`
		Display string `json:"display"`
		Icons   []struct {
			Src   string `json:"src"`
			Sizes string `json:"sizes"`
		} `json:"icons"`
	}
	g.Expect(json.Unmarshal(res.Body.Bytes(), &m)).To(Succeed())
	g.Expect([]string{m.Name, m.Start, m.Display}).To(Equal([]string{"Eyeball", "/", "standalone"}))
	g.Expect(m.Icons).To(HaveLen(2))
	icons := []string{m.Icons[0].Src, m.Icons[1].Src, links["apple-touch-icon"]}
	for _, src := range icons {
		res := serveSurface(surface, http.MethodGet, src)
		g.Expect(res.Code).To(Equal(http.StatusOK), src)
		g.Expect(res.Header().Get("Content-Type")).To(Equal("image/png"))
		g.Expect(res.Body.Bytes()[:4]).To(Equal([]byte("\x89PNG")))
	}
	g.Expect(serveSurface(surface, http.MethodGet, "/static/icons/nope.png").Code).To(Equal(http.StatusNotFound))

	res = serveSurface(surface, http.MethodGet, "/sw.js")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Header().Get("Content-Type")).To(HavePrefix("text/javascript"))
	g.Expect(res.Body.String()).To(ContainSubstring("notificationclick"))
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

// The template page's writes and fragments are gone: each address answers 404,
// and a write sent to one stores nothing.
func TestServer_Surface_removedTemplateRoutes(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	c := f.addComment(g, t, store.NewComment{RoundID: f.latestRound(g, t, id), Level: store.LevelReview, Body: "Stays."})
	review := "/reviews/" + itoa(id)
	gone := []struct{ method, path string }{
		{method: http.MethodPost, path: review + "/comments"},
		{method: http.MethodPut, path: review + "/comments/" + itoa(c.ID)},
		{method: http.MethodDelete, path: review + "/comments/" + itoa(c.ID)},
		{method: http.MethodGet, path: review + "/comments/" + itoa(c.ID)},
		{method: http.MethodGet, path: review + "/composer?round=1&level=review"},
		{method: http.MethodPost, path: review + "/decision"},
		{method: http.MethodGet, path: review + "/rounds/1/cards/a.txt"},
	}
	for _, r := range gone {
		g.Expect(serveSurface(f.s.Surface(), r.method, r.path).Code).To(Equal(http.StatusNotFound), r.method+" "+r.path)
	}
	comments, err := f.store.Comments(t.Context(), f.latestRound(g, t, id))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(comments).To(HaveLen(1))
	g.Expect(comments[0].Body).To(Equal("Stays."))
	_, err = f.store.Decision(t.Context(), f.latestRound(g, t, id))
	g.Expect(err).To(MatchError(store.ErrNotFound))
}

// serveSurface sends one request to a surface handler.
func serveSurface(h http.Handler, method, path string) *httptest.ResponseRecorder {
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(method, "http://eyeball"+path, nil))
	return res
}
