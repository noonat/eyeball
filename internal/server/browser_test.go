//go:build browser

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"golang.org/x/net/websocket"

	"github.com/noonat/eyeball"
	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/store"
)

// browserToken is the loopback token the tests hand over. It has the form
// rand.Text draws, which is the only form the bundle reads.
const browserToken = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// The review page the app draws, driven in headless Chromium: the token handed
// over, a line comment added, edited and deleted through the JSON routes, a
// review comment, and text still being written held in the bar's notice. The
// verdict's half is the template page's until it moves.
func TestServer_Surface_browserActing(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	seen := &requestLog{}
	wrote := &statusLog{}
	srv := httptest.NewServer(seen.wrap(wrote.wrap(f.s.LoopbackSurface(browserToken))))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	round := f.latestRound(g, t, id)

	// The printed address hands the token over and leaves no trace of it.
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	g.Expect(b.eval(g, `location.hash`)).To(Equal(""))
	g.Expect(b.eval(g, `localStorage.getItem("eyeball:token")`)).To(Equal(browserToken))
	g.Expect(seen.lines()).NotTo(ContainElement(ContainSubstring(browserToken)), "the token reached a request line or a Referer")
	// The page is the app's, which the shell's root holds, and its writes are
	// JSON under /api. None goes to a form route.
	g.Expect(b.eval(g, `!!document.querySelector("#root .page")`)).To(BeTrue())

	// A tap on a line opens the composer in a row under it, and Add stores
	// the comment there.
	line2 := `document.querySelector("details.file tr[data-line='2']")`
	thread := line2 + `.nextElementSibling`
	b.eval(g, line2+`.click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	g.Expect(b.eval(g, thread+`.matches("tr.thread")`)).To(BeTrue(), "the composer opens under its line")
	g.Expect(b.eval(g, `document.querySelector("tr.sel").dataset.line`)).To(Equal("2"))
	b.typeInto(g, "tr.thread form.composer textarea", "Why two?")
	b.eval(g, `document.querySelector("tr.thread form.composer button[type=submit]").click()`)
	b.waitFor(g, `!document.querySelector("tr.thread form.composer")`)
	b.waitFor(g, `document.getElementById("tally").textContent === "1 commentReady to send"`)
	b.waitFor(g, thread+`?.querySelectorAll(".comment").length === 1`)
	g.Expect(b.eval(g, `document.querySelectorAll("tr.sel").length`)).To(BeEquivalentTo(0))
	comments := f.comments(g, t, round)
	g.Expect(comments).To(HaveLen(1))
	g.Expect(comments[0].Quoted).To(Equal("two"))
	g.Expect(wrote.lines()).To(ContainElement("POST /api/reviews/" + itoa(id) + "/comments 200"))
	g.Expect(wrote.lines()).NotTo(ContainElement(HavePrefix("POST /reviews/")), "a write went to the form route")

	// The comment is still under its line after a reload.
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	b.waitFor(g, thread+`?.querySelector(".comment p")?.textContent === "Why two?"`)

	// Edit swaps the card for its form in the row, and Save stores the new
	// body.
	b.eval(g, thread+`.querySelector(".acts a").click()`)
	b.waitFor(g, thread+`.querySelector("form.composer textarea") !== null`)
	b.typeInto(g, "tr.thread form.composer textarea", "Why two lines?")
	b.eval(g, thread+`.querySelector("form.composer button[type=submit]").click()`)
	b.waitFor(g, thread+`.querySelector(".comment p")?.textContent === "Why two lines?"`)
	g.Expect(f.comments(g, t, round)[0].Body).To(Equal("Why two lines?"))
	g.Expect(wrote.lines()).To(ContainElement(HavePrefix("PUT /api/reviews/" + itoa(id) + "/comments/")))

	// Delete removes it, the row goes with its last comment, and the tally
	// follows.
	b.eval(g, thread+`.querySelectorAll(".acts a")[1].click()`)
	b.waitFor(g, `!`+thread+`?.matches("tr.thread")`)
	b.waitFor(g, `document.getElementById("tally").textContent === "No comments yet"`)
	g.Expect(f.comments(g, t, round)).To(BeEmpty())
	g.Expect(wrote.lines()).To(ContainElement(HavePrefix("DELETE /api/reviews/" + itoa(id) + "/comments/")))

	// Cancel closes the composer and keeps nothing it held.
	b.eval(g, line2+`.click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "Never mind")
	b.waitFor(g, `!!document.getElementById("unsaved")`)
	b.eval(g, `document.querySelector("tr.thread form.composer .btn.ghost").click()`)
	b.waitFor(g, `document.querySelectorAll("tr.thread").length === 0`)
	g.Expect(b.eval(g, `document.querySelectorAll("tr.sel").length`)).To(BeEquivalentTo(0))
	g.Expect(b.eval(g, `document.getElementById("unsaved")`)).To(BeNil())

	// The chips move the same text to the file without a request, and the
	// comment is sent as a file's.
	list := listID(store.LevelFile, "a.txt")
	b.eval(g, `document.querySelector("details.file tr[data-line='1']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "About the whole file.")
	b.eval(g, `[...document.querySelectorAll("tr.thread form.composer .chip")].find((c) => c.textContent === "File").click()`)
	g.Expect(b.eval(g, `[...document.querySelectorAll("tr.thread form.composer .chip")].map((c) => c.getAttribute("aria-pressed")).join()`)).To(Equal("false,true,false"))
	b.eval(g, `document.querySelector("tr.thread form.composer button[type=submit]").click()`)
	b.waitFor(g, fmt.Sprintf(`document.querySelectorAll("#%s .comment").length === 1`, list))
	b.waitFor(g, `document.querySelectorAll("tr.thread").length === 0`)
	g.Expect(f.comments(g, t, round)[0].Level).To(Equal(store.LevelFile))

	// The header's link opens the file's own page without toggling the card.
	b.eval(g, `document.querySelector("details.file .fileopen").click()`)
	b.waitFor(g, `location.pathname === "/reviews/`+itoa(id)+`/files/a.txt"`)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	b.waitFor(g, `!!document.querySelector("#root details.file")`)

	// The review's own box is always on an open round.
	b.typeInto(g, ".page > form.composer textarea", "Overall fine.")
	b.eval(g, `document.querySelector(".page > form.composer button[type=submit]").click()`)
	b.waitFor(g, `document.querySelectorAll("#comments-review .comment").length === 1`)
	g.Expect(b.eval(g, `document.querySelector(".page > form.composer textarea").value`)).To(Equal(""))

	// Text still being written is said in the bar.
	b.typeInto(g, ".page > form.composer textarea", "Half a thought")
	b.waitFor(g, `!!document.getElementById("unsaved")`)
	b.typeInto(g, ".page > form.composer textarea", "")
	b.waitFor(g, `!document.getElementById("unsaved")`)
}

// A verdict on the app's page: text still being written holds it back, a comment
// on its way does too, and once nothing is held Approve records it and the
// router moves to the queue.
func TestServer_Surface_browserActingVerdict(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	inner := f.s.LoopbackSurface(browserToken)
	// A comment's write waits here until the test lets it go, so the comment is
	// on its way while a verdict is tried.
	release := make(chan struct{})
	var released sync.Once
	letGo := func() { released.Do(func() { close(release) }) }
	var hold atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/comments") && hold.Load() {
			<-release
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	// Registered after the server's, so it runs first: a write still waiting
	// would hold the server's close.
	t.Cleanup(letGo)
	b := openBrowser(g, t)
	round := f.latestRound(g, t, id)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root #bar")`)
	approve := `document.querySelector("#bar button[value=approve]")`
	waitNoVerdict := func(why string) {
		g.THelper()
		time.Sleep(300 * time.Millisecond)
		_, err := f.store.Decision(t.Context(), round)
		g.Expect(err).To(MatchError(store.ErrNotFound), why)
	}

	// Text still being written holds the verdict back.
	b.typeInto(g, ".page > form.composer textarea", "Half a thought")
	b.waitFor(g, `!!document.getElementById("unsaved")`)
	g.Expect(b.eval(g, approve+`.getAttribute("aria-disabled")`)).To(Equal("true"))
	b.eval(g, approve+`.click()`)
	waitNoVerdict("a verdict went out while a comment was being written")
	g.Expect(b.eval(g, `location.pathname`)).To(Equal("/reviews/" + itoa(id)))

	// A comment on its way holds it back as well, since its text was in the form
	// until the answer came.
	hold.Store(true)
	b.eval(g, `document.querySelector(".page > form.composer button[type=submit]").click()`)
	b.waitFor(g, `!!document.querySelector("#comments-review .comment[data-sending]")`)
	g.Expect(b.eval(g, `document.querySelector(".page > form.composer textarea").value`)).To(Equal(""))
	g.Expect(b.eval(g, `!!document.getElementById("unsaved")`)).To(BeTrue(), "a comment on its way was not counted")
	b.eval(g, approve+`.click()`)
	waitNoVerdict("a verdict went out while a comment was on its way")
	letGo()
	hold.Store(false)
	b.waitFor(g, `document.querySelectorAll("#comments-review .comment:not([data-sending])").length === 1`)
	b.waitFor(g, `!document.getElementById("unsaved")`)

	// Once nothing is held, Approve records the verdict and the router moves to
	// the queue with no page load.
	b.eval(g, `window.__before = true`)
	b.eval(g, approve+`.click()`)
	b.waitFor(g, `location.pathname === "/" && !!document.querySelector("#root .page .section, #root .empty")`)
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "the verdict loaded a page")
	d, err := f.store.Decision(t.Context(), round)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(d.Verdict).To(Equal(store.VerdictApprove))
}

// The queue is the React app, served by the surface at the queue's address: a
// shell that loads the bundle and the stylesheets the template pages load, and
// an app that draws the queue from /api/ with the stylesheet applied.
func TestServer_Surface_browserApp(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	srv := httptest.NewServer(f.s.Surface())
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/")
	b.waitFor(g, `!!document.querySelector("#root .empty .headline")`)
	g.Expect(b.eval(g, `document.querySelector("#root .empty .headline").textContent`)).To(Equal("Nothing waiting"))
	g.Expect(b.eval(g, `document.querySelector(".topbar .eye").getAttribute("aria-label")`)).To(Equal("Nothing waiting"))

	// The stylesheet is the surface's: the empty state centers its headline.
	g.Expect(b.eval(g, `getComputedStyle(document.querySelector("#root .empty")).display`)).To(Equal("flex"))
	// The page took its theme from the device before the app drew.
	g.Expect(b.eval(g, `document.documentElement.dataset.theme`)).To(Equal("light"))
}

// Back to a review after its verdict was stored draws the round the verdict
// decided, with no bar and no draft. The cached open round would draw the bar
// again and take the note from the round's draft, which holds the fetched round
// back, so the decided round was never taken up and a second verdict was
// refused as stale.
func TestServer_Surface_browserBackAfterVerdict(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	for i := range 12 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", 40))
	}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	// The round is cached at both of its addresses before the verdict.
	other := "/reviews/" + itoa(id) + "/rounds/1"
	move := func(to string) {
		// The worker's tap message moves the router, as a notification's tap does.
		b.eval(g, `navigator.serviceWorker.dispatchEvent(new MessageEvent("message", { data: { url: `+strconv.Quote(to)+` }, ports: [new MessageChannel().port1] })), true`)
		b.waitFor(g, `location.pathname === `+strconv.Quote(to)+` && !!document.querySelector("#root details.file")`)
	}
	move(other)
	b.waitFor(g, `performance.getEntriesByType("resource").some((e) => e.name.endsWith("/api`+other+`"))`)
	move("/reviews/" + itoa(id))
	card := `document.querySelector("details.file[data-path='d/00.txt']")`
	b.eval(g, card+`.scrollIntoView()`)
	b.waitFor(g, card+`.querySelectorAll(".src").length === 40`)
	b.eval(g, card+`.querySelector("tr[data-line='2']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "A comment")
	b.eval(g, `document.querySelector("tr.thread form.composer button[type=submit]").click()`)
	b.waitFor(g, card+`.querySelectorAll("tr.thread .comment:not(.unsent):not([data-sending])").length === 1`)
	b.typeInto(g, "#bar textarea", "Please fix")
	b.eval(g, `window.__before = true`)
	b.eval(g, `document.querySelector("#bar button[value=changes]").click()`)
	b.waitFor(g, `location.pathname === "/"`)

	// Any frame that draws the bar, or Edit on a comment, is the open round
	// from the cache, which is what back must not draw first.
	b.eval(g, `(() => {
		window.__open = false;
		new MutationObserver(() => {
			if (document.querySelector("#bar, .comment .acts")) {
				window.__open = true;
			}
		}).observe(document, { subtree: true, childList: true });
		return true;
	})()`)
	b.eval(g, `history.back(), true`)
	b.waitFor(g, `!!document.querySelector("#root details.file") && document.querySelector(".page").dataset.acting === "late"`)
	time.Sleep(500 * time.Millisecond)
	g.Expect(b.eval(g, `window.__open`)).To(BeFalse(), "the cached open round was drawn")
	// The round's other address holds it too.
	move(other)
	time.Sleep(500 * time.Millisecond)
	g.Expect(b.eval(g, `window.__open`)).To(BeFalse(), "the round cached at its other address was drawn open")
	g.Expect(b.eval(g, `!!document.querySelector("#bar")`)).To(BeFalse())
	g.Expect(b.eval(g, `Object.keys(localStorage).filter((k) => k.includes("drafts:")).length`)).To(BeEquivalentTo(0), "the stored verdict left its draft")
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "a page loaded")
}

// A verdict returns to the queue, and the eye blinks there once, whichever
// verdict it was. A failed verdict leaves no mark, a mark left by a request
// whose answer never arrived does not blink, and under reduced motion the eye
// does not move.
func TestServer_Surface_browserBlink(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "b.txt", "b\n")
	write(g, repo, "c.txt", "c\n")
	write(g, repo, "d.txt", "d\n")
	me := api.Caller{Dir: repo}
	first := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	second := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"b.txt"}, Note: "second"}).Review.ID
	third := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"c.txt"}, Note: "third"}).Review.ID
	fourth := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"d.txt"}, Note: "fourth"}).Review.ID
	var failing sync.Mutex
	fail := false
	surface := f.s.Surface()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failing.Lock()
		refuse := fail && r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/decision")
		failing.Unlock()
		if refuse {
			http.Error(w, "the store is gone", http.StatusInternalServerError)
			return
		}
		surface.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	// Every document records what its eye says, and how shut the lid is when it
	// says it, as the label changes. The queue's eye is read at the moments it
	// changes, which a read after the page loads would miss.
	b.call(g, "Page.enable", map[string]any{})
	b.call(g, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": `(() => {
		window.__eye = [];
		// Every label the eye shows while the queue is on screen, whether or not
		// it changed, so a count drawn again from a stale view is seen.
		window.__all = [];
		const note = () => {
			const eye = document.querySelector(".topbar .eye");
			if (eye === null) return;
			const label = eye.getAttribute("aria-label");
			if (window.__eye.at(-1)?.label === label) return;
			window.__eye.push({ label, lid: new DOMMatrix(getComputedStyle(eye).transform).d });
		};
		// The label the eye shows when it first takes the blink class, which is the
		// count the queue starts the blink with.
		const everyLabel = () => {
			const eye = document.querySelector(".topbar .eye");
			if (eye !== null && location.pathname === "/") window.__all.push(eye.getAttribute("aria-label"));
		};
		const blinkStart = () => {
			const eye = document.querySelector(".topbar .eye");
			if (eye !== null && eye.classList.contains("blink") && window.__blinkSeen === undefined) {
				window.__blinkSeen = eye.getAttribute("aria-label");
			}
		};
		new MutationObserver(() => { note(); blinkStart(); everyLabel(); }).observe(document, { subtree: true, childList: true, attributes: true, attributeFilter: ["aria-label", "class"] });
	})()`})
	labels := `JSON.stringify(window.__eye.map((e) => e.label))`
	blinking := `document.querySelector(".topbar .eye").classList.contains("blink")`
	// drawn is true once the app has drawn the queue from its view.
	drawn := `!!document.querySelector(".scroll .page .section")`
	// decide opens a review from the queue by a click, which moves the router, and
	// sends a verdict from it. The queue's view is cached when the verdict returns
	// to it, so the queue draws from the cache first and again once the fetch the
	// stream asks for arrives.
	decide := func(id int64, verdict string) {
		b.navigate(g, srv.URL+"/")
		b.waitFor(g, drawn)
		b.eval(g, `window.__before = true`)
		b.eval(g, `document.querySelector("a.row[href='/reviews/`+itoa(id)+`']").click()`)
		b.waitFor(g, `!!document.querySelector("#root #bar")`)
		if verdict == "changes" {
			b.typeInto(g, "#bar textarea", "Rework this.")
		}
		b.eval(g, `window.__blinkSeen = undefined; window.__all = []`)
		b.eval(g, `document.querySelector("#bar button[value=`+verdict+`]").click()`)
		b.waitFor(g, `location.pathname === "/"`)
		b.waitFor(g, drawn)
		g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "a page loaded between the queue and the review")
	}
	section := func(id int64) string {
		return `document.querySelector("a.row[href='/reviews/` + itoa(id) + `']").closest(".section").querySelector("h2").firstChild.textContent.trim()`
	}

	// Approve lands on the queue at the new count with the review in Settled,
	// and the eye blinks once.
	decide(first, "approve")
	g.Expect(b.eval(g, `location.pathname`)).To(Equal("/"))
	// The eye is drawn with the count from before the verdict, and changes to the
	// new one while the lid is shut.
	b.waitFor(g, `window.__eye.length === 2`)
	g.Expect(b.eval(g, labels)).To(Equal(`["4 waiting","3 waiting"]`))
	g.Expect(b.eval(g, `window.__eye[1].lid < 0.5`)).To(BeTrue(), "the count changed with the lid open")
	// The blink starts with the count from before the verdict, which only the
	// mark holds: the queue drawn from the cache would be stale, and the one
	// fetched fresh already has the new count.
	g.Expect(b.eval(g, `window.__blinkSeen`)).To(Equal("4 waiting"))
	g.Expect(b.eval(g, `document.querySelector(".topbar .eye").getAttribute("aria-label")`)).To(Equal("3 waiting"))
	g.Expect(b.eval(g, section(first))).To(Equal("Settled"))
	g.Expect(b.eval(g, blinking)).To(BeTrue())
	g.Expect(b.eval(g, `getComputedStyle(document.querySelector(".topbar .eye")).animationName`)).To(Equal("eye-blink"))
	g.Expect(b.eval(g, `sessionStorage.getItem("eyeball:blink")`)).To(BeNil(), "the mark is read once")

	// The queue loaded again does not blink.
	b.navigate(g, srv.URL+"/")
	b.waitFor(g, drawn)
	g.Expect(b.eval(g, blinking)).To(BeFalse())

	// Request changes returns to the queue too, with the review in Working.
	decide(fourth, "changes")
	g.Expect(b.eval(g, `location.pathname`)).To(Equal("/"))
	g.Expect(b.eval(g, section(fourth))).To(Equal("Working"))
	g.Expect(b.eval(g, blinking)).To(BeTrue())
	g.Expect(b.eval(g, `sessionStorage.getItem("eyeball:blink")`)).To(BeNil())

	// A verdict the server refuses leaves no mark for a later load.
	failing.Lock()
	fail = true
	failing.Unlock()
	b.navigate(g, srv.URL+"/reviews/"+itoa(second))
	b.waitFor(g, `!!document.querySelector("#root #bar")`)
	b.eval(g, `document.querySelector("#bar button[value=approve]").click()`)
	b.waitFor(g, `!!document.querySelector("#bar .failed")`)
	g.Expect(b.eval(g, `sessionStorage.getItem("eyeball:blink")`)).To(BeNil())
	failing.Lock()
	fail = false
	failing.Unlock()

	// A mark left by a request whose answer never arrived, as when a tab is
	// closed mid-request, names a review that is still under Waiting. It does
	// not blink.
	b.eval(g, `sessionStorage.setItem("eyeball:blink", "`+itoa(third)+`")`)
	b.navigate(g, srv.URL+"/")
	b.waitFor(g, drawn)
	g.Expect(b.eval(g, section(third))).To(Equal("Waiting on you"))
	g.Expect(b.eval(g, blinking)).To(BeFalse())
	g.Expect(b.eval(g, `sessionStorage.getItem("eyeball:blink")`)).To(BeNil())

	// A mark naming a review with no row on the queue does not blink either.
	b.eval(g, `sessionStorage.setItem("eyeball:blink", "999999")`)
	b.navigate(g, srv.URL+"/")
	b.waitFor(g, drawn)
	g.Expect(b.eval(g, blinking)).To(BeFalse())
	g.Expect(b.eval(g, `sessionStorage.getItem("eyeball:blink")`)).To(BeNil())

	// Under reduced motion the eye takes the class and does not move.
	b.call(g, "Emulation.setEmulatedMedia", map[string]any{
		"features": []map[string]string{{"name": "prefers-reduced-motion", "value": "reduce"}},
	})
	decide(third, "approve")
	g.Expect(b.eval(g, blinking)).To(BeTrue())
	g.Expect(b.eval(g, `getComputedStyle(document.querySelector(".topbar .eye")).animationName`)).To(Equal("none"))
	// There is no lid to change the count behind, so the queue shows the new
	// count from its first paint and never the old one: nothing the eye showed
	// after the verdict was the count from before it.
	time.Sleep(700 * time.Millisecond)
	g.Expect(b.eval(g, `JSON.stringify([...new Set(window.__all)])`)).To(Equal(`["1 waiting"]`), "the queue drew the count from before the verdict")
	g.Expect(b.eval(g, `window.__blinkSeen`)).To(Equal("1 waiting"))
}

// A large review's cards all open with empty bodies, and each asks for its
// body as it nears the screen inside the page's own scroller. One far below
// stays empty, and a folded card loads only once it is unfolded.
func TestServer_Surface_browserBodies(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	for i := range 60 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", 40))
	}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)

	rows := func(path string) string {
		return `document.querySelector("details.file[data-path='` + path + `']").querySelectorAll(".src").length`
	}
	// The list of files tops the page, so the first card starts below it.
	b.eval(g, `document.querySelector("details.file").scrollIntoView()`)
	b.waitFor(g, rows("d/00.txt")+` === 40`)
	g.Expect(b.eval(g, `[...document.querySelectorAll("details.file")].every((d) => d.open)`)).To(BeTrue())
	// The skeleton's heights were measured on this device, once, and each is a
	// row's height and not the stylesheet's fallback left in place.
	g.Expect(b.eval(g, `["--row-h", "--hunk-h", "--code-pad"].map((p) => document.documentElement.style.getPropertyValue(p)).every((v) => v.endsWith("px"))`)).To(BeTrue(), "the skeleton's heights were not measured")
	time.Sleep(300 * time.Millisecond)
	g.Expect(b.eval(g, rows("d/59.txt"))).To(BeEquivalentTo(0), "a card far below the screen loaded its body")
	g.Expect(b.eval(g, rows("d/58.txt"))).To(BeEquivalentTo(0))

	folded := `document.querySelector("details.file[data-path='d/58.txt']")`
	b.eval(g, folded+`.querySelector("summary").click()`)
	b.waitFor(g, `!`+folded+`.open`)
	b.eval(g, `document.querySelector("details.file[data-path='d/59.txt']").scrollIntoView()`)
	b.waitFor(g, rows("d/59.txt")+` === 40`)
	time.Sleep(300 * time.Millisecond)
	g.Expect(b.eval(g, rows("d/58.txt"))).To(BeEquivalentTo(0), "a folded card loaded its body")
	// Bodies arriving above d/59 push it and d/58 down, so d/58 is brought
	// back into view before it is unfolded.
	// The wait lets the observer see it near before it is unfolded, so only
	// the unfolding can load it.
	b.eval(g, folded+`.scrollIntoView()`)
	time.Sleep(300 * time.Millisecond)
	g.Expect(b.eval(g, rows("d/58.txt"))).To(BeEquivalentTo(0), "a folded card on screen loaded its body")
	b.eval(g, folded+`.querySelector("summary").click()`)
	b.waitFor(g, rows("d/58.txt")+` === 40`)
}

// The two comparisons of a long review have bodies of their own: a body read for
// what the round changed is not drawn for the whole review, and the whole
// review's is read at its own address.
func TestServer_Surface_browserBodiesAgainst(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	body := strings.Repeat("line\n", 40)
	for i := range 12 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), body)
	}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"d"}, Note: "first"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "more")
	for i := range 12 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Replace(body, "line", "edited", 1))
	}
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
	var gated atomic.Bool
	release := make(chan struct{})
	var released sync.Once
	letGo := func() { released.Do(func() { close(release) }) }
	surface := f.s.LoopbackSurface(browserToken)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/cards/") && gated.Load() {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		surface.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(letGo)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	card := `document.querySelector("details.file[data-path='d/00.txt']")`
	b.eval(g, card+`.scrollIntoView()`)
	b.waitFor(g, card+`.querySelectorAll(".src").length > 0`)
	changed := b.eval(g, card+`.querySelectorAll(".src").length`)
	g.Expect(changed).To(BeNumerically("<", 40))

	gated.Store(true)
	b.eval(g, `document.querySelectorAll(".segmented a")[1].click()`)
	b.waitFor(g, `location.search === "?against=base" && !!`+card)
	b.eval(g, card+`.scrollIntoView()`)
	time.Sleep(300 * time.Millisecond)
	g.Expect(b.eval(g, card+`.querySelectorAll(".src").length`)).To(BeEquivalentTo(0), "a body read for the other comparison was drawn")
	letGo()
	b.waitFor(g, card+`.querySelectorAll(".src").length >= 40`)
}

// A body's request ends when the screen goes, so a body asked for and not yet
// answered does not hold the connection once the reader has left.
func TestServer_Surface_browserBodiesCancelled(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	for i := range 12 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", 40))
	}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID
	var asked, cancelled atomic.Bool
	surface := f.s.LoopbackSurface(browserToken)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cards/d/00.txt") {
			asked.Store(true)
			select {
			case <-r.Context().Done():
				cancelled.Store(true)
			case <-time.After(20 * time.Second):
			}
			return
		}
		surface.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	b.eval(g, `document.querySelector("details.file[data-path='d/00.txt']").scrollIntoView()`)
	g.Eventually(asked.Load, 5*time.Second, 50*time.Millisecond).Should(BeTrue())
	b.eval(g, `document.querySelector(".topbar .back").click()`)
	b.waitFor(g, `location.pathname === "/"`)
	g.Eventually(cancelled.Load, 5*time.Second, 50*time.Millisecond).Should(BeTrue(), "the request outlived the screen")
}

// A body that fails to load says so in its card, with a retry that asks again,
// and the card does not ask again by itself. A held card whose Load fails says
// so beside the link, with the same retry.
func TestServer_Surface_browserBodiesFailed(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	for i := range 12 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", 40))
	}
	write(g, repo, "d/lock.json", strings.Repeat("line\n", 600))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID
	var broken, gated atomic.Bool
	var asked atomic.Int32
	release := make(chan struct{})
	var released sync.Once
	letGo := func() { released.Do(func() { close(release) }) }
	surface := f.s.LoopbackSurface(browserToken)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/cards/") {
			asked.Add(1)
			if gated.Load() {
				<-release
			}
			if broken.Load() {
				http.Error(w, "the store is gone", http.StatusInternalServerError)
				return
			}
		}
		surface.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	// Registered after the server's, so it runs first: a read still held would
	// hold the server's close.
	t.Cleanup(letGo)
	b := openBrowser(g, t)
	broken.Store(true)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)

	// The first card is on screen, so it asks, is refused, and says so beside
	// the skeleton. It asks once: a card does not retry by itself.
	first := `document.querySelector("details.file[data-path='d/00.txt']")`
	b.eval(g, first+`.scrollIntoView()`)
	b.waitFor(g, `!!`+first+`.querySelector(":scope > .failed")`)
	g.Expect(b.eval(g, first+`.querySelector(":scope > .failed").textContent`)).To(Equal("Eyeball could not do that. Retry"))
	g.Expect(b.eval(g, `!!`+first+`.querySelector(":scope > .skeleton")`)).To(BeTrue(), "the skeleton went with the failure")
	g.Expect(b.eval(g, first+`.querySelector(":scope > .failed .retry").getAttribute("role")`)).To(Equal("button"))
	settled := asked.Load()
	time.Sleep(500 * time.Millisecond)
	g.Expect(asked.Load()).To(Equal(settled), "a card that failed asked again by itself")

	// Retry asks again, and the line goes as soon as it does, not when the answer
	// comes.
	broken.Store(false)
	gated.Store(true)
	b.eval(g, first+`.querySelector(":scope > .failed .retry").click()`)
	b.waitFor(g, `!`+first+`.querySelector(".failed")`)
	g.Expect(b.eval(g, first+`.querySelectorAll(".src").length`)).To(BeEquivalentTo(0))
	letGo()
	b.waitFor(g, first+`.querySelectorAll(".src").length === 40`)

	// A held card's Load that fails says so after the link.
	gated.Store(false)
	lock := `document.querySelector("details.file[data-path='d/lock.json']")`
	b.eval(g, lock+`.scrollIntoView()`)
	broken.Store(true)
	b.eval(g, lock+`.querySelector("a.load").click()`)
	b.waitFor(g, `!!`+lock+`.querySelector(".filefacts .failed")`)
	g.Expect(b.eval(g, lock+`.querySelector(".filefacts .failed").textContent`)).To(Equal("Eyeball could not do that. Retry"))
	g.Expect(b.eval(g, lock+`.querySelectorAll(".src").length`)).To(BeEquivalentTo(0))
	broken.Store(false)
	b.eval(g, lock+`.querySelector(".failed .retry").click()`)
	b.waitFor(g, lock+`.querySelectorAll(".src").length === 600`)
}

// A body read again on focus would be read outside the hold: after a verdict
// given elsewhere its comments take the decided state, and lose Edit and
// Delete, while the bar is still drawn. The focus asks nothing of the server.
func TestServer_Surface_browserBodiesFocus(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	for i := range 12 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", 40))
	}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID
	seen := &requestLog{}
	srv := httptest.NewServer(seen.wrap(f.s.LoopbackSurface(browserToken)))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	card := `document.querySelector("details.file[data-path='d/00.txt']")`
	b.eval(g, card+`.scrollIntoView()`)
	b.waitFor(g, card+`.querySelectorAll(".src").length === 40`)
	b.eval(g, card+`.querySelector("tr[data-line='2']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "Stored one")
	b.eval(g, `document.querySelector("tr.thread form.composer button[type=submit]").click()`)
	b.waitFor(g, card+`.querySelectorAll("tr.thread .comment:not(.unsent):not([data-sending])").length === 1`)
	b.typeInto(g, "#bar textarea", "Holding note")
	f.decide(g, t, id, store.VerdictChanges, "elsewhere")
	time.Sleep(700 * time.Millisecond)
	acts := card + `.querySelectorAll("tr.thread .comment .acts").length`
	g.Expect(b.eval(g, acts)).To(BeEquivalentTo(1))

	reads := func() int {
		n := 0
		for _, line := range seen.lines() {
			if strings.Contains(line, "/cards/d/00.txt") {
				n++
			}
		}
		return n
	}
	before := reads()
	b.eval(g, `window.dispatchEvent(new Event("visibilitychange")), true`)
	time.Sleep(1500 * time.Millisecond)
	g.Expect(reads()).To(Equal(before), "a focus read the body outside the hold")
	g.Expect(b.eval(g, acts)).To(BeEquivalentTo(1), "the comment took the decided state under a bar")
	g.Expect(b.eval(g, `!!document.querySelector("#bar")`)).To(BeTrue())
}

// A comment written on a body the app fetched is kept with it, and a round that
// moves reads the body again: after a verdict elsewhere the comment is still
// there and no longer offers Edit or Delete.
func TestServer_Surface_browserBodiesRefreshed(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	for i := range 12 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", 40))
	}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)

	card := `document.querySelector("details.file[data-path='d/00.txt']")`
	b.eval(g, card+`.scrollIntoView()`)
	b.waitFor(g, card+`.querySelectorAll(".src").length === 40`)
	b.eval(g, card+`.querySelector("tr[data-line='2']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "Why two?")
	b.eval(g, `document.querySelector("tr.thread form.composer button[type=submit]").click()`)
	comment := card + `.querySelector("tr.thread .comment:not(.unsent):not([data-sending])")`
	b.waitFor(g, `!!`+comment)
	g.Expect(b.eval(g, comment+`.querySelector(".acts") !== null`)).To(BeTrue())
	g.Expect(b.eval(g, `document.getElementById("tally").textContent`)).To(Equal("1 commentReady to send"))

	// A verdict given elsewhere moves the round's stamp. The body is read again
	// and holds the comment, now past editing.
	f.decide(g, t, id, store.VerdictChanges, "more")
	b.waitFor(g, `!document.querySelector("#bar")`)
	b.waitFor(g, `!!`+comment+` && `+comment+`.querySelector(".acts") === null`)
	g.Expect(b.eval(g, comment+`.querySelector("p").textContent`)).To(Equal("Why two?"))
}

// A review the reader returns to draws its bodies at once, from the cache, with
// no skeleton, and reads them again behind them: the read is held here, so only
// a body drawn from the cache can be on screen.
func TestServer_Surface_browserBodiesReturn(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	for i := range 12 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", 40))
	}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID
	var gated atomic.Bool
	var reads atomic.Int32
	release := make(chan struct{})
	var released sync.Once
	letGo := func() { released.Do(func() { close(release) }) }
	surface := f.s.LoopbackSurface(browserToken)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/cards/") && gated.Load() {
			reads.Add(1)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		surface.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(letGo)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	card := `document.querySelector("details.file[data-path='d/00.txt']")`
	b.eval(g, card+`.scrollIntoView()`)
	b.waitFor(g, card+`.querySelectorAll(".src").length === 40`)

	// To the file's own page and back, which move the router and load no page.
	gated.Store(true)
	b.eval(g, `window.__before = true`)
	b.eval(g, card+`.querySelector(".fileopen").click()`)
	b.waitFor(g, `!!document.querySelector("#root .crumb .name")`)
	b.eval(g, `history.back()`)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	b.waitFor(g, card+`.querySelectorAll(".src").length === 40 && !`+card+`.querySelector(".skeleton")`)
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "a page loaded")
	// Behind them the bodies are read again.
	g.Eventually(reads.Load, 5*time.Second, 50*time.Millisecond).Should(BeNumerically(">", 0))
}

// A body read before a write of the reader's lacks the comment that write
// stored, so it is read again, and the comment is not taken off the screen when
// the first read arrives.
func TestServer_Surface_browserBodiesStale(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	for i := range 12 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", 40))
	}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID
	// The next read of the first card's body is answered from the store as it is
	// and then held, so what arrives is older than a write made while it waits.
	var hold atomic.Bool
	var held atomic.Bool
	release := make(chan struct{})
	var released sync.Once
	letGo := func() { released.Do(func() { close(release) }) }
	surface := f.s.LoopbackSurface(browserToken)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cards/d/00.txt") && hold.CompareAndSwap(true, false) {
			rec := httptest.NewRecorder()
			surface.ServeHTTP(rec, r)
			held.Store(true)
			<-release
			for k, v := range rec.Header() {
				w.Header()[k] = v
			}
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
			return
		}
		surface.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	// Registered after the server's, so it runs first: a read still held would
	// hold the server's close.
	t.Cleanup(letGo)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	card := `document.querySelector("details.file[data-path='d/00.txt']")`
	b.eval(g, card+`.scrollIntoView()`)
	b.waitFor(g, card+`.querySelectorAll(".src").length === 40`)

	// A verdict given elsewhere makes the body stale, and the read for it is held.
	hold.Store(true)
	f.decide(g, t, id, store.VerdictChanges, "more")
	g.Eventually(held.Load, 10*time.Second, 50*time.Millisecond).Should(BeTrue())
	b.waitFor(g, `!document.querySelector("#bar")`)

	// A comment is written while it waits, on a decided round where it is sent at
	// once.
	b.eval(g, card+`.querySelector("tr[data-line='2']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "Written meanwhile")
	b.eval(g, `document.querySelector("tr.thread form.composer button[type=submit]").click()`)
	comment := card + `.querySelector("tr.thread .comment:not(.unsent):not([data-sending])")`
	b.waitFor(g, `!!`+comment)

	letGo()
	time.Sleep(1500 * time.Millisecond)
	g.Expect(b.eval(g, `!!`+comment)).To(BeTrue(), "a body read before the write took the comment off the screen")
	g.Expect(b.eval(g, comment+`.querySelector("p").textContent`)).To(Equal("Written meanwhile"))
}

// Two answers to writes that land in one tick are both kept: each is built on
// the one before it, not on what was last drawn. The two requests are held and
// let go together, so their answers are handled before the screen has drawn
// either, and both comments are on a body the app fetched.
func TestServer_Surface_browserBodiesTogether(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	for i := range 12 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", 40))
	}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	card := `document.querySelector("details.file[data-path='d/00.txt']")`
	b.eval(g, card+`.scrollIntoView()`)
	b.waitFor(g, card+`.querySelectorAll(".src").length === 40`)

	// Every answer to a comment waits until the test lets them all go at once.
	b.eval(g, `(() => {
		const real = window.fetch.bind(window);
		window.__held = [];
		window.fetch = (url, init) => {
			if (init && init.method === "POST" && String(url).endsWith("/comments")) {
				return new Promise((resolve, reject) => {
					real(url, init).then((res) => window.__held.push(() => resolve(res)), reject);
				});
			}
			return real(url, init);
		};
		window.__flush = () => window.__held.splice(0).forEach((go) => go());
		return true;
	})()`)
	lines := []string{"2", "3"}
	for _, line := range lines {
		b.eval(g, card+`.querySelector("tr[data-line='`+line+`']").click()`)
		b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
		b.typeInto(g, "tr.thread form.composer textarea", "About line "+line)
		b.eval(g, `document.querySelector("tr.thread form.composer button[type=submit]").click()`)
		b.waitFor(g, `!document.querySelector("tr.thread form.composer")`)
	}
	b.waitFor(g, `window.__held.length === 2`)
	b.eval(g, `window.__flush()`)
	b.waitFor(g, card+`.querySelectorAll("tr.thread .comment:not(.unsent):not([data-sending])").length >= 1`)
	time.Sleep(500 * time.Millisecond)
	g.Expect(b.eval(g, card+`.querySelectorAll("tr.thread .comment:not(.unsent):not([data-sending])").length`)).To(BeEquivalentTo(2), "an answer built on a stale round dropped the other")
}

// A link to a comment inside a card lands below the card's stuck header, not
// under it, once the comment is drawn. A notification's tap reaches the review
// by a router move from a scrolled queue, where the comment is not drawn yet and
// the router scrolls to the top, and a page load names it too.
func TestServer_Surface_browserCommentJump(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := longFileReview(g, t, f)
	fillQueue(g, t, f, 14)
	review, err := f.store.Review(t.Context(), id)
	g.Expect(err).NotTo(HaveOccurred())
	round, err := f.store.Round(t.Context(), id, review.LatestRound)
	g.Expect(err).NotTo(HaveOccurred())
	c, err := f.s.db.AddComment(t.Context(), store.NewComment{RoundID: round.ID, Level: store.LevelLine, Path: "d/long.txt", StartLine: 200, EndLine: 200, Quoted: "line 200", Body: "Why here?"})
	g.Expect(err).NotTo(HaveOccurred())
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	comment := `document.getElementById("comment-` + itoa(c.ID) + `")`
	summary := `document.querySelector("details.file[data-path='d/long.txt'] > summary")`
	landed := func() {
		g.THelper()
		b.waitFor(g, `!!`+comment)
		b.waitFor(g, `window.scrollY > 0`)
		time.Sleep(100 * time.Millisecond)
		g.Expect(b.eval(g, `Math.round(Math.abs(`+summary+`.getBoundingClientRect().top - document.querySelector(".head").getBoundingClientRect().bottom))`)).To(BeEquivalentTo(0), "the card's header is not stuck")
		// Adding zero turns the -0 a small negative rounds to into 0, which the
		// DevTools protocol sends as a number.
		below := `Math.round(` + comment + `.getBoundingClientRect().top - ` + summary + `.getBoundingClientRect().bottom) + 0`
		g.Expect(b.eval(g, below)).To(BeNumerically(">=", 0), "the comment landed under the card's header")
		g.Expect(b.eval(g, below)).To(BeNumerically("<", 40), "the comment landed far below the card's header")
	}

	// A page load with the address, where no place is saved for the review that
	// the jump would give way to.
	b.navigate(g, srv.URL+"/#token="+browserToken)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#comment-"+itoa(c.ID))
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	landed()

	// The tap arrives while the queue is scrolled, which the router leaves for
	// the top of the new screen before the view has come. The place the first
	// visit saved is not what the link asked for.
	b.navigate(g, srv.URL+"/")
	b.waitFor(g, `document.querySelectorAll("#root a.row").length > 10`)
	b.loadWorkerInPage(g)
	b.eval(g, `window.fake.postMessage = (message, ports) => navigator.serviceWorker.dispatchEvent(new MessageEvent("message", { data: message, ports }))`)
	b.eval(g, `window.scrollTo(0, 800)`)
	b.waitFor(g, `window.scrollY > 500`)
	b.eval(g, `window.__before = true`)
	b.eval(g, `(() => { window.tap("/reviews/`+itoa(id)+`#comment-`+itoa(c.ID)+`"); return true; })()`)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	landed()
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "the tap loaded a page")
}

// A tap on a line of a decided round opens a composer that sends at once, the
// comment lands under its line as late and reaches the agent as late, and
// nothing typed there is kept as a draft, since it is sent and never held.
func TestServer_Surface_browserDecidedTap(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	write(g, repo, "b.txt", "b\n")
	open := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"b.txt"}, Note: "open"}).Review.ID
	write(g, repo, "blob.bin", "\x00\x01\x02")
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt", "blob.bin"}, Note: "first"}).Review.ID
	round := f.latestRound(g, t, id)
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	drafts := func(review int64) string {
		return `localStorage.getItem("eyeball:drafts:` + itoa(review) + `:1")`
	}
	line2 := `document.querySelector("details.file tr[data-line='2']")`

	// On an open round typing is kept, which is what the check below is
	// measured against.
	b.navigate(g, srv.URL+"/reviews/"+itoa(open)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file tr[data-line='1']")`)
	b.eval(g, `document.querySelector("details.file tr[data-line='1']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "Kept")
	b.waitFor(g, drafts(open)+` !== null`)

	f.decide(g, t, id, store.VerdictApprove, "")
	nextMillisecond()
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	b.waitFor(g, `document.querySelector("#root .page")?.dataset.acting === "late"`)
	b.eval(g, line2+`.click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	g.Expect(b.eval(g, `document.querySelector("tr.thread form.composer").textContent`)).To(ContainSubstring("Sends immediately"))
	g.Expect(b.eval(g, `document.querySelector("tr.thread form.composer button[type=submit]").textContent`)).To(Equal("Send"))
	b.typeInto(g, "tr.thread form.composer textarea", "Not kept")
	time.Sleep(300 * time.Millisecond)
	g.Expect(b.eval(g, drafts(id))).To(BeNil(), "a decided round kept a draft of a comment it sends at once")

	b.eval(g, `document.querySelector("tr.thread form.composer button[type=submit]").click()`)
	thread := `document.getElementById("` + threadID("a.txt", 2) + `")`
	b.waitFor(g, thread+`?.querySelectorAll(".comment:not(.unsent):not([data-sending])").length === 1`)
	g.Expect(b.eval(g, thread+`.querySelector(".comment .anchor").textContent`)).To(ContainSubstring("after the verdict"))
	g.Expect(b.eval(g, thread+`.querySelector(".comment .acts")`)).To(BeNil())
	g.Expect(b.eval(g, drafts(id))).To(BeNil())
	comments := f.comments(g, t, round)
	g.Expect(comments).To(HaveLen(1))
	g.Expect(comments[0].Body).To(Equal("Not kept"))

	// The agent reads it as late.
	got := f.wait(g, t, id, me)
	g.Expect(got.Kind).To(Equal(api.KindVerdict))
	g.Expect(got.Comments).To(HaveLen(1))
	g.Expect(got.Comments[0].Body).To(Equal("Not kept"))
	g.Expect(got.Comments[0].Late).To(BeTrue())

	// A tap on a line switched to the File chip sends a late comment on the
	// file, which lands in that file's list.
	list := listID(store.LevelFile, "a.txt")
	b.eval(g, `document.querySelector("details.file tr[data-line='1']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.eval(g, `[...document.querySelectorAll("tr.thread form.composer .chip")].find((c) => c.textContent === "File").click()`)
	b.typeInto(g, "tr.thread form.composer textarea", "About the file.")
	b.eval(g, `document.querySelector("tr.thread form.composer button[type=submit]").click()`)
	// The file's list holds the comments on the file. The line comment sits
	// under its line, where a reload puts it, and not in the list as the
	// template's answer left it.
	b.waitFor(g, fmt.Sprintf(`document.querySelectorAll("#%s .comment:not(.unsent):not([data-sending])").length === 1`, list))
	card := fmt.Sprintf(`[...document.querySelectorAll("#%s .comment")].find((c) => c.querySelector("p").textContent === "About the file.")`, list)
	g.Expect(b.eval(g, card+`.querySelector(".anchor").textContent`)).To(ContainSubstring("after the verdict"))
	g.Expect(b.eval(g, card+`.querySelector(".acts")`)).To(BeNil())
	// A file with no rows to tap takes its comment in the file's slot.
	binary := `document.querySelector("details.file[data-path='blob.bin']")`
	b.eval(g, binary+`.querySelector(".filefacts").click()`)
	b.waitFor(g, `!!`+binary+`.querySelector("form.composer textarea")`)
	g.Expect(b.eval(g, binary+`.querySelector("form.composer").textContent`)).To(ContainSubstring("Sends immediately"))
	b.typeInto(g, `details.file[data-path='blob.bin'] form.composer textarea`, "About the binary.")
	b.eval(g, binary+`.querySelector("form.composer button[type=submit]").click()`)
	b.waitFor(g, `document.querySelectorAll("#`+listID(store.LevelFile, "blob.bin")+` .comment:not(.unsent):not([data-sending])").length === 1`)
	g.Expect(b.eval(g, `document.querySelector("#`+listID(store.LevelFile, "blob.bin")+` .comment .anchor").textContent`)).To(ContainSubstring("after the verdict"))

	var onFile store.Comment
	for _, c := range f.comments(g, t, round) {
		if c.Body == "About the file." {
			onFile = c
		}
	}
	g.Expect(onFile.Level).To(Equal(store.LevelFile))
	got = f.wait(g, t, id, me)
	late := map[string]bool{}
	for _, c := range got.Comments {
		late[c.Body] = c.Late
	}
	g.Expect(late).To(Equal(map[string]bool{"Not kept": true, "About the file.": true, "About the binary.": true}))
}

// The document scrolls, not .scroll, since iOS sends its status bar tap only to
// the document's scroller. The header stays at the top as it does.
func TestServer_Surface_browserDocumentScrolls(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	for i := range 11 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", 40))
	}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)

	scroller := `document.querySelector(".scroll")`
	g.Expect(b.eval(g, `document.scrollingElement.scrollHeight > window.innerHeight * 2`)).To(BeTrue(), "the review is not long enough to scroll")
	g.Expect(b.eval(g, scroller+`.scrollHeight === `+scroller+`.clientHeight`)).To(BeTrue(), ".scroll has content to scroll")
	b.eval(g, `window.scrollTo(0, 500)`)
	g.Expect(b.eval(g, `Math.round(window.scrollY)`)).To(BeEquivalentTo(500))
	g.Expect(b.eval(g, scroller+`.scrollTop`)).To(BeEquivalentTo(0))
	g.Expect(b.eval(g, `Math.round(document.querySelector(".head").getBoundingClientRect().top)`)).To(BeEquivalentTo(0), "the header scrolled away")
	g.Expect(b.eval(g, `document.documentElement.style.getPropertyValue("--head-height") === document.querySelector(".head").getBoundingClientRect().height + "px"`)).To(BeTrue())
}

// What is typed is saved as it is typed, in the shape the template page saved
// it, and is back after a reload: the review's box, the verdict's note, the open
// composer where it was, and a comment's edit form. A draft the template page
// saved is read, and one is cleared once its text is stored.
func TestServer_Surface_browserDrafts(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	round := f.latestRound(g, t, id)
	existing := f.addComment(g, t, store.NewComment{RoundID: round, Level: store.LevelReview, Body: "Existing."})
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	key := `localStorage.getItem("eyeball:drafts:` + itoa(id) + `:1")`

	b.typeInto(g, ".page > form.composer textarea", "Half a review comment")
	b.typeInto(g, "#bar textarea", "A note in progress")
	b.eval(g, `document.querySelector("details.file tr[data-line='2']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "About line two")
	b.eval(g, `document.querySelector("#comments-review .comment .acts a").click()`)
	b.waitFor(g, `!!document.querySelector("#comments-review form.composer textarea")`)
	b.typeInto(g, "#comments-review form.composer textarea", "Existing, edited.")

	// The draft is the template's, key for key.
	b.waitFor(g, `JSON.parse(`+key+`).value.edits !== undefined`)
	g.Expect(b.eval(g, `JSON.stringify(JSON.parse(`+key+`).value)`)).To(MatchJSON(fmt.Sprintf(
		`{"note": "A note in progress", "review": "Half a review comment", "anchored": {"query": {"level": "line", "path": "a.txt", "start": "2"}, "text": "About line two"}, "edits": {"%d": "Existing, edited."}}`,
		existing.ID,
	)))

	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	g.Expect(b.eval(g, `document.querySelector(".page > form.composer textarea").value`)).To(Equal("Half a review comment"))
	g.Expect(b.eval(g, `document.querySelector("#bar textarea").value`)).To(Equal("A note in progress"))
	g.Expect(b.eval(g, `document.querySelector("tr.thread form.composer textarea").value`)).To(Equal("About line two"))
	g.Expect(b.eval(g, `document.querySelector("tr.thread").previousElementSibling.dataset.line`)).To(Equal("2"))
	g.Expect(b.eval(g, `document.querySelector("#comments-review form.composer textarea").value`)).To(Equal("Existing, edited."))
	g.Expect(b.eval(g, `!!document.getElementById("unsaved")`)).To(BeTrue(), "text kept is text not yet stored")

	// A draft clears once its text is stored.
	b.eval(g, `document.querySelector(".page > form.composer button[type=submit]").click()`)
	b.waitFor(g, `document.querySelectorAll("#comments-review .comment:not([data-sending])").length === 1`)
	b.eval(g, `document.querySelector("tr.thread form.composer .btn.ghost").click()`)
	b.eval(g, `document.querySelector("#comments-review form.composer .btn.ghost").click()`)
	b.typeInto(g, "#bar textarea", "")
	b.waitFor(g, key+` === null`)

	// A draft the template page saved is restored: the review's box, a card it
	// could not send, and the composer on a line.
	planted := fmt.Sprintf(`{"at": Date.now(), "value": {"review": "From the template page", "anchored": {"query": {"level": "line", "path": "a.txt", "start": "1"}, "text": "On line one"}, "unsent": [{"action": "/reviews/%d/comments", "target": "#comments-review", "where": "This review", "values": {"round": "1", "level": "review", "body": "Never sent", "submission": "kept-key"}}]}}`, id)
	b.eval(g, `localStorage.setItem("eyeball:drafts:`+itoa(id)+`:1", JSON.stringify(`+planted+`))`)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	g.Expect(b.eval(g, `document.querySelector(".page > form.composer textarea").value`)).To(Equal("From the template page"))
	g.Expect(b.eval(g, `document.querySelector("tr.thread form.composer textarea").value`)).To(Equal("On line one"))
	g.Expect(b.eval(g, `document.querySelector("#comments-review .comment.unsent p").textContent`)).To(Equal("Never sent"))
	b.eval(g, `document.querySelector("#comments-review .comment.unsent .acts a").click()`)
	b.waitFor(g, `document.querySelectorAll("#comments-review .comment:not(.unsent):not([data-sending])").length === 3`)
	var bodies []string
	for _, c := range f.comments(g, t, round) {
		bodies = append(bodies, c.Body)
	}
	g.Expect(bodies).To(ContainElement("Never sent"))

	// An entry older than the memory's age is dropped as the app opens, and a
	// recent one is kept.
	b.eval(g, `localStorage.setItem("eyeball:drafts:77:1", JSON.stringify({ at: 1, value: { review: "Long ago" } }))`)
	b.eval(g, `localStorage.setItem("eyeball:drafts:78:1", JSON.stringify({ at: Date.now(), value: { review: "Today" } }))`)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	g.Expect(b.eval(g, `localStorage.getItem("eyeball:drafts:77:1")`)).To(BeNil())
	g.Expect(b.eval(g, `localStorage.getItem("eyeball:drafts:78:1") !== null`)).To(BeTrue())
}

// A draft whose composer sits on a row of a card that has no body yet waits for
// the body. The draft is kept while it waits, and the composer opens with its
// text once the card comes near and its body arrives.
func TestServer_Surface_browserDraftsDeferred(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	for i := range 60 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", 40))
	}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	key := `localStorage.getItem("eyeball:drafts:` + itoa(id) + `:1")`
	b.eval(g, `localStorage.setItem("eyeball:drafts:`+itoa(id)+`:1", JSON.stringify({ at: Date.now(), value: { anchored: { query: { level: "line", path: "d/58.txt", start: "2" }, text: "Waiting for its body" } } }))`)

	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	late := `document.querySelector("details.file[data-path='d/58.txt']")`
	time.Sleep(300 * time.Millisecond)
	g.Expect(b.eval(g, `!!document.querySelector("form.composer[data-query], tr.thread")`)).To(BeFalse(), "a composer opened where no row was drawn")
	g.Expect(b.eval(g, `JSON.parse(`+key+`).value.anchored.text`)).To(Equal("Waiting for its body"), "the draft was lost while its body was away")

	b.eval(g, late+`.scrollIntoView()`)
	b.waitFor(g, `document.querySelector("tr.thread form.composer textarea")?.value === "Waiting for its body"`)
	g.Expect(b.eval(g, `document.querySelector("tr.thread").previousElementSibling.dataset.line`)).To(Equal("2"))
	g.Expect(b.eval(g, late+`.contains(document.querySelector("tr.thread"))`)).To(BeTrue())
}

// A comment that cannot be saved becomes an unsent card in its list, Retry
// stores it once the server is back and says why when it fails again, and
// Delete drops it. A comment the server refuses stays in its composer with the
// server's sentence. An edit and a delete that fail say so beside their
// control. A save made while the server is gone is kept in the draft, and a
// retry after a reload sends the key it first had.
func TestServer_Surface_browserFailedSave(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	round := f.latestRound(g, t, id)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	g.Expect(err).NotTo(HaveOccurred())
	addr := ln.Addr().String()
	inner := f.s.LoopbackSurface(browserToken)
	// lose makes the next write reach the store and its answer never leave: the
	// connection is closed with no response, as a lost signal would.
	var lose atomic.Bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/") && lose.CompareAndSwap(true, false) {
			inner.ServeHTTP(httptest.NewRecorder(), r)
			panic(http.ErrAbortHandler)
		}
		inner.ServeHTTP(w, r)
	})
	serve := func(l net.Listener) *http.Server {
		hs := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
		go func() { _ = hs.Serve(l) }()
		return hs
	}
	hs := serve(ln)
	t.Cleanup(func() { _ = hs.Close() })
	restart := func() {
		g.THelper()
		ln, err := net.Listen("tcp", addr)
		g.Expect(err).NotTo(HaveOccurred())
		hs = serve(ln)
		b := hs
		t.Cleanup(func() { _ = b.Close() })
	}
	b := openBrowser(g, t)
	b.navigate(g, "http://"+addr+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	tap := func(selector string) {
		g.THelper()
		b.eval(g, `document.querySelector(`+strconv.Quote(selector)+`).click()`)
	}

	// A comment the server refuses stays where it was typed, with its reason.
	tap("details.file tr[data-line='2']")
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "   ")
	tap("tr.thread form.composer button[type=submit]")
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer .failed")`)
	g.Expect(b.eval(g, `document.querySelector("tr.thread form.composer .failed").textContent`)).To(Equal(emptyComment))
	g.Expect(b.eval(g, `document.querySelector("tr.thread form.composer textarea").value`)).To(Equal("   "))
	g.Expect(b.eval(g, `document.querySelectorAll(".comment").length`)).To(BeEquivalentTo(0), "a refusal made no card")
	g.Expect(f.comments(g, t, round)).To(BeEmpty())

	// Two saves made while the server is gone become two cards.
	b.typeInto(g, "tr.thread form.composer textarea", "Why two?")
	b.typeInto(g, ".page > form.composer textarea", "Overall fine.")
	g.Expect(hs.Close()).To(Succeed())
	tap("tr.thread form.composer button[type=submit]")
	b.waitFor(g, `document.querySelectorAll(".comment.unsent").length === 1`)
	tap(".page > form.composer button[type=submit]")
	b.waitFor(g, `document.querySelectorAll(".comment.unsent").length === 2`)
	g.Expect(b.eval(g, `!!document.querySelector("tr.thread form.composer")`)).To(BeFalse(), "the line's composer closed")
	g.Expect(b.eval(g, `document.querySelector(".page > form.composer textarea").value`)).To(Equal(""))
	g.Expect(b.eval(g, `!!document.getElementById("unsent")`)).To(BeTrue())
	g.Expect(b.eval(g, `document.getElementById("unsent-count").textContent`)).To(Equal("2 comments"))
	g.Expect(b.eval(g, `!!document.querySelector(".comment.unsent .failed")`)).To(BeFalse(), "a first failure leaves no line beside the card")

	// The cards are kept as a draft, with the key they were first sent under.
	key := `JSON.parse(localStorage.getItem("eyeball:drafts:` + itoa(id) + `:1")).value`
	g.Expect(b.eval(g, key+`.unsent.length`)).To(BeEquivalentTo(2))
	g.Expect(b.eval(g, key+`.unsent[0].action`)).To(Equal("/reviews/" + itoa(id) + "/comments"))
	submission := b.eval(g, key+`.unsent[0].values.submission`)
	g.Expect(submission).NotTo(BeEmpty())

	// Retry while the server is still gone says so beside the card's actions.
	tap("tr.thread .comment.unsent .acts a")
	b.waitFor(g, `!!document.querySelector("tr.thread .comment.unsent .failed")`)
	g.Expect(b.eval(g, `document.querySelector("tr.thread .comment.unsent .failed").textContent`)).To(Equal("Cannot reach Eyeball."))
	g.Expect(b.eval(g, `document.querySelector("tr.thread .comment.unsent .acts").nextElementSibling.className`)).To(Equal("failed"))

	// Back online, a reload keeps the cards and Retry stores the line's comment
	// once, under the key it first had.
	restart()
	b.navigate(g, "http://"+addr+"/reviews/"+itoa(id))
	b.waitFor(g, `document.querySelectorAll(".comment.unsent").length === 2`)
	b.waitFor(g, `document.getElementById("offline").hidden === true`)
	tap("tr.thread .comment.unsent .acts a")
	b.waitFor(g, `document.querySelectorAll("tr.thread .comment:not(.unsent):not([data-sending])").length === 1`)
	stored := f.comments(g, t, round)
	g.Expect(stored).To(HaveLen(1))
	g.Expect(b.eval(g, `document.querySelectorAll(".comment.unsent").length`)).To(BeEquivalentTo(1))
	g.Expect(b.eval(g, key+`.unsent.length`)).To(BeEquivalentTo(1), "a stored card leaves the draft")

	// Delete drops the other card and sends nothing.
	tap(".comment.unsent .acts a[role=button]:last-of-type")
	b.waitFor(g, `document.querySelectorAll(".comment.unsent").length === 0`)
	g.Expect(b.eval(g, `!!document.getElementById("unsent")`)).To(BeFalse())
	g.Expect(f.comments(g, t, round)).To(HaveLen(1))

	// An edit that fails says so in its form, and keeps its text.
	tap("tr.thread .comment .acts a")
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "Why two lines?")
	g.Expect(hs.Close()).To(Succeed())
	tap("tr.thread form.composer button[type=submit]")
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer .failed")`)
	g.Expect(b.eval(g, `document.querySelector("tr.thread form.composer .failed").textContent`)).To(Equal("Cannot reach Eyeball."))
	g.Expect(b.eval(g, `document.querySelector("tr.thread form.composer textarea").value`)).To(Equal("Why two lines?"))
	g.Expect(f.comments(g, t, round)[0].Body).To(Equal("Why two?"))

	// A delete that fails says so beside the comment's actions.
	tap("tr.thread form.composer .btn.ghost")
	b.waitFor(g, `!document.querySelector("tr.thread form.composer")`)
	tap("tr.thread .comment .acts a:last-of-type")
	b.waitFor(g, `!!document.querySelector("tr.thread .comment .failed")`)
	g.Expect(b.eval(g, `document.querySelector("tr.thread .comment .acts").nextElementSibling.className`)).To(Equal("failed"))
	g.Expect(f.comments(g, t, round)).To(HaveLen(1))

	// A first try that reached the store and lost its answer is stored once: the
	// card's retry sends the key the first try had.
	restart()
	b.waitFor(g, `document.getElementById("offline").hidden === true`)
	b.typeInto(g, ".page > form.composer textarea", "Stored, never answered.")
	lose.Store(true)
	tap(".page > form.composer button[type=submit]")
	b.waitFor(g, `document.querySelectorAll(".comment.unsent").length === 1`)
	g.Expect(f.comments(g, t, round)).To(HaveLen(2), "the first try reached the store")
	tap(".comment.unsent .acts a")
	b.waitFor(g, `document.querySelectorAll(".comment.unsent").length === 0`)
	b.waitFor(g, `document.querySelectorAll("#comments-review .comment:not(.unsent):not([data-sending])").length === 1`)
	g.Expect(f.comments(g, t, round)).To(HaveLen(2), "the retry stored the comment again")
}

// A verdict that cannot be sent says so in the bar and keeps its note and leaves
// no mark, and the verdict waits while a comment that did not save remains.
func TestServer_Surface_browserFailedVerdict(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	round := f.latestRound(g, t, id)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	g.Expect(err).NotTo(HaveOccurred())
	addr := ln.Addr().String()
	serve := func(l net.Listener) *http.Server {
		hs := &http.Server{Handler: f.s.LoopbackSurface(browserToken), ReadHeaderTimeout: 5 * time.Second}
		go func() { _ = hs.Serve(l) }()
		return hs
	}
	hs := serve(ln)
	t.Cleanup(func() { _ = hs.Close() })
	restart := func() {
		g.THelper()
		ln, err := net.Listen("tcp", addr)
		g.Expect(err).NotTo(HaveOccurred())
		hs = serve(ln)
		b := hs
		t.Cleanup(func() { _ = b.Close() })
	}
	b := openBrowser(g, t)
	b.navigate(g, "http://"+addr+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root #bar")`)
	approve := `document.querySelector("#bar button[value=approve]")`

	// A verdict sent while the server is gone says so in the bar and keeps
	// its note. It leaves no mark for a later load.
	b.typeInto(g, "#bar textarea", "Close.")
	g.Expect(hs.Close()).To(Succeed())
	b.eval(g, approve+`.click()`)
	b.waitFor(g, `!!document.querySelector("#bar .failed")`)
	g.Expect(b.eval(g, `document.querySelector("#bar .failed").textContent`)).To(Equal("Cannot reach Eyeball."))
	g.Expect(b.eval(g, `document.querySelector("#bar textarea").value`)).To(Equal("Close."))
	g.Expect(b.eval(g, `sessionStorage.getItem("eyeball:blink")`)).To(BeNil())
	b.typeInto(g, "#bar textarea", "")
	restart()
	b.waitFor(g, `document.getElementById("offline").hidden === true`)

	// A comment made while the server is gone is an unsent card, and the verdict
	// waits for it.
	b.typeInto(g, ".page > form.composer textarea", "Overall fine.")
	g.Expect(hs.Close()).To(Succeed())
	b.eval(g, `document.querySelector(".page > form.composer button[type=submit]").click()`)
	b.waitFor(g, `document.querySelectorAll(".comment.unsent").length === 1`)
	restart()
	b.waitFor(g, `document.getElementById("offline").hidden === true`)
	g.Expect(b.eval(g, `!!document.getElementById("unsent")`)).To(BeTrue())
	b.eval(g, approve+`.click()`)
	time.Sleep(300 * time.Millisecond)
	_, err = f.store.Decision(t.Context(), round)
	g.Expect(err).To(MatchError(store.ErrNotFound), "a verdict went out with a comment unsent")

	// Delete drops the card, and the verdict goes out.
	b.eval(g, `document.querySelector(".comment.unsent .acts a[role=button]:last-of-type").click()`)
	b.waitFor(g, `document.querySelectorAll(".comment.unsent").length === 0`)
	b.eval(g, approve+`.click()`)
	b.waitFor(g, `location.pathname === "/"`)
	d, err := f.store.Decision(t.Context(), round)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(d.Verdict).To(Equal(store.VerdictApprove))
}

// First run on the queue, with the browser's push manager stubbed so nothing
// reaches a real push service and its permission granted through the
// protocol: the steps show until Allow subscribes, a
// request then reaches the device, and a lapsed subscription says so and
// subscribes again.
func TestServer_Surface_browserFirstRun(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	svc := newPushService(t, map[string]int{"/device/1": http.StatusCreated, "/device/2": http.StatusCreated})
	f.s.surface = func() api.Surface { return api.Surface{URL: "https://eyeball.example.ts.net"} }
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)

	var keys struct {
		Keys map[string]string `json:"keys"`
	}
	g.Expect(json.Unmarshal([]byte(svc.subscription(g, "/x")), &keys)).To(Succeed())
	stub := fmt.Sprintf(`(() => {
		let sub = null, n = 0;
		const make = () => {
			const endpoint = %q + "/device/" + (++n);
			const json = { endpoint, keys: { p256dh: %q, auth: %q } };
			return { endpoint, toJSON: () => json, unsubscribe: async () => { sub = null; return true; } };
		};
		PushManager.prototype.getSubscription = async () => sub;
		PushManager.prototype.subscribe = async () => (sub = make());
	})()`, svc.url, keys.Keys["p256dh"], keys.Keys["auth"])
	b.call(g, "Page.enable", map[string]any{})
	b.call(g, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": stub})

	// The steps show until the device subscribes. Chromium offers push in a
	// tab, so the install step reads done.
	b.navigate(g, srv.URL+"/#token="+browserToken)
	b.waitFor(g, `!!document.querySelector(".steps")`)
	g.Expect(b.eval(g, `document.querySelector(".steps .step.done .what").textContent`)).To(Equal("Installed to the home screen"))
	// The prompt is the browser's, so the permission is granted through the
	// protocol before the tap, and requestPermission answers granted.
	b.call(g, "Browser.grantPermissions", map[string]any{"permissions": []string{"notifications"}, "origin": srv.URL})
	b.eval(g, `[...document.querySelectorAll(".steps button")].find((b) => b.textContent === "Allow").click()`)
	b.waitFor(g, `!document.querySelector(".steps")`)
	subs, err := f.store.ActiveSubscriptions(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(subs).To(HaveLen(1))
	g.Expect(subs[0].Endpoint).To(Equal(svc.url + "/device/1"))

	// A request now reaches the device.
	actingReview(g, t, f)
	f.s.WaitPushes(t.Context())
	g.Expect(svc.paths()).To(Equal([]string{"/device/1"}))

	// A subscription the push service called gone says so on the next load,
	// and Re-subscribe makes a new one.
	g.Expect(f.store.MarkLapsed(t.Context(), svc.url+"/device/1")).To(Succeed())
	b.navigate(g, srv.URL+"/")
	b.waitFor(g, `!!document.querySelector(".notice.warn a[role=button]")`)
	g.Expect(b.eval(g, `document.querySelector(".notice.warn strong").textContent`)).To(Equal("Notifications stopped arriving."))
	b.eval(g, `document.querySelector(".notice.warn a[role=button]").click()`)
	b.waitFor(g, `!document.querySelector(".notice")`)
	subs, err = f.store.ActiveSubscriptions(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(subs).To(HaveLen(1))
	g.Expect(subs[0].Endpoint).To(Equal(svc.url + "/device/2"))
}

// heldThroughANewRound opens a review with a note in the bar and a composer
// with text under line two, gives the verdict elsewhere and opens a second
// round. The round that is drawn is the first, held by the text, while the
// server's newest is the second. It returns the browser, and the reads of the
// round's view the server has had, which a later event must not add to.
func heldThroughANewRound(g *WithT, t *testing.T, f *fixture) (*browser, func() int) {
	t.Helper()
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	seen := &requestLog{}
	srv := httptest.NewServer(seen.wrap(f.s.LoopbackSurface(browserToken)))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root #bar")`)
	b.typeInto(g, "#bar textarea", "A note in progress")
	b.eval(g, `document.querySelector("details.file tr[data-line='2']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "About line two")
	b.eval(g, `window.__composer = document.querySelector("tr.thread form.composer textarea"), true`)
	f.decide(g, t, id, store.VerdictChanges, "From another device.")
	time.Sleep(700 * time.Millisecond)
	write(g, repo, "a.txt", "one\ntwo\nthree\n")
	f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Note: "second", Review: id})
	time.Sleep(700 * time.Millisecond)
	reads := func() int {
		n := 0
		for _, line := range seen.lines() {
			if strings.HasPrefix(line, "/api/reviews/"+itoa(id)) && !strings.Contains(line, "/cards/") {
				n++
			}
		}
		return n
	}
	return b, reads
}

// keepsHeldText checks that what was typed is still there, in the same
// element, and that the round was not asked for again.
func keepsHeldText(g *WithT, b *browser, reads func() int, before int) {
	g.THelper()
	g.Expect(reads()).To(Equal(before), "the event read the round outside the hold")
	g.Expect(b.eval(g, `document.querySelector("tr.thread form.composer textarea") === window.__composer`)).To(BeTrue(), "the screen was drawn again")
	g.Expect(b.eval(g, `document.querySelector("tr.thread form.composer textarea").value`)).To(Equal("About line two"))
	g.Expect(b.eval(g, `document.querySelector("#bar textarea").value`)).To(Equal("A note in progress"))
}

// The app coming back to the foreground does not read the round. A round read
// then, after a verdict given elsewhere and a new round, would be a different
// screen above the hold, and its text would go.
func TestServer_Surface_browserFocusKeepsText(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	b, reads := heldThroughANewRound(g, t, f)
	before := reads()
	b.eval(g, `window.dispatchEvent(new Event("visibilitychange")), true`)
	time.Sleep(1500 * time.Millisecond)
	keepsHeldText(g, b, reads, before)
}

// A fold opens with a tap on its line, and a passage inside it takes a comment
// like any other. A draft written there reopens with its fold open, since a
// passage with nothing saved under it folds again on reload.
func TestServer_Surface_browserFold(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "notes"))
	write(g, repo, "a.txt", "one\n")
	commitFile(g, repo, "doc.md", "Old.\n\nA.\n\nB.\n\nC.\n\nD.\n")
	write(g, repo, "doc.md", "New.\n\nA.\n\nB.\n\nC.\n\nD.\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"doc.md"}, Note: "docs"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.fold")`)

	fold := `document.querySelector("details.fold")`
	inside := fold + `.querySelector(".blk[data-lines='7-7']")`
	g.Expect(b.eval(g, fold+`.querySelector("summary").textContent`)).To(Equal("3 unchanged passages"))
	g.Expect(b.eval(g, inside+`.checkVisibility()`)).To(BeFalse(), "a closed fold hides its passages")
	b.eval(g, fold+`.querySelector("summary").click()`)
	b.waitFor(g, fold+`.open && `+inside+`.checkVisibility()`)
	b.eval(g, inside+`.click()`)
	b.waitFor(g, inside+`.nextElementSibling?.querySelector("form.composer textarea") != null`)
	b.typeInto(g, "div[id^='thread-'] form.composer textarea", "Is C still true?")

	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	// The page draws after its fetch, so the fold may not be there yet.
	b.waitFor(g, `!!document.querySelector("#root details.fold")`)
	b.waitFor(g, inside+`?.nextElementSibling?.querySelector("form.composer textarea")?.value === "Is C still true?"`)
	g.Expect(b.eval(g, fold+`.open`)).To(BeTrue(), "the draft's passage is folded out of sight")
}

// A passage with a comment is kept out of every fold, which is the server's
// rule, so a comment saved on a passage inside a fold moves it out once the
// round is fetched again, with each fold's count right, and a delete puts it
// back.
func TestServer_Surface_browserFoldComment(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "notes"))
	write(g, repo, "a.txt", "one\n")
	commitFile(g, repo, "doc.md", "Old.\n\nA.\n\nB.\n\nC.\n\nD.\n")
	write(g, repo, "doc.md", "New.\n\nA.\n\nB.\n\nC.\n\nD.\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"doc.md"}, Note: "docs"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.fold")`)

	inFold := `document.querySelector("details.fold .blk[data-lines='7-7']")`
	counted := `[...document.querySelectorAll("details.fold")].every((d) => d.querySelector("summary").textContent === d.querySelectorAll(":scope > .blk").length + " unchanged passages")`
	g.Expect(b.eval(g, `!!`+inFold)).To(BeTrue())
	g.Expect(b.eval(g, counted)).To(BeTrue())

	b.eval(g, `document.querySelector("details.fold summary").click()`)
	b.waitFor(g, `document.querySelector("details.fold").open`)
	b.eval(g, inFold+`.click()`)
	b.waitFor(g, `!!document.querySelector("div[id^='thread-'] form.composer textarea")`)
	b.typeInto(g, "div[id^='thread-'] form.composer textarea", "Is C still true?")
	b.eval(g, `document.querySelector("div[id^='thread-'] form.composer button[type=submit]").click()`)

	// The passage is out of the fold with its comment, and no fold counts one it
	// does not hold.
	b.waitFor(g, `!`+inFold+` && !!document.querySelector(".prose > .blk[data-lines='7-7']")`)
	b.waitFor(g, `document.querySelector(".prose > .blk[data-lines='7-7']").nextElementSibling?.querySelectorAll(".comment:not(.unsent):not([data-sending])").length === 1`)
	g.Expect(b.eval(g, counted)).To(BeTrue(), "a fold counts passages it does not hold")

	// A delete puts it back in its fold.
	b.eval(g, `document.querySelector(".prose > .blk[data-lines='7-7']").nextElementSibling.querySelectorAll(".acts a")[1].click()`)
	b.waitFor(g, `!!`+inFold)
	g.Expect(b.eval(g, counted)).To(BeTrue())
	g.Expect(f.comments(g, t, f.latestRound(g, t, id))).To(BeEmpty())
}

// A comment saved on a passage of a document whose body the app fetched moves it
// out of its fold, as the round's own does: the body is read again, since
// whether a passage sits in a fold is the server's rule.
func TestServer_Surface_browserFoldCommentDeferred(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "notes"))
	write(g, repo, "a.txt", "one\n")
	for i := range 11 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), "line\n")
	}
	commitFile(g, repo, "doc.md", "Old.\n\nA.\n\nB.\n\nC.\n\nD.\n")
	write(g, repo, "doc.md", "New.\n\nA.\n\nB.\n\nC.\n\nD.\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d", "doc.md"}, Note: "docs"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	b.eval(g, `document.querySelector("details.file[data-path='doc.md']").scrollIntoView()`)
	b.waitFor(g, `!!document.querySelector("details.fold")`)
	g.Expect(b.eval(g, `!!document.querySelector("details.file[data-path='doc.md']").dataset.path`)).To(BeTrue())

	inFold := `document.querySelector("details.fold .blk[data-lines='7-7']")`
	b.eval(g, `document.querySelector("details.fold summary").click()`)
	b.waitFor(g, `document.querySelector("details.fold").open`)
	b.eval(g, inFold+`.click()`)
	b.waitFor(g, `!!document.querySelector("div[id^='thread-'] form.composer textarea")`)
	b.typeInto(g, "div[id^='thread-'] form.composer textarea", "Is C still true?")
	b.eval(g, `document.querySelector("div[id^='thread-'] form.composer button[type=submit]").click()`)
	b.waitFor(g, `!`+inFold+` && !!document.querySelector(".prose > .blk[data-lines='7-7']")`)
	b.waitFor(g, `document.querySelector(".prose > .blk[data-lines='7-7']").nextElementSibling?.querySelectorAll(".comment:not(.unsent):not([data-sending])").length === 1`)

	// A delete puts it back in its fold.
	b.eval(g, `document.querySelector(".prose > .blk[data-lines='7-7']").nextElementSibling.querySelectorAll(".acts a")[1].click()`)
	b.waitFor(g, `!!`+inFold)
	g.Expect(f.comments(g, t, f.latestRound(g, t, id))).To(BeEmpty())
}

// The stylesheet's three faces and the eye load under the surface's own
// policy. A font the policy blocks falls back to the system face and a mask it
// blocks draws nothing, and both look close enough to right that nobody
// reports them.
func TestServer_Surface_browserFonts(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo, Agent: "spec/012-surface"}, Paths: []string{"a.txt"}, Note: "A letter."}).Review.ID
	srv := httptest.NewServer(f.s.Surface())
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)

	// Installed before the page loads, so a violation during load is heard.
	b.call(g, "Page.enable", map[string]any{})
	b.call(g, "Page.addScriptToEvaluateOnNewDocument", map[string]any{
		"source": `window.__blocked = []; addEventListener("securitypolicyviolation", (e) => window.__blocked.push(e.violatedDirective + " " + e.blockedURI));`,
	})
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	// The app draws the letter after its view arrives, and only then does the
	// page use the faces.
	b.waitFor(g, `!!document.querySelector("#root .letter")`)
	b.waitFor(g, `new Set([...document.fonts].filter((f) => f.status === "loaded").map((f) => f.family)).size === 3`)
	b.waitFor(g, `document.fonts.ready.then(() => true)`)
	loaded := b.eval(g, `document.fonts.ready.then(() => [...new Set([...document.fonts].filter((f) => f.status === "loaded").map((f) => f.family.replaceAll('"', "")))].sort().join(", "))`)
	g.Expect(loaded).To(Equal("Atkinson Hyperlegible Mono, Atkinson Hyperlegible Next, Literata"), "the chrome, the agent's name and the letter each drew its own face")
	// The computed mask stays the data URL even when the policy blocks it, so
	// this only shows the rule names the mask. The policy is what __blocked
	// checks.
	g.Expect(b.eval(g, `getComputedStyle(document.querySelector(".eye"), "::before").maskImage.startsWith("url(\"data:image/svg+xml")`)).To(BeTrue())
	g.Expect(b.eval(g, `window.__blocked.join("; ")`)).To(BeEmpty())
}

// Load anyway on a held card fills the card with its body, and the card
// stays open.
func TestServer_Surface_browserHeld(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	write(g, repo, "d/lock.json", strings.Repeat("line\n", 600))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "lock"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)

	card := `document.querySelector("details.file[data-path='d/lock.json']")`
	time.Sleep(300 * time.Millisecond)
	g.Expect(b.eval(g, card+`.querySelectorAll(".src").length`)).To(BeEquivalentTo(0), "a held card loaded on its own")
	b.eval(g, card+`.querySelector("a.load").click()`)
	b.waitFor(g, card+`.querySelectorAll(".src").length === 600`)
	g.Expect(b.eval(g, card+`.open && !`+card+`.querySelector(".filefacts")`)).To(BeTrue())
}

// A tap on the line of a card that has no body yet moves no composer. The card
// has no slot to draw one in, so the open composer would be kept, hidden and
// holding the text.
func TestServer_Surface_browserHeldTap(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "d/a.txt", "one\ntwo\n")
	write(g, repo, "d/lock.json", strings.Repeat("line\n", 600))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "lock"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("details.file[data-path='d/a.txt'] tr[data-line]")`)
	b.eval(g, `document.querySelector("details.file[data-path='d/a.txt'] tr[data-line]").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "Half a thought")
	b.eval(g, `document.querySelector("details.file[data-path='d/lock.json'] .filefacts .kind").click()`)
	time.Sleep(500 * time.Millisecond)
	g.Expect(b.eval(g, `document.querySelector("details.file[data-path='d/a.txt'] tr.thread form.composer textarea")?.value ?? null`)).To(Equal("Half a thought"), "a tap on a held card took the composer")
}

// A card that changes height above the reader's place does not move what is
// read, and a page at its top holds nothing, since what grows above the first
// card there is what the reader is looking at.
func TestServer_Surface_browserHoldPlace(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := longFileReview(g, t, f)
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)

	// Something above the first card grows by 40px as the card changes size,
	// which is when a hold looks at where the card is.
	grow := func(above, height int) {
		g.THelper()
		b.eval(g, fmt.Sprintf(`document.querySelector(".brief").style.paddingTop = "%dpx"; document.querySelector("details.file").style.minHeight = "%dpx"; true`, above, height))
		time.Sleep(200 * time.Millisecond)
	}
	grow(40, 7000)
	g.Expect(b.eval(g, `Math.round(window.scrollY)`)).To(BeEquivalentTo(0), "a page at its top was scrolled")

	// Scrolled down, the same growth is held: the card stays where it was
	// under the header, so the page scrolls by what it grew.
	b.eval(g, `window.scrollTo(0, 300)`)
	time.Sleep(200 * time.Millisecond)
	grow(80, 7100)
	g.Expect(b.eval(g, `Math.round(window.scrollY)`)).To(BeEquivalentTo(340))
}

// A tap in the list of files jumps to that file's card, and the bodies that
// load around it on arrival leave it where it landed, because each card holds
// its rows' height until its body comes.
func TestServer_Surface_browserJump(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	// Each file is committed and then changed at lines 2 and 14, so its body
	// holds two hunks and the skeleton has to count both.
	var base, changed []string
	for n := 1; n <= 20; n++ {
		base = append(base, fmt.Sprintf("line %d", n))
		changed = append(changed, fmt.Sprintf("line %d", n))
	}
	changed[1], changed[13] = "line two", "line fourteen"
	for i := range 60 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Join(base, "\n")+"\n")
	}
	git(g, repo, "add", "-A")
	git(g, repo, "update-ref", "refs/heads/main", git(g, repo, "commit-tree", git(g, repo, "write-tree"), "-p", "main", "-m", "files"))
	write(g, repo, "a.txt", "one\n")
	for i := range 60 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Join(changed, "\n")+"\n")
	}
	// A long document, d/39z.md, sorts just above the target. Its skeleton is six lines
	// whatever its length, since a document sends no counts, so its body
	// arriving grows the card by far more than that.
	var paragraphs []string
	for n := range 80 {
		paragraphs = append(paragraphs, fmt.Sprintf("Paragraph %d of the manual.", n))
	}
	write(g, repo, "d/39z.md", strings.Join(paragraphs, "\n\n")+"\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID
	fillQueue(g, t, f, 14)
	// A body takes a moment to come, as it does over the tailnet, so the
	// skeletons are on screen when the jump lands and the bodies arrive around
	// the target afterward.
	surface := f.s.LoopbackSurface(browserToken)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/cards/") {
			time.Sleep(400 * time.Millisecond)
		}
		surface.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)

	// The review is reached from a queue scrolled well down, by a move of the
	// router. A link to a card is then a router link too, since a plain anchor
	// would restore the queue's place.
	b.navigate(g, srv.URL+"/#token="+browserToken)
	b.waitFor(g, `document.querySelectorAll("#root a.row").length > 10`)
	b.eval(g, `window.scrollTo(0, 800)`)
	b.waitFor(g, `window.scrollY > 500`)
	b.eval(g, `window.__before = true`)
	b.eval(g, `document.querySelector("#root a.row[href='/reviews/`+itoa(id)+`']").click()`)
	b.waitFor(g, `!!document.querySelector("#root .filelist a")`)
	// The screen turns Chromium's scroll anchoring off for itself, which Safari
	// needs, so a browser test runs the path Safari runs.
	g.Expect(b.eval(g, `document.documentElement.style.overflowAnchor`)).To(Equal("none"))
	target := `document.querySelector("details.file[data-path='d/40.txt']")`
	top := target + `.getBoundingClientRect().top - document.querySelector(".head").getBoundingClientRect().bottom`
	b.eval(g, `document.querySelector(".filelist a[href='#" + `+target+`.id + "']").click()`)
	b.waitFor(g, `Math.abs(`+top+`) < 2`)
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "the jump loaded a page")
	// The document above the target is within a screen of view, so its body
	// loads above the target, and so does the target's own.
	g.Expect(b.eval(g, `document.querySelector("details.file[data-path='d/40.txt'] .skeleton").dataset.hunks`)).To(Equal("2"))
	b.waitFor(g, `document.querySelector("details.file[data-path='d/40.txt']").querySelectorAll(".hunkhead").length === 2`)
	b.waitFor(g, `!document.querySelector("details.file[data-path='d/39z.md'] .skeleton")`)
	// Math.abs first: Math.round gives -0 for a small negative offset, which
	// the DevTools protocol sends as a value with no number.
	g.Expect(b.eval(g, `Math.round(Math.abs(`+top+`))`)).To(BeNumerically("<=", 4), "the target moved as bodies loaded around it")

	// Reopened, the page goes back to where it was left and sets that place
	// again as bodies arrive. A tap in the list straight away is not pulled
	// back by it.
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	b.waitFor(g, `!!document.querySelector("#root .filelist a")`)
	other := `document.querySelector("details.file[data-path='d/10.txt']")`
	otherTop := other + `.getBoundingClientRect().top - document.querySelector(".head").getBoundingClientRect().bottom`
	b.eval(g, `document.querySelector(".filelist a[href='#" + `+other+`.id + "']").click()`)
	b.waitFor(g, other+`.querySelectorAll(".hunkhead").length === 2`)
	time.Sleep(300 * time.Millisecond)
	g.Expect(b.eval(g, `Math.round(Math.abs(`+otherTop+`))`)).To(BeNumerically("<=", 4), "the saved place pulled the jump back")
}

// A comment on a passage reads the round again, for the folds the server
// decides. Once a verdict was given elsewhere and a new round opened, that read
// returns the new round, which must not be a new screen above the hold: the
// text in the review's box stays, in the same element.
func TestServer_Surface_browserPassageRefetchKeepsText(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := filepath.Join(t.TempDir(), "notes")
	g.Expect(os.MkdirAll(repo, 0o700)).To(Succeed())
	git(g, repo, "init", "--quiet", "--template=", "-b", "main")
	write(g, repo, "doc.md", "Old paragraph.\n\nKept.\n")
	git(g, repo, "add", "-A")
	git(g, repo, "update-ref", "refs/heads/main", git(g, repo, "commit-tree", git(g, repo, "write-tree"), "-m", "base"))
	write(g, repo, "doc.md", "New paragraph.\n\nKept.\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"doc.md"}, Note: "docs"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root div.blk.changed")`)
	b.typeInto(g, ".page > form.composer textarea", "Half written")
	b.eval(g, `window.__box = document.querySelector(".page > form.composer textarea"), true`)
	f.decide(g, t, id, store.VerdictChanges, "From another device.")
	time.Sleep(700 * time.Millisecond)
	write(g, repo, "doc.md", "Newer paragraph.\n\nKept.\n")
	f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Note: "second", Review: id})
	time.Sleep(700 * time.Millisecond)

	kept := `document.querySelector("div.blk[data-lines='3-3']")`
	b.eval(g, kept+`.click()`)
	b.waitFor(g, kept+`.nextElementSibling?.querySelector("form.composer textarea") != null`)
	b.typeInto(g, "div[id^='thread-'] form.composer textarea", "Why keep this?")
	b.eval(g, `document.querySelector("div[id^='thread-'] form.composer button[type=submit]").click()`)
	b.waitFor(g, kept+`.nextElementSibling?.querySelectorAll(".comment:not(.unsent):not([data-sending])").length === 1`)
	time.Sleep(1000 * time.Millisecond)
	g.Expect(b.eval(g, `document.querySelector(".page > form.composer textarea") === window.__box`)).To(BeTrue(), "the screen was drawn again")
	g.Expect(b.eval(g, `window.__box.value`)).To(Equal("Half written"))
}

// A passage takes a comment under it, in a document card and in the layout a
// single new document gets. A tap selects a passage, and only its hint shows
// what a changed one replaced.
func TestServer_Surface_browserPassages(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := filepath.Join(t.TempDir(), "notes")
	g.Expect(os.MkdirAll(repo, 0o700)).To(Succeed())
	git(g, repo, "init", "--quiet", "--template=", "-b", "main")
	write(g, repo, "doc.md", "Old paragraph.\n\nKept.\n")
	git(g, repo, "add", "-A")
	git(g, repo, "update-ref", "refs/heads/main", git(g, repo, "commit-tree", git(g, repo, "write-tree"), "-m", "base"))
	write(g, repo, "doc.md", "New paragraph.\n\nKept.\n\nSee [the top](#top).\n")
	write(g, repo, "new.md", "# Fresh\n\nA new document.\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"doc.md"}, Note: "docs"}).Review.ID
	fresh := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"new.md"}, Note: "new"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root div.blk.changed")`)

	// A tap on a changed passage selects it, a tap on one of its marked
	// words included.
	changed := `document.querySelector("div.blk.changed")`
	g.Expect(b.eval(g, changed+`.querySelector("del").textContent`)).To(Equal("Old"))
	b.eval(g, changed+`.querySelector("ins").click()`)
	b.waitFor(g, changed+`.nextElementSibling?.querySelector("form.composer textarea") != null`)
	g.Expect(b.eval(g, changed+`.classList.contains("sel")`)).To(BeTrue())
	b.eval(g, changed+`.nextElementSibling.querySelector(".btn.ghost").click()`)
	b.waitFor(g, `!document.querySelector("div[id^='thread-'] form.composer") && !`+changed+`.classList.contains("sel")`)

	// A tap on a link in a passage opens no composer. The click is kept from
	// following so the page stays to be read.
	link := `document.querySelector("div.blk[data-lines='5-5'] a")`
	b.eval(g, link+`.addEventListener("click", (e) => e.preventDefault())`)
	b.eval(g, link+`.click()`)
	time.Sleep(300 * time.Millisecond)
	g.Expect(b.eval(g, `document.querySelectorAll("tr.thread, div[id^='thread-']").length`)).To(BeEquivalentTo(0), "a tap on a link opened a composer")
	g.Expect(b.eval(g, link+`.getAttribute("href")`)).To(Equal("/reviews/"+itoa(id)+"/files/doc.md#top"), "the link goes to the document's own page")

	// A comment on a kept passage lands under it, and stays there.
	kept := `document.querySelector("div.blk[data-lines='3-3']")`
	b.eval(g, kept+`.click()`)
	b.waitFor(g, kept+`.nextElementSibling?.querySelector("form.composer textarea") != null`)
	b.typeInto(g, "div[id^='thread-'] form.composer textarea", "Why keep this?")
	b.eval(g, `document.querySelector("div[id^='thread-'] form.composer button[type=submit]").click()`)
	b.waitFor(g, `!document.querySelector("div[id^='thread-'] form.composer")`)
	b.waitFor(g, kept+`.nextElementSibling?.querySelectorAll(".comment:not(.unsent):not([data-sending])").length === 1`)
	comments := f.comments(g, t, f.latestRound(g, t, id))
	g.Expect(comments).To(HaveLen(1))
	g.Expect(comments[0].Level).To(Equal(store.LevelPassage))
	g.Expect([]int{comments[0].StartLine, comments[0].EndLine}).To(Equal([]int{3, 3}))
	g.Expect(comments[0].Quoted).To(Equal("Kept."))
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	b.waitFor(g, kept+`?.nextElementSibling?.querySelector(".comment p")?.textContent === "Why keep this?"`)

	// The single new document takes one the same way.
	b.navigate(g, srv.URL+"/reviews/"+itoa(fresh))
	b.waitFor(g, `!!document.querySelector("#root .page > .prose")`)
	block := `document.querySelector(".page > .prose .blk[data-lines='3-3']")`
	b.eval(g, block+`.click()`)
	b.waitFor(g, block+`.nextElementSibling?.querySelector("form.composer textarea") != null`)
	b.typeInto(g, "div[id^='thread-'] form.composer textarea", "Say more.")
	b.eval(g, `document.querySelector("div[id^='thread-'] form.composer button[type=submit]").click()`)
	b.waitFor(g, block+`.nextElementSibling?.querySelectorAll(".comment:not(.unsent):not([data-sending])").length === 1`)
	g.Expect(f.comments(g, t, f.latestRound(g, t, fresh))[0].Quoted).To(Equal("A new document."))
}

// A page in a review is a program the agent wrote. Framed as the surface
// frames it, its script runs, cannot read the loopback token, and its write is
// refused by the cross-origin check on the tailnet listener, which has no
// token to stop it.
func TestServer_Surface_browserPreviewSandbox(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "more")
	write(g, repo, "evil.html", fmt.Sprintf(`<script>
const out = {ran: true};
try { out.token = localStorage.getItem("eyeball:token"); } catch (e) { out.token = e.name; }
const body = JSON.stringify({round: 2, verdict: "approve", note: ""});
fetch("/api/reviews/%d/decision", {method: "POST", body, mode: "no-cors"})
  .then(() => { out.post = "sent"; }, (e) => { out.post = String(e); })
  .finally(() => parent.postMessage(JSON.stringify(out), "*"));
</script>`, id))
	f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Review: id, Note: "second"})
	round := f.latestRound(g, t, id)
	frame := fmt.Sprintf(`(() => {
  window.__got = null;
  addEventListener("message", (e) => { window.__got = JSON.parse(e.data); });
  const f = document.createElement("iframe");
  f.sandbox = "allow-scripts";
  f.src = "/reviews/%d/raw/2/evil.html";
  document.body.append(f);
})()`, id)

	statuses := &statusLog{}
	tailnet := httptest.NewServer(statuses.wrap(f.s.Surface()))
	t.Cleanup(tailnet.Close)
	loopback := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(loopback.Close)
	b := openBrowser(g, t)

	b.navigate(g, loopback.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	g.Expect(b.eval(g, `localStorage.getItem("eyeball:token")`)).To(Equal(browserToken))
	b.eval(g, frame)
	b.waitFor(g, `window.__got !== null`)
	g.Expect(b.eval(g, `window.__got.ran`)).To(BeTrue())
	g.Expect(b.eval(g, `window.__got.token`)).To(Equal("SecurityError"))

	b.navigate(g, tailnet.URL+"/reviews/"+itoa(id))
	b.eval(g, frame)
	b.waitFor(g, `window.__got !== null`)
	g.Expect(b.eval(g, `window.__got.post`)).To(Equal("sent"))
	g.Eventually(statuses.lines).Should(ContainElement(fmt.Sprintf("POST /api/reviews/%d/decision 403", id)))
	_, err := f.store.Decision(t.Context(), round)
	g.Expect(err).To(MatchError(store.ErrNotFound))
}

// A page in its Page view loads its own stylesheet from the round it is
// shown at, through the raw route the relative link resolves into.
func TestServer_Surface_browserPreviewStylesheet(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	commitFile(g, repo, "site/app.css", "body { color: rgb(1, 2, 3) }\n")
	write(g, repo, "site/index.html", `<link rel="stylesheet" href="app.css"><p>page</p>
<script>setInterval(() => parent.postMessage(getComputedStyle(document.body).color, "*"), 50);</script>`)
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"site"}, Note: "first", Views: map[string]string{"site/index.html": "rendered"}}).Review.ID
	srv := httptest.NewServer(f.s.Surface())
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)

	// The frame posts until the test stops it, so a listener added after
	// the page loads still hears it.
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"/files/site/index.html")
	b.eval(g, `addEventListener("message", (e) => { window.__got = e.data; })`)
	b.waitFor(g, `typeof window.__got === "string"`)
	g.Expect(b.eval(g, `window.__got`)).To(Equal("rgb(1, 2, 3)"))
}

// A review an agent opens while the queue is on screen appears in it with no
// reload: the stream's stamp differs from the view's, the app refetches, and
// a value set on the window is still there afterward.
func TestServer_Surface_browserQueueArrives(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	srv := httptest.NewServer(f.s.Surface())
	t.Cleanup(srv.Close)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "b.txt", "b\n")
	me := api.Caller{Dir: repo}
	f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"})
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/")
	b.waitFor(g, `document.querySelectorAll("a.row.waiting").length === 1`)
	// The stream is open, which is when the first stamp has been heard.
	b.waitFor(g, `document.getElementById("offline").hidden === true`)
	b.eval(g, `window.__before = true`)
	b.eval(g, `window.__loads = performance.getEntriesByType("navigation").length`)

	second := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"b.txt"}, Note: "second"}).Review.ID
	b.waitFor(g, `document.querySelectorAll("a.row.waiting").length === 2`)
	g.Expect(b.eval(g, `!!document.querySelector("a.row[href='/reviews/`+itoa(second)+`']")`)).To(BeTrue())
	g.Expect(b.eval(g, `document.querySelector(".topbar .eye").getAttribute("aria-label")`)).To(Equal("2 waiting"))
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "the page reloaded")
	g.Expect(b.eval(g, `performance.getEntriesByType("navigation").length`)).To(Equal(b.eval(g, `window.__loads`)))

	// A verdict given elsewhere moves the row out of Waiting in place too.
	f.decide(g, t, second, store.VerdictApprove, "")
	b.waitFor(g, `document.querySelectorAll("a.row.waiting").length === 1`)
	g.Expect(b.eval(g, `document.querySelector("a.row[href='/reviews/`+itoa(second)+`']").closest(".section").querySelector("h2").firstChild.textContent.trim()`)).To(Equal("Settled"))
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "the page reloaded")
}

// The app reconnecting does not read the round either. The stream asks again
// for its stamp when it returns, which is what finds a change missed while it
// was down, and that read waits for the hold.
func TestServer_Surface_browserReconnectKeepsText(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	b, reads := heldThroughANewRound(g, t, f)
	before := reads()
	b.eval(g, `window.dispatchEvent(new Event("offline")), window.dispatchEvent(new Event("online")), true`)
	time.Sleep(1500 * time.Millisecond)
	keepsHeldText(g, b, reads, before)
}

// A change request with nothing in it draws the bar's refused state and leaves
// no mark, and a verdict given elsewhere takes the bar away with no reload. Sent
// again with a note on a review of its own, it goes out.
func TestServer_Surface_browserRefusedChangeRequest(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	round := f.latestRound(g, t, id)

	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root #bar")`)
	b.eval(g, `document.querySelector("#bar button[value=changes]").click()`)
	b.waitFor(g, `!!document.querySelector("#bar .notice.warn")`)
	_, err := f.store.Decision(t.Context(), round)
	g.Expect(err).To(MatchError(store.ErrNotFound))
	// The refusal is an answer, so it clears the mark a verdict leaves.
	g.Expect(b.eval(g, `sessionStorage.getItem("eyeball:blink")`)).To(BeNil())
	b.eval(g, `window.__before = true`)
	f.decide(g, t, id, store.VerdictApprove, "")
	b.waitFor(g, `!document.querySelector("#bar")`)
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "the page reloaded for the decided review")

	// Sent again with a note, on a review of its own, it goes out.
	id = actingReview(g, t, f)
	round = f.latestRound(g, t, id)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	b.waitFor(g, `!!document.querySelector("#root #bar")`)
	b.eval(g, `document.querySelector("#bar button[value=changes]").click()`)
	b.waitFor(g, `!!document.querySelector("#bar .notice.warn")`)
	b.typeInto(g, "#bar textarea", "Name the control.")
	b.eval(g, `window.__before = true`)
	b.eval(g, `document.querySelector("#bar button[value=changes]").click()`)
	b.waitFor(g, `location.pathname === "/"`)
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue())
	d, err := f.store.Decision(t.Context(), round)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(d.Verdict).To(Equal(store.VerdictChanges))
	g.Expect(d.Note).To(Equal("Name the control."))
}

// A large review reopens where it was left: the same card folded, the others
// open with their bodies loaded, at the same scroll. What was typed survives a
// reload too.
func TestServer_Surface_browserRemembers(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	for i := range 11 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", 40))
	}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	// Every write of a review's place, so a place written before the one saved
	// is applied is seen.
	b.call(g, "Page.enable", map[string]any{})
	b.call(g, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": `(() => {
		window.__placeWrites = [];
		const set = Storage.prototype.setItem;
		Storage.prototype.setItem = function (key, value) {
			if (key.startsWith("eyeball:place:")) window.__placeWrites.push(value);
			return set.call(this, key, value);
		};
	})()`})
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)

	card := `document.querySelector("details.file[data-path='d/05.txt']")`
	folded := `document.querySelector("details.file[data-path='d/07.txt']")`
	// Each skeleton holds its forty lines, so d/05.txt starts far below the
	// screen.
	b.eval(g, card+`.scrollIntoView()`)
	b.waitFor(g, card+`.querySelectorAll(".src").length === 40`)
	b.eval(g, folded+`.querySelector("summary").click()`)
	b.waitFor(g, `!`+folded+`.open`)
	b.eval(g, `window.scrollTo(0, 600)`)
	time.Sleep(400 * time.Millisecond)

	b.typeInto(g, ".page > form.composer textarea", "Half a review comment")
	b.typeInto(g, "#bar textarea", "A note in progress")
	b.eval(g, card+`.querySelector("tr[data-line='3']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "About line three")
	// Every card is open, so d/05.txt sits below 600 and focusing its composer
	// scrolled the page to it. The place is wherever the page was left.
	scroll := b.eval(g, `Math.round(window.scrollY)`)
	g.Expect(scroll).NotTo(BeEquivalentTo(0))

	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	b.waitFor(g, card+`.open && `+card+`.querySelectorAll(".src").length === 40`)
	g.Expect(b.eval(g, folded+`.open`)).To(BeFalse(), "a folded card opened again after a reload")
	// The place is stored as the template page stored it, under its key.
	key := itoa(id) + ":1:"
	g.Expect(b.eval(g, `JSON.parse(localStorage.getItem("eyeball:place:`+key+`")).value.folded.join()`)).To(Equal("d/07.txt"))
	b.waitFor(g, fmt.Sprintf(`Math.abs(window.scrollY - %v) < 2`, scroll))
	g.Expect(b.eval(g, `document.querySelector(".page > form.composer textarea").value`)).To(Equal("Half a review comment"))
	g.Expect(b.eval(g, `document.querySelector("#bar textarea").value`)).To(Equal("A note in progress"))
	b.waitFor(g, `document.querySelector("tr.thread form.composer textarea")?.value === "About line three"`)
	g.Expect(b.eval(g, `document.querySelector("tr.thread").previousElementSibling.dataset.line`)).To(Equal("3"))

	// A draft clears once its text is stored.
	b.eval(g, `document.querySelector(".page > form.composer button[type=submit]").click()`)
	b.waitFor(g, `document.querySelectorAll("#comments-review .comment").length === 1`)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	g.Expect(b.eval(g, `document.querySelector(".page > form.composer textarea").value`)).To(Equal(""))

	// A place saved before every card opened lists the open cards. Its scroll
	// is kept and its list ignored, so every card opens rather than all but
	// the one it named folding. Leaving a review page saves its place, so the
	// entry is planted from a page that keeps none.
	b.navigate(g, srv.URL+"/reviews/999")
	b.waitFor(g, `!!document.querySelector("#root .empty")`)
	b.eval(g, fmt.Sprintf(`localStorage.setItem("eyeball:place:%v", JSON.stringify({ at: Date.now(), value: { scroll: 300, open: ["d/05.txt"] } }))`, key))
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	b.waitFor(g, `Math.abs(window.scrollY - 300) < 2`)
	g.Expect(b.eval(g, `[...document.querySelectorAll("details.file")].every((d) => d.open)`)).To(BeTrue())

	// Nothing is saved while the place is being applied: the router's scroll to
	// the top and a page that is still too short would write over it. The
	// reader's first tap lets the next scroll save.
	time.Sleep(1 * time.Second)
	g.Expect(b.eval(g, `window.__placeWrites.length`)).To(BeEquivalentTo(0), "a place was written before the reader acted")
	b.eval(g, `document.dispatchEvent(new Event("pointerdown"))`)
	b.eval(g, `window.scrollTo(0, 700)`)
	b.waitFor(g, `window.__placeWrites.length > 0`)
	g.Expect(b.eval(g, `JSON.parse(window.__placeWrites.at(-1)).value.scroll`)).To(BeEquivalentTo(700))

	// A reader who does nothing is given three seconds, and the place is saved
	// as it moves after them. A scroll the page makes in the first of them is
	// not the reader's.
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	b.waitFor(g, `Math.abs(window.scrollY - 700) < 2`)
	b.eval(g, `window.scrollTo(0, 500)`)
	time.Sleep(600 * time.Millisecond)
	g.Expect(b.eval(g, `window.__placeWrites.length`)).To(BeEquivalentTo(0), "a place was written before the reader acted or the time ran out")
	time.Sleep(3 * time.Second)
	b.eval(g, `window.scrollTo(0, 600)`)
	b.waitFor(g, `window.__placeWrites.length > 0`)
	g.Expect(b.eval(g, `JSON.parse(window.__placeWrites.at(-1)).value.scroll`)).To(BeEquivalentTo(600))
}

// A review left by a move of the router saves its place as it goes, and the
// review opened again from the queue is put back there, from the cached view, in
// the same commit as the router's own scroll to the top.
func TestServer_Surface_browserRemembersByMove(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	for i := range 11 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", 40))
	}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	b.eval(g, `window.__before = true`)

	// The way back is taken as soon as the scroll has been seen, before the
	// scroll's own save is due, so only the save made as the screen goes keeps it.
	g.Expect(b.eval(g, `(async () => {
		window.scrollTo(0, 900);
		await new Promise((seen) => addEventListener("scroll", seen, { once: true }));
		document.querySelector(".topbar .back").click();
		return true;
	})()`)).To(BeTrue())
	b.waitFor(g, `location.pathname === "/" && !!document.querySelector("#root a.row")`)
	b.eval(g, `document.querySelector("#root a.row[href='/reviews/`+itoa(id)+`']").click()`)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	b.waitFor(g, `Math.abs(window.scrollY - 900) < 2`)
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "a page loaded")
}

// A saved place beyond what the review measures before its bodies come is
// reached as they arrive: a document is taller than its skeleton, so the page
// grows under the reader who was left far below.
func TestServer_Surface_browserRemembersPastTheSkeleton(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	for i := range 11 {
		write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", 40))
	}
	var paragraphs []string
	for n := range 200 {
		paragraphs = append(paragraphs, fmt.Sprintf("Paragraph %d of the manual.", n))
	}
	write(g, repo, "d/zz.md", strings.Join(paragraphs, "\n\n")+"\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	// The entry is planted from a page that keeps none.
	b.navigate(g, srv.URL+"/reviews/999#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root .empty")`)
	b.eval(g, fmt.Sprintf(`localStorage.setItem("eyeball:place:%d:1:", JSON.stringify({ at: Date.now(), value: { scroll: 14000, folded: [] } }))`, id))
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	b.waitFor(g, `Math.abs(window.scrollY - 14000) < 2`)
	g.Expect(b.eval(g, `!!document.querySelector("details.file[data-path='d/zz.md'] .blk")`)).To(BeTrue(), "the document's body had not arrived")
}

// A tap on an image in a card, or on the line naming a file with no
// rendering, opens a comment on the file. A tap on the Open link follows it.
func TestServer_Surface_browserRenderedCard(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "icon.png", pngOf(g, 2, 2))
	write(g, repo, "data.zip", "PK\x03\x04")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"icon.png", "data.zip"}, Note: "first"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	round := f.latestRound(g, t, id)

	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file[data-path='icon.png'] .rendered img")`)
	b.eval(g, `document.querySelector("details.file[data-path='icon.png'] .rendered img").click()`)
	b.waitFor(g, `!!document.querySelector("details.file[data-path='icon.png'] form.composer textarea")`)
	b.typeInto(g, "details.file[data-path='icon.png'] form.composer textarea", "Too dark.")
	b.eval(g, `document.querySelector("details.file[data-path='icon.png'] form.composer button[type=submit]").click()`)
	g.Eventually(func() []store.Comment { return f.comments(g, t, round) }, 5*time.Second).Should(HaveLen(1))
	c := f.comments(g, t, round)[0]
	g.Expect(c.Level).To(Equal(store.LevelFile))
	g.Expect(c.Path).To(Equal("icon.png"))

	b.waitFor(g, `!document.querySelector("details.file[data-path='icon.png'] form.composer")`)
	b.eval(g, `document.querySelector("details.file[data-path='data.zip'] .filefacts").click()`)
	b.waitFor(g, `!!document.querySelector("details.file[data-path='data.zip'] form.composer textarea")`)
}

// A fetch of the round that read the store before a write must not take the
// written comment off the screen when it arrives. The write's answer cancels the
// fetch in flight and fetches again, so the fetch shown starts after the store.
func TestServer_Surface_browserStaleFetch(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	inner := f.s.LoopbackSurface(browserToken)
	// The next fetch of the round answers with what it read, late.
	var slow atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/reviews/"+itoa(id) && slow.CompareAndSwap(true, false) {
			rec := httptest.NewRecorder()
			inner.ServeHTTP(rec, r)
			time.Sleep(1500 * time.Millisecond)
			for k, v := range rec.Header() {
				w.Header()[k] = v
			}
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
			return
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file") && document.getElementById("offline").hidden === true`)

	// A verdict given elsewhere moves the stamp, and the screen fetches the round,
	// which answers late with what the store held before the write below.
	slow.Store(true)
	f.decide(g, t, id, store.VerdictApprove, "")
	time.Sleep(500 * time.Millisecond)
	b.eval(g, `document.querySelector("details.file tr[data-line='2']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "Why two?")
	b.eval(g, `document.querySelector("tr.thread form.composer button[type=submit]").click()`)
	b.waitFor(g, `document.querySelectorAll("tr.thread .comment:not([data-sending]):not(.unsent)").length === 1`)

	// The late answer arrives, and the comment is still there, with the verdict
	// the fetch that followed the write brought.
	time.Sleep(2 * time.Second)
	g.Expect(b.eval(g, `document.querySelectorAll("tr.thread .comment").length`)).To(BeEquivalentTo(1), "a stale fetch took the comment off the screen")
	b.waitFor(g, `!document.querySelector("#bar")`)
	g.Expect(f.comments(g, t, f.latestRound(g, t, id))).To(HaveLen(1))
}

// A card's header stays under the page header while its file scrolls, and a
// fold from there keeps it at the top of the screen with the next file under
// it, rather than leaving the reader deep inside the next file.
func TestServer_Surface_browserStickyHeader(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := longFileReview(g, t, f)
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file")`)
	// The screen turns Chromium's scroll anchoring off for itself, which Safari
	// needs, and puts it back when it goes.
	g.Expect(b.eval(g, `document.documentElement.style.overflowAnchor`)).To(Equal("none"))

	long := `document.querySelector("details.file[data-path='d/long.txt']")`
	next := `document.querySelector("details.file[data-path='d/next.txt']")`
	head := `document.querySelector(".head").getBoundingClientRect().bottom`
	fromHead := func(card string) string {
		return `Math.round(Math.abs(` + card + `.querySelector("summary").getBoundingClientRect().top - ` + head + `))`
	}
	b.eval(g, `window.scrollTo(0, window.scrollY + `+long+`.getBoundingClientRect().top - `+head+` + 2000)`)
	g.Expect(b.eval(g, long+`.getBoundingClientRect().top < `+head+` - 1000`)).To(BeTrue(), "the card's top did not scroll far above the screen")
	g.Expect(b.eval(g, fromHead(long))).To(BeEquivalentTo(0), "the header scrolled away with its file")

	b.eval(g, long+`.querySelector("summary").click()`)
	b.waitFor(g, `!`+long+`.open`)
	time.Sleep(100 * time.Millisecond)
	// The folded card's top border sits at the header's bottom, a pixel above
	// its summary.
	g.Expect(b.eval(g, fromHead(long))).To(BeNumerically("<=", 1), "the folded header left the top of the screen")
	g.Expect(b.eval(g, next+`.getBoundingClientRect().top < window.innerHeight / 2`)).To(BeTrue(), "the next file is not under the folded header")

	// A fold the page makes, as a draft does around its composer, is not one the
	// reader made, and leaves the scroll where it is.
	b.eval(g, long+`.open = true`)
	b.waitFor(g, long+`.open`)
	b.eval(g, `window.scrollTo(0, window.scrollY + `+long+`.getBoundingClientRect().top - `+head+` + 2000)`)
	time.Sleep(100 * time.Millisecond)
	at := b.eval(g, `Math.round(window.scrollY)`)
	b.eval(g, long+`.open = false`)
	time.Sleep(100 * time.Millisecond)
	g.Expect(b.eval(g, `Math.round(window.scrollY)`)).To(Equal(at), "a fold the page made moved the page")
}

// The review follows its stream. A verdict given elsewhere takes the bar away
// and makes a composer late, but only once nothing typed would go with it: text
// in the review's box, in a composer, in the note or in an edit form holds the
// round as it is, whatever the change, until it is stored, sent or cleared.
// The server going away shows the notice and holds the verdict.
func TestServer_Surface_browserStream(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	other := actingReview(g, t, f)
	third := actingReview(g, t, f)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	g.Expect(err).NotTo(HaveOccurred())
	addr := ln.Addr().String()
	serve := func(l net.Listener) *http.Server {
		hs := &http.Server{Handler: f.s.LoopbackSurface(browserToken), ReadHeaderTimeout: 5 * time.Second}
		go func() { _ = hs.Serve(l) }()
		return hs
	}
	hs := serve(ln)
	t.Cleanup(func() { _ = hs.Close() })
	base := "http://" + addr
	b := openBrowser(g, t)
	bar := `!!document.querySelector("#bar")`

	// A verdict given elsewhere takes the bar away with no reload.
	b.navigate(g, base+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, bar+` && document.getElementById("offline").hidden === true`)
	b.eval(g, `window.__before = true`)
	f.decide(g, t, id, store.VerdictChanges, "From another device.")
	b.waitFor(g, `!document.querySelector("#bar") && document.querySelector(".page").dataset.acting === "late"`)
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "the page reloaded")

	// Text in the review's box holds the change back until it is gone.
	b.navigate(g, base+"/reviews/"+itoa(other))
	b.waitFor(g, bar)
	b.typeInto(g, ".page > form.composer textarea", "Half written")
	f.decide(g, t, other, store.VerdictApprove, "")
	time.Sleep(500 * time.Millisecond)
	g.Expect(b.eval(g, bar)).To(BeTrue(), "the bar went with text in the review's box")
	g.Expect(b.eval(g, `document.querySelector(".page > form.composer textarea").value`)).To(Equal("Half written"))
	b.typeInto(g, ".page > form.composer textarea", "")
	b.waitFor(g, `!document.querySelector("#bar")`)

	// The note alone holds it, and is back to nothing once it is cleared.
	fifth := actingReview(g, t, f)
	b.navigate(g, base+"/reviews/"+itoa(fifth))
	b.waitFor(g, bar)
	b.typeInto(g, "#bar textarea", "Only a note")
	f.decide(g, t, fifth, store.VerdictApprove, "")
	time.Sleep(500 * time.Millisecond)
	g.Expect(b.eval(g, `document.querySelector("#bar textarea")?.value`)).To(Equal("Only a note"), "the note went with the bar")
	b.typeInto(g, "#bar textarea", "")
	b.waitFor(g, `!document.querySelector("#bar")`)

	// A composer holds it as well. Neither loses its text, and the composer turns
	// late when the change comes.
	b.navigate(g, base+"/reviews/"+itoa(third))
	b.waitFor(g, bar)
	b.typeInto(g, "#bar textarea", "A note in progress")
	b.eval(g, `document.querySelector("details.file tr[data-line='2']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "About line two")
	f.decide(g, t, third, store.VerdictApprove, "")
	time.Sleep(500 * time.Millisecond)
	g.Expect(b.eval(g, `document.querySelector("#bar textarea").value`)).To(Equal("A note in progress"), "the note went with the bar")
	g.Expect(b.eval(g, `document.querySelector("tr.thread form.composer textarea").value`)).To(Equal("About line two"))
	g.Expect(b.eval(g, `document.querySelector("tr.thread form.composer").textContent.includes("Sends immediately")`)).To(BeFalse(), "the composer turned late under the reader")
	// Clearing the note is not enough while the composer holds text.
	b.typeInto(g, "#bar textarea", "")
	time.Sleep(300 * time.Millisecond)
	g.Expect(b.eval(g, bar)).To(BeTrue())
	// Sent, the comment is stored as the late one it is, and the round changes
	// once nothing is held.
	b.eval(g, `document.querySelector("tr.thread form.composer button[type=submit]").click()`)
	b.waitFor(g, `!document.querySelector("#bar") && document.querySelector(".page").dataset.acting === "late"`)
	b.waitFor(g, `document.querySelectorAll("tr.thread .comment:not([data-sending]):not(.unsent)").length === 1`)
	comments := f.comments(g, t, f.latestRound(g, t, third))
	g.Expect(comments).To(HaveLen(1))
	g.Expect(comments[0].Body).To(Equal("About line two"))

	// The server going away shows the notice and holds the verdict, and its
	// return clears both.
	fourth := actingReview(g, t, f)
	b.navigate(g, base+"/reviews/"+itoa(fourth))
	b.waitFor(g, bar)
	g.Expect(hs.Close()).To(Succeed())
	b.waitFor(g, `document.getElementById("offline").hidden === false`)
	b.waitFor(g, `!!document.getElementById("whynot")`)
	g.Expect(b.eval(g, `document.querySelector("#whynot .why-not").textContent`)).To(Equal("You can\u2019t submit a review while offline."))
	g.Expect(b.eval(g, `document.querySelector("#bar button[value=approve]").getAttribute("aria-disabled")`)).To(Equal("true"))
	// A verdict tried now is held back, not sent into a daemon that is not there.
	b.eval(g, `document.querySelector("#bar button[value=approve]").click()`)
	time.Sleep(300 * time.Millisecond)
	g.Expect(b.eval(g, `!!document.querySelector("#bar .failed")`)).To(BeFalse(), "a verdict was sent while offline")
	ln, err = net.Listen("tcp", addr)
	g.Expect(err).NotTo(HaveOccurred())
	hs = serve(ln)
	b.waitFor(g, `document.getElementById("offline").hidden === true`)
	b.waitFor(g, `document.querySelector("#bar button[value=approve]").getAttribute("aria-disabled") === null`)
}

// A fetch of the round that fails, even after the query's own retries, is asked
// for again, since the stream sends the stamp once and the stamp already asked
// for is not asked for twice.
func TestServer_Surface_browserStreamFailedFetch(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	inner := f.s.LoopbackSurface(browserToken)
	// The fetch and its three retries fail, and the fetches after them do not.
	var failing atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/reviews/"+itoa(id) && failing.Add(-1) >= 0 {
			http.Error(w, "the store is gone", http.StatusServiceUnavailable)
			return
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file") && document.getElementById("offline").hidden === true`)
	failing.Store(4)
	f.decide(g, t, id, store.VerdictApprove, "")
	g.Eventually(func() any { return b.eval(g, `!document.querySelector("#bar")`) }, 30*time.Second, 200*time.Millisecond).Should(Equal(true), "a fetch that failed was never asked for again")
}

// A fetch that is lost with the connection is asked for again when the stream
// comes back and sends the stamp it sent before.
func TestServer_Surface_browserStreamReconnect(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	inner := f.s.LoopbackSurface(browserToken)
	// A blip takes the open streams, refuses new ones, and leaves the next fetch
	// of the round unanswered until the test ends.
	var mu sync.Mutex
	var cancels []context.CancelFunc
	blip := false
	hang := make(chan struct{})
	var hung atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/events") {
			mu.Lock()
			if blip {
				mu.Unlock()
				// The connection is dropped. An answer with a status would end the
				// stream for good, where a lost connection is retried.
				panic(http.ErrAbortHandler)
			}
			ctx, cancel := context.WithCancel(r.Context())
			cancels = append(cancels, cancel)
			mu.Unlock()
			inner.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/reviews/"+itoa(id) && hung.CompareAndSwap(true, false) {
			select {
			case <-hang:
			case <-r.Context().Done():
			}
			return
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(hang) })
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root details.file") && document.getElementById("offline").hidden === true`)

	hung.Store(true)
	f.decide(g, t, id, store.VerdictApprove, "")
	time.Sleep(500 * time.Millisecond)
	mu.Lock()
	blip = true
	for _, cancel := range cancels {
		cancel()
	}
	mu.Unlock()
	b.waitFor(g, `document.getElementById("offline").hidden === false`)
	mu.Lock()
	blip = false
	mu.Unlock()
	b.waitFor(g, `document.getElementById("offline").hidden === true`)
	g.Eventually(func() any { return b.eval(g, `!document.querySelector("#bar")`) }, 10*time.Second, 200*time.Millisecond).Should(Equal(true), "a fetch lost with the connection was never asked for again, or was asked for behind the one that never answered")
}

// loadWorkerInPage runs the real worker script in the page's own scope, with
// a fake of the clients the tap sees: one window the worker does not control,
// whose navigate is refused as Chromium refuses it, which keeps what is posted
// to it without answering, and which openWindow returns. A page's window takes the notificationclick a test dispatches, since
// DevTools cannot tap a notification. window.tap(url) dispatches one and waits
// for every promise handed to waitUntil, as the worker's lifetime does, and
// rejects with the first failure once all have settled.
func (b *browser) loadWorkerInPage(g *WithT) {
	g.THelper()
	b.eval(g, `(async () => {
		const w = {
			url: location.origin + "/",
			visibilityState: "visible",
			focused: true,
			navigate: () => Promise.reject(new TypeError("This service worker is not the client's active service worker.")),
			focus: () => Promise.resolve(w),
			postMessage: (message) => { window.posted.push(message); },
		};
		window.fake = w;
		window.posted = [];
		window.opened = [];
		window.clients = {
			matchAll: (o) => Promise.resolve(o && o.includeUncontrolled ? [w] : []),
			openWindow: (u) => { window.opened.push(u); return Promise.resolve(w); },
			claim: () => Promise.resolve(),
		};
		await new Promise((ok, no) => {
			const s = document.createElement("script");
			s.src = "/sw.js";
			s.onload = ok;
			s.onerror = no;
			document.head.append(s);
		});
		window.tap = async (url) => {
			const waits = [];
			const ev = new Event("notificationclick");
			ev.notification = { close() {}, data: url };
			ev.waitUntil = (p) => { waits.push(p); };
			window.dispatchEvent(ev);
			let failure;
			for (let i = 0; i < waits.length; i++) {
				try {
					await waits[i];
				} catch (e) {
					failure ??= e;
				}
			}
			if (failure) {
				throw failure;
			}
			return true;
		};
		return true;
	})()`)
}

// beaconsIn reads the beacons out of the server's log, in the order the
// worker numbered them.
func beaconsIn(g *WithT, log string) []map[string]any {
	g.THelper()
	var out []map[string]any
	for _, m := range regexp.MustCompile(`beacon=("(?:[^"\\]|\\.)*")`).FindAllStringSubmatch(log, -1) {
		raw, err := strconv.Unquote(m[1])
		g.Expect(err).NotTo(HaveOccurred())
		var one map[string]any
		g.Expect(json.Unmarshal([]byte(raw), &one)).To(Succeed())
		out = append(out, one)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i]["n"].(float64) < out[j]["n"].(float64) })
	return out
}

// A tap's worker reports to the daemon: where it started, with every window it
// can see, then each step it takes and what the step resolved to or how it
// failed. The beacons come from the real worker script, through the tailnet
// listener's route, and the stamp they carry is the one the daemon serves. Each
// branch of openReview is reached once, and a step that fails fails the tap as
// it did before the beacons.
func TestServer_Surface_browserTapBeacons(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	out := f.logInto()
	srv := httptest.NewServer(f.s.Surface())
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/")
	b.loadWorkerInPage(g)
	stamp, err := eyeball.WorkerStamp()
	g.Expect(err).NotTo(HaveOccurred())
	window := map[string]any{"url": srv.URL + "/", "visibilityState": "visible", "focused": true}
	refused := map[string]any{
		"name":    "TypeError",
		"message": "This service worker is not the client's active service worker.",
	}

	// tap sends one tap and returns how it settled and the beacons it logged.
	tap := func(g *WithT, url string) (any, []map[string]any) {
		g.THelper()
		before := len(out.String())
		settled := b.eval(g, `window.tap(`+strconv.Quote(url)+`).then(() => "settled", (e) => e.name + ": " + e.message)`)
		return settled, beaconsIn(g, out.String()[before:])
	}
	// steps names each beacon by its step and phase.
	steps := func(beacons []map[string]any) []string {
		var named []string
		for _, one := range beacons {
			step, _ := one["step"].(string)
			phase, _ := one["phase"].(string)
			named = append(named, strings.TrimSpace(step+" "+phase))
		}
		return named
	}

	t.Run("a window that refuses navigate", func(t *testing.T) {
		g := NewWithT(t)
		settled, beacons := tap(g, "/reviews/9")
		g.Expect(settled).To(Equal("settled"))
		g.Expect(b.eval(g, `window.opened`)).To(Equal([]any{"/reviews/9"}), "the tap still opened the window")
		for _, one := range beacons {
			g.Expect(one["build"]).To(Equal(stamp), "the worker's stamp is the one served")
		}
		g.Expect(steps(beacons)).To(Equal([]string{"start", "post", "answer", "navigate call", "navigate failed", "openWindow call", "openWindow done"}))
		g.Expect(beacons[0]["url"]).To(Equal("/reviews/9"))
		g.Expect(beacons[0]["windows"]).To(Equal([]any{window}))
		g.Expect(beacons[0]["controlled"]).To(Equal([]any{}))
		g.Expect(beacons[2]["answered"]).To(BeFalse())
		g.Expect(beacons[2]["ms"]).To(BeNumerically(">=", 900), "the wait is a second")
		g.Expect(beacons[4]["error"]).To(Equal(refused))
		g.Expect(beacons[6]["result"]).To(Equal(srv.URL + "/"))
		g.Expect(out.String()).To(ContainSubstring("served=" + stamp))
	})

	t.Run("a window already on the review", func(t *testing.T) {
		g := NewWithT(t)
		b.eval(g, `window.fake.url = location.origin + "/reviews/9"`)
		settled, beacons := tap(g, "/reviews/9")
		g.Expect(settled).To(Equal("settled"))
		g.Expect(steps(beacons)).To(Equal([]string{"start", "focus call", "focus done"}))
		g.Expect(beacons[2]["result"]).To(Equal(srv.URL + "/reviews/9"))
	})

	t.Run("a window that navigates", func(t *testing.T) {
		g := NewWithT(t)
		b.eval(g, `window.fake.url = location.origin + "/"; window.fake.navigate = () => Promise.resolve(window.fake)`)
		settled, beacons := tap(g, "/reviews/10")
		g.Expect(settled).To(Equal("settled"))
		g.Expect(steps(beacons)).To(Equal([]string{
			"start",
			"post",
			"answer",
			"navigate call",
			"navigate done",
			"focusAfterNavigate call",
			"focusAfterNavigate done",
		}))
	})

	t.Run("a navigate that resolves to nothing", func(t *testing.T) {
		g := NewWithT(t)
		b.eval(g, `window.fake.navigate = () => Promise.resolve(null)`)
		settled, beacons := tap(g, "/reviews/11")
		g.Expect(settled).To(Equal("settled"))
		g.Expect(steps(beacons)).To(Equal([]string{"start", "post", "answer", "navigate call", "navigate done", "openWindow call", "openWindow done"}))
		g.Expect(beacons[4]).To(HaveKeyWithValue("result", BeNil()))
	})

	t.Run("an openWindow that fails", func(t *testing.T) {
		g := NewWithT(t)
		b.eval(g, `window.clients.openWindow = () => Promise.reject(new Error("no window"))`)
		settled, beacons := tap(g, "/reviews/12")
		g.Expect(settled).To(Equal("Error: no window"), "the tap fails as it would without the beacons")
		g.Expect(steps(beacons)).To(Equal([]string{"start", "post", "answer", "navigate call", "navigate done", "openWindow call", "openWindow failed"}))
		g.Expect(beacons[6]["error"]).To(Equal(map[string]any{"name": "Error", "message": "no window"}))
	})

	t.Run("a window that answers", func(t *testing.T) {
		g := NewWithT(t)
		b.eval(g, `window.fake.postMessage = (m, ports) => { window.posted.push(m); ports[0].postMessage("moving"); }`)
		b.eval(g, `window.clients.openWindow = (u) => { window.opened.push(u); return Promise.resolve(window.fake); }; window.posted = []`)
		settled, beacons := tap(g, "/reviews/13")
		g.Expect(settled).To(Equal("settled"))
		g.Expect(b.eval(g, `window.posted`)).To(Equal([]any{map[string]any{"url": "/reviews/13"}}))
		g.Expect(steps(beacons)).To(Equal([]string{"start", "post", "answer", "focusAfterAnswer call", "focusAfterAnswer done"}))
		g.Expect(beacons[2]["answered"]).To(BeTrue())
		g.Expect(beacons[2]["ms"]).To(BeNumerically("<", 900))
	})
}

// A beacon the loopback listener refuses, and one whose request fails, leave
// the tap opening the window all the same. A worker holds no token, so on
// loopback every beacon is refused with 403.
func TestServer_Surface_browserTapBeaconsLost(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	out := f.logInto()
	seen := &statusLog{}
	srv := httptest.NewServer(seen.wrap(f.s.LoopbackSurface(browserToken)))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/")
	b.loadWorkerInPage(g)

	g.Expect(b.eval(g, `window.tap("/reviews/9")`)).To(BeTrue())
	g.Expect(b.eval(g, `window.opened`)).To(Equal([]any{"/reviews/9"}))
	g.Expect(seen.lines()).To(ContainElement("POST /taps 403"))
	g.Expect(out.String()).NotTo(ContainSubstring("msg=tap"))

	b.eval(g, `window.fetch = () => Promise.reject(new TypeError("Failed to fetch"))`)
	g.Expect(b.eval(g, `window.tap("/reviews/10")`)).To(BeTrue())
	g.Expect(b.eval(g, `window.opened`)).To(Equal([]any{"/reviews/9", "/reviews/10"}))
}

// A tap while the React app is open on the queue is answered by the page, which
// loads the review itself, so the worker never calls navigate. The fake window
// hands the worker's message to the page's own listener, and the beacons show
// which branch ran.
func TestServer_Surface_browserTapMovesApp(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	out := f.logInto()
	id := actingReview(g, t, f)
	srv := httptest.NewServer(f.s.Surface())
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/")
	b.waitFor(g, `!!document.querySelector("#root .scroll")`)
	b.loadWorkerInPage(g)
	b.eval(g, `window.fake.postMessage = (message, ports) => navigator.serviceWorker.dispatchEvent(new MessageEvent("message", { data: message, ports }))`)

	// The page leaves while the tap is still settling, so the call is not
	// awaited.
	b.eval(g, `(() => { window.tap("/reviews/`+itoa(id)+`"); return true; })()`)
	b.waitFor(g, `location.pathname === "/reviews/`+itoa(id)+`" && !!document.querySelector("#bar")`)
	g.Eventually(func() string { return out.String() }, 5*time.Second, 50*time.Millisecond).Should(ContainSubstring(`step\":\"post`))
	g.Expect(out.String()).NotTo(ContainSubstring("navigate"), "the worker navigated a window that answered")
}

// A window with no listener is given the address and gets navigate once the
// second is up.
func TestServer_Surface_browserTapNoListener(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	out := f.logInto()
	srv := httptest.NewServer(f.s.Surface())
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/")
	b.loadWorkerInPage(g)
	b.eval(g, `window.navigations = []; window.fake.navigate = (u) => { window.navigations.push(u); return Promise.resolve(window.fake); }`)

	g.Expect(b.eval(g, `window.tap("/reviews/9")`)).To(BeTrue())
	g.Expect(b.eval(g, `window.posted`)).To(Equal([]any{map[string]any{"url": "/reviews/9"}}))
	g.Expect(b.eval(g, `window.navigations`)).To(Equal([]any{"/reviews/9"}))
	var named []string
	for _, one := range beaconsIn(g, out.String()) {
		step, _ := one["step"].(string)
		phase, _ := one["phase"].(string)
		named = append(named, strings.TrimSpace(step+" "+phase))
	}
	g.Expect(named).To(Equal([]string{"start", "post", "answer", "navigate call", "navigate done", "focusAfterNavigate call", "focusAfterNavigate done"}))
}

// A page served by an older build says so once the stream hears a newer one,
// and Refresh loads the page from the build running now.
func TestServer_Surface_browserUpdate(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	g.Expect(err).NotTo(HaveOccurred())
	addr := ln.Addr().String()
	serve := func(l net.Listener) *http.Server {
		hs := &http.Server{Handler: f.s.LoopbackSurface(browserToken), ReadHeaderTimeout: 5 * time.Second}
		go func() { _ = hs.Serve(l) }()
		return hs
	}
	hs := serve(ln)
	b := openBrowser(g, t)
	b.navigate(g, "http://"+addr+"/")
	b.waitFor(g, `!!document.getElementById("update")`)
	g.Expect(b.eval(g, `!!document.querySelector("script[src^='/static/react.js']")`)).To(BeTrue(), "the queue is the React app, whose notices the template pages' do not stand in for")
	g.Expect(b.eval(g, `document.getElementById("update").hidden`)).To(BeTrue(), "the page is the running build")

	// A daemon restarted on a newer build: the stream reconnects and hears it.
	g.Expect(hs.Close()).To(Succeed())
	f.s.daemon.Build = "newer"
	ln, err = net.Listen("tcp", addr)
	g.Expect(err).NotTo(HaveOccurred())
	hs = serve(ln)
	t.Cleanup(func() { _ = hs.Close() })
	b.waitFor(g, `document.getElementById("update").hidden === false`)

	b.eval(g, `window.__before = true`)
	b.eval(g, `document.querySelector("#update .refresh").click()`)
	b.waitFor(g, `window.__before === undefined && document.readyState === "complete"`)
	b.waitFor(g, `!!document.getElementById("update")`)
	g.Expect(b.eval(g, `document.querySelector("meta[name='eyeball-build']").content`)).To(Equal("newer"))
	g.Expect(b.eval(g, `document.getElementById("update").hidden`)).To(BeTrue())
}

// The stream announces a verdict as soon as it is stored, which can be before
// the answer to the request that gave it. The screen does not take up the
// decided round in that gap, since the router is about to leave it, and does not
// refetch it either.
func TestServer_Surface_browserVerdictInFlight(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	inner := f.s.LoopbackSurface(browserToken)
	// The verdict's answer is held back until the stream has had time to say it
	// was stored.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/decision") {
			rec := httptest.NewRecorder()
			inner.ServeHTTP(rec, r)
			time.Sleep(900 * time.Millisecond)
			for k, v := range rec.Header() {
				w.Header()[k] = v
			}
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
			return
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root #bar")`)
	b.eval(g, `document.querySelector("#bar button[value=approve]").click()`)
	time.Sleep(500 * time.Millisecond)
	_, err := f.store.Decision(t.Context(), f.latestRound(g, t, id))
	g.Expect(err).NotTo(HaveOccurred(), "the verdict was not stored yet")
	g.Expect(b.eval(g, `!!document.querySelector("#bar") && document.querySelector(".page").dataset.acting === "open"`)).To(BeTrue(), "the screen took up the decided round before the verdict was answered")
	b.waitFor(g, `location.pathname === "/"`)
}

// A visual viewport a keyboard's height short of the window, with nothing
// focused, is the fault iOS leaves after a notification is tapped over another
// app's keyboard. The page answers the next tap by focusing and blurring a
// hidden field, which has iOS show a keyboard and take it away, and stops
// after two. Chromium has no such fault, so the test makes the viewport report
// it.
func TestServer_Surface_browserViewport(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "short"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root .page")`)

	// Counted as the hidden field arrives. A headless page without focus
	// fires no focus events.
	b.eval(g, `window.flicks = 0; new MutationObserver((records) => { for (const r of records) for (const n of r.addedNodes) if (n.dataset && "flick" in n.dataset) window.flicks++ }).observe(document.body, {childList: true})`)
	tap := `document.dispatchEvent(new Event("click"))`
	resize := `window.visualViewport.dispatchEvent(new Event("resize"))`
	b.eval(g, tap)
	g.Expect(b.eval(g, `window.flicks`)).To(BeEquivalentTo(0), "a whole viewport is left alone")

	b.eval(g, `Object.defineProperty(window.visualViewport, "height", {configurable: true, get: () => window.innerHeight - 318})`)
	b.eval(g, resize)
	g.Expect(b.eval(g, `window.flicks`)).To(BeEquivalentTo(0), "nothing is focused before a tap")
	// Tap and resize every 100ms, as a reader tapping through a keyboard
	// that comes and goes. A reset is given three seconds to settle, so the
	// second comes after that, and there is no third.
	b.eval(g, `window.tapping = setInterval(() => { `+tap+`; `+resize+` }, 100)`)
	g.Expect(b.eval(g, `new Promise((done) => setTimeout(() => done(window.flicks), 1000))`)).To(BeEquivalentTo(1), "one reset while the first settles")
	b.waitFor(g, `window.flicks === 2`)
	g.Expect(b.eval(g, `new Promise((done) => setTimeout(() => done(window.flicks), 3500))`)).To(BeEquivalentTo(2), "two tries and no more")
	b.eval(g, `clearInterval(window.tapping)`)
	g.Expect(b.eval(g, `document.querySelector("[data-flick]") === null`)).To(BeTrue(), "the hidden field is removed")

	// A return to the foreground with no resize, as when the page was left
	// open while another app's keyboard was up, gives fresh tries.
	b.eval(g, `document.dispatchEvent(new Event("visibilitychange"))`)
	b.eval(g, tap)
	b.waitFor(g, `window.flicks === 3`)
}

// Before the reader's first touch, a short viewport that has slid down inside
// the layout viewport is scrolled past the reader's place by the shortfall and
// back, which on the phone puts the header and the bar at the screen's edges.
// After a touch the page leaves scrolling to the reader. Chromium has no such
// fault, so the test makes the viewport report it and records each scroll.
func TestServer_Surface_browserViewportRealign(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := longFileReview(g, t, f)
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root .page")`)

	b.eval(g, `window.scrollTo(0, 2000)`)
	g.Expect(b.eval(g, `Math.round(window.scrollY)`)).To(BeEquivalentTo(2000))
	b.eval(g, `window.scrolls = []; const scroll = window.scrollTo.bind(window); window.scrollTo = (x, y) => { window.scrolls.push(Math.round(y)); scroll(x, y) }`)
	b.eval(g, `Object.defineProperty(window.visualViewport, "height", {configurable: true, get: () => window.innerHeight - 318})`)
	b.eval(g, `Object.defineProperty(window.visualViewport, "offsetTop", {configurable: true, get: () => 318})`)
	b.eval(g, `window.visualViewport.dispatchEvent(new Event("resize"))`)
	b.waitFor(g, `window.scrolls.length === 4`)
	g.Expect(b.eval(g, `window.scrolls`)).To(Equal([]any{2318.0, 2000.0, 2318.0, 2000.0}))

	// The touch also resets the viewport, which holds every check for three
	// seconds while it settles, so the scroll comes after that.
	b.eval(g, `document.dispatchEvent(new Event("touchstart"))`)
	g.Expect(b.eval(g, `new Promise((done) => setTimeout(() => { window.dispatchEvent(new Event("scroll")); setTimeout(() => done(window.scrolls.length), 200) }, 3200))`)).To(BeEquivalentTo(4), "no realign after a touch")
}

// The first touch resets a short viewport as a tap does, since a reader's first
// gesture is usually a scroll. A short viewport just after a field loses focus
// is a keyboard on its way down, and is left alone until it has had time to go.
func TestServer_Surface_browserViewportTouch(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "short"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root .page")`)

	b.eval(g, `window.flicks = 0; new MutationObserver((records) => { for (const r of records) for (const n of r.addedNodes) if (n.dataset && "flick" in n.dataset) window.flicks++ }).observe(document.body, {childList: true})`)
	touch := `document.dispatchEvent(new Event("touchstart"))`
	b.eval(g, `Object.defineProperty(window.visualViewport, "height", {configurable: true, get: () => window.innerHeight - 318})`)
	b.eval(g, `document.dispatchEvent(new Event("focusout"))`)
	b.eval(g, `window.visualViewport.dispatchEvent(new Event("resize"))`)
	b.eval(g, touch)
	g.Expect(b.eval(g, `window.flicks`)).To(BeEquivalentTo(0), "a keyboard going away is not the fault")
	// The page looks again once the keyboard has had its second, by itself.
	g.Expect(b.eval(g, `new Promise((done) => setTimeout(() => { `+touch+`; setTimeout(() => done(window.flicks), 100) }, 1100))`)).To(BeEquivalentTo(1), "the first touch after it resets")
}

// The page registers its service worker, scoped to the whole surface, which
// push needs.
func TestServer_Surface_browserWorker(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/")
	b.waitFor(g, `navigator.serviceWorker.getRegistration().then(r => !!r && !!r.active)`)
	g.Expect(b.eval(g, `navigator.serviceWorker.getRegistration().then(r => r.scope)`)).To(Equal(srv.URL + "/"))
}

// Two answers to comments on the review that land in one tick are both kept even
// while the round on screen is held, by a note being typed, from taking up the
// cached round that has both.
func TestServer_Surface_browserWritesTogether(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.waitFor(g, `!!document.querySelector("#root #bar")`)
	b.typeInto(g, "#bar textarea", "A note in progress")
	b.eval(g, `(() => {
		const real = window.fetch.bind(window);
		window.__held = [];
		window.fetch = (url, init) => {
			if (init && init.method === "POST" && String(url).endsWith("/comments")) {
				return new Promise((resolve, reject) => {
					real(url, init).then((res) => window.__held.push(() => resolve(res)), reject);
				});
			}
			return real(url, init);
		};
		window.__flush = () => window.__held.splice(0).forEach((go) => go());
		return true;
	})()`)
	words := []string{"First.", "Second."}
	for _, word := range words {
		b.typeInto(g, ".page > form.composer textarea", word)
		b.eval(g, `document.querySelector(".page > form.composer button[type=submit]").click()`)
		b.waitFor(g, `document.querySelector(".page > form.composer textarea").value === ""`)
	}
	b.waitFor(g, `window.__held.length === 2`)
	b.eval(g, `window.__flush()`)
	time.Sleep(500 * time.Millisecond)
	g.Expect(b.eval(g, `document.querySelectorAll("#comments-review .comment:not(.unsent):not([data-sending])").length`)).To(BeEquivalentTo(2), "an answer built on a stale round dropped the other")
}

// longFileReview opens a review of two files of 300 changed lines each,
// d/long.txt and d/next.txt. Each is several screens long, so the page can
// scroll far into either, and still far once d/long.txt is folded.
func longFileReview(g *WithT, t *testing.T, f *fixture) int64 {
	t.Helper()
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	var lines []string
	for n := 1; n <= 300; n++ {
		lines = append(lines, fmt.Sprintf("line %d", n))
	}
	write(g, repo, "d/long.txt", strings.Join(lines, "\n")+"\n")
	write(g, repo, "d/next.txt", strings.Repeat("next\n", 300))
	return f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "long"}).Review.ID
}

// requestLog records every request line and Referer the server receives.
type requestLog struct {
	mu   sync.Mutex
	seen []string
}

func (l *requestLog) wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		l.seen = append(l.seen, r.RequestURI, r.Header.Get("Referer"))
		l.mu.Unlock()
		h.ServeHTTP(w, r)
	})
}

func (l *requestLog) lines() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.seen...)
}

// statusLog records each request's method, path and status.
type statusLog struct {
	mu   sync.Mutex
	seen []string
}

func (l *statusLog) wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rec, r)
		l.mu.Lock()
		l.seen = append(l.seen, fmt.Sprintf("%s %s %d", r.Method, r.URL.Path, rec.status))
		l.mu.Unlock()
	})
}

func (l *statusLog) lines() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.seen...)
}

// statusWriter keeps the status a handler writes.
type statusWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader keeps the status and passes it on.
func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// browser is one headless Chromium page, driven over the DevTools protocol.
// The protocol's Runtime.evaluate runs outside the page's content policy, so
// a test reads and taps the page as a person would without the page carrying
// any script of the test's.
type browser struct {
	ws *websocket.Conn
	id int
}

// openBrowser starts headless Chromium and attaches to its first page.
func openBrowser(g *WithT, t *testing.T) *browser {
	g.THelper()
	bin, err := exec.LookPath("chromium")
	if err != nil {
		t.Skip("chromium is not installed")
	}
	// Chromium's helper processes outlive its main one and keep writing to
	// the profile, so the profile is not t.TempDir, whose cleanup fails on a
	// directory still being written, and the whole process group is killed.
	dir, err := os.MkdirTemp("", "eb-chromium-")
	g.Expect(err).NotTo(HaveOccurred())
	cmd := exec.Command(
		bin,
		"--headless=new",
		"--remote-debugging-port=0",
		"--user-data-dir="+dir,
		"--remote-allow-origins=http://127.0.0.1",
		// Everything but this machine goes to a proxy that is not there, so a
		// test can reach no service on the internet; Chromium bypasses a proxy
		// for loopback by itself. Without it, Chromium registered a real
		// subscription with Google's push service when a stub failed to
		// apply. A host resolver rule blocked it too, and slowed a page's
		// own loads past what the place test allows.
		"--proxy-server=http://127.0.0.1:9",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-gpu",
		"about:blank",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	g.Expect(cmd.Start()).To(Succeed())
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
		for range 20 {
			if os.RemoveAll(dir) == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
	var port string
	g.Eventually(func() error {
		raw, err := os.ReadFile(filepath.Join(dir, "DevToolsActivePort"))
		if err != nil {
			return err
		}
		port, _, _ = strings.Cut(string(raw), "\n")
		return nil
	}, 10*time.Second, 50*time.Millisecond).Should(Succeed())
	var targets []struct {
		Type string `json:"type"`
		URL  string `json:"webSocketDebuggerUrl"`
	}
	g.Eventually(func() error {
		res, err := http.Get("http://127.0.0.1:" + port + "/json/list")
		if err != nil {
			return err
		}
		defer func() { _ = res.Body.Close() }()
		return json.NewDecoder(res.Body).Decode(&targets)
	}, 10*time.Second, 50*time.Millisecond).Should(Succeed())
	var page string
	for _, tg := range targets {
		if tg.Type == "page" {
			page = tg.URL
		}
	}
	g.Expect(page).NotTo(BeEmpty())
	ws, err := websocket.Dial(page, "", "http://127.0.0.1")
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = ws.Close() })
	return &browser{ws: ws}
}

// call sends one protocol command and returns its result, skipping the events
// the page sends meanwhile.
func (b *browser) call(g *WithT, method string, params any) json.RawMessage {
	g.THelper()
	b.id++
	g.Expect(websocket.JSON.Send(b.ws, map[string]any{"id": b.id, "method": method, "params": params})).To(Succeed())
	for {
		var msg struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		g.Expect(websocket.JSON.Receive(b.ws, &msg)).To(Succeed())
		if msg.ID != b.id {
			continue
		}
		g.Expect(msg.Error).To(BeNil(), method)
		return msg.Result
	}
}

// eval runs an expression in the page and returns its value. An exception in
// the page fails the test at once.
func (b *browser) eval(g *WithT, expr string) any {
	g.THelper()
	value, thrown := b.tryEval(g, expr)
	g.Expect(thrown).To(BeEmpty(), expr)
	return value
}

// tryEval runs an expression and returns its value and the text of the
// exception it threw, which is empty when it threw none.
func (b *browser) tryEval(g *WithT, expr string) (any, string) {
	g.THelper()
	var out struct {
		Result struct {
			Value any `json:"value"`
		} `json:"result"`
		Exception *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	raw := b.call(g, "Runtime.evaluate", map[string]any{"expression": expr, "awaitPromise": true, "returnByValue": true})
	g.Expect(json.Unmarshal(raw, &out)).To(Succeed())
	if out.Exception != nil {
		thrown := out.Exception.Exception.Description
		if thrown == "" {
			thrown = out.Exception.Text
		}
		return out.Result.Value, thrown
	}
	return out.Result.Value, ""
}

// waitFor polls an expression until it is true, in real time: the page's
// requests and the server's answers take as long as they take. An exception is
// not yet: an expression that reads a property of an element the page has not
// drawn throws until it does. The last one is in the failure message.
func (b *browser) waitFor(g *WithT, expr string) {
	g.THelper()
	var thrown string
	g.Eventually(func() any {
		var value any
		value, thrown = b.tryEval(g, expr)
		return value
	}, 5*time.Second, 50*time.Millisecond).Should(Equal(true), func() string {
		return expr + "\nlast exception: " + thrown
	})
}

// navigate opens a page and waits for it to load.
func (b *browser) navigate(g *WithT, url string) {
	g.THelper()
	b.call(g, "Page.navigate", map[string]any{"url": url})
	b.waitFor(g, `document.readyState === "complete" && location.href !== "about:blank"`)
}

// typeInto sets a field's text and fires input, as typing does.
func (b *browser) typeInto(g *WithT, selector, text string) {
	g.THelper()
	quoted, err := json.Marshal(text)
	g.Expect(err).NotTo(HaveOccurred())
	// The element's own setter, since React keeps the value it last saw and a
	// field set the plain way reads as unchanged, so its input event is dropped.
	b.eval(g, fmt.Sprintf(`(() => { const f = document.querySelector(%q); Object.getOwnPropertyDescriptor(f.constructor.prototype, "value").set.call(f, %s); f.dispatchEvent(new Event("input", { bubbles: true })); })()`, selector, quoted))
}

// fillQueue opens count reviews of one-line files in a repository of their
// own, so the queue is taller than a screen.
func fillQueue(g *WithT, t *testing.T, f *fixture, count int) {
	t.Helper()
	repo := gitRepo(g, filepath.Join(t.TempDir(), "crowd"))
	me := api.Caller{Dir: repo}
	for i := range count {
		name := fmt.Sprintf("q%02d.txt", i)
		write(g, repo, name, "q\n")
		f.request(g, t, api.RequestBody{Caller: me, Paths: []string{name}, Note: fmt.Sprintf("review %d", i)})
	}
}
