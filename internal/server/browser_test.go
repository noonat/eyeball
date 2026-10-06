//go:build browser

package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"golang.org/x/net/websocket"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/store"
)

// browserToken is the loopback token the tests hand over. It has the form
// rand.Text draws, which is the only form the bundle reads.
const browserToken = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// The page a reviewer sees, driven in headless Chromium: the token handed
// over, a line comment added, edited and deleted, a review comment, an
// approval held back while text is unsaved, and an approval reloading the
// page.
func TestServer_Surface_browserActing(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	seen := &requestLog{}
	srv := httptest.NewServer(seen.wrap(f.s.LoopbackSurface(browserToken)))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	round := f.latestRound(g, t, id)

	// The printed address hands the token over and leaves no trace of it.
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	g.Expect(b.eval(g, `location.hash`)).To(Equal(""))
	g.Expect(b.eval(g, `localStorage.getItem("eyeball:token")`)).To(Equal(browserToken))
	g.Expect(seen.lines()).NotTo(ContainElement(ContainSubstring(browserToken)), "the token reached a request line or a Referer")

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
	b.waitFor(g, thread+`?.querySelectorAll(".comment").length === 1`)
	b.waitFor(g, `!document.querySelector("tr.thread form.composer")`)
	g.Expect(b.eval(g, `document.querySelectorAll("tr.sel").length`)).To(BeEquivalentTo(0))
	g.Expect(b.eval(g, `document.getElementById("tally").textContent`)).To(Equal("1 commentReady to send"))
	comments := f.comments(g, t, round)
	g.Expect(comments).To(HaveLen(1))
	g.Expect(comments[0].Quoted).To(Equal("two"))

	// The comment is still under its line after a reload.
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	g.Expect(b.eval(g, thread+`.querySelector(".comment p").textContent`)).To(Equal("Why two?"))

	// Edit swaps the card for its form in the row, and Save stores the new
	// body.
	b.eval(g, thread+`.querySelector(".acts a").click()`)
	b.waitFor(g, thread+`.querySelector("form.composer textarea") !== null`)
	b.eval(g, `(() => { const f = `+thread+`.querySelector("form.composer textarea"); f.value = "Why two lines?"; f.dispatchEvent(new Event("input", { bubbles: true })); })()`)
	b.eval(g, thread+`.querySelector("form.composer button[type=submit]").click()`)
	b.waitFor(g, thread+`.querySelector(".comment p")?.textContent === "Why two lines?"`)
	g.Expect(f.comments(g, t, round)[0].Body).To(Equal("Why two lines?"))

	// Delete removes it, the row goes with its last comment, and the tally
	// follows.
	b.eval(g, thread+`.querySelectorAll(".acts a")[1].click()`)
	b.waitFor(g, `!`+thread+`?.matches("tr.thread")`)
	b.waitFor(g, `document.getElementById("tally").textContent === "No comments yet"`)
	g.Expect(f.comments(g, t, round)).To(BeEmpty())

	// Cancel closes the composer and keeps nothing it held.
	b.eval(g, line2+`.click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "Never mind")
	b.eval(g, `document.querySelector("tr.thread form.composer [data-cancel]").click()`)
	g.Expect(b.eval(g, `document.querySelectorAll("tr.thread").length`)).To(BeEquivalentTo(0))
	g.Expect(b.eval(g, `document.querySelectorAll("tr.sel").length`)).To(BeEquivalentTo(0))
	g.Expect(b.eval(g, `document.getElementById("unsaved").hidden`)).To(BeTrue())

	// The chips move the same text to the file without a request, and the
	// form then sends only what a file-level comment takes.
	list := listID(store.LevelFile, "a.txt")
	b.eval(g, `document.querySelector("details.file tr[data-line='1']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "About the whole file.")
	b.eval(g, `document.querySelector("tr.thread form.composer .chip[data-level=file]").click()`)
	g.Expect(b.eval(g, `document.querySelector("tr.thread form.composer input[name=start]").disabled`)).To(BeTrue())
	b.eval(g, `document.querySelector("tr.thread form.composer button[type=submit]").click()`)
	b.waitFor(g, fmt.Sprintf(`document.querySelectorAll("#%s .comment").length === 1`, list))
	b.waitFor(g, `document.querySelectorAll("tr.thread").length === 0`)
	g.Expect(f.comments(g, t, round)[0].Level).To(Equal(store.LevelFile))

	// The header's link opens the file's own page without toggling the card.
	b.eval(g, `document.querySelector("details.file .fileopen").click()`)
	b.waitFor(g, `location.pathname === "/reviews/`+itoa(id)+`/files/a.txt" && document.readyState === "complete"`)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))

	// The review's own box is always on an open round.
	b.typeInto(g, ".page > form.composer textarea", "Overall fine.")
	b.eval(g, `document.querySelector(".page > form.composer button[type=submit]").click()`)
	b.waitFor(g, `document.querySelectorAll("#comments-review .comment").length === 1`)
	g.Expect(b.eval(g, `document.querySelector(".page > form.composer textarea").value`)).To(Equal(""))

	// Text still being written holds the verdict back.
	b.typeInto(g, ".page > form.composer textarea", "Half a thought")
	g.Expect(b.eval(g, `document.getElementById("unsaved").hidden`)).To(BeFalse())
	g.Expect(b.eval(g, `document.querySelector("#bar button[value=approve]").getAttribute("aria-disabled")`)).To(Equal("true"))
	b.eval(g, `document.querySelector("#bar button[value=approve]").click()`)
	time.Sleep(300 * time.Millisecond)
	_, err := f.store.Decision(t.Context(), round)
	g.Expect(err).To(MatchError(store.ErrNotFound), "a verdict went out while a comment was being written")

	// Once the text is gone, Approve records the verdict and the page reloads
	// as the decided page.
	b.typeInto(g, ".page > form.composer textarea", "")
	g.Expect(b.eval(g, `document.getElementById("unsaved").hidden`)).To(BeTrue())
	b.eval(g, `window.__before = true`)
	b.eval(g, `document.querySelector("#bar button[value=approve]").click()`)
	b.waitFor(g, `window.__before === undefined && document.readyState === "complete"`)
	g.Expect(b.eval(g, `!!document.querySelector("#bar")`)).To(BeFalse())
	d, err := f.store.Decision(t.Context(), round)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(d.Verdict).To(Equal(store.VerdictApprove))
}

// A comment that cannot be saved becomes an unsent card in its list, Retry
// stores it once the server is back, Delete drops it, and the verdict waits
// while any card remains. A verdict that cannot be sent says so in the bar and
// keeps its note.
func TestServer_Surface_browserFailedSave(t *testing.T) {
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

	// A verdict sent while the server is gone says so in the bar and keeps
	// its note.
	b.typeInto(g, "#bar textarea", "Close.")
	g.Expect(hs.Close()).To(Succeed())
	b.eval(g, `document.querySelector("#bar button[value=approve]").click()`)
	b.waitFor(g, `!!document.querySelector("#bar .failed")`)
	g.Expect(b.eval(g, `document.querySelector("#bar textarea").value`)).To(Equal("Close."))
	b.typeInto(g, "#bar textarea", "")
	restart()
	b.waitFor(g, `document.getElementById("offline").hidden === true`)

	// Two saves made while the server is gone become two cards.
	b.eval(g, `document.querySelector("details.file tr[data-line='2']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "Why two?")
	b.typeInto(g, ".page > form.composer textarea", "Overall fine.")
	g.Expect(hs.Close()).To(Succeed())
	b.eval(g, `document.querySelector("tr.thread form.composer button[type=submit]").click()`)
	b.waitFor(g, `document.querySelectorAll(".comment.unsent").length === 1`)
	b.eval(g, `document.querySelector(".page > form.composer button[type=submit]").click()`)
	b.waitFor(g, `document.querySelectorAll(".comment.unsent").length === 2`)
	g.Expect(b.eval(g, `!!document.querySelector("tr.thread form.composer")`)).To(BeFalse(), "the line's composer closed")
	g.Expect(b.eval(g, `document.querySelector(".page > form.composer textarea").value`)).To(Equal(""))
	g.Expect(b.eval(g, `document.getElementById("unsent").hidden`)).To(BeFalse())
	g.Expect(b.eval(g, `document.getElementById("unsent-count").textContent`)).To(Equal("2 comments"))

	// Back online, Retry stores the line's comment once.
	restart()
	b.waitFor(g, `document.getElementById("offline").hidden === true`)
	b.eval(g, `document.querySelector("tr.thread .comment.unsent [data-retry]").click()`)
	b.waitFor(g, `document.querySelectorAll("tr.thread .comment:not(.unsent)").length === 1`)
	g.Expect(f.comments(g, t, round)).To(HaveLen(1))
	g.Expect(b.eval(g, `document.querySelectorAll(".comment.unsent").length`)).To(BeEquivalentTo(1))

	// The verdict waits while a card remains.
	b.eval(g, `document.querySelector("#bar button[value=approve]").click()`)
	time.Sleep(300 * time.Millisecond)
	_, err = f.store.Decision(t.Context(), round)
	g.Expect(err).To(MatchError(store.ErrNotFound), "a verdict went out with a comment unsent")

	// Delete drops the other card and sends nothing.
	b.eval(g, `document.querySelector(".comment.unsent [data-discard]").click()`)
	g.Expect(b.eval(g, `document.querySelectorAll(".comment.unsent").length`)).To(BeEquivalentTo(0))
	g.Expect(b.eval(g, `document.getElementById("unsent").hidden`)).To(BeTrue())
	g.Expect(f.comments(g, t, round)).To(HaveLen(1))
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
	b.waitFor(g, `document.getElementById("firstrun").hidden === false`)
	g.Expect(b.eval(g, `document.getElementById("install-done").hidden`)).To(BeFalse())
	// The prompt is the browser's, so the permission is granted through the
	// protocol before the tap, and requestPermission answers granted.
	b.call(g, "Browser.grantPermissions", map[string]any{"permissions": []string{"notifications"}, "origin": srv.URL})
	b.eval(g, `document.getElementById("allow").click()`)
	b.waitFor(g, `document.getElementById("firstrun").hidden === true`)
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
	b.waitFor(g, `document.getElementById("lapsed").hidden === false`)
	b.eval(g, `document.getElementById("resubscribe").click()`)
	b.waitFor(g, `document.getElementById("lapsed").hidden === true`)
	subs, err = f.store.ActiveSubscriptions(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(subs).To(HaveLen(1))
	g.Expect(subs[0].Endpoint).To(Equal(svc.url + "/device/2"))
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
	write(g, repo, "doc.md", "New paragraph.\n\nKept.\n")
	write(g, repo, "new.md", "# Fresh\n\nA new document.\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"doc.md"}, Note: "docs"}).Review.ID
	fresh := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"new.md"}, Note: "new"}).Review.ID
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)

	// A tap on a changed passage selects it rather than opening it.
	changed := `document.querySelector("details.blk.changed")`
	b.eval(g, changed+`.querySelector("p").click()`)
	b.waitFor(g, changed+`.nextElementSibling?.querySelector("form.composer textarea") != null`)
	g.Expect(b.eval(g, changed+`.open`)).To(BeFalse(), "a tap off the hint opened the replaced text")
	g.Expect(b.eval(g, changed+`.classList.contains("sel")`)).To(BeTrue())
	b.eval(g, changed+`.nextElementSibling.querySelector("[data-cancel]").click()`)
	b.eval(g, changed+`.querySelector(".hint.cue-show").click()`)
	g.Expect(b.eval(g, changed+`.open`)).To(BeTrue(), "the hint opens the replaced text")

	// A comment on a kept passage lands under it, and stays there.
	kept := `document.querySelector("div.blk[data-lines='3-3']")`
	b.eval(g, kept+`.click()`)
	b.waitFor(g, kept+`.nextElementSibling?.querySelector("form.composer textarea") != null`)
	b.typeInto(g, "div[id^='thread-'] form.composer textarea", "Why keep this?")
	b.eval(g, `document.querySelector("div[id^='thread-'] form.composer button[type=submit]").click()`)
	b.waitFor(g, kept+`.nextElementSibling?.querySelectorAll(".comment").length === 1`)
	b.waitFor(g, `!document.querySelector("div[id^='thread-'] form.composer")`)
	comments := f.comments(g, t, f.latestRound(g, t, id))
	g.Expect(comments).To(HaveLen(1))
	g.Expect(comments[0].Level).To(Equal(store.LevelPassage))
	g.Expect([]int{comments[0].StartLine, comments[0].EndLine}).To(Equal([]int{3, 3}))
	g.Expect(comments[0].Quoted).To(Equal("Kept."))
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	g.Expect(b.eval(g, kept+`.nextElementSibling.querySelector(".comment p").textContent`)).To(Equal("Why keep this?"))

	// The single new document takes one the same way.
	b.navigate(g, srv.URL+"/reviews/"+itoa(fresh))
	block := `document.querySelector(".page > .prose .blk[data-lines='3-3']")`
	b.eval(g, block+`.click()`)
	b.waitFor(g, block+`.nextElementSibling?.querySelector("form.composer textarea") != null`)
	b.typeInto(g, "div[id^='thread-'] form.composer textarea", "Say more.")
	b.eval(g, `document.querySelector("div[id^='thread-'] form.composer button[type=submit]").click()`)
	b.waitFor(g, block+`.nextElementSibling?.querySelectorAll(".comment").length === 1`)
	g.Expect(f.comments(g, t, f.latestRound(g, t, fresh))[0].Quoted).To(Equal("A new document."))
}

// A change request with nothing in it is refused on the bar, and sent again
// with a note it goes out.
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
const body = new URLSearchParams({round: "2", verdict: "approve", note: ""});
fetch("/reviews/%d/decision", {method: "POST", body, mode: "no-cors"})
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
	g.Eventually(statuses.lines).Should(ContainElement(fmt.Sprintf("POST /reviews/%d/decision 403", id)))
	_, err := f.store.Decision(t.Context(), round)
	g.Expect(err).To(MatchError(store.ErrNotFound))
}

func TestServer_Surface_browserRefusedChangeRequest(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	srv := httptest.NewServer(f.s.LoopbackSurface(browserToken))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)
	round := f.latestRound(g, t, id)

	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)
	b.eval(g, `document.querySelector("#bar button[value=changes]").click()`)
	b.waitFor(g, `!!document.querySelector("#bar .notice.warn")`)
	_, err := f.store.Decision(t.Context(), round)
	g.Expect(err).To(MatchError(store.ErrNotFound))

	b.typeInto(g, "#bar textarea", "Name the control.")
	b.eval(g, `window.__before = true`)
	b.eval(g, `document.querySelector("#bar button[value=changes]").click()`)
	b.waitFor(g, `window.__before === undefined && document.readyState === "complete"`)
	d, err := f.store.Decision(t.Context(), round)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(d.Verdict).To(Equal(store.VerdictChanges))
	g.Expect(d.Note).To(Equal("Name the control."))
}

// A large review keeps its bodies until a card opens, and reopens where it was
// left: the same card open with its body loaded, at the same scroll. What was
// typed survives a reload too.
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
	b.navigate(g, srv.URL+"/reviews/"+itoa(id)+"#token="+browserToken)

	card := `document.querySelector("details.file[data-path='d/05.txt']")`
	g.Expect(b.eval(g, card+`.querySelector(".code") === null`)).To(BeTrue(), "a collapsed card's body came with the page")
	b.eval(g, card+`.querySelector("summary").click()`)
	b.waitFor(g, card+`.querySelectorAll(".src").length === 40`)
	b.eval(g, `document.querySelector(".scroll").scrollTop = 600`)
	b.eval(g, `document.querySelector(".scroll").dispatchEvent(new Event("scroll"))`)
	time.Sleep(400 * time.Millisecond)

	b.typeInto(g, ".page > form.composer textarea", "Half a review comment")
	b.typeInto(g, "#bar textarea", "A note in progress")
	b.eval(g, card+`.querySelector("tr[data-line='3']").click()`)
	b.waitFor(g, `!!document.querySelector("tr.thread form.composer textarea")`)
	b.typeInto(g, "tr.thread form.composer textarea", "About line three")

	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	b.waitFor(g, card+`.open && `+card+`.querySelectorAll(".src").length === 40`)
	b.waitFor(g, `Math.abs(document.querySelector(".scroll").scrollTop - 600) < 2`)
	g.Expect(b.eval(g, `document.querySelector(".page > form.composer textarea").value`)).To(Equal("Half a review comment"))
	g.Expect(b.eval(g, `document.querySelector("#bar textarea").value`)).To(Equal("A note in progress"))
	b.waitFor(g, `document.querySelector("tr.thread form.composer textarea")?.value === "About line three"`)
	g.Expect(b.eval(g, `document.querySelector("tr.thread").previousElementSibling.dataset.line`)).To(Equal("3"))

	// A draft clears once its text is stored.
	b.eval(g, `document.querySelector(".page > form.composer button[type=submit]").click()`)
	b.waitFor(g, `document.querySelectorAll("#comments-review .comment").length === 1`)
	b.navigate(g, srv.URL+"/reviews/"+itoa(id))
	g.Expect(b.eval(g, `document.querySelector(".page > form.composer textarea").value`)).To(Equal(""))
}

// The live stream: a verdict recorded elsewhere reloads an open review page,
// but not while something typed is unsaved; a page with no stamp stays put;
// and a dropped connection shows the notice, holds the verdicts back, and
// clears once the server is back.
func TestServer_Surface_browserStream(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	other := actingReview(g, t, f)
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

	// Text being written holds the reload back until it is gone.
	b.navigate(g, base+"/reviews/"+itoa(other)+"#token="+browserToken)
	b.typeInto(g, ".page > form.composer textarea", "Half written")
	b.eval(g, `window.__before = true`)
	f.decide(g, t, other, store.VerdictApprove, "")
	time.Sleep(500 * time.Millisecond)
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "the page reloaded over unsaved text")
	b.typeInto(g, ".page > form.composer textarea", "")
	b.waitFor(g, `window.__before === undefined && document.readyState === "complete"`)
	g.Expect(b.eval(g, `!!document.querySelector("#bar")`)).To(BeFalse())

	// A file page has no stamp, so nothing published reloads it.
	b.navigate(g, base+"/reviews/"+itoa(id)+"/files/a.txt")
	b.eval(g, `window.__before = true`)
	f.comment(g, t, id, "A point.")
	time.Sleep(500 * time.Millisecond)
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "a page with no stamp reloaded")

	// A verdict given elsewhere reloads the open review page.
	b.navigate(g, base+"/reviews/"+itoa(id))
	b.waitFor(g, `document.getElementById("offline").hidden === true`)
	b.eval(g, `window.__before = true`)
	f.decide(g, t, id, store.VerdictChanges, "From another device.")
	b.waitFor(g, `window.__before === undefined && document.readyState === "complete"`)
	g.Expect(b.eval(g, `!!document.querySelector("#bar")`)).To(BeFalse())

	// The server going away shows the notice, and its return clears it.
	third := actingReview(g, t, f)
	b.navigate(g, base+"/reviews/"+itoa(third))
	g.Expect(hs.Close()).To(Succeed())
	b.waitFor(g, `document.getElementById("offline").hidden === false`)
	g.Expect(b.eval(g, `document.getElementById("whynot").hidden`)).To(BeFalse())
	g.Expect(b.eval(g, `document.querySelector("#bar button[value=approve]").getAttribute("aria-disabled")`)).To(Equal("true"))
	ln, err = net.Listen("tcp", addr)
	g.Expect(err).NotTo(HaveOccurred())
	hs = serve(ln)
	b.waitFor(g, `document.getElementById("offline").hidden === true`)
	g.Expect(b.eval(g, `document.querySelector("#bar button[value=approve]").getAttribute("aria-disabled")`)).To(BeNil())
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
	g.Expect(b.eval(g, `document.querySelector("meta[name='eyeball-build']").content`)).To(Equal("newer"))
	g.Expect(b.eval(g, `document.getElementById("update").hidden`)).To(BeTrue())
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

// comments reads a round's comments.
func (f *fixture) comments(g *WithT, t *testing.T, roundID int64) []store.Comment {
	g.THelper()
	out, err := f.store.Comments(t.Context(), roundID)
	g.Expect(err).NotTo(HaveOccurred())
	return out
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

// eval runs an expression in the page and returns its value.
func (b *browser) eval(g *WithT, expr string) any {
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
		g.Expect(out.Exception.Exception.Description).To(BeEmpty(), expr)
	}
	return out.Result.Value
}

// waitFor polls an expression until it is true, in real time: the page's
// requests and the server's answers take as long as they take.
func (b *browser) waitFor(g *WithT, expr string) {
	g.THelper()
	g.Eventually(func() any { return b.eval(g, expr) }, 5*time.Second, 50*time.Millisecond).Should(Equal(true), expr)
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
	b.eval(g, fmt.Sprintf(`(() => { const f = document.querySelector(%q); f.value = %s; f.dispatchEvent(new Event("input", { bubbles: true })); })()`, selector, quoted))
}
