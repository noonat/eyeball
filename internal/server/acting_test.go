package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/store"
)

func Test_dedent(t *testing.T) {
	cases := []struct{ in, want string }{
		{in: "\t\tcase <-done:", want: "case <-done:"},
		{in: "\tif x {\n\t\treturn\n\t}", want: "if x {\n\treturn\n}"},
		{in: "  a\n\n  b", want: "a\n\nb"},
		{in: "prose with no indent", want: "prose with no indent"},
		{in: "\ta\n  b", want: "\ta\n  b"},
	}
	for _, c := range cases {
		g := NewWithT(t)
		g.Expect(dedent(c.in)).To(Equal(c.want), c.in)
	}
}

// Every write is the surface's alone. Sent to the socket's handler, each is a
// 404 and changes nothing.
func TestServer_AgentHandler_refusesEveryWrite(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	round := f.latestRound(g, t, id)
	c := f.addComment(g, t, store.NewComment{RoundID: round, Level: store.LevelReview, Body: "Stays."})
	writes := []struct {
		method, path string
		form         url.Values
	}{
		{
			method: http.MethodPost,
			path:   "/api/reviews/" + itoa(id) + "/comments",
			form:   url.Values{"round": {"1"}, "level": {"review"}, "body": {"x"}},
		},
		{
			method: http.MethodPut,
			path:   "/api/reviews/" + itoa(id) + "/comments/" + itoa(c.ID),
			form:   url.Values{"body": {"changed"}},
		},
		{
			method: http.MethodDelete,
			path:   "/api/reviews/" + itoa(id) + "/comments/" + itoa(c.ID),
		},
		{
			method: http.MethodPost,
			path:   "/api/reviews/" + itoa(id) + "/decision",
			form:   url.Values{"round": {"1"}, "verdict": {"approve"}},
		},
	}
	for _, w := range writes {
		res := sendForm(f.s.AgentHandler(), w.method, w.path, w.form)
		g.Expect(res.Code).To(Equal(http.StatusNotFound), w.method+" "+w.path)
	}
	comments, err := f.store.Comments(t.Context(), round)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(comments).To(HaveLen(1))
	g.Expect(comments[0].Body).To(Equal("Stays."))
	_, err = f.store.Decision(t.Context(), round)
	g.Expect(err).To(MatchError(store.ErrNotFound))
}

// actingReview opens a review of a.txt, which reads "one\ntwo\n" as of round 1
// and "one\n" in the base.
func actingReview(g *WithT, t *testing.T, f *fixture) int64 {
	g.THelper()
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	return f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
}

// sendForm sends a form to a handler.
func sendForm(h http.Handler, method, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://eyeball"+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}
