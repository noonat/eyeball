//go:build browser

package server

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/noonat/eyeball/internal/api"
)

// screenReview opens a review of a.txt and a directory of many files, so a tree
// of it is longer than a phone's screen.
func screenReview(g *WithT, t *testing.T, f *fixture) (review, project int64) {
	g.THelper()
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	for i := range 120 {
		write(g, repo, fmt.Sprintf("d/f%03d.txt", i), "one\n")
	}
	review = f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt", "d"}, Note: "first"}).Review.ID
	r, err := f.store.Review(t.Context(), review)
	g.Expect(err).NotTo(HaveOccurred())
	return review, r.ProjectID
}

// A screen opens its own stream for the connection, so a daemon that goes away
// shows the offline notice on a file page, and its return clears it.
func TestServer_Surface_browserFilePageOffline(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	review, _ := screenReview(g, t, f)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	g.Expect(err).NotTo(HaveOccurred())
	addr := ln.Addr().String()
	serve := func(l net.Listener) *http.Server {
		hs := &http.Server{Handler: f.s.Surface(), ReadHeaderTimeout: 5 * time.Second}
		go func() { _ = hs.Serve(l) }()
		return hs
	}
	hs := serve(ln)
	t.Cleanup(func() { _ = hs.Close() })
	b := openBrowser(g, t)
	b.navigate(g, "http://"+addr+"/reviews/"+itoa(review)+"/files/a.txt")
	b.waitFor(g, `!!document.querySelector("#root .crumb .name") && document.getElementById("offline").hidden === true`)

	g.Expect(hs.Close()).To(Succeed())
	b.waitFor(g, `document.getElementById("offline").hidden === false`)
	ln, err = net.Listen("tcp", addr)
	g.Expect(err).NotTo(HaveOccurred())
	hs = serve(ln)
	t.Cleanup(func() { _ = hs.Close() })
	b.waitFor(g, `document.getElementById("offline").hidden === true`)
}

// A link to a screen the app draws moves the router, so no page loads and a
// value on the window survives. A link to a template page is a full load.
func TestServer_Surface_browserLinksMoveTheRouter(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	review, project := screenReview(g, t, f)
	srv := httptest.NewServer(f.s.Surface())
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	base := "/reviews/" + itoa(review)

	b.navigate(g, srv.URL+base+"/files/")
	b.waitFor(g, `document.querySelectorAll("#root .tree .treerow").length > 100`)
	b.eval(g, `window.__before = true`)
	b.eval(g, `window.__loads = performance.getEntriesByType("navigation").length`)

	// A directory, the way back to the root, a file the review leaves alone,
	// and the way back from it. A file the review changes goes back to its
	// round, which is a template page.
	b.eval(g, `[...document.querySelectorAll(".treerow")].find((a) => a.textContent === "d").click()`)
	b.waitFor(g, `location.pathname === "`+base+`/files/d/" && !!document.querySelector(".crumb .name") && document.querySelector(".crumb .name").textContent === "d"`)
	b.eval(g, `document.querySelector(".topbar .back").click()`)
	b.waitFor(g, `location.pathname === "`+base+`/files/" && document.querySelector(".crumb .name").textContent === "harbor"`)
	b.eval(g, `[...document.querySelectorAll(".treerow")].find((a) => a.textContent === "keep.txt").click()`)
	b.waitFor(g, `location.pathname === "`+base+`/files/keep.txt" && !!document.querySelector(".code")`)
	b.eval(g, `document.querySelector(".topbar .back").click()`)
	b.waitFor(g, `location.pathname === "`+base+`/files/" && !!document.querySelector(".tree")`)
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "a page loaded between two screens the app draws")
	g.Expect(b.eval(g, `performance.getEntriesByType("navigation").length`)).To(Equal(b.eval(g, `window.__loads`)))

	// The project's row for an open review leads to a template page, which
	// loads.
	b.navigate(g, srv.URL+"/projects/"+itoa(project))
	b.waitFor(g, `!!document.querySelector("#root a.row")`)
	b.eval(g, `window.__before = true`)
	b.eval(g, `document.querySelector("a.row").click()`)
	b.waitFor(g, `location.pathname === "`+base+`" && !!document.querySelector("#bar")`)
	g.Expect(b.eval(g, `window.__before`)).To(BeNil(), "a link to a template page did not load it")
}

// Each address the app draws answers the shell, and the app draws its screen
// from the view with the view's title. An address with nothing at it is the
// shell too, and the app draws the 404 from the view's.
func TestServer_Surface_browserScreens(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	review, project := screenReview(g, t, f)
	srv := httptest.NewServer(f.s.Surface())
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	base := "/reviews/" + itoa(review)

	b.navigate(g, srv.URL+"/projects/"+itoa(project))
	b.waitFor(g, `!!document.querySelector("#root .brief .title")`)
	g.Expect(b.eval(g, `document.title`)).To(Equal("harbor · Eyeball"))
	g.Expect(b.eval(g, `document.querySelector("#root .brief .title").textContent`)).To(Equal("harbor"))

	b.navigate(g, srv.URL+base+"/files/")
	b.waitFor(g, `document.querySelectorAll("#root .tree .treerow").length > 100`)
	g.Expect(b.eval(g, `document.title`)).To(Equal("Files · Eyeball"))

	b.navigate(g, srv.URL+base+"/files/a.txt")
	b.waitFor(g, `!!document.querySelector("#root .crumb .name")`)
	g.Expect(b.eval(g, `document.querySelector("#root .crumb .name").textContent`)).To(Equal("a.txt"))
	g.Expect(b.eval(g, `document.title`)).To(Equal("a.txt · Eyeball"))
	g.Expect(b.eval(g, `document.querySelectorAll("#root .code tr").length > 0`)).To(BeTrue())

	b.navigate(g, srv.URL+base+"/rounds/1/files/a.txt")
	b.waitFor(g, `!!document.querySelector("#root .crumb .name")`)
	g.Expect(b.eval(g, `document.title`)).To(Equal("a.txt · Eyeball"))

	// The working copy's live file, by its query.
	b.navigate(g, srv.URL+"/projects/"+itoa(project)+"/files/a.txt")
	b.waitFor(g, `!!document.querySelector("#root .crumb .name")`)
	g.Expect(b.eval(g, `document.querySelector("#root .notice").textContent`)).To(ContainSubstring("Live, not from any round."))

	b.navigate(g, srv.URL+"/reviews/9999/files/a.txt")
	b.waitFor(g, `!!document.querySelector("#root .empty .headline")`)
	g.Expect(b.eval(g, `document.querySelector("#root .empty .headline").textContent`)).To(Equal("Nothing here"))
	g.Expect(b.eval(g, `document.title`)).To(Equal("Not found · Eyeball"))
}

// Back from a file returns to the scroll the tree had, which the browser does
// not restore for a move that loads no page.
func TestServer_Surface_browserScrollRestored(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	review, _ := screenReview(g, t, f)
	srv := httptest.NewServer(f.s.Surface())
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	base := "/reviews/" + itoa(review)

	b.navigate(g, srv.URL+base+"/files/d/")
	b.waitFor(g, `document.querySelectorAll(".treerow").length === 120`)
	b.eval(g, `window.scrollTo(0, 1500)`)
	b.waitFor(g, `window.scrollY > 1000`)
	scrolled := b.eval(g, `window.scrollY`)
	b.eval(g, `document.querySelectorAll(".treerow")[60].click()`)
	b.waitFor(g, `!!document.querySelector(".code")`)
	b.eval(g, `history.back()`)
	b.waitFor(g, `document.querySelectorAll(".treerow").length === 120`)
	b.waitFor(g, fmt.Sprintf(`Math.abs(window.scrollY - %v) < 5`, scrolled))

	// Chromium puts a same-document back where it was by itself when the list
	// draws at once from the cache, so the scroll alone does not tell that the
	// router did it. Measured 2026-11-01: the scroll came back with
	// ScrollRestoration removed. iOS does not. ScrollRestoration takes the
	// restoring from the browser, which is what stays true on a device that
	// does not restore.
	g.Expect(b.eval(g, `history.scrollRestoration`)).To(Equal("manual"))
}

// A notification's tap on an address the app draws moves the router, and one on
// a template page loads it.
func TestServer_Surface_browserTapMovesAppToAFile(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	review, _ := screenReview(g, t, f)
	srv := httptest.NewServer(f.s.Surface())
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/")
	b.waitFor(g, `!!document.querySelector("#root .scroll")`)
	b.loadWorkerInPage(g)
	b.eval(g, `window.fake.postMessage = (message, ports) => navigator.serviceWorker.dispatchEvent(new MessageEvent("message", { data: message, ports }))`)
	b.eval(g, `window.__before = true`)

	b.eval(g, `(() => { window.tap("/reviews/`+itoa(review)+`/files/a.txt"); return true; })()`)
	b.waitFor(g, `location.pathname === "/reviews/`+itoa(review)+`/files/a.txt" && !!document.querySelector(".crumb .name")`)
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "the tap loaded a page the app draws")
}
