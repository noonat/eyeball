//go:build browser

package server

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/store"
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
// value on the window survives. A review's round is one of them, whatever the
// server answers a page load with.
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
	// and the way back from it.
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

	// The project's row for an open review leads to the review's round, which
	// the app draws with no page load.
	b.navigate(g, srv.URL+"/projects/"+itoa(project))
	b.waitFor(g, `!!document.querySelector("#root a.row")`)
	b.eval(g, `window.__before = true`)
	b.eval(g, `document.querySelector("a.row").click()`)
	b.waitFor(g, `location.pathname === "`+base+`" && !!document.querySelector("#root .page .brief") && !!document.querySelector("#bar")`)
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "a link to a review loaded a page")
}

// A review's round is drawn by the app from its view, with the view's title.
// The comparison's links and the way back move the router, so no page loads. A
// tap far down a long file brings the composer into view, and the composer
// moves to the next tap with what was typed in it.
func TestServer_Surface_browserReviewScreen(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	f.s.reviewInApp = true
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "long.txt", strings.Repeat("a line of the file\n", 150))
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt", "long.txt"}, Note: "The long file"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "more")
	write(g, repo, "a.txt", "one\ntwo\nthree\n")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "Second."})
	srv := httptest.NewServer(f.s.Surface())
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	base := "/reviews/" + itoa(id)
	title := f.roundAt(g, base).Title

	b.navigate(g, srv.URL+base)
	b.waitFor(g, `!!document.querySelector("#root .page .brief")`)
	g.Expect(b.eval(g, `document.title`)).To(Equal(title + " · Eyeball"))
	g.Expect(b.eval(g, `document.querySelector("#root .topbar h1").textContent`)).To(Equal(title))

	// The comparison's links keep their address as the page wrote it, and a
	// click moves the router to it.
	b.eval(g, `window.__before = true`)
	g.Expect(b.eval(g, `[...document.querySelectorAll(".segmented a")].map((a) => a.getAttribute("href")).join()`)).To(Equal("?against=previous,?against=base"))
	b.eval(g, `document.querySelectorAll(".segmented a")[1].click()`)
	b.waitFor(g, `location.search === "?against=base" && document.querySelectorAll(".segmented a")[1]?.getAttribute("aria-current") === "true"`)
	g.Expect(b.eval(g, `document.querySelector(".toolbar .counts").textContent`)).To(ContainSubstring("2 files"))
	b.eval(g, `document.querySelectorAll(".segmented a")[0].click()`)
	b.waitFor(g, `location.search === "?against=previous" && document.querySelectorAll(".segmented a")[0]?.getAttribute("aria-current") === "true"`)
	g.Expect(b.eval(g, `document.querySelector(".toolbar .counts").textContent`)).To(ContainSubstring("1 file"))
	b.eval(g, `document.querySelector(".topbar .back").click()`)
	b.waitFor(g, `location.pathname === "/" && !!document.querySelector("#root .empty, #root .section")`)
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "a move between the review and the queue loaded a page")

	// The first round's own address is a page of its own.
	b.navigate(g, srv.URL+base+"/rounds/1")
	b.waitFor(g, `!!document.querySelector("#root .page .brief")`)
	g.Expect(b.eval(g, `document.title`)).To(Equal("Round 1 · Eyeball"))
	g.Expect(b.eval(g, `document.querySelector(".topbar .back").getAttribute("href")`)).To(Equal(base))

	// A tap far down a long file brings its composer into view.
	b.navigate(g, srv.URL+base+"?against=base")
	b.waitFor(g, `!!document.querySelector("#root details.file[data-path='long.txt'] tr[data-line='140']")`)
	g.Expect(b.eval(g, `window.scrollY`)).To(BeEquivalentTo(0))
	b.eval(g, `document.querySelector("details.file[data-path='long.txt'] tr[data-line='140']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.waitFor(g, `window.scrollY > 500`)
	g.Expect(b.eval(g, `(() => { const r = document.querySelector("tr.thread").getBoundingClientRect(); return r.top >= 0 && r.bottom <= window.innerHeight + 1; })()`)).To(BeTrue(), "the composer is out of sight")
	g.Expect(b.eval(g, `document.activeElement === document.querySelector("tr.thread form.composer textarea")`)).To(BeTrue())

	// A tap on the place the composer is open at brings the field back, as it
	// does on iOS where it is how the keyboard returns, and clears the level the
	// chips chose and the line a failed send left. The text stays.
	row := `document.querySelector("details.file[data-path='long.txt'] tr[data-line='140']")`
	b.typeInto(g, "tr.thread form.composer textarea", "   ")
	b.eval(g, `[...document.querySelectorAll("tr.thread form.composer .chip")].find((c) => c.textContent === "File").click()`)
	b.eval(g, `document.querySelector("tr.thread form.composer button[type=submit]").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer .failed")`)
	g.Expect(b.eval(g, `[...document.querySelectorAll("tr.thread form.composer .chip")].map((c) => c.getAttribute("aria-pressed")).join()`)).To(Equal("false,true,false"))
	b.eval(g, `document.activeElement.blur(); window.scrollTo(0, 0)`)
	b.waitFor(g, `window.scrollY === 0`)
	b.eval(g, row+`.click()`)
	b.waitFor(g, `window.scrollY > 500 && document.activeElement === document.querySelector("tr.thread form.composer textarea")`)
	g.Expect(b.eval(g, `!!document.querySelector("tr.thread form.composer .failed")`)).To(BeFalse(), "the failed line stayed")
	g.Expect(b.eval(g, `[...document.querySelectorAll("tr.thread form.composer .chip")].map((c) => c.getAttribute("aria-pressed")).join()`)).To(Equal("true,false,false"))
	g.Expect(b.eval(g, `document.querySelector("tr.thread form.composer textarea").value`)).To(Equal("   "))

	// A tap on another line moves the composer, and keeps what was typed.
	b.typeInto(g, "tr.thread form.composer textarea", "Carried along")
	b.eval(g, `document.querySelector("details.file[data-path='long.txt'] tr[data-line='142']").click()`)
	b.waitFor(g, `document.querySelector("tr.thread")?.previousElementSibling?.dataset.line === "142"`)
	g.Expect(b.eval(g, `document.querySelectorAll("form.composer").length`)).To(BeEquivalentTo(2), "one anchored composer and the review's own box")
	g.Expect(b.eval(g, `document.querySelector("tr.thread form.composer textarea").value`)).To(Equal("Carried along"))
	g.Expect(b.eval(g, `document.querySelectorAll("tr.sel").length`)).To(BeEquivalentTo(1))
	g.Expect(b.eval(g, `document.querySelector("tr.sel").dataset.line`)).To(Equal("142"))
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
