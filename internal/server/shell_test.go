package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/noonat/eyeball"
	"github.com/noonat/eyeball/internal/store"
	"github.com/noonat/eyeball/internal/view"
	. "github.com/onsi/gomega"
)

// The worker's list has to hold every address the shell's head links and every
// file app.css names. A file missing from it is not cached, and the installed
// app then draws that file from the daemon, which is the blank launch the
// worker exists to prevent. Nothing but the phone shows it otherwise.
func Test_shellFiles(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	token := f.s.buildToken()
	files := shellFiles(token)
	doc := f.page(g, "/", http.StatusOK)
	var linked []string
	for _, l := range findAll(doc, isElement("link")) {
		linked = append(linked, attr(l, "href"))
	}
	for _, s := range findAll(doc, isElement("script")) {
		linked = append(linked, attr(s, "src"))
	}
	g.Expect(linked).To(ContainElement("/static/react.js?b=" + token))
	for _, address := range linked {
		g.Expect(files).To(ContainElement(address), "the head links it")
	}

	named := regexp.MustCompile(`url\("([^"]+)"\)`).FindAllStringSubmatch(string(eyeball.Stylesheet), -1)
	fonts := 0
	for _, m := range named {
		if strings.HasPrefix(m[1], "data:") {
			continue
		}
		fonts++
		g.Expect(files).To(ContainElement(path.Join("/static", m[1])), "app.css names it")
	}
	g.Expect(fonts).To(Equal(6))

	manifest := serveSurface(f.s.Surface(), http.MethodGet, "/manifest.webmanifest")
	icons := regexp.MustCompile(`"src": "([^"]+)"`).FindAllStringSubmatch(manifest.Body.String(), -1)
	g.Expect(icons).To(HaveLen(2))
	for _, m := range icons {
		g.Expect(files).To(ContainElement(m[1]), "the manifest names it")
	}

	worker := serveSurface(f.s.Surface(), http.MethodGet, "/sw.js").Body.String()
	for _, file := range files {
		g.Expect(worker).To(ContainSubstring(`"`+file+`"`), "sw.js lists it")
	}
	g.Expect(worker).To(HavePrefix(`var shellBuild="` + token + `",shellFiles=[`))
}

// The served worker is the compiled script with the build written ahead of it,
// so the stamp the taps compare still hashes the script alone.
func TestServer_workerStamp(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	base, err := eyeball.Worker()
	g.Expect(err).NotTo(HaveOccurred())
	stamp, err := eyeball.WorkerStamp()
	g.Expect(err).NotTo(HaveOccurred())
	first := serveSurface(f.s.Surface(), http.MethodGet, "/sw.js").Body.String()
	f.s.daemon.Build = "another"
	second := serveSurface(f.s.Surface(), http.MethodGet, "/sw.js").Body.String()
	g.Expect(first).To(HaveSuffix(string(base)))
	g.Expect(second).To(HaveSuffix(string(base)))
	g.Expect(second).NotTo(Equal(first), "a new build is a new worker to the browser")
	g.Expect(first).To(ContainSubstring(stamp))
}

// A write under /api without the loopback token is refused with a Refusal the
// app can show, and any other address with the page it always got.
func TestServer_LoopbackSurface_apiRefusal(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	h := f.s.LoopbackSurface("a-token")
	res := sendJSON(h, http.MethodPost, commentsAPI(id), view.CommentRequest{Level: view.CommentLevelReview, Round: 1, Body: "x"})
	g.Expect(res.Code).To(Equal(http.StatusForbidden))
	g.Expect(readRefusal(g, res).Message).To(Equal(noToken))
	comments, err := f.store.Comments(t.Context(), f.latestRound(g, t, id))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(comments).To(BeEmpty())

	req := httptest.NewRequest(http.MethodPost, "http://eyeball/api/reviews/"+itoa(id)+"/comments", strings.NewReader(`{"round":1,"level":"review","body":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(tokenHeader, "a-token")
	stored := httptest.NewRecorder()
	h.ServeHTTP(stored, req)
	g.Expect(stored.Code).To(Equal(http.StatusOK), stored.Body.String())

	page := sendForm(h, http.MethodPost, "/reviews/"+itoa(id)+"/comments", url.Values{"round": {"1"}}, false)
	g.Expect(page.Code).To(Equal(http.StatusForbidden))
	g.Expect(page.Header().Get("Content-Type")).To(HavePrefix("text/html"))
}

// A request made for another build is refused on both listeners, and a request
// for this build or for none is served.
func TestServer_Surface_otherBuild(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	listeners := map[string]http.Handler{
		"tailnet":  f.s.Surface(),
		"loopback": f.s.LoopbackSurface("a-token"),
	}
	for name, h := range listeners {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			ask := func(build string) int {
				req := httptest.NewRequest(http.MethodGet, "http://eyeball/static/app.css", nil)
				if build != "" {
					req.Header.Set(buildHeader, build)
				}
				res := httptest.NewRecorder()
				h.ServeHTTP(res, req)
				return res.Code
			}
			g.Expect(ask("0123abcd")).To(Equal(http.StatusConflict))
			g.Expect(ask(f.s.buildToken())).To(Equal(http.StatusOK))
			g.Expect(ask("")).To(Equal(http.StatusOK))
		})
	}
}

// The option answers a review's round with the shell and leaves everything
// else as it was. Without it the template page answers.
func TestServer_Surface_reviewInApp(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	addresses := []string{"/reviews/" + itoa(id), "/reviews/" + itoa(id) + "/rounds/1"}
	for _, address := range addresses {
		page := serveSurface(f.s.Surface(), http.MethodGet, address).Body.String()
		g.Expect(page).To(ContainSubstring(` data-stamp="`), address+" is the template page")
		g.Expect(page).NotTo(ContainSubstring(`<div id="root"></div>`), address)
	}
	f.s.reviewInApp = true
	for _, address := range addresses {
		page := serveSurface(f.s.Surface(), http.MethodGet, address).Body.String()
		g.Expect(page).To(ContainSubstring(`<div id="root"></div>`), address+" is the shell")
		g.Expect(page).NotTo(ContainSubstring(` data-stamp="`), address)
	}
	// A missing review is the shell too, since the app draws the 404 from the
	// view, and the view is unchanged by the option.
	g.Expect(serveSurface(f.s.Surface(), http.MethodGet, "/reviews/999").Body.String()).To(ContainSubstring(`<div id="root"></div>`))
	g.Expect(getView(f, "/api/reviews/"+itoa(id), nil).Code).To(Equal(http.StatusOK))
}

// The view names the /api routes for a comment's Edit and Delete and for the
// verdict, so the app sends each where the JSON routes are.
func TestServer_Surface_viewAddresses(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	c := f.addComment(g, t, store.NewComment{RoundID: f.latestRound(g, t, id), Level: store.LevelReview, Body: "A point."})
	round := f.roundAt(g, "/reviews/"+itoa(id))
	g.Expect(round.Acting.Comments).To(HaveLen(1))
	g.Expect(round.Acting.Comments[0].Href).To(Equal("/api/reviews/" + itoa(id) + "/comments/" + itoa(c.ID)))
	g.Expect(round.Acting.Bar.Action).To(Equal("/api/reviews/" + itoa(id) + "/decision"))
}
