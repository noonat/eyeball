package server

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/store"
	"github.com/noonat/eyeball/internal/view"
)

// markupBody is a comment that would draw as markup if anything inserted it
// as it is.
const markupBody = `<b onclick="x()">bold</b> & "quoted"`

// getView reads an address of the surface, with the request headers given, and
// returns the response.
func getView(f *fixture, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "http://eyeball"+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res := httptest.NewRecorder()
	f.s.Surface().ServeHTTP(res, req)
	return res
}

// readReview reads a round's view and decodes it, refusing any field the type
// does not hold.
func readReview(g *WithT, f *fixture, path string) view.Review {
	g.THelper()
	res := getView(f, path, nil)
	g.Expect(res.Code).To(Equal(http.StatusOK), res.Body.String())
	var out view.Review
	dec := json.NewDecoder(res.Body)
	dec.DisallowUnknownFields()
	g.Expect(dec.Decode(&out)).To(Succeed())
	return out
}

// readBody reads one card's body from the cards route.
func readBody(g *WithT, f *fixture, path string) view.CardBody {
	g.THelper()
	res := getView(f, path, nil)
	g.Expect(res.Code).To(Equal(http.StatusOK), path+" "+res.Body.String())
	var out view.CardBody
	dec := json.NewDecoder(res.Body)
	dec.DisallowUnknownFields()
	g.Expect(dec.Decode(&out)).To(Succeed())
	return out
}

// largeReview opens a round past the sending thresholds that holds a code file
// with a line comment, a document with a passage comment, a file with a file
// comment and one comment on the review, and returns its id.
func largeReview(g *WithT, t *testing.T, f *fixture) int64 {
	g.THelper()
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	commitFile(g, repo, "docs/g.md", "# G\n\nOne.\n\nTwo.\n")
	write(g, repo, "docs/g.md", "# G\n\nOne changed.\n\nTwo.\n")
	write(g, repo, "d/big.txt", strings.Repeat("line\n", maxSentLines+20))
	paths := []string{"a.txt", "docs", "d"}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: paths, Note: "A large round."}).Review.ID
	round, err := f.store.Round(t.Context(), id, 1)
	g.Expect(err).NotTo(HaveOccurred())
	comments := []store.NewComment{
		{
			Level:     store.LevelLine,
			Path:      "a.txt",
			StartLine: 2,
			EndLine:   2,
			Quoted:    "two",
			Body:      markupBody,
		},
		{
			Level:     store.LevelPassage,
			Path:      "docs/g.md",
			StartLine: 3,
			EndLine:   3,
			Quoted:    "One changed.",
			Body:      "Which one?",
		},
		{Level: store.LevelFile, Path: "d/big.txt", Body: "Too many lines."},
		{Level: store.LevelReview, Body: "Looks large."},
	}
	for _, c := range comments {
		c.RoundID = round.ID
		_, err := f.s.db.AddComment(t.Context(), c)
		g.Expect(err).NotTo(HaveOccurred())
	}
	return id
}

// roundID is a review's first round's id.
func roundID(g *WithT, t *testing.T, f *fixture, review int64) int64 {
	g.THelper()
	r, err := f.store.Round(t.Context(), review, 1)
	g.Expect(err).NotTo(HaveOccurred())
	return r.ID
}

// requestFor is a request for a review's round, the way the mux hands one to a
// handler. A zero round is the review's latest.
func requestFor(review int64, round int, query string) *http.Request {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://eyeball/x"+query, nil)
	req.SetPathValue("review", itoa(review))
	if round != 0 {
		req.SetPathValue("round", itoa(int64(round)))
	}
	return req
}

func Test_acceptsGzip(t *testing.T) {
	cases := []struct {
		header string
		want   bool
	}{
		{header: "", want: false},
		{header: "gzip", want: true},
		{header: "GZIP", want: true},
		{header: "br, gzip;q=0.8", want: true},
		{header: "identity", want: false},
		{header: "gzip;q=0", want: false},
		{header: "deflate, br", want: false},
	}
	for _, c := range cases {
		t.Run(c.header, func(t *testing.T) {
			g := NewWithT(t)
			req := httptest.NewRequest(http.MethodGet, "http://eyeball/x", nil)
			if c.header != "" {
				req.Header.Set("Accept-Encoding", c.header)
			}
			g.Expect(acceptsGzip(req)).To(Equal(c.want))
		})
	}
}

func Test_cardBody(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := largeReview(g, t, f)
	_, page, ok, err := f.s.roundPage(requestFor(id, 0, ""))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())

	byPath := map[string]view.CardBody{}
	for _, file := range page.Shown {
		byPath[file.Path] = cardBody(file)
	}
	g.Expect(byPath).To(HaveLen(3))

	code := byPath["a.txt"]
	g.Expect(code.Kind).To(Equal(view.FileBodyRows))
	g.Expect(code.Threads).To(HaveLen(1))
	thread := code.Threads[0]
	g.Expect(thread.Line).To(BeEquivalentTo(2))
	g.Expect(thread.ID).To(Equal(threadID("a.txt", 2)))
	g.Expect(thread.Comments).To(HaveLen(1))
	g.Expect(thread.Comments[0].Body).To(Equal(markupBody), "a comment is a string")
	g.Expect(thread.Comments[0].Editable).To(BeTrue())
	g.Expect(code.ListID).To(Equal(listID(store.LevelFile, "a.txt")))
	g.Expect(code.Slot).To(Equal(slotID("a.txt")))

	doc := byPath["docs/g.md"]
	g.Expect(doc.Kind).To(Equal(view.FileBodyDocument))
	var threads []string
	for _, part := range doc.Passages {
		if part.Passage != nil && part.Passage.Thread != nil {
			threads = append(threads, fmt.Sprintf("%d-%d %s", part.Passage.First, part.Passage.Last, part.Passage.Thread.Comments[0].Body))
			g.Expect(part.Passage.Thread.ID).To(Equal(passageID("docs/g.md", 3, 3)))
			g.Expect(part.Passage.Changed).To(BeTrue())
		}
	}
	g.Expect(threads).To(Equal([]string{"3-3 Which one?"}))

	big := byPath["d/big.txt"]
	g.Expect(big.Comments).To(HaveLen(1))
	g.Expect(big.Comments[0].Where).To(Equal("d/big.txt"))
	g.Expect(big.Comments[0].Href).To(ContainSubstring("/comments/"))
}

func Test_compressed(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := largeReview(g, t, f)
	targets := []string{
		"/api/reviews/" + itoa(id),
		"/api/reviews/" + itoa(id) + "/rounds/1/cards/a.txt",
		"/api/",
		"/static/app.js",
		"/static/app.css",
		"/static/shell.css",
	}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			g := NewWithT(t)
			plain := getView(f, target, nil)
			g.Expect(plain.Code).To(Equal(http.StatusOK))
			g.Expect(plain.Header().Get("Content-Encoding")).To(BeEmpty(), "no Accept-Encoding, identity bytes")
			g.Expect(plain.Header().Get("Vary")).To(ContainSubstring("Accept-Encoding"))

			zipped := getView(f, target, map[string]string{"Accept-Encoding": "gzip, deflate"})
			g.Expect(zipped.Code).To(Equal(http.StatusOK))
			g.Expect(zipped.Header().Get("Content-Encoding")).To(Equal("gzip"))
			g.Expect(zipped.Header().Get("Vary")).To(ContainSubstring("Accept-Encoding"))
			g.Expect(zipped.Header().Get("Content-Type")).To(Equal(plain.Header().Get("Content-Type")))
			g.Expect(zipped.Body.Len()).To(BeNumerically("<", plain.Body.Len()))
			reader, err := gzip.NewReader(zipped.Body)
			g.Expect(err).NotTo(HaveOccurred())
			unzipped, err := io.ReadAll(reader)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(unzipped).To(Equal(plain.Body.Bytes()))

			refused := getView(f, target, map[string]string{"Accept-Encoding": "gzip;q=0, identity"})
			g.Expect(refused.Header().Get("Content-Encoding")).To(BeEmpty(), "gzip with a quality of zero")
		})
	}
}

// The stream and the raw route are never wrapped: gzip would hold the stream's
// events until the buffer filled, and a range names bytes of the stored file.
func Test_compressedLeftAlone(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	commitFile(g, repo, "site/index.html", "<p>before</p>\n")
	write(g, repo, "site/index.html", "<p>round two</p>\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"site"}, Note: "first"}).Review.ID
	srv := httptest.NewServer(f.s.Surface())
	t.Cleanup(srv.Close)
	accept := map[string]string{"Accept-Encoding": "gzip"}

	t.Run("the raw route answers a range as it is stored", func(t *testing.T) {
		g := NewWithT(t)
		res := getView(f, "/reviews/"+itoa(id)+"/raw/1/site/index.html", map[string]string{"Accept-Encoding": "gzip", "Range": "bytes=3-7"})
		g.Expect(res.Code).To(Equal(http.StatusPartialContent))
		g.Expect(res.Header().Get("Content-Range")).To(HavePrefix("bytes 3-7/"))
		g.Expect(res.Header().Get("Content-Encoding")).To(BeEmpty())
		g.Expect(res.Body.String()).To(Equal("round"))
	})

	t.Run("the stream is sent as it is", func(t *testing.T) {
		g := NewWithT(t)
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events?review="+itoa(id), nil)
		g.Expect(err).NotTo(HaveOccurred())
		for k, v := range accept {
			req.Header.Set(k, v)
		}
		res, err := http.DefaultClient.Do(req)
		g.Expect(err).NotTo(HaveOccurred())
		t.Cleanup(func() { _ = res.Body.Close() })
		g.Expect(res.Header.Get("Content-Type")).To(HavePrefix("text/event-stream"))
		g.Expect(res.Header.Get("Content-Encoding")).To(BeEmpty())
		line := make([]byte, len("retry: 2000"))
		_, err = io.ReadFull(res.Body, line)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(string(line)).To(Equal("retry: 2000"), "the first event arrives without waiting for a buffer to fill")
	})

	t.Run("no route but the views and the assets is wrapped", func(t *testing.T) {
		g := NewWithT(t)
		var wrapped []string
		for _, rt := range routes {
			if rt.gzip {
				wrapped = append(wrapped, rt.pattern)
			}
		}
		g.Expect(wrapped).To(ConsistOf(
			"GET /api/{$}",
			"GET /api/reviews/{review}",
			"GET /api/reviews/{review}/rounds/{round}",
			"GET /api/reviews/{review}/rounds/{round}/cards/{path...}",
			"GET /api/reviews/{review}/files/{path...}",
			"GET /api/reviews/{review}/rounds/{round}/files/{path...}",
			"GET /api/projects/{project}",
			"GET /api/projects/{project}/files/{path...}",
			"GET /static/app.css",
			"GET /static/shell.css",
			"GET /static/app.js",
		))
	})

	t.Run("a font keeps its ETag and its 304", func(t *testing.T) {
		g := NewWithT(t)
		target := "/static/fonts/atkinson-hyperlegible-mono/latin-wght-normal.woff2"
		res := getView(f, target, accept)
		g.Expect(res.Code).To(Equal(http.StatusOK))
		g.Expect(res.Header().Get("Content-Encoding")).To(BeEmpty())
		tag := res.Header().Get("ETag")
		g.Expect(tag).NotTo(BeEmpty())
		again := getView(f, target, map[string]string{"Accept-Encoding": "gzip", "If-None-Match": tag})
		g.Expect(again.Code).To(Equal(http.StatusNotModified))
	})
}

// A 304 or a 204 has no body, so Content-Encoding on it would describe nothing.
func Test_compressedNoBody(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{name: "not modified", status: http.StatusNotModified},
		{name: "no content", status: http.StatusNoContent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			req := httptest.NewRequest(http.MethodGet, "http://eyeball/x", nil)
			req.Header.Set("Accept-Encoding", "gzip")
			res := httptest.NewRecorder()
			w, done := compressed(res, req)
			w.Header().Set("ETag", `"abc"`)
			w.WriteHeader(c.status)
			done()
			g.Expect(res.Code).To(Equal(c.status))
			g.Expect(res.Header().Get("Content-Encoding")).To(BeEmpty())
			g.Expect(res.Header().Get("ETag")).To(Equal(`"abc"`))
			g.Expect(res.Body.Len()).To(BeZero())
		})
	}
}

// A round's view and its cards route build a body with one function, so the
// bodies match for every card of a round past the thresholds, which the view
// leaves out, and of a round under them, which the view holds.
func expectCardBodies(g *WithT, f *fixture, id int64, round int, query string) []string {
	g.THelper()
	_, page, ok, err := f.s.roundPage(requestFor(id, round, query))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	var paths []string
	for _, file := range page.Shown {
		target := fmt.Sprintf("/api/reviews/%d/rounds/%d/cards/%s%s", id, page.Round, file.Path, query)
		g.Expect(readBody(g, f, target)).To(Equal(cardBody(file)), target)
		paths = append(paths, file.Path)
	}
	return paths
}

func TestServer_cardViewComparesOnlyItsFile(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := largeReview(g, t, f)
	want := readBody(g, f, "/api/reviews/"+itoa(id)+"/rounds/1/cards/a.txt")

	files, err := f.store.RoundFiles(t.Context(), roundID(g, t, f, id))
	g.Expect(err).NotTo(HaveOccurred())
	var digest string
	for _, file := range files {
		if file.Path == "d/big.txt" {
			digest = file.Digest
		}
	}
	g.Expect(digest).NotTo(BeEmpty())
	blobPath := filepath.Join(f.state, "blobs", digest[0:2], digest[2:4], digest)
	g.Expect(os.Remove(blobPath)).To(Succeed())

	_, _, _, err = f.s.roundPage(requestFor(id, 0, ""))
	g.Expect(err).To(HaveOccurred(), "the whole round reads every file")
	g.Expect(readBody(g, f, "/api/reviews/"+itoa(id)+"/rounds/1/cards/a.txt")).To(Equal(want))
}

func TestServer_cardViewFreshDocument(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	cases := []struct {
		name  string
		extra int
		lines int
		fresh bool
	}{
		{
			name:  "alone",
			extra: 0,
			lines: maxSentLines + 20,
			fresh: true,
		},
		{
			name:  "beside many files",
			extra: maxSentFiles + 1,
			lines: 3,
			fresh: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			repo := gitRepo(g, filepath.Join(t.TempDir(), "fresh"))
			write(g, repo, "docs/new.md", "# New\n\n"+strings.Repeat("A line of prose.\n\n", c.lines))
			for i := range c.extra {
				write(g, repo, fmt.Sprintf("files/f%02d.txt", i), "text\n")
			}
			paths := []string{"docs"}
			if c.extra > 0 {
				paths = append(paths, "files")
			}
			id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: paths, Note: "A new document."}).Review.ID
			_, err := f.s.db.AddComment(t.Context(), store.NewComment{RoundID: roundID(g, t, f, id), Level: store.LevelFile, Path: "docs/new.md", Body: "On the file."})
			g.Expect(err).NotTo(HaveOccurred())

			_, page, ok, err := f.s.roundPage(requestFor(id, 0, ""))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ok).To(BeTrue())
			g.Expect(page.Fresh != nil).To(Equal(c.fresh))
			compared := expectCardBodies(g, f, id, 0, "")
			g.Expect(compared).To(HaveLen(1 + c.extra))
			body := readBody(g, f, fmt.Sprintf("/api/reviews/%d/rounds/1/cards/docs/new.md", id))
			if c.fresh {
				g.Expect(body.Comments).To(BeEmpty(), "the page itself holds the review's comments")
			} else {
				g.Expect(body.Comments).To(HaveLen(1), "a card holds the file's comments")
			}
		})
	}
}

func TestServer_cardViewLaterRound(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := largeReview(g, t, f)
	g.Expect(f.s.db.Decide(t.Context(), roundID(g, t, f, id), store.VerdictChanges, "More.")).To(Succeed())
	review, err := f.store.Review(t.Context(), id)
	g.Expect(err).NotTo(HaveOccurred())
	project, err := f.store.Project(t.Context(), review.ProjectID)
	g.Expect(err).NotTo(HaveOccurred())
	write(g, project.Root, "a.txt", "one\ntwo\nthree\n")
	write(g, project.Root, "docs/g.md", "# G\n\nOne changed again.\n\nTwo.\n")
	write(g, project.Root, "d/big.txt", strings.Repeat("line\n", maxSentLines+40))
	f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: project.Root}, Note: "Round two.", Review: id})
	second, err := f.store.Round(t.Context(), id, 2)
	g.Expect(err).NotTo(HaveOccurred())
	comments := []store.NewComment{
		{
			RoundID:   second.ID,
			Level:     store.LevelLine,
			Path:      "a.txt",
			StartLine: 3,
			EndLine:   3,
			Quoted:    "three",
			Body:      "New line.",
		},
		{
			RoundID: second.ID,
			Level:   store.LevelFile,
			Path:    "d/big.txt",
			Body:    "Still long.",
		},
	}
	for _, c := range comments {
		_, err := f.s.db.AddComment(t.Context(), c)
		g.Expect(err).NotTo(HaveOccurred())
	}

	queries := []struct {
		query string
		paths []string
	}{
		{query: "", paths: []string{"a.txt"}},
		{query: "?against=base", paths: []string{"a.txt", "d/big.txt", "docs/g.md"}},
	}
	rounds := []int{0, 2, 1}
	for _, q := range queries {
		for _, round := range rounds {
			paths := expectCardBodies(g, f, id, round, q.query)
			if round != 1 {
				g.Expect(paths).To(ContainElements(q.paths), q.query)
			}
		}
	}
}

// A file the review covers that did not move has no card, so the route answers
// 404 for it as the round's view shows none.
func TestServer_cardViewUnmovedFile(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "unmoved"))
	commitFile(g, repo, "docs/same.txt", "unchanged\n")
	write(g, repo, "docs/changed.txt", "new\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"docs"}, Note: "One moved."}).Review.ID

	g.Expect(expectCardBodies(g, f, id, 0, "")).To(Equal([]string{"docs/changed.txt"}))
	res := getView(f, "/api/reviews/"+itoa(id)+"/rounds/1/cards/docs/same.txt", nil)
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
}

func TestServer_handleCardView(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := largeReview(g, t, f)

	deferred := readReview(g, f, "/api/reviews/"+itoa(id))
	g.Expect(deferred.Cards).To(HaveLen(3))
	_, page, ok, err := f.s.roundPage(requestFor(id, 0, ""))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	want := map[string]view.CardBody{}
	for _, file := range page.Shown {
		want[file.Path] = cardBody(file)
	}
	for _, card := range deferred.Cards {
		g.Expect(card.Deferred).To(BeTrue(), card.Path)
		g.Expect(card.Body).To(BeNil(), card.Path)
		g.Expect(card.BodyHref).To(Equal(fmt.Sprintf("/api/reviews/%d/rounds/1/cards/%s", id, card.Path)))
		got := readBody(g, f, card.BodyHref)
		g.Expect(got).To(Equal(want[card.Path]), card.Path)
	}
	g.Expect(deferred.Cards[0].SkeletonLines).To(BeNumerically(">", 0))

	// Under the thresholds the round holds the bodies the route would send.
	repo := gitRepo(g, filepath.Join(t.TempDir(), "small"))
	small := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "Small."}).Review.ID
	sent := readReview(g, f, "/api/reviews/"+itoa(small))
	g.Expect(sent.Cards).To(HaveLen(1))
	g.Expect(sent.Cards[0].Deferred).To(BeFalse())
	g.Expect(sent.Cards[0].Body).NotTo(BeNil())
	g.Expect(readBody(g, f, sent.Cards[0].BodyHref)).To(Equal(*sent.Cards[0].Body))

	// A bad path or round is the view that says nothing is there.
	missing := []string{
		"/api/reviews/" + itoa(id) + "/rounds/1/cards/nope.txt",
		"/api/reviews/" + itoa(id) + "/rounds/1/cards/keep.txt",
		"/api/reviews/" + itoa(id) + "/rounds/9/cards/a.txt",
		"/api/reviews/" + itoa(id) + "/rounds/x/cards/a.txt",
		"/api/reviews/999/rounds/1/cards/a.txt",
	}
	for _, target := range missing {
		res := getView(f, target, nil)
		g.Expect(res.Code).To(Equal(http.StatusNotFound), target)
		var body map[string]any
		g.Expect(json.Unmarshal(res.Body.Bytes(), &body)).To(Succeed())
		g.Expect(body).To(HaveKey("notice"), target)
	}
}

// expectCardBodies finds the cards route's body for every card of a round equal
// to the body the whole round builds for it, and returns the paths it compared.
// A later round compares against the previous one, or with ?against=base
// against the base, and the route compares only its file either way.
// Whether a document draws as a card or as the page itself depends on how many
// files the round moved, which a card built alone cannot see. The route reads
// the count from the captures, so its body matches the round's either way.
// The route compares its own file and no other. A round whose other file can no
// longer be read still answers for this one, which a route that built the
// whole round could not.
func TestServer_handleReviewView(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := largeReview(g, t, f)

	round := readReview(g, f, "/api/reviews/"+itoa(id))
	g.Expect(round.ID).To(Equal(id))
	g.Expect(round.Round).To(BeEquivalentTo(1))
	g.Expect(round.Note).To(Equal([]string{"A large round."}))
	g.Expect(round.Files).To(Equal("3 files"))
	g.Expect(round.Listed).To(BeFalse())
	g.Expect(round.Acting).NotTo(BeNil())
	g.Expect(round.Acting.State).To(Equal("open"))
	g.Expect(round.Acting.Bar).NotTo(BeNil())
	g.Expect(round.Acting.Bar.Count).To(BeEquivalentTo(4))
	g.Expect(round.Acting.Comments).To(HaveLen(1))
	g.Expect(round.Acting.Comments[0].Body).To(Equal("Looks large."))
	g.Expect(round.Against).To(BeNil(), "a first round has only its base")
	g.Expect(round.Comments).To(BeEmpty())
	paths := make([]string, len(round.Cards))
	for i, card := range round.Cards {
		paths[i] = card.Path
		g.Expect(card.ID).To(Equal("file-" + pathKey(card.Path)))
	}
	g.Expect(paths).To(Equal([]string{"a.txt", "d/big.txt", "docs/g.md"}))

	// The round's address and the review's are one view, and the round's page
	// title is the one the template gave it.
	g.Expect(readReview(g, f, "/api/reviews/"+itoa(id)+"/rounds/1")).To(Equal(round))

	t.Run("the stamp is the stream's", func(t *testing.T) {
		g := NewWithT(t)
		stamp, err := f.s.reviewStamp(t.Context(), id)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(stamp).NotTo(BeEmpty())
		g.Expect(round.Stamp).To(Equal(stamp))
		g.Expect(round.Stream).To(Equal("/events?review=" + itoa(id)))
		// A reply moves the stamp, and the view follows it.
		c, err := f.store.Comments(t.Context(), roundID(g, t, f, id))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(f.s.db.Decide(t.Context(), roundID(g, t, f, id), store.VerdictChanges, "")).To(Succeed())
		g.Expect(f.s.db.Reply(t.Context(), c[0].ID, "Done.")).To(Succeed())
		moved, err := f.s.reviewStamp(t.Context(), id)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(moved).NotTo(Equal(stamp))
		decided := readReview(g, f, "/api/reviews/"+itoa(id))
		g.Expect(decided.Stamp).To(Equal(moved))
		g.Expect(decided.Decided).NotTo(BeNil())
		g.Expect(decided.Decided.Approved).To(BeFalse())
		g.Expect(decided.Acting.Bar).To(BeNil())
		g.Expect(decided.Acting.State).To(Equal("late"))
		g.Expect(decided.Replies).To(HaveLen(1))
		g.Expect(decided.Replies[0].Body).To(Equal("Done."))
	})

	t.Run("the comparison control follows ?against", func(t *testing.T) {
		g := NewWithT(t)
		repo := gitRepo(g, filepath.Join(t.TempDir(), "again"))
		second := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "One."}).Review.ID
		g.Expect(f.s.db.Decide(t.Context(), roundID(g, t, f, second), store.VerdictChanges, "More.")).To(Succeed())
		write(g, repo, "a.txt", "one\ntwo\nthree\n")
		f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Note: "Two.", Review: second})
		since := readReview(g, f, "/api/reviews/"+itoa(second))
		g.Expect(since.Against).To(Equal(&view.Against{Previous: "Since round 1"}))
		whole := readReview(g, f, "/api/reviews/"+itoa(second)+"?against=base")
		g.Expect(whole.Against).To(Equal(&view.Against{Previous: "Since round 1", Base: true}))
		g.Expect(whole.AllFiles).To(HaveSuffix("?against=base"))
		// Any other value is the default, as the page treats it today.
		other := readReview(g, f, "/api/reviews/"+itoa(second)+"?against=sideways")
		g.Expect(other).To(Equal(since))
		earlier := readReview(g, f, "/api/reviews/"+itoa(second)+"/rounds/1")
		g.Expect(earlier.Round).To(BeEquivalentTo(1))
		g.Expect(earlier.Closed).To(ContainSubstring("Round 2 is the current one"))
		g.Expect(earlier.Back).To(Equal("/reviews/" + itoa(second)))
	})
}

func TestServer_reviewViewNotFound(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := largeReview(g, t, f)
	targets := []string{
		"/api/reviews/999",
		"/api/reviews/abc",
		"/api/reviews/0",
		"/api/reviews/" + itoa(id) + "/rounds/9",
		"/api/reviews/" + itoa(id) + "/rounds/x",
		"/api/reviews/999/rounds/1",
	}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			g := NewWithT(t)
			res := getView(f, target, nil)
			g.Expect(res.Code).To(Equal(http.StatusNotFound))
			g.Expect(res.Header().Get("Content-Type")).To(Equal("application/json"))
			var missing view.NotFound
			g.Expect(json.Unmarshal(res.Body.Bytes(), &missing)).To(Succeed())
			g.Expect(missing.Notice).To(BeNil())
			g.Expect(missing.WaitingCount).NotTo(BeNil())
			// The page's address answers the shell, since the app draws the 404 from
			// the view.
			page := getView(f, strings.TrimPrefix(target, "/api"), nil)
			g.Expect(page.Code).To(Equal(http.StatusOK))
			g.Expect(page.Body.String()).To(ContainSubstring(`<div id="root"></div>`))
		})
	}
}
