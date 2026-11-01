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
			path:   "/reviews/" + itoa(id) + "/comments",
			form:   url.Values{"round": {"1"}, "level": {"review"}, "body": {"x"}},
		},
		{
			method: http.MethodPut,
			path:   "/reviews/" + itoa(id) + "/comments/" + itoa(c.ID),
			form:   url.Values{"body": {"changed"}},
		},
		{
			method: http.MethodDelete,
			path:   "/reviews/" + itoa(id) + "/comments/" + itoa(c.ID),
		},
		{
			method: http.MethodPost,
			path:   "/reviews/" + itoa(id) + "/decision",
			form:   url.Values{"round": {"1"}, "verdict": {"approve"}},
		},
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
		res := sendForm(f.s.AgentHandler(), w.method, w.path, w.form, false)
		g.Expect(res.Code).To(Equal(http.StatusNotFound), w.method+" "+w.path)
	}
	comments, err := f.store.Comments(t.Context(), round)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(comments).To(HaveLen(1))
	g.Expect(comments[0].Body).To(Equal("Stays."))
	_, err = f.store.Decision(t.Context(), round)
	g.Expect(err).To(MatchError(store.ErrNotFound))
}

// A failing write that htmx sent is answered with a sentence, since htmx
// shows a plain answer beside the control and would swap no page.
func TestServer_Surface_actingFailureIsPlain(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	g.Expect(f.store.Close()).To(Succeed())
	res := sendForm(f.s.Surface(), http.MethodPost, "/reviews/"+itoa(id)+"/comments", url.Values{"round": {"1"}, "level": {"review"}, "body": {"x"}}, true)
	g.Expect(res.Code).To(Equal(http.StatusInternalServerError))
	g.Expect(res.Header().Get("Content-Type")).To(HavePrefix("text/plain"))
	g.Expect(res.Body.String()).To(Equal(failedWrite))
}

// The form route stores what the JSON route stores, and answers the list or
// the row its comment sits in. The comment as the app draws it is read through
// the JSON route's tests, since the form route goes with the template.
func TestServer_Surface_addComment(t *testing.T) {
	cases := []struct {
		name   string
		form   url.Values
		level  store.Level
		list   string
		start  int
		end    int
		quoted string
	}{
		{
			name:   "a line",
			form:   url.Values{"level": {"line"}, "path": {"a.txt"}, "start": {"2"}},
			level:  store.LevelLine,
			list:   threadID("a.txt", 2),
			start:  2,
			end:    2,
			quoted: "two",
		},
		{
			name:   "a passage",
			form:   url.Values{"level": {"passage"}, "path": {"a.txt"}, "start": {"1"}, "end": {"2"}},
			level:  store.LevelPassage,
			list:   passageID("a.txt", 1, 2),
			start:  1,
			end:    2,
			quoted: "one\ntwo",
		},
		{
			name:  "a file",
			form:  url.Values{"level": {"file"}, "path": {"a.txt"}},
			level: store.LevelFile,
			list:  listID(store.LevelFile, "a.txt"),
		},
		{
			name:   "a removed line, read from the base",
			form:   url.Values{"level": {"file"}, "path": {"a.txt"}, "oldline": {"1"}},
			level:  store.LevelFile,
			list:   listID(store.LevelFile, "a.txt"),
			quoted: "one",
		},
		{
			name:  "the review",
			form:  url.Values{"level": {"review"}},
			level: store.LevelReview,
			list:  "comments-review",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			f := newFixture(g, t)
			id := actingReview(g, t, f)
			c.form.Set("round", "1")
			c.form.Set("body", "A point.")
			res := sendForm(f.s.Surface(), http.MethodPost, "/reviews/"+itoa(id)+"/comments", c.form, true)
			g.Expect(res.Code).To(Equal(http.StatusOK), res.Body.String())
			g.Expect(res.Body.String()).To(ContainSubstring(`id="`+c.list+`"`), "the answer replaces the anchor's list")
			g.Expect(res.Body.String()).To(ContainSubstring("A point."))
			g.Expect(res.Body.String()).To(ContainSubstring(`hx-swap-oob="true"`), "the tally rides along while the bar is there")
			g.Expect(res.Body.String()).NotTo(ContainSubstring("Request changes"), "a comment write never replaces the bar")

			stored, err := f.store.Comments(t.Context(), f.latestRound(g, t, id))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(stored).To(HaveLen(1))
			got := stored[0]
			g.Expect([]any{got.Level, got.Path == "a.txt" || c.level == store.LevelReview, got.StartLine, got.EndLine, got.Quoted, got.Body}).To(Equal([]any{c.level, true, c.start, c.end, c.quoted, "A point."}))
		})
	}
}

func TestServer_Surface_addCommentLate(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	f.decide(g, t, id, store.VerdictApprove, "")
	nextMillisecond()
	res := sendForm(f.s.Surface(), http.MethodPost, "/reviews/"+itoa(id)+"/comments", url.Values{"round": {"1"}, "level": {"review"}, "body": {"One more thing."}}, true)
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Header().Get("HX-Refresh")).To(BeEmpty(), "text may be on the page, so nothing reloads it")
	g.Expect(res.Body.String()).To(ContainSubstring("after the verdict"))
	g.Expect(res.Body.String()).NotTo(ContainSubstring(`class="acts"`), "a released comment cannot change")
	g.Expect(res.Body.String()).NotTo(ContainSubstring("hx-swap-oob"), "a decided round has no bar to count for")
}

func TestServer_Surface_addCommentRefused(t *testing.T) {
	cases := []struct {
		name   string
		form   url.Values
		status int
	}{
		{
			name:   "a line past the end of the file",
			form:   url.Values{"level": {"line"}, "path": {"a.txt"}, "start": {"3"}},
			status: http.StatusBadRequest,
		},
		{
			name:   "a path the round cannot read",
			form:   url.Values{"level": {"line"}, "path": {"missing.txt"}, "start": {"1"}},
			status: http.StatusBadRequest,
		},
		{
			name:   "a line with no line",
			form:   url.Values{"level": {"line"}, "path": {"a.txt"}},
			status: http.StatusBadRequest,
		},
		{
			name:   "a line with a range",
			form:   url.Values{"level": {"line"}, "path": {"a.txt"}, "start": {"1"}, "end": {"2"}},
			status: http.StatusBadRequest,
		},
		{
			name:   "a review comment naming a path",
			form:   url.Values{"level": {"review"}, "path": {"a.txt"}},
			status: http.StatusBadRequest,
		},
		{
			name:   "a level that does not exist",
			form:   url.Values{"level": {"chapter"}},
			status: http.StatusBadRequest,
		},
		{
			name:   "no text",
			form:   url.Values{"level": {"review"}, "body": {"  "}},
			status: http.StatusBadRequest,
		},
		{
			name:   "a climb out of the project",
			form:   url.Values{"level": {"line"}, "path": {"../x"}, "start": {"1"}},
			status: http.StatusBadRequest,
		},
		{
			name:   "a round the review does not have",
			form:   url.Values{"level": {"review"}, "round": {"9"}},
			status: http.StatusNotFound,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			f := newFixture(g, t)
			id := actingReview(g, t, f)
			if !c.form.Has("round") {
				c.form.Set("round", "1")
			}
			if !c.form.Has("body") {
				c.form.Set("body", "A point.")
			}
			res := sendForm(f.s.Surface(), http.MethodPost, "/reviews/"+itoa(id)+"/comments", c.form, true)
			g.Expect(res.Code).To(Equal(c.status))
			g.Expect(res.Header().Get("Content-Type")).To(HavePrefix("text/plain"))
			g.Expect(res.Body.String()).NotTo(BeEmpty())
			comments, err := f.store.Comments(t.Context(), f.latestRound(g, t, id))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(comments).To(BeEmpty())
		})
	}
}

// A retry after a lost answer sends the same key, and the comment is stored
// once.
func TestServer_Surface_addCommentRepeated(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	form := url.Values{"round": {"1"}, "level": {"review"}, "body": {"Once."}, "submission": {"key-1"}}
	for range 2 {
		res := sendForm(f.s.Surface(), http.MethodPost, "/reviews/"+itoa(id)+"/comments", form, true)
		g.Expect(res.Code).To(Equal(http.StatusOK))
	}
	stored, err := f.store.Comments(t.Context(), f.latestRound(g, t, id))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(stored).To(HaveLen(1))
}

func TestServer_Surface_comment(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	c := f.addComment(g, t, store.NewComment{RoundID: f.latestRound(g, t, id), Level: store.LevelReview, Body: "First."})
	path := "/reviews/" + itoa(id) + "/comments/" + itoa(c.ID)

	res := serveSurface(f.s.Surface(), http.MethodGet, path)
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Body.String()).To(ContainSubstring(`id="comment-` + itoa(c.ID) + `"`))

	res = serveSurface(f.s.Surface(), http.MethodGet, path+"?edit=1")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Body.String()).To(ContainSubstring(`hx-put="` + path + `"`))
	g.Expect(res.Body.String()).To(ContainSubstring(">First.</textarea>"), "the edit form holds the comment's text")

	other := actingReview(g, t, f)
	res = serveSurface(f.s.Surface(), http.MethodGet, "/reviews/"+itoa(other)+"/comments/"+itoa(c.ID))
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
}

func TestServer_Surface_composer(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	res := serveSurface(f.s.Surface(), http.MethodGet, "/reviews/"+itoa(id)+"/composer?round=1&level=line&path=a.txt&start=2")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Body.String()).To(ContainSubstring(`hx-post="/reviews/` + itoa(id) + `/comments"`))
	g.Expect(res.Body.String()).To(ContainSubstring(`hx-target="#` + threadID("a.txt", 2) + `"`))
	g.Expect(res.Body.String()).To(ContainSubstring(`id="` + threadID("a.txt", 2) + `"`))

	res = serveSurface(f.s.Surface(), http.MethodGet, "/reviews/"+itoa(id)+"/composer?round=1&level=line")
	g.Expect(res.Code).To(Equal(http.StatusBadRequest))
	res = serveSurface(f.s.Surface(), http.MethodGet, "/reviews/"+itoa(id)+"/composer?round=7&level=review")
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
}

// A decided round takes a comment at every level and sends it at once, so each
// composer says so.
func TestServer_Surface_composerDecided(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	levels := []struct {
		name  string
		query string
	}{
		{name: "review", query: "level=review"},
		{name: "file", query: "level=file&path=a.txt"},
		{
			name:  "removed line",
			query: "level=file&path=a.txt&oldline=2&against=base",
		},
		{name: "line", query: "level=line&path=a.txt&start=2"},
		{name: "passage", query: "level=passage&path=a.txt&start=1&end=2"},
	}
	read := func(query string) string {
		res := serveSurface(f.s.Surface(), http.MethodGet, "/reviews/"+itoa(id)+"/composer?round=1&"+query)
		g.Expect(res.Code).To(Equal(http.StatusOK), query)
		return res.Body.String()
	}
	for _, l := range levels {
		body := read(l.query)
		g.Expect(body).To(ContainSubstring(">Add</button>"), l.name)
		g.Expect(body).NotTo(ContainSubstring("Sends immediately"), l.name)
	}
	f.decide(g, t, id, store.VerdictApprove, "")
	for _, l := range levels {
		body := read(l.query)
		g.Expect(body).To(ContainSubstring("Sends immediately"), l.name)
		g.Expect(body).NotTo(ContainSubstring("Saved as you type"), l.name)
		g.Expect(body).To(ContainSubstring(">Send</button>"), l.name)
		g.Expect(body).NotTo(ContainSubstring(">Add</button>"), l.name)
	}
}

func TestServer_Surface_decide(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	path := "/reviews/" + itoa(id) + "/decision"

	// A change request with nothing in it is the bar again, the note as sent.
	res := sendForm(f.s.Surface(), http.MethodPost, path, url.Values{"round": {"1"}, "verdict": {"changes"}, "note": {"  "}}, true)
	g.Expect(res.Code).To(Equal(http.StatusUnprocessableEntity))
	g.Expect(res.Body.String()).To(ContainSubstring("Requesting changes needs something to act on."))
	g.Expect(strings.Count(res.Body.String(), "<textarea")).To(Equal(1))
	g.Expect(res.Body.String()).To(ContainSubstring(`placeholder="A note with the verdict…">  </textarea>`), "the note comes back as it was sent")

	res = sendForm(f.s.Surface(), http.MethodPost, path, url.Values{"round": {"1"}, "verdict": {"maybe"}}, true)
	g.Expect(res.Code).To(Equal(http.StatusBadRequest))
	res = sendForm(f.s.Surface(), http.MethodPost, path, url.Values{"round": {"4"}, "verdict": {"approve"}}, true)
	g.Expect(res.Code).To(Equal(http.StatusNotFound))

	res = sendForm(f.s.Surface(), http.MethodPost, path, url.Values{"round": {"1"}, "verdict": {"approve"}, "note": {"Good."}}, true)
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Header().Get("HX-Redirect")).To(Equal("/"), "a verdict returns to the queue")
	g.Expect(res.Header().Get("HX-Refresh")).To(BeEmpty(), "a reload would leave the reviewer on the decided round")
	d, err := f.store.Decision(t.Context(), f.latestRound(g, t, id))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(d.Verdict).To(Equal(store.VerdictApprove))
	g.Expect(d.Note).To(Equal("Good."))

	// The same form again names a round decided since.
	res = sendForm(f.s.Surface(), http.MethodPost, path, url.Values{"round": {"1"}, "verdict": {"approve"}}, true)
	g.Expect(res.Code).To(Equal(http.StatusConflict))
	g.Expect(res.Header().Get("HX-Refresh")).To(BeEmpty())
	g.Expect(res.Body.String()).To(Equal(staleVerdict))
}

func TestServer_Surface_editAndDeleteComment(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	round := f.latestRound(g, t, id)
	c := f.addComment(g, t, store.NewComment{RoundID: round, Level: store.LevelReview, Body: "First."})
	keep := f.addComment(g, t, store.NewComment{RoundID: round, Level: store.LevelReview, Body: "Second."})
	path := "/reviews/" + itoa(id) + "/comments/" + itoa(c.ID)

	res := sendForm(f.s.Surface(), http.MethodPut, path, url.Values{"body": {"Edited."}}, true)
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Body.String()).To(ContainSubstring("Edited."))
	edited, err := f.store.Comments(t.Context(), round)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect([]string{edited[0].Body, edited[1].Body}).To(ConsistOf("Edited.", "Second."))

	other := actingReview(g, t, f)
	res = sendForm(f.s.Surface(), http.MethodPut, "/reviews/"+itoa(other)+"/comments/"+itoa(c.ID), url.Values{"body": {"Hijacked."}}, true)
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
	res = sendForm(f.s.Surface(), http.MethodDelete, "/reviews/"+itoa(other)+"/comments/"+itoa(c.ID), nil, true)
	g.Expect(res.Code).To(Equal(http.StatusNotFound))

	res = sendForm(f.s.Surface(), http.MethodDelete, path, nil, true)
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Body.String()).To(ContainSubstring(`<span class="pill plain">1 comment</span>`))
	res = sendForm(f.s.Surface(), http.MethodDelete, path, nil, true)
	g.Expect(res.Code).To(Equal(http.StatusOK), "a comment already gone is gone")

	f.decide(g, t, id, store.VerdictApprove, "")
	keepPath := "/reviews/" + itoa(id) + "/comments/" + itoa(keep.ID)
	res = sendForm(f.s.Surface(), http.MethodPut, keepPath, url.Values{"body": {"Too late."}}, true)
	g.Expect(res.Code).To(Equal(http.StatusConflict))
	g.Expect(res.Header().Get("HX-Refresh")).To(BeEmpty())
	g.Expect(res.Body.String()).To(Equal(releasedComment))
	res = sendForm(f.s.Surface(), http.MethodDelete, keepPath, nil, true)
	g.Expect(res.Code).To(Equal(http.StatusConflict))
}

// actingReview opens a review of a.txt, which reads "one\ntwo\n" as of round 1
// and "one\n" in the base.
func actingReview(g *WithT, t *testing.T, f *fixture) int64 {
	g.THelper()
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	return f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
}

// sendForm sends a form to a handler, as htmx sends it when hx is set.
func sendForm(h http.Handler, method, path string, form url.Values, hx bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://eyeball"+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}
