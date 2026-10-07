package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/kind"
	"github.com/noonat/eyeball/internal/store"
	. "github.com/onsi/gomega"
)

func TestServer_Surface_raw(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	commitFile(g, repo, "site/app.css", "body { color: red }\n")
	commitFile(g, repo, "site/gone.html", "<p>gone</p>\n")
	commitFile(g, repo, ".gitignore", "ignored.html\n")
	write(g, repo, "site/index.html", "<link rel=stylesheet href=app.css><p>now</p>\n")
	write(g, repo, "site/icon.png", "\x89PNG\r\n\x1a\nnot really")
	write(g, repo, "site/paper.pdf", "%PDF-1.4\n")
	write(g, repo, "site/data.bin", "a\x00b")
	write(g, repo, "site/ignored.html", "<p>never captured</p>\n")
	g.Expect(os.Remove(filepath.Join(repo, "site/gone.html"))).To(Succeed())
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"site"}, Note: "first"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "more")
	write(g, repo, "site/index.html", "<p>round two</p>\n")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
	raw := "/reviews/" + itoa(id) + "/raw/"

	cases := []struct {
		name   string
		path   string
		status int
		typ    string
		body   string
		policy string
	}{
		{name: "a captured page", path: "2/site/index.html", status: http.StatusOK, typ: "text/html; charset=utf-8", body: "<p>round two</p>\n", policy: sandboxPolicy},
		{name: "the previous round's side", path: "1/site/index.html", status: http.StatusOK, typ: "text/html; charset=utf-8", body: "<link rel=stylesheet href=app.css><p>now</p>\n", policy: sandboxPolicy},
		{name: "a stylesheet from the base, served as one", path: "2/site/app.css", status: http.StatusOK, typ: "text/css; charset=utf-8", body: "body { color: red }\n", policy: sandboxPolicy},
		{name: "an image", path: "2/site/icon.png", status: http.StatusOK, typ: "image/png", body: "\x89PNG\r\n\x1a\nnot really", policy: sandboxPolicy},
		{name: "a pdf, unsandboxed", path: "2/site/paper.pdf", status: http.StatusOK, typ: "application/pdf", body: "%PDF-1.4\n"},
		{name: "the base holds a deleted file", path: "0/site/gone.html", status: http.StatusOK, typ: "text/html; charset=utf-8", body: "<p>gone</p>\n", policy: sandboxPolicy},
		{name: "a round that deleted it does not", path: "2/site/gone.html", status: http.StatusNotFound, policy: sandboxPolicy},
		{name: "the base lacks a new file", path: "0/site/index.html", status: http.StatusNotFound, policy: sandboxPolicy},
		{name: "a binary file is not sent", path: "2/site/data.bin", status: http.StatusNotFound, policy: sandboxPolicy},
		{name: "the working copy is never read", path: "2/site/ignored.html", status: http.StatusNotFound, policy: sandboxPolicy},
		{name: "a round past the latest", path: "3/site/index.html", status: http.StatusNotFound, policy: sandboxPolicy},
		{name: "a round that is not a number", path: "x/site/index.html", status: http.StatusNotFound, policy: sandboxPolicy},
		{name: "a negative round", path: "-1/site/index.html", status: http.StatusNotFound, policy: sandboxPolicy},
		{name: "a path leaving the project", path: "2/../../etc/passwd", status: http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			res := serveSurface(f.s.Surface(), http.MethodGet, raw+c.path)
			g.Expect(res.Code).To(Equal(c.status), res.Body.String())
			if c.policy != "" || c.status == http.StatusOK {
				g.Expect(res.Header().Get("Content-Security-Policy")).To(Equal(c.policy))
			}
			g.Expect(res.Header().Get("X-Content-Type-Options")).To(Equal("nosniff"))
			if c.status != http.StatusOK {
				return
			}
			g.Expect(res.Header().Get("Content-Type")).To(Equal(c.typ))
			g.Expect(res.Body.String()).To(Equal(c.body))
		})
	}

	t.Run("an unknown review", func(t *testing.T) {
		g := NewWithT(t)
		res := serveSurface(f.s.Surface(), http.MethodGet, "/reviews/999/raw/1/site/index.html")
		g.Expect(res.Code).To(Equal(http.StatusNotFound))
	})

	t.Run("a range", func(t *testing.T) {
		// iOS Safari plays no video from a server that does not answer one.
		g := NewWithT(t)
		req := httptest.NewRequest(http.MethodGet, "http://eyeball"+raw+"2/site/index.html", nil)
		req.Header.Set("Range", "bytes=3-7")
		res := httptest.NewRecorder()
		f.s.Surface().ServeHTTP(res, req)
		g.Expect(res.Code).To(Equal(http.StatusPartialContent))
		g.Expect(res.Body.String()).To(Equal("round"))
	})
}

func TestServer_Surface_rawTooLarge(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	big := strings.Repeat("x", kind.MaxSend+1)
	commitFile(g, repo, "base.png", big)
	write(g, repo, "big.png", big)
	write(g, repo, "fits.png", strings.Repeat("x", kind.MaxSend))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"big.png", "fits.png"}, Note: "first"}).Review.ID
	raw := "/reviews/" + itoa(id) + "/raw/"

	res := serveSurface(f.s.Surface(), http.MethodGet, raw+"1/big.png")
	g.Expect(res.Code).To(Equal(http.StatusRequestEntityTooLarge))
	g.Expect(pageText(g, res.Body.String())).To(ContainSubstring(pastSendLimit))

	res = serveSurface(f.s.Surface(), http.MethodGet, raw+"1/fits.png")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Body.Len()).To(Equal(kind.MaxSend))

	// Asked for, a file past the limit is sent: a captured one from its blob,
	// and one in the base from git through a temporary file, each seekable
	// so a range is answered.
	paths := []string{"1/big.png", "0/base.png"}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			g := NewWithT(t)
			res := serveSurface(f.s.Surface(), http.MethodGet, raw+p+"?load=1")
			g.Expect(res.Code).To(Equal(http.StatusOK))
			g.Expect(res.Body.Len()).To(Equal(kind.MaxSend + 1))
			g.Expect(res.Header().Get("Content-Type")).To(Equal("image/png"))

			req := httptest.NewRequest(http.MethodGet, "http://eyeball"+raw+p+"?load=1", nil)
			req.Header.Set("Range", "bytes=10-19")
			ranged := httptest.NewRecorder()
			f.s.Surface().ServeHTTP(ranged, req)
			g.Expect(ranged.Code).To(Equal(http.StatusPartialContent))
			g.Expect(ranged.Body.String()).To(Equal("xxxxxxxxxx"))
		})
	}
}
