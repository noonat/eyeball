package server

import (
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/store"
	. "github.com/onsi/gomega"
	"golang.org/x/net/html"
)

func Test_thousands(t *testing.T) {
	g := NewWithT(t)
	g.Expect(thousands(0)).To(Equal("0"))
	g.Expect(thousands(999)).To(Equal("999"))
	g.Expect(thousands(1284)).To(Equal("1,284"))
	g.Expect(thousands(1234567)).To(Equal("1,234,567"))
	g.Expect(thousands(-1284)).To(Equal("-1,284"))
}

func TestServer_Surface_review(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo, Agent: "spec/012-surface"}
	note := "Upload worker and its retry ladder\n\nThe second paragraph says what to look at."
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: note}).Review.ID

	doc := f.page(g, "/reviews/"+itoa(id), http.StatusOK)
	g.Expect(text(find(doc, "topbar"))).To(Equal("QueueUpload worker and its retry ladder"), "the full title, which app.css truncates")
	g.Expect(text(find(find(doc, "brief"), "where"))).To(Equal("Waiting nowRound 1harborspec/012-surface"))
	g.Expect(text(find(doc, "title"))).To(Equal("Upload worker and its retry ladder"))
	var paragraphs []string
	for _, p := range findAll(find(doc, "note"), func(n *html.Node) bool { return n.Data == "p" }) {
		paragraphs = append(paragraphs, text(p))
	}
	g.Expect(paragraphs).To(Equal([]string{"Upload worker and its retry ladder", "The second paragraph says what to look at."}))
	g.Expect(text(find(doc, "toolbar"))).To(Equal("1 file +1 −0"))
	g.Expect(find(doc, "segmented")).To(BeNil(), "a first round has only its base")
	g.Expect(find(doc, "decide")).To(BeNil(), "no decision bar until the write path can act on it")

	files := findAll(doc, hasClass("file"))
	g.Expect(files).To(HaveLen(1))
	g.Expect(text(find(files[0], "path"))).To(Equal("a.txt"))
	g.Expect(hasAttr(files[0], "open")).To(BeTrue())
	var rows []string
	for _, tr := range findAll(files[0], func(n *html.Node) bool { return n.Data == "tr" }) {
		rows = append(rows, strings.TrimSpace(attr(tr, "class")+" "+text(tr)))
	}
	g.Expect(rows).To(Equal([]string{"hunkhead @@ -1 +1,2 @@", "1one", "add 2+two"}))
}

func TestServer_Surface_reviewAgainst(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "more")
	write(g, repo, "a.txt", "one\ntwo\nthree\n")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})

	// From round two the previous round is the default: one line moved.
	doc := f.page(g, "/reviews/"+itoa(id), http.StatusOK)
	links := findAll(find(doc, "segmented"), func(n *html.Node) bool { return n.Data == "a" })
	g.Expect(links).To(HaveLen(2))
	g.Expect(text(links[0])).To(Equal("Since round 1"))
	g.Expect(attr(links[0], "href")).To(Equal("?against=previous"))
	g.Expect(attr(links[0], "aria-current")).To(Equal("true"))
	g.Expect(text(links[1])).To(Equal("Whole review"))
	g.Expect(attr(links[1], "aria-current")).To(BeEmpty())
	g.Expect(text(find(doc, "toolbar"))).To(HavePrefix("1 file +1 −0"))

	// The whole review is measured from the base: two lines.
	doc = f.page(g, "/reviews/"+itoa(id)+"?against=base", http.StatusOK)
	links = findAll(find(doc, "segmented"), func(n *html.Node) bool { return n.Data == "a" })
	g.Expect(attr(links[1], "aria-current")).To(Equal("true"))
	g.Expect(text(find(doc, "toolbar"))).To(HavePrefix("1 file +2 −0"))
}

func TestServer_Surface_reviewChangesRequested(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "Redo it.")

	doc := f.page(g, "/reviews/"+itoa(id), http.StatusOK)
	g.Expect(text(find(find(doc, "brief"), "where"))).To(HavePrefix("Changes requested nowRound 1"))
	g.Expect(text(find(doc, "notice"))).To(Equal("Changes requested. The verdict went out just now."))
}

func TestServer_Surface_reviewComments(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	roundID := f.latestRound(g, t, id)
	f.addComment(g, t, store.NewComment{RoundID: roundID, Level: store.LevelLine, Path: "a.txt", StartLine: 2, EndLine: 2, Quoted: "two", Body: "Why two?"})
	f.addComment(g, t, store.NewComment{RoundID: roundID, Level: store.LevelLine, Path: "a.txt", StartLine: 1, EndLine: 2, Quoted: "one\ntwo", Body: "Both lines."})
	f.addComment(g, t, store.NewComment{RoundID: roundID, Level: store.LevelReview, Body: "Overall fine."})
	f.decide(g, t, id, store.VerdictApprove, "Good.\n\nShip it.")
	// The store keeps times to the millisecond, and a comment in the same
	// millisecond as the verdict is not after it.
	nextMillisecond()
	f.addComment(g, t, store.NewComment{RoundID: roundID, Level: store.LevelReview, Body: "One more thing."})

	doc := f.page(g, "/reviews/"+itoa(id), http.StatusOK)
	g.Expect(text(find(find(doc, "brief"), "where"))).To(HavePrefix("Approved nowRound 1"))
	g.Expect(text(find(doc, "notice"))).To(Equal("Approved. 3 comments and the verdict went out just now."), "the late comment went out on its own")
	notes := findAll(doc, hasClass("note"))
	g.Expect(notes).To(HaveLen(2))
	g.Expect(text(notes[0])).To(Equal("Verdict note Good. Ship it."))
	g.Expect(text(notes[1])).To(Equal("Note first"))
	g.Expect(find(doc, "decide")).To(BeNil(), "a decided round has no decision bar")

	// A file's comments follow its diff, in line order.
	file := find(doc, "file")
	var onFile []string
	for _, c := range findAll(file, hasClass("comment")) {
		onFile = append(onFile, text(c))
	}
	g.Expect(onFile).To(Equal([]string{"a.txt:1–2 one two Both lines.", "a.txt:2 two Why two?"}))

	// The review's own comments follow the files, and the late one says so.
	section := find(doc, "section")
	g.Expect(text(find(section, "count"))).To(Equal("2"))
	var onReview []string
	for _, c := range findAll(section, hasClass("comment")) {
		onReview = append(onReview, text(c))
	}
	g.Expect(onReview).To(Equal([]string{"This review Overall fine.", "This reviewafter the verdict One more thing."}))
	g.Expect(findAll(doc, func(n *html.Node) bool { return n.Data == "form" || n.Data == "textarea" })).To(BeEmpty(), "comments are read-only")
}

func TestServer_Surface_reviewFilesWithNoRows(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", strings.Repeat("x\n", 20001))
	write(g, repo, "a.bin", "\x00\x01binary")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt", "a.bin"}, Note: "big"}).Review.ID

	doc := f.page(g, "/reviews/"+itoa(id), http.StatusOK)
	got := map[string]string{}
	for _, file := range findAll(doc, hasClass("file")) {
		got[text(find(file, "path"))] = text(find(file, "toobig"))
		g.Expect(find(file, "code")).To(BeNil())
	}
	g.Expect(got).To(Equal(map[string]string{
		"a.bin": "Binary file, not shown as a diff.",
		"a.txt": "Too large to show as a diff.",
	}))
}

func TestServer_Surface_reviewMarksAChangedWord(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	write(g, repo, "x.go", "package x\n\nfunc f() int {\n\treturn 1\n}\n")
	git(g, repo, "add", "-A")
	tree := git(g, repo, "write-tree")
	commit := git(g, repo, "commit-tree", tree, "-p", "main", "-m", "x")
	git(g, repo, "update-ref", "refs/heads/main", commit)
	write(g, repo, "x.go", "package x\n\nfunc f() int {\n\treturn 2\n}\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"x.go"}, Note: "two"}).Review.ID

	res := serveSurface(f.s.Surface(), http.MethodGet, "/reviews/"+itoa(id))
	g.Expect(res.Code).To(Equal(http.StatusOK))
	// The changed word is a mark around its role, and the keyword before it
	// is highlighted on its own.
	g.Expect(res.Body.String()).To(ContainSubstring(`<td class="src">	<span class="k">return</span> <span class="w"><span class="n">2</span></span></td>`))
	g.Expect(res.Body.String()).To(ContainSubstring(`<td class="src">	<span class="k">return</span> <span class="w"><span class="n">1</span></span></td>`))
}

func TestServer_Surface_reviewNotFound(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	paths := []string{"/reviews/999", "/reviews/abc", "/reviews/0"}
	for _, path := range paths {
		doc := f.page(g, path, http.StatusNotFound)
		g.Expect(text(find(doc, "empty"))).To(ContainSubstring("There is no page at this address."), path)
	}
}

func TestServer_Surface_reviewOpensUpToItsLimits(t *testing.T) {
	cases := []struct {
		name  string
		files int
		lines int
		open  bool
	}{
		{name: "ten files", files: 10, lines: 1, open: true},
		{name: "eleven files", files: 11, lines: 1, open: false},
		{name: "three hundred lines", files: 1, lines: 300, open: true},
		{name: "three hundred and one lines", files: 1, lines: 301, open: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			f := newFixture(g, t)
			repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
			// gitRepo's own edit to a.txt is undone, so only these files move.
			write(g, repo, "a.txt", "one\n")
			for i := range c.files {
				write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", c.lines))
			}
			id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID

			doc := f.page(g, "/reviews/"+itoa(id), http.StatusOK)
			files := findAll(doc, hasClass("file"))
			g.Expect(files).To(HaveLen(c.files))
			for _, file := range files {
				g.Expect(hasAttr(file, "open")).To(Equal(c.open), text(find(file, "path")))
			}
		})
	}
}

func TestServer_Surface_reviewTotalsMatchTheStoredSize(t *testing.T) {
	binary := "\x00\x01\x02binary"
	cases := []struct {
		name  string
		paths []string
		// first and second change the working copy before each round.
		first, second func(g *WithT, repo string)
	}{
		{
			name:  "a file past the diff limit",
			paths: []string{"a.txt"},
			first: func(g *WithT, repo string) {
				write(g, repo, "a.txt", strings.Repeat("x\n", 20001))
			},
		},
		{
			name:  "a file deleted and then restored",
			paths: []string{"keep.txt"},
			first: func(g *WithT, repo string) {
				git(g, repo, "rm", "--quiet", "keep.txt")
			},
			second: func(g *WithT, repo string) {
				write(g, repo, "keep.txt", "unchanged\n")
			},
		},
		{
			name:  "a path reverted out of the capture",
			paths: []string{"a.txt"},
			second: func(g *WithT, repo string) {
				write(g, repo, "a.txt", "one\n")
			},
		},
		{
			name:  "a file outside the review's paths",
			paths: []string{"a.txt"},
			first: func(g *WithT, repo string) {
				write(g, repo, "other.txt", "context\n")
			},
			second: func(g *WithT, repo string) {
				write(g, repo, "other.txt", "more context\nand more\n")
				write(g, repo, "a.txt", "one\ntwo\nthree\n")
			},
		},
		{
			name:  "a binary file",
			paths: []string{"a.bin"},
			first: func(g *WithT, repo string) {
				write(g, repo, "a.bin", binary)
			},
			second: func(g *WithT, repo string) {
				write(g, repo, "a.bin", binary+"more")
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			f := newFixture(g, t)
			repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
			me := api.Caller{Dir: repo}
			if c.first != nil {
				c.first(g, repo)
			}
			id := f.request(g, t, api.RequestBody{Caller: me, Paths: c.paths, Note: "first"}).Review.ID
			f.expectStoredTotals(g, t, id)
			if c.second == nil {
				return
			}
			f.decide(g, t, id, store.VerdictChanges, "more")
			c.second(g, repo)
			f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
			f.expectStoredTotals(g, t, id)
		})
	}
}

// page fetches a surface page and parses it, failing on any other status.
func (f *fixture) page(g *WithT, path string, status int) *html.Node {
	g.THelper()
	res := serveSurface(f.s.Surface(), http.MethodGet, path)
	g.Expect(res.Code).To(Equal(status), res.Body.String())
	return parse(g, res.Body.String())
}

// latestRound is the id of a review's latest round.
func (f *fixture) latestRound(g *WithT, t *testing.T, reviewID int64) int64 {
	g.THelper()
	review, err := f.store.Review(t.Context(), reviewID)
	g.Expect(err).NotTo(HaveOccurred())
	round, err := f.store.Round(t.Context(), reviewID, review.LatestRound)
	g.Expect(err).NotTo(HaveOccurred())
	return round.ID
}

// addComment writes a comment through the server's wrapper.
func (f *fixture) addComment(g *WithT, t *testing.T, c store.NewComment) {
	g.THelper()
	_, err := f.s.db.AddComment(t.Context(), c)
	g.Expect(err).NotTo(HaveOccurred())
}

// expectStoredTotals holds the review page's totals to the size the store
// summed when the latest round was written.
func (f *fixture) expectStoredTotals(g *WithT, t *testing.T, reviewID int64) {
	g.THelper()
	review, err := f.store.Review(t.Context(), reviewID)
	g.Expect(err).NotTo(HaveOccurred())
	round, err := f.store.Round(t.Context(), reviewID, review.LatestRound)
	g.Expect(err).NotTo(HaveOccurred())
	doc := f.page(g, "/reviews/"+itoa(reviewID), http.StatusOK)
	toolbar := find(doc, "toolbar")
	want := fmt.Sprintf("%s +%s −%s", count(round.Files, "file", "files"), thousands(round.Added), thousands(round.Removed))
	g.Expect(text(find(toolbar, "counts"))).To(Equal(want), "round "+strconv.Itoa(round.Number))
	g.Expect(findAll(doc, hasClass("file"))).To(HaveLen(round.Files), "the files listed are the files counted")
}

// nextMillisecond waits until the clock has passed into the next millisecond.
func nextMillisecond() {
	start := time.Now().Truncate(time.Millisecond)
	for time.Now().Truncate(time.Millisecond).Equal(start) {
		runtime.Gosched()
	}
}

// hasAttr reports whether an element carries an attribute at all.
func hasAttr(n *html.Node, name string) bool {
	for _, a := range n.Attr {
		if a.Key == name {
			return true
		}
	}
	return false
}
