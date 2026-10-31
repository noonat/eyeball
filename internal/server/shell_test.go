package server

import (
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/noonat/eyeball"
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
