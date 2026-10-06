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
	"github.com/noonat/eyeball/internal/diff"
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
	// The brief's project is a link to its view.
	project := find(find(doc, "who"), "bit")
	g.Expect(project.Data).To(Equal("a"))
	g.Expect(text(project)).To(Equal("harbor"))
	review, err := f.store.Review(t.Context(), id)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(attr(project, "href")).To(Equal(projectHref(review.ProjectID)))
	f.page(g, attr(project, "href"), http.StatusOK)
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

func TestServer_Surface_reviewAbandoned(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	f.ok(g, t, http.MethodPost, "/agent/reviews/"+itoa(id)+"/abandon", api.AbandonBody{Caller: me}, &api.AbandonResult{})

	doc := f.page(g, "/reviews/"+itoa(id), http.StatusOK)
	g.Expect(text(find(find(doc, "brief"), "where"))).To(HavePrefix("AbandonedRound 1"), "a withdrawn review does not wait")
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
	g.Expect(text(find(find(doc, "brief"), "where"))).To(HavePrefix("Changes requested just nowRound 1"))
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
	g.Expect(text(find(find(doc, "brief"), "where"))).To(HavePrefix("Approved just nowRound 1"))
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
	g.Expect(find(doc, "section")).NotTo(BeNil())
	g.Expect(text(doc)).NotTo(ContainSubstring("What was done"), "nothing has been answered yet")

	// An approved round is the last, so a reply the agent writes to it can
	// show nowhere else.
	comments, err := f.store.Comments(t.Context(), roundID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(f.store.Reply(t.Context(), comments[0].ID, "Renamed it.")).To(Succeed())
	doc = f.page(g, "/reviews/"+itoa(id), http.StatusOK)
	sections := findAll(doc, hasClass("section"))
	g.Expect(text(sections[len(sections)-1])).To(Equal("What was done 1 a.txt:1\u20132 Renamed it."))
}

func TestServer_Surface_reviewDocument(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	commitFile(g, repo, "docs/product.md", "# Product\n")
	write(g, repo, "docs/product.md", "# Product\n\nKept one.\n\nOld wording.\n\nStays between.\n\nRemoved passage.\n\nKept two.\n")
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"docs"}, Note: "first"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "more")
	write(g, repo, "docs/product.md", "# Product\n\nKept one.\n\nNew wording.\n\nStays between.\n\nKept two.\n\nAdded at the end.\n")
	write(g, repo, "docs/attack.md", attackCorpus)
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
	base := "/reviews/" + itoa(id)

	// Since round 1, each block is kept, replaced, removed or added, in the
	// card's prose.
	doc := f.page(g, base, http.StatusOK)
	cards := documentCards(doc)
	g.Expect(cards).To(HaveKey("docs/product.md"))
	g.Expect(proseBlocks(cards["docs/product.md"])).To(Equal([]string{
		"kept: Product",
		"kept: Kept one.",
		"replaced: New wording. | Show what it replaced | Hide what it replaced | was: Old wording.",
		"kept: Stays between.",
		"removed: was: Removed passage.",
		"kept: Kept two.",
		"added: Added at the end.",
	}))
	g.Expect(findAll(cards["docs/product.md"], func(n *html.Node) bool { return attr(n, "id") != "" })).To(BeEmpty(), "two documents on one page could repeat an id")
	links := find(cards["docs/product.md"], "filelinks")
	g.Expect(text(links)).To(Equal("Raw diff"))
	g.Expect(attr(find(links, "btn"), "href")).To(Equal(base + "/files/docs/product.md?view=source"))

	// The whole review compares with the base, which held the heading alone,
	// and every link keeps that comparison.
	doc = f.page(g, base+"?against=base", http.StatusOK)
	cards = documentCards(doc)
	g.Expect(proseBlocks(cards["docs/product.md"])).To(Equal([]string{
		"kept: Product",
		"added: Kept one.",
		"added: New wording.",
		"added: Stays between.",
		"added: Kept two.",
		"added: Added at the end.",
	}))
	g.Expect(attr(find(find(cards["docs/product.md"], "filelinks"), "btn"), "href")).To(Equal(base + "/files/docs/product.md?against=base&view=source"))

	// The attack corpus leaves no script, no style, no image and no event
	// attribute anywhere on the page, and every link it kept stays inside the
	// round. An HTML block's placeholder links to Source, keeping the
	// comparison.
	pages := []string{base, base + "?against=base"}
	for _, page := range pages {
		doc := f.page(g, page, http.StatusOK)
		attack := documentCards(doc)["docs/attack.md"]
		g.Expect(attack).NotTo(BeNil(), page)
		elements := findAll(doc, func(n *html.Node) bool {
			return n.Type == html.ElementNode && (n.Data == "script" && !layoutScript(n) || n.Data == "style" || n.Data == "img" || n.Data == "svg" || n.Data == "iframe")
		})
		g.Expect(elements).To(BeEmpty(), page)
		g.Expect(eventAttributes(doc)).To(BeEmpty(), page)
		hrefs := documentLinks(find(attack, "prose"))
		source := base + "/files/docs/attack.md?view=source"
		if strings.Contains(page, "against") {
			source = base + "/files/docs/attack.md?against=base&view=source"
		}
		g.Expect(hrefs).To(ContainElement(source), page)
		for _, href := range hrefs {
			g.Expect(href).To(HavePrefix(base+"/files/"), page)
		}
	}
}

func TestServer_Surface_reviewDocumentLinks(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	commitFile(g, repo, "docs/architecture.md", "# Architecture\n")
	commitFile(g, repo, "docs/product.md", "# Product\n")
	links := "[beside](architecture.md) [root](../) [part](#part) [up](../keep.txt#top) [missing](missing.md) [slash](/\\evil.com) [quote](x\"y.md) [image ![alt](d.png)](https://example.com) ![diagram](d.png)"
	write(g, repo, "docs/product.md", "# Product\n\nRound one. "+links+"\n")
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"docs/product.md"}, Note: "first"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "more")
	write(g, repo, "docs/product.md", "# Product\n\nRound two. "+links+"\n")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
	base := "/reviews/" + itoa(id)

	// Each link resolves against the document's own path, not the page's
	// address, into the round's file routes, escaped a segment at a time.
	// The round's form and the comparison come from the page, and a fragment
	// goes to the document's own page, which writes the ids the card cannot.
	pages := []struct {
		page   string
		prefix string
		query  string
	}{
		{page: base, prefix: base},
		{page: base + "?against=base", prefix: base, query: "?against=base"},
		{page: base + "/rounds/1", prefix: base + "/rounds/1"},
		{page: base + "/rounds/1?against=base", prefix: base + "/rounds/1", query: "?against=base"},
	}
	for _, c := range pages {
		t.Run(c.page, func(t *testing.T) {
			g := NewWithT(t)
			card := documentCards(f.page(g, c.page, http.StatusOK))["docs/product.md"]
			files := c.prefix + "/files/"
			g.Expect(documentLinks(find(card, "prose"))).To(Equal([]string{
				files + "docs/architecture.md" + c.query,
				files + c.query,
				files + "docs/product.md" + c.query + "#part",
				files + "keep.txt" + c.query + "#top",
				files + "docs/missing.md" + c.query,
				files + "%5Cevil.com" + c.query,
				files + "docs/x%22y.md" + c.query,
				"https://example.com",
				files + "docs/d.png" + c.query,
			}))
		})
	}

	// Followed, each lands on the page it names. A path the round lacks reads
	// live where the disk has it and is the 404 page where it does not.
	card := documentCards(f.page(g, base, http.StatusOK))["docs/product.md"]
	followed := map[string]string{}
	for _, a := range findAll(find(card, "prose"), isElement("a")) {
		href, _, _ := strings.Cut(attr(a, "href"), "#")
		if !strings.HasPrefix(href, "/") {
			continue
		}
		res := serveSurface(f.s.Surface(), http.MethodGet, href)
		doc := parse(g, res.Body.String())
		followed[text(a)] = strconv.Itoa(res.Code) + " " + text(find(doc, "crumb"))
	}
	g.Expect(followed).To(Equal(map[string]string{
		"beside":  "200 docs/architecture.md",
		"root":    "200 harbor",
		"part":    "200 docs/product.md",
		"up":      "200 keep.txt",
		"missing": "404 ",
		"slash":   "404 ",
		"quote":   "404 ",
		"diagram": "404 ",
	}))
	write(g, repo, "docs/missing.md", "written later\n")
	doc := f.page(g, base+"/files/docs/missing.md", http.StatusOK)
	g.Expect(notices(doc)).To(Equal([]string{"warn: Live, not from this round. Nothing captured this file, so it is shown as it is on disk now and may have moved."}))
	write(g, repo, "\\evil.com", "a name with a backslash\n")
	doc = f.page(g, base+"/files/%5Cevil.com", http.StatusOK)
	g.Expect(text(find(find(doc, "crumb"), "name"))).To(Equal("\\evil.com"))
}

func TestServer_Surface_reviewEarlierRounds(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	first := f.latestRound(g, t, id)
	line := f.addComment(g, t, store.NewComment{RoundID: first, Level: store.LevelLine, Path: "a.txt", StartLine: 2, EndLine: 2, Quoted: "two", Body: "Name it."})
	f.addComment(g, t, store.NewComment{RoundID: first, Level: store.LevelReview, Body: "Move the empty case."})
	f.decide(g, t, id, store.VerdictChanges, "Two things.")
	g.Expect(f.store.Reply(t.Context(), line.ID, "Named it.")).To(Succeed())
	write(g, repo, "a.txt", "one\ntwo\nthree\n")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})

	// The current round lists the earlier one.
	doc := f.page(g, "/reviews/"+itoa(id), http.StatusOK)
	rows := earlierRows(doc)
	g.Expect(rows).To(HaveLen(1))
	g.Expect(attr(rows[0], "href")).To(Equal("/reviews/" + itoa(id) + "/rounds/1"))
	g.Expect(text(find(rows[0], "nm"))).To(Equal("Round 1 \u00b7 changes requested"))
	g.Expect(text(find(rows[0], "hint"))).To(Equal("just now"))

	// The earlier round is closed, and lists its comments and what was done.
	doc = f.page(g, "/reviews/"+itoa(id)+"/rounds/1", http.StatusOK)
	g.Expect(text(find(doc, "topbar"))).To(Equal("ReviewRound 1"))
	g.Expect(attr(find(doc, "back"), "href")).To(Equal("/reviews/" + itoa(id)))
	g.Expect(text(find(find(doc, "brief"), "where"))).To(HavePrefix("Changes requestedRound 1"))
	g.Expect(text(find(doc, "notice"))).To(Equal("Closed just now. Round 2 is the current one."))
	g.Expect(earlierRows(doc)).To(BeEmpty(), "an earlier round lists no earlier rounds")
	// The tearout's sections come first, then the notes and the files.
	g.Expect(classOrder(doc, "section", "note", "toolbar")).To(Equal([]string{"section", "section", "note", "note", "toolbar"}))
	var sections []string
	for _, sec := range findAll(doc, hasClass("section")) {
		var cards []string
		for _, c := range findAll(sec, hasClass("comment")) {
			cards = append(cards, text(c))
		}
		sections = append(sections, text(find(sec, "count"))+": "+strings.Join(cards, " | "))
	}
	g.Expect(sections).To(Equal([]string{
		"2: a.txt:2 two Name it. | This review Move the empty case.",
		"1: a.txt:2 Named it.",
	}))

	// The latest round under the rounds address is the review's own page.
	doc = f.page(g, "/reviews/"+itoa(id)+"/rounds/2", http.StatusOK)
	g.Expect(text(find(doc, "topbar"))).To(Equal("Queuefirst"))
	g.Expect(earlierRows(doc)).NotTo(BeEmpty())

	missing := []string{"/reviews/" + itoa(id) + "/rounds/3", "/reviews/" + itoa(id) + "/rounds/0", "/reviews/" + itoa(id) + "/rounds/x", "/reviews/999/rounds/1"}
	for _, path := range missing {
		doc := f.page(g, path, http.StatusNotFound)
		g.Expect(text(find(doc, "empty"))).To(ContainSubstring("There is no page at this address."), path)
	}
}

func TestServer_Surface_reviewEarlierRoundsNewestFirst(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	contents := []string{"one\ntwo\nthree\n", "one\ntwo\nthree\nfour\n"}
	for i, content := range contents {
		f.decide(g, t, id, store.VerdictChanges, "more")
		write(g, repo, "a.txt", content)
		f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "round " + strconv.Itoa(i+2)})
	}
	f.decide(g, t, id, store.VerdictApprove, "")
	// The server's clock is its own, so moving it ages everything the store
	// stamped. Two hours tells the pill's short form from the notice's long
	// one.
	f.s.now = func() time.Time { return time.Now().Add(2 * time.Hour) }

	doc := f.page(g, "/reviews/"+itoa(id), http.StatusOK)
	g.Expect(text(find(find(doc, "brief"), "where"))).To(HavePrefix("Approved 2h agoRound 3"))
	g.Expect(text(find(doc, "notice"))).To(Equal("Approved. The verdict went out 2 hours ago."))
	var rows []string
	for _, row := range earlierRows(doc) {
		rows = append(rows, text(find(row, "nm"))+" "+text(find(row, "hint")))
	}
	g.Expect(rows).To(Equal([]string{"Round 2 \u00b7 changes requested 2h ago", "Round 1 \u00b7 changes requested 2h ago"}))
}

func TestServer_Surface_reviewFilesWithNoRows(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", strings.Repeat("x\n", 20001))
	write(g, repo, "a.bin", "\x00\x01binary")
	// A document falls back to the same lines: binary, past the byte limit,
	// and past the line limit with one block to a line, which is the only
	// way a document reaches the block comparison's own limit.
	write(g, repo, "binary.md", "# Binary\n\x00\x01")
	write(g, repo, "huge.md", strings.Repeat("x", diff.MaxBytes)+"\n")
	write(g, repo, "long.md", strings.Repeat("# a\n", 20001))
	paths := []string{"a.txt", "a.bin", "binary.md", "huge.md", "long.md"}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: paths, Note: "big"}).Review.ID

	doc := f.page(g, "/reviews/"+itoa(id), http.StatusOK)
	got := map[string]string{}
	for _, file := range findAll(doc, hasClass("file")) {
		got[text(find(file, "path"))] = text(find(file, "toobig")) + " " + text(find(file, "filelinks"))
		g.Expect(find(file, "code")).To(BeNil())
		g.Expect(find(file, "prose")).To(BeNil())
	}
	g.Expect(got).To(Equal(map[string]string{
		"a.bin":     "Binary file, not shown as a diff. Whole file",
		"a.txt":     "Too large to show as a diff. Whole file",
		"binary.md": "Binary file, not shown as a diff. Whole file",
		"huge.md":   "Too large to show as a diff. Whole file",
		"long.md":   "Too large to show as a diff. Whole file",
	}))
}

func TestServer_Surface_reviewFresh(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	write(g, repo, "specs/018.md", "# Bulk import\n\nTagging one at a time.\n\n## Iteration 1\n")
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"specs"}, Note: "first"}).Review.ID
	base := "/reviews/" + itoa(id)
	roundID := f.latestRound(g, t, id)
	f.addComment(g, t, store.NewComment{RoundID: roundID, Level: store.LevelFile, Path: "specs/018.md", Body: "Name the manifest."})

	// One new document reads on the page, with nothing marked and no card.
	doc := f.page(g, base, http.StatusOK)
	g.Expect(text(find(doc, "toolbar"))).To(Equal("1 file, new Raw diff"))
	g.Expect(attr(find(find(doc, "toolbar"), "btn"), "href")).To(Equal(base + "/files/specs/018.md?view=source"))
	g.Expect(hasClass("small")(find(find(doc, "toolbar"), "btn"))).To(BeTrue())
	g.Expect(findAll(doc, hasClass("file"))).To(BeEmpty())
	prose := find(doc, "prose")
	g.Expect(prose).NotTo(BeNil())
	g.Expect(hasClass("page")(prose.Parent)).To(BeTrue(), "app.css pulls prose back by the gutter only directly in .page")
	g.Expect(proseBlocks(prose.Parent)).To(Equal([]string{"kept: Bulk import", "kept: Tagging one at a time.", "kept: Iteration 1"}))
	// With no card to hold it, the file's comment joins the review's own.
	g.Expect(text(find(find(doc, "section"), "comment"))).To(Equal("specs/018.md Name the manifest."))

	// A document new since round 1, alone in its round, keeps the layout
	// beside the comparison control. The whole review holds both documents,
	// which puts each in a card.
	f.decide(g, t, id, store.VerdictChanges, "more")
	write(g, repo, "specs/019.md", "# Next\n")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
	doc = f.page(g, base, http.StatusOK)
	g.Expect(text(find(doc, "toolbar"))).To(Equal("1 file, new Raw diff Since round 1Whole review"))
	g.Expect(attr(find(find(doc, "toolbar"), "btn"), "href")).To(Equal(base + "/files/specs/019.md?view=source"))
	g.Expect(hasClass("page")(find(doc, "prose").Parent)).To(BeTrue())
	doc = f.page(g, base+"?against=base", http.StatusOK)
	g.Expect(text(find(find(doc, "toolbar"), "counts"))).To(HavePrefix("2 files"))
	g.Expect(documentCards(doc)).To(HaveLen(2))
}

func TestServer_Surface_reviewFreshConditions(t *testing.T) {
	cases := []struct {
		name string
		// base is committed before the round, and edit written after.
		base, edit map[string]string
		paths      []string
		fresh      bool
	}{
		{
			name:  "one new document",
			edit:  map[string]string{"new.md": "# New\n"},
			paths: []string{"new.md"},
			fresh: true,
		},
		{
			name:  "an old side of whitespace alone",
			base:  map[string]string{"blank.md": "\n\n   \n"},
			edit:  map[string]string{"blank.md": "# Now written\n"},
			paths: []string{"blank.md"},
			fresh: true,
		},
		{
			name:  "two files, one a new document",
			edit:  map[string]string{"new.md": "# New\n", "a.txt": "one\ntwo\n"},
			paths: []string{"new.md", "a.txt"},
		},
		{
			name:  "one new file that is not markdown",
			edit:  map[string]string{"new.txt": "new\n"},
			paths: []string{"new.txt"},
		},
		{
			name:  "one document whose old side has blocks",
			base:  map[string]string{"old.md": "# Old\n"},
			edit:  map[string]string{"old.md": "# Old\n\nMore.\n"},
			paths: []string{"old.md"},
		},
		{
			name:  "one new document past the line limit",
			edit:  map[string]string{"long.md": strings.Repeat("# a\n", 20001)},
			paths: []string{"long.md"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			f := newFixture(g, t)
			repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
			write(g, repo, "a.txt", "one\n")
			for p, content := range c.base {
				commitFile(g, repo, p, content)
			}
			for p, content := range c.edit {
				write(g, repo, p, content)
			}
			id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: c.paths, Note: "first"}).Review.ID

			doc := f.page(g, "/reviews/"+itoa(id), http.StatusOK)
			counts := text(find(find(doc, "toolbar"), "counts"))
			if c.fresh {
				g.Expect(counts).To(Equal("1 file, new"))
				g.Expect(findAll(doc, hasClass("file"))).To(BeEmpty())
				return
			}
			g.Expect(counts).NotTo(ContainSubstring("new"))
			g.Expect(findAll(doc, hasClass("file"))).To(HaveLen(len(c.paths)))
		})
	}
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

// earlierRows is each row of a review page's list of earlier rounds, which
// is its first section when there is one.
func earlierRows(doc *html.Node) []*html.Node {
	sec := find(doc, "section")
	if sec == nil {
		return nil
	}
	heading := findAll(sec, func(n *html.Node) bool { return n.Data == "h2" })
	if len(heading) == 0 || !strings.HasPrefix(text(heading[0]), "Earlier rounds") {
		return nil
	}
	return findAll(sec, hasClass("treerow"))
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
func (f *fixture) addComment(g *WithT, t *testing.T, c store.NewComment) store.Comment {
	g.THelper()
	comment, err := f.s.db.AddComment(t.Context(), c)
	g.Expect(err).NotTo(HaveOccurred())
	return comment
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

// attackCorpus is a document written to break out of the page: raw HTML,
// script in every place markdown lets HTML in, and links under schemes the
// allowlist refuses.
const attackCorpus = `# Attack

<script>alert(1)</script>

A paragraph <img src=x onerror=alert(1)> and <a href="javascript:x" onclick="alert(1)">a link</a>.

[js](javascript:alert(1)) [upper](JAVASCRIPT:alert(1)) [vs](vscode://file/etc/passwd) [tab](java&#x09;script:alert(1)) [data](data:text/html;base64,PHNjcmlwdD4=) [file](file:///etc/passwd) [far](//evil.example/x)

<javascript:alert(1)> <vscode://x>

![x](javascript:alert(1)) ![y](x.png "a title onerror=alert(1)")

<style>body { display: none }</style>

<svg onload=alert(1)><circle/></svg>

<iframe src="https://evil.example"></iframe>

- <script>alert(2)</script>
- [nested <img src=x onerror=alert(3)>](x.md)

> <div onmouseover="alert(4)">quoted</div>

| a | b |
|---|---|
| <script>x</script> | <img src=x onerror=y> |
`

// documentCards maps each file card shown as a document to its path.
func documentCards(doc *html.Node) map[string]*html.Node {
	out := map[string]*html.Node{}
	for _, card := range findAll(doc, hasClass("file")) {
		if find(find(card, "filebody"), "prose") != nil {
			out[text(find(card, "path"))] = card
		}
	}
	return out
}

// proseBlocks reads each block of the prose under n: how it is marked, the
// text it shows, its cues, and the text it replaced.
func proseBlocks(n *html.Node) []string {
	var out []string
	prose := find(n, "prose")
	if prose == nil {
		return nil
	}
	for block := prose.FirstChild; block != nil; block = block.NextSibling {
		if block.Type != html.ElementNode {
			continue
		}
		var parts []string
		var was []string
		for _, w := range findAll(block, hasClass("was-text")) {
			was = append(was, text(w))
		}
		switch {
		case block.Data == "details" && attr(block, "class") == "blk changed":
			summary := findAll(block, isElement("summary"))[0]
			var shown []string
			for c := summary.FirstChild; c != nil; c = c.NextSibling {
				if !hasClass("hint")(c) {
					shown = append(shown, text(c))
				}
			}
			parts = append(parts, "replaced: "+strings.Join(strings.Fields(strings.Join(shown, " ")), " "))
			for _, cue := range findAll(summary, hasClass("hint")) {
				parts = append(parts, text(cue))
			}
			parts = append(parts, "was: "+strings.Join(was, " / "))
		case attr(block, "class") == "blk changed was":
			parts = append(parts, "removed: was: "+strings.Join(was, " / "))
		case attr(block, "class") == "blk changed":
			parts = append(parts, "added: "+text(block))
		case attr(block, "class") == "blk":
			parts = append(parts, "kept: "+text(block))
		default:
			parts = append(parts, "unknown "+block.Data+"."+attr(block, "class")+": "+text(block))
		}
		out = append(out, strings.Join(parts, " | "))
	}
	return out
}

// isElement matches an element named name. Matching on Data alone also
// matches a text node holding the same word.
func isElement(name string) func(*html.Node) bool {
	return func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == name
	}
}

// eventAttributes is every element under n carrying an attribute that runs
// script, such as onerror or onclick.
func eventAttributes(n *html.Node) []*html.Node {
	return findAll(n, func(c *html.Node) bool {
		for _, a := range c.Attr {
			if strings.HasPrefix(strings.ToLower(a.Key), "on") {
				return true
			}
		}
		return false
	})
}

// nextMillisecond waits until the clock has passed into the next millisecond.
func nextMillisecond() {
	start := time.Now().Truncate(time.Millisecond)
	for time.Now().Truncate(time.Millisecond).Equal(start) {
		runtime.Gosched()
	}
}

// classOrder lists, in document order, each element carrying one of classes,
// by that class.
func classOrder(n *html.Node, classes ...string) []string {
	var out []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for _, c := range classes {
			if hasClass(c)(n) {
				out = append(out, c)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return out
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

// layoutScript reports whether n is one of the two scripts the layout loads in
// the head, which are the only scripts a page may hold.
func layoutScript(n *html.Node) bool {
	src := attr(n, "src")
	return n.Parent != nil && n.Parent.Data == "head" && (src == "/static/htmx.js" || src == "/static/app.js")
}
