//go:build browser

package server

import (
	"encoding/json"
	"fmt"
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

// A change request with nothing in it is refused on the bar, and sent again
// with a note it goes out.
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
