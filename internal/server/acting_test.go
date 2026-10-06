package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"golang.org/x/net/html"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/store"
)

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
		{method: http.MethodPost, path: "/reviews/" + itoa(id) + "/comments", form: url.Values{"round": {"1"}, "level": {"review"}, "body": {"x"}}},
		{method: http.MethodPut, path: "/reviews/" + itoa(id) + "/comments/" + itoa(c.ID), form: url.Values{"body": {"changed"}}},
		{method: http.MethodDelete, path: "/reviews/" + itoa(id) + "/comments/" + itoa(c.ID)},
		{method: http.MethodPost, path: "/reviews/" + itoa(id) + "/decision", form: url.Values{"round": {"1"}, "verdict": {"approve"}}},
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

func TestServer_Surface_addComment(t *testing.T) {
	cases := []struct {
		name   string
		form   url.Values
		list   string
		where  string
		quoted string
	}{
		{name: "a line", form: url.Values{"level": {"line"}, "path": {"a.txt"}, "start": {"2"}}, list: threadID("a.txt", 2), where: "a.txt:2", quoted: "two"},
		{name: "a passage", form: url.Values{"level": {"passage"}, "path": {"a.txt"}, "start": {"1"}, "end": {"2"}}, list: passageID("a.txt", 1, 2), where: "a.txt:1–2", quoted: "one two"},
		{name: "a file", form: url.Values{"level": {"file"}, "path": {"a.txt"}}, list: listID(store.LevelFile, "a.txt"), where: "a.txt"},
		{name: "a removed line, read from the base", form: url.Values{"level": {"file"}, "path": {"a.txt"}, "oldline": {"1"}}, list: listID(store.LevelFile, "a.txt"), where: "a.txt", quoted: "one"},
		{name: "the review", form: url.Values{"level": {"review"}}, list: "comments-review", where: "This review"},
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
			doc := parseFragment(g, res.Body.String())
			list := byID(doc, c.list)
			g.Expect(list).NotTo(BeNil())
			cards := findAll(list, hasClass("comment"))
			g.Expect(cards).To(HaveLen(1))
			g.Expect(text(find(cards[0], "where"))).To(Equal(c.where))
			g.Expect(text(find(cards[0], "quoted"))).To(Equal(c.quoted))
			g.Expect(find(cards[0], "acts")).NotTo(BeNil(), "an open round's comment can still change")
			tally := byID(doc, "tally")
			g.Expect(attr(tally, "hx-swap-oob")).To(Equal("true"))
			g.Expect(text(tally)).To(Equal("1 commentReady to send"))
			g.Expect(find(doc, "decide")).To(BeNil(), "a comment write never replaces the bar")
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
	doc := parseFragment(g, res.Body.String())
	card := find(byID(doc, "comments-review"), "comment")
	g.Expect(text(find(card, "anchor"))).To(Equal("This reviewafter the verdict"))
	g.Expect(find(card, "acts")).To(BeNil(), "a released comment cannot change")
	g.Expect(byID(doc, "tally")).To(BeNil(), "a decided round has no bar to count for")
}

func TestServer_Surface_addCommentRefused(t *testing.T) {
	cases := []struct {
		name   string
		form   url.Values
		status int
	}{
		{name: "a line past the end of the file", form: url.Values{"level": {"line"}, "path": {"a.txt"}, "start": {"3"}}, status: http.StatusBadRequest},
		{name: "a path the round cannot read", form: url.Values{"level": {"line"}, "path": {"missing.txt"}, "start": {"1"}}, status: http.StatusBadRequest},
		{name: "a line with no line", form: url.Values{"level": {"line"}, "path": {"a.txt"}}, status: http.StatusBadRequest},
		{name: "a line with a range", form: url.Values{"level": {"line"}, "path": {"a.txt"}, "start": {"1"}, "end": {"2"}}, status: http.StatusBadRequest},
		{name: "a review comment naming a path", form: url.Values{"level": {"review"}, "path": {"a.txt"}}, status: http.StatusBadRequest},
		{name: "a level that does not exist", form: url.Values{"level": {"chapter"}}, status: http.StatusBadRequest},
		{name: "no text", form: url.Values{"level": {"review"}, "body": {"  "}}, status: http.StatusBadRequest},
		{name: "a climb out of the project", form: url.Values{"level": {"line"}, "path": {"../x"}, "start": {"1"}}, status: http.StatusBadRequest},
		{name: "a round the review does not have", form: url.Values{"level": {"review"}, "round": {"9"}}, status: http.StatusNotFound},
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
		g.Expect(findAll(byID(parseFragment(g, res.Body.String()), "comments-review"), hasClass("comment"))).To(HaveLen(1))
	}
}

func TestServer_Surface_comment(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	c := f.addComment(g, t, store.NewComment{RoundID: f.latestRound(g, t, id), Level: store.LevelReview, Body: "First."})
	path := "/reviews/" + itoa(id) + "/comments/" + itoa(c.ID)

	res := serveSurface(f.s.Surface(), http.MethodGet, path)
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(attr(find(parseFragment(g, res.Body.String()), "comment"), "id")).To(Equal("comment-" + itoa(c.ID)))

	res = serveSurface(f.s.Surface(), http.MethodGet, path+"?edit=1")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	form := find(parseFragment(g, res.Body.String()), "composer")
	g.Expect(attr(form, "hx-put")).To(Equal(path))
	g.Expect(text(findAll(form, isElement("textarea"))[0])).To(Equal("First."))

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
	form := find(parseFragment(g, res.Body.String()), "composer")
	g.Expect(attr(form, "hx-post")).To(Equal("/reviews/" + itoa(id) + "/comments"))
	g.Expect(attr(form, "hx-target")).To(Equal("#" + threadID("a.txt", 2)))
	g.Expect(form.Parent.Data).To(Equal("div"), "a line's composer opens in the row under the line")
	g.Expect(attr(find(parseFragment(g, res.Body.String()), "thread"), "id")).To(Equal(threadID("a.txt", 2)))
	g.Expect(text(find(form, "anchor"))).To(Equal("a.txt:2"))
	var chips []string
	for _, c := range findAll(form, hasClass("chip")) {
		chips = append(chips, text(c)+"="+attr(c, "aria-pressed"))
	}
	g.Expect(chips).To(Equal([]string{"Line 2=true", "File=false", "Review=false"}))

	res = serveSurface(f.s.Surface(), http.MethodGet, "/reviews/"+itoa(id)+"/composer?round=1&level=line")
	g.Expect(res.Code).To(Equal(http.StatusBadRequest))
	res = serveSurface(f.s.Surface(), http.MethodGet, "/reviews/"+itoa(id)+"/composer?round=7&level=review")
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
}

func TestServer_Surface_decide(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	path := "/reviews/" + itoa(id) + "/decision"

	// A change request with nothing in it is the bar again, the note as sent.
	res := sendForm(f.s.Surface(), http.MethodPost, path, url.Values{"round": {"1"}, "verdict": {"changes"}, "note": {"  "}}, true)
	g.Expect(res.Code).To(Equal(http.StatusUnprocessableEntity))
	bar := find(parseFragment(g, res.Body.String()), "decide")
	g.Expect(text(find(bar, "notice"))).To(HavePrefix("Requesting changes needs something to act on."))
	g.Expect(findAll(bar, isElement("textarea"))).To(HaveLen(1))
	g.Expect(res.Body.String()).To(ContainSubstring(`placeholder="A note with the verdict…">  </textarea>`), "the note comes back as it was sent")

	res = sendForm(f.s.Surface(), http.MethodPost, path, url.Values{"round": {"1"}, "verdict": {"maybe"}}, true)
	g.Expect(res.Code).To(Equal(http.StatusBadRequest))
	res = sendForm(f.s.Surface(), http.MethodPost, path, url.Values{"round": {"4"}, "verdict": {"approve"}}, true)
	g.Expect(res.Code).To(Equal(http.StatusNotFound))

	res = sendForm(f.s.Surface(), http.MethodPost, path, url.Values{"round": {"1"}, "verdict": {"approve"}, "note": {"Good."}}, true)
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Header().Get("HX-Refresh")).To(Equal("true"))
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
	g.Expect(text(findAll(find(parseFragment(g, res.Body.String()), "comment"), isElement("p"))[0])).To(Equal("Edited."))

	other := actingReview(g, t, f)
	res = sendForm(f.s.Surface(), http.MethodPut, "/reviews/"+itoa(other)+"/comments/"+itoa(c.ID), url.Values{"body": {"Hijacked."}}, true)
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
	res = sendForm(f.s.Surface(), http.MethodDelete, "/reviews/"+itoa(other)+"/comments/"+itoa(c.ID), nil, true)
	g.Expect(res.Code).To(Equal(http.StatusNotFound))

	res = sendForm(f.s.Surface(), http.MethodDelete, path, nil, true)
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(text(byID(parseFragment(g, res.Body.String()), "tally"))).To(Equal("1 commentReady to send"))
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

// byID finds the element with an id.
func byID(n *html.Node, id string) *html.Node {
	found := findAll(n, func(m *html.Node) bool { return m.Type == html.ElementNode && attr(m, "id") == id })
	if len(found) == 0 {
		return nil
	}
	return found[0]
}

// parseFragment parses an answer as the page receives it. A row under a line
// is a tr, which an HTML parser drops outside a table, so an answer that starts
// with one is parsed inside a table. Anything after the row, such as the tally,
// is moved before the table by the parser and is still found.
func parseFragment(g *WithT, body string) *html.Node {
	g.THelper()
	if !strings.HasPrefix(body, "<tr") {
		return parse(g, body)
	}
	return parse(g, "<table><tbody>"+body+"</tbody></table>")
}
