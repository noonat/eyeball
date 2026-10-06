package server

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"golang.org/x/net/html"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/store"
)

// A device posts its subscription and is told whether it still works, and
// the page carries the key it subscribes with.
func TestServer_handleSubscribe(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	doc := f.page(g, "/", http.StatusOK)
	metas := findAll(doc, func(n *html.Node) bool { return n.Data == "meta" && attr(n, "name") == "vapid-key" })
	g.Expect(metas).To(HaveLen(1))
	g.Expect(attr(metas[0], "content")).To(HaveLen(87), "an uncompressed P-256 point, base64url")

	body := `{"endpoint":"https://push.example/abc","keys":{"p256dh":"k","auth":"a"}}`
	g.Expect(subscribe(f.s.Surface(), body, "")).To(Equal("active"))
	g.Expect(subscribe(f.s.Surface(), body, "")).To(Equal("active"), "a known subscription is confirmed")
	g.Expect(f.store.MarkLapsed(t.Context(), "https://push.example/abc")).To(Succeed())
	g.Expect(subscribe(f.s.Surface(), body, "")).To(Equal("lapsed"))

	bad := []string{
		`not json`,
		`{"endpoint":"https://push.example/x","keys":{"p256dh":"k"}}`,
		`{"endpoint":"ftp://push.example/x","keys":{"p256dh":"k","auth":"a"}}`,
		`{"endpoint":"http://push.example/x","keys":{"p256dh":"k","auth":"a"}}`,
		`{"endpoint":"/relative","keys":{"p256dh":"k","auth":"a"}}`,
	}
	for _, b := range bad {
		res := postJSON(f.s.Surface(), b, "")
		g.Expect(res.Code).To(Equal(http.StatusBadRequest), b)
	}
}

// On loopback a subscription is a write like any other, and needs the token.
func TestServer_handleSubscribeLoopback(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	body := `{"endpoint":"https://push.example/abc","keys":{"p256dh":"k","auth":"a"}}`
	g.Expect(postJSON(f.s.LoopbackSurface("RUN"), body, "").Code).To(Equal(http.StatusForbidden))
	g.Expect(subscribe(f.s.LoopbackSurface("RUN"), body, "RUN")).To(Equal("active"))
}

// A request reaches every device, a subscription the push service calls gone
// is marked lapsed, and one it merely failed stays active.
func TestServer_notify(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	svc := newPushService(t, map[string]int{"/a": http.StatusCreated, "/b": http.StatusGone, "/c": http.StatusInternalServerError})
	f.s.surface = func() api.Surface { return api.Surface{URL: "https://eyeball.example.ts.net"} }
	devices := []string{"/a", "/b", "/c"}
	for _, p := range devices {
		g.Expect(subscribe(f.s.Surface(), svc.subscription(g, p), "")).To(Equal("active"))
	}
	actingReview(g, t, f)
	f.s.WaitPushes(t.Context())
	g.Expect(svc.paths()).To(ConsistOf("/a", "/b", "/c"))
	g.Expect(svc.encodings()).To(HaveEach("aes128gcm"))
	active, err := f.store.ActiveSubscriptions(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	var endpoints []string
	for _, a := range active {
		endpoints = append(endpoints, a.Endpoint)
	}
	g.Expect(endpoints).To(ConsistOf(svc.url+"/a", svc.url+"/c"))
}

// A push service that hangs holds neither the request's answer nor shutdown
// past its grace.
func TestServer_notifyHung(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	svc := newPushService(t, map[string]int{"/a": http.StatusCreated})
	svc.hang = make(chan struct{})
	t.Cleanup(func() { close(svc.hang) })
	f.s.surface = func() api.Surface { return api.Surface{URL: "https://eyeball.example.ts.net"} }
	g.Expect(subscribe(f.s.Surface(), svc.subscription(g, "/a"), "")).To(Equal("active"))
	began := time.Now()
	actingReview(g, t, f)
	g.Expect(time.Since(began)).To(BeNumerically("<", 3*time.Second), "the request waited on the push")
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	began = time.Now()
	f.s.WaitPushes(ctx)
	g.Expect(time.Since(began)).To(BeNumerically("<", time.Second))
}

// Comments and verdicts send nothing, and neither does a daemon with no
// tailnet address.
func TestServer_notifyOnlyRequests(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	svc := newPushService(t, map[string]int{"/a": http.StatusCreated})
	g.Expect(subscribe(f.s.Surface(), svc.subscription(g, "/a"), "")).To(Equal("active"))
	id := actingReview(g, t, f)
	f.s.WaitPushes(t.Context())
	g.Expect(svc.paths()).To(BeEmpty(), "no tailnet address, no push")

	f.s.surface = func() api.Surface { return api.Surface{URL: "https://eyeball.example.ts.net"} }
	f.comment(g, t, id, "A point.")
	f.decide(g, t, id, store.VerdictChanges, "")
	f.s.WaitPushes(t.Context())
	g.Expect(svc.paths()).To(BeEmpty(), "a comment or a verdict sent a push")
}

// subscribe posts a subscription and returns the state it is answered with.
func subscribe(h http.Handler, body, token string) string {
	res := postJSON(h, body, token)
	var out struct {
		State string `json:"state"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &out)
	return out.State
}

// postJSON posts a JSON body to /subscriptions.
func postJSON(h http.Handler, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "http://eyeball/subscriptions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Eyeball-Token", token)
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

// pushService stands in for a push service: it records each push and answers
// with the status its path is given.
type pushService struct {
	url    string
	status map[string]int
	hang   chan struct{}
	mu     sync.Mutex
	got    []*http.Request
}

func newPushService(t *testing.T, status map[string]int) *pushService {
	p := &pushService{status: status}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.hang != nil {
			<-p.hang
		}
		p.mu.Lock()
		p.got = append(p.got, r)
		p.mu.Unlock()
		w.WriteHeader(p.status[r.URL.Path])
	}))
	t.Cleanup(srv.Close)
	p.url = srv.URL
	return p
}

// subscription is a browser's subscription to this service at a path, with
// real keys, since the payload is encrypted to them.
func (p *pushService) subscription(g *WithT, path string) string {
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	g.Expect(err).NotTo(HaveOccurred())
	auth := make([]byte, 16)
	_, err = rand.Read(auth)
	g.Expect(err).NotTo(HaveOccurred())
	b, err := json.Marshal(map[string]any{"endpoint": p.url + path, "keys": map[string]string{
		"p256dh": base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
		"auth":   base64.RawURLEncoding.EncodeToString(auth),
	}})
	g.Expect(err).NotTo(HaveOccurred())
	return string(b)
}

func (p *pushService) paths() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, r := range p.got {
		out = append(out, r.URL.Path)
	}
	return out
}

func (p *pushService) encodings() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, r := range p.got {
		out = append(out, r.Header.Get("Content-Encoding"))
	}
	return out
}
