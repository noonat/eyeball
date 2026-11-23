//go:build browser

package server

import (
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// surfaceAt serves h on addr, which an earlier server on the same address can
// have freed. A worker's scope is its origin, so a daemon that restarts has to
// come back on the port it left for the browser to treat it as the same one.
func surfaceAt(g *WithT, t *testing.T, addr string, h http.Handler) *http.Server {
	g.THelper()
	var ln net.Listener
	g.Eventually(func() error {
		var err error
		ln, err = net.Listen("tcp", addr)
		return err
	}, 5*time.Second, 50*time.Millisecond).Should(Succeed())
	hs := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = hs.Serve(ln) }()
	t.Cleanup(func() { _ = hs.Close() })
	return hs
}

// freeAddress is an address nothing listens on, which the test then serves on.
func freeAddress(g *WithT) string {
	g.THelper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	g.Expect(err).NotTo(HaveOccurred())
	addr := ln.Addr().String()
	g.Expect(ln.Close()).To(Succeed())
	return addr
}

// kept loads the queue from the daemon and waits until the worker holds the
// shell and controls navigations, which is the state an installed app is in
// when it is launched.
func (b *browser) kept(g *WithT, base, token string) {
	g.THelper()
	b.navigate(g, base+"/")
	b.waitFor(g, `!!document.querySelector("#root .scroll")`)
	b.waitFor(g, `navigator.serviceWorker.getRegistration().then(r => !!r && !!r.active)`)
	b.waitFor(g, `caches.keys().then(k => k.join() === "shell-`+token+`")`)
}

// A failing address gets the network's answer with the worker in control: the
// shell carrying the failure, not the cached shell, which carries none. The
// raw address has no route in the app, so the screen can only come from the tag.
func TestServer_Surface_browserShellFailurePage(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	addr := freeAddress(g)
	base := "http://" + addr
	surfaceAt(g, t, addr, f.s.LoopbackSurface(browserToken))
	b := openBrowser(g, t)
	b.kept(g, base, f.s.buildToken())

	b.navigate(g, base+"/reviews/"+itoa(id)+"/raw/1/missing.txt")
	b.waitFor(g, `!!document.querySelector("#root .empty .headline")`)
	g.Expect(b.eval(g, `document.querySelector("#root .empty .headline").textContent`)).To(Equal("Nothing here"))
	g.Expect(b.eval(g, `performance.getEntriesByType("navigation")[0].responseStatus`)).To(BeEquivalentTo(404))
}

// A launch with the daemon stopped draws the loading state, from the
// worker's cache: an eye and one line, with no bar and no notices. Once the
// daemon is back the page draws the queue with no reload: the stream's stamp
// differs from the empty one the page holds, so the view is fetched.
func TestServer_Surface_browserShellOffline(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	f.s.daemon.Build = "first"
	addr := freeAddress(g)
	base := "http://" + addr
	hs := surfaceAt(g, t, addr, f.s.LoopbackSurface(browserToken))
	b := openBrowser(g, t)
	b.kept(g, base, f.s.buildToken())

	g.Expect(hs.Close()).To(Succeed())
	b.navigate(g, base+"/")
	b.waitFor(g, `!!document.querySelector("#root .connecting")`)
	g.Expect(b.eval(g, `document.querySelector(".connecting p").textContent`)).To(Equal("Eyeball is loading\u2026"))
	g.Expect(b.eval(g, `document.querySelector(".connecting .iris").textContent`)).To(Equal("0"))
	g.Expect(b.eval(g, `!!document.querySelector(".connecting .eye.splash .iris")`)).To(BeTrue())
	g.Expect(b.eval(g, `document.querySelectorAll(".head, .topbar, .wordmark, #offline, #update, a.row, .empty").length`)).To(BeEquivalentTo(0))
	g.Expect(b.eval(g, `getComputedStyle(document.querySelector(".connecting")).display`)).To(Equal("flex"), "the shell's stylesheet came from the cache")
	// The eye and its line are centered as a group, so the eye sits a little
	// above the middle. On the phone the group drew at the top of the screen,
	// because #root had no height for .app's min-height to resolve against.
	off := b.eval(g, `(() => { const r = document.querySelector(".connecting .eye").getBoundingClientRect(); return Math.abs((r.top + r.bottom) / 2 - innerHeight / 2); })()`)
	g.Expect(off).To(BeNumerically("<", 40), "the connecting state is not centered in the viewport")
	b.eval(g, `window.__before = true`)

	surfaceAt(g, t, addr, f.s.LoopbackSurface(browserToken))
	b.waitFor(g, `!!document.querySelector("#root .empty .headline")`)
	b.waitFor(g, `document.getElementById("offline").hidden === true`)
	g.Expect(b.eval(g, `window.__before`)).To(BeTrue(), "the page reloaded")
}

// A daemon restarted on the same address with another build serves that build
// on the next reload, with the old worker still in control. The new worker then
// replaces the old one and deletes its cache.
func TestServer_Surface_browserShellRebuilt(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	f.s.daemon.Build = "first"
	oldToken := f.s.buildToken()
	addr := freeAddress(g)
	base := "http://" + addr
	hs := surfaceAt(g, t, addr, f.s.LoopbackSurface(browserToken))
	b := openBrowser(g, t)
	b.kept(g, base, oldToken)

	g.Expect(hs.Close()).To(Succeed())
	f.s.daemon.Build = "second"
	newToken := f.s.buildToken()
	surfaceAt(g, t, addr, f.s.LoopbackSurface(browserToken))
	b.navigate(g, base+"/")
	g.Expect(b.eval(g, `document.querySelector("meta[name='eyeball-build']").content`)).To(Equal("second"))
	g.Expect(b.eval(g, `document.querySelector("script[src^='/static/app.js']").getAttribute("src")`)).To(Equal("/static/app.js?b=" + newToken))
	b.waitFor(g, `!!document.querySelector("#root .empty .headline")`)
	b.waitFor(g, `document.getElementById("offline").hidden === true`)
	g.Expect(b.eval(g, `document.getElementById("update").hidden`)).To(BeTrue(), "the page is the build running now")
	b.waitFor(g, `caches.keys().then(k => k.join() === "shell-`+newToken+`")`)
}

// An install whose fetch is answered 409 leaves the worker in control as it
// was, and no cache for the build it was for. The wrapper changes the header's
// token the way a daemon restarted between the fetch of sw.js and the fill of
// the cache would see another build's.
func TestServer_Surface_browserShellRefusedInstall(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	f.s.daemon.Build = "first"
	oldToken := f.s.buildToken()
	addr := freeAddress(g)
	base := "http://" + addr
	hs := surfaceAt(g, t, addr, f.s.LoopbackSurface(browserToken))
	b := openBrowser(g, t)
	b.kept(g, base, oldToken)
	b.eval(g, `navigator.serviceWorker.getRegistration().then(r => { window.__active = r.active; return true; })`)

	g.Expect(hs.Close()).To(Succeed())
	f.s.daemon.Build = "second"
	var fills atomic.Int32
	inner := f.s.LoopbackSurface(browserToken)
	surfaceAt(g, t, addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(buildHeader) != "" {
			fills.Add(1)
			r.Header.Set(buildHeader, "00000000")
		}
		inner.ServeHTTP(w, r)
	}))
	b.eval(g, `navigator.serviceWorker.getRegistration().then(r => r.update().then(() => true, () => true))`)
	g.Eventually(fills.Load, 5*time.Second, 50*time.Millisecond).Should(BeNumerically(">", 0))
	b.waitFor(g, `navigator.serviceWorker.getRegistration().then(r => !r.installing && !r.waiting)`)
	g.Expect(b.eval(g, `navigator.serviceWorker.getRegistration().then(r => r.active === window.__active)`)).To(BeTrue(), "the new worker replaced the old one")
	g.Expect(b.eval(g, `caches.keys().then(k => k.join())`)).To(Equal("shell-" + oldToken))
}
