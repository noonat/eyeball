package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"golang.org/x/net/html"
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
