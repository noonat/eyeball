//go:build browser

package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/kind"
	. "github.com/onsi/gomega"
)

// answered is what the server sent to one request.
type answered struct {
	dest        string
	status      int
	contentType string
	body        string
}

// answerLog records what the wrapped handler answered, by path.
type answerLog struct {
	mu   sync.Mutex
	seen map[string][]answered
}

// wrap records each answer of inner.
func (l *answerLog) wrap(inner http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		inner.ServeHTTP(rec, r)
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.seen == nil {
			l.seen = map[string][]answered{}
		}
		body := rec.Body.String()
		if len(body) > 200 {
			body = body[:200]
		}
		l.seen[r.URL.Path] = append(l.seen[r.URL.Path], answered{
			dest:        r.Header.Get("Sec-Fetch-Dest"),
			status:      rec.Code,
			contentType: rec.Header().Get("Content-Type"),
			body:        body,
		})
	})
}

// of is what the server answered at path.
func (l *answerLog) of(path string) []answered {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]answered(nil), l.seen[path]...)
}

// A browser loading an unknown address as a page gets the error screen with a
// 404 status. The same browser loading a too-large raw file as an image gets
// the status and a plain body, since an image cannot use a page.
func TestServer_Surface_browserFailure(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "big.png", strings.Repeat("x", kind.MaxSend+1))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"big.png"}, Note: "first"}).Review.ID
	log := &answerLog{}
	srv := httptest.NewServer(log.wrap(f.s.Surface()))
	t.Cleanup(srv.Close)
	b := openBrowser(g, t)

	b.navigate(g, srv.URL+"/no/such/page")
	b.waitFor(g, `!!document.querySelector("#root .empty .headline")`)
	g.Expect(b.eval(g, `document.querySelector("#root .empty .headline").textContent`)).To(Equal("Nothing here"))
	g.Expect(b.eval(g, `document.querySelector("#root .empty p:not(.headline)").textContent`)).To(Equal("There is no page at this address."))
	g.Expect(b.eval(g, `document.querySelector(".topbar h1").textContent`)).To(Equal("Not found"))
	g.Expect(b.eval(g, `performance.getEntriesByType("navigation")[0].responseStatus`)).To(BeEquivalentTo(404))
	g.Expect(b.eval(g, `document.querySelector(".topbar a.back").getAttribute("href")`)).To(Equal("/"))
	pageLoads := log.of("/no/such/page")
	g.Expect(pageLoads).To(HaveLen(1))
	g.Expect(pageLoads[0].dest).To(Equal("document"))
	g.Expect(pageLoads[0].status).To(Equal(http.StatusNotFound))

	raw := "/reviews/" + itoa(id) + "/raw/1/big.png"
	b.navigate(g, srv.URL+"/")
	b.eval(g, `new Promise((done) => {
		const img = document.createElement("img");
		img.onerror = () => done(true);
		img.onload = () => done(false);
		img.src = "`+raw+`";
		document.body.append(img);
	})`)
	b.waitFor(g, `performance.getEntriesByName(location.origin + "`+raw+`").length > 0`)
	g.Expect(b.eval(g, `performance.getEntriesByName(location.origin + "`+raw+`")[0].responseStatus`)).To(BeEquivalentTo(413))
	loads := log.of(raw)
	g.Expect(loads).To(HaveLen(1))
	g.Expect(loads[0].dest).To(Equal("image"))
	g.Expect(loads[0].status).To(Equal(http.StatusRequestEntityTooLarge))
	g.Expect(loads[0].contentType).To(Equal("text/plain; charset=utf-8"))
	g.Expect(loads[0].body).To(Equal(pastSendLimit + "\n"))

	// Opened as a page of its own, the same address is the error screen with
	// the sentence, though the app has no route for it.
	b.navigate(g, srv.URL+raw)
	b.waitFor(g, `!!document.querySelector("#root .empty .headline")`)
	g.Expect(b.eval(g, `document.querySelector("#root .empty .headline").textContent`)).To(Equal("Too large"))
	g.Expect(b.eval(g, `document.querySelector("#root .empty p:not(.headline)").textContent`)).To(Equal(pastSendLimit))
}
