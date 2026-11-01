package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/diff"
	"github.com/noonat/eyeball/internal/kind"
	"github.com/noonat/eyeball/internal/store"
	"github.com/noonat/eyeball/internal/view"
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

// roundAt reads the view of the review page at an address.
func (f *fixture) roundAt(g *WithT, address string) view.Review {
	g.THelper()
	return readReview(g, f, "/api"+address)
}

// bodiesOf is each card's body by path: the one the round holds, or the one
// the card's own address answers where the round holds none.
func (f *fixture) bodiesOf(g *WithT, round view.Review) map[string]view.CardBody {
	g.THelper()
	out := map[string]view.CardBody{}
	for _, card := range round.Cards {
		if card.Body != nil {
			out[card.Path] = *card.Body
			continue
		}
		out[card.Path] = readBody(g, f, card.BodyHref)
	}
	return out
}

// cardPaths is the paths of a round's cards, in order.
func cardPaths(round view.Review) []string {
	var out []string
	for _, card := range round.Cards {
		out = append(out, card.Path)
	}
	return out
}

// commentLines is each comment as its anchor, its quote and its text, with
// the mark of one written after the verdict.
func commentLines(comments []view.Comment) []string {
	var out []string
	for _, c := range comments {
		line := c.Where
		if c.Late {
			line += " (after the verdict)"
		}
		if c.Quoted != "" {
			line += " | " + c.Quoted
		}
		out = append(out, line+" | "+c.Body)
	}
	return out
}

// rowLines is each row of a card's code as its kind, its number and its text,
// the way a reader sees the line.
func rowLines(rows []view.CodeRow) []string {
	var out []string
	for _, row := range rows {
		if row.Kind == view.CodeRowKindHunk {
			out = append(out, "hunk "+row.Header)
			continue
		}
		var text strings.Builder
		for _, tok := range row.Tokens {
			text.WriteString(tok.Text)
		}
		out = append(out, fmt.Sprintf("%s %d %s", row.Kind, row.Number, text.String()))
	}
	return out
}

// markedRow is a row's text with each token's role and mark written out, so a
// test sees what the app is told to draw.
func markedRow(row view.CodeRow) string {
	var out strings.Builder
	for _, tok := range row.Tokens {
		text := tok.Text
		if tok.Role != view.TokenRolePlain {
			text = "[" + string(tok.Role) + " " + text + "]"
		}
		if tok.Mark {
			text = "{" + text + "}"
		}
		out.WriteString(text)
	}
	return out.String()
}

// documentDoc draws a document's passages through the file page's drawing, so
// a test reads each block as the template page marked it. The passages are the
// card's own, and nothing of the review page is drawn.
func documentDoc(g *WithT, parts []view.DocPart) *html.Node {
	g.THelper()
	body, err := json.Marshal(view.Files{
		Kind: view.FilesKindFile,
		File: &view.File{Name: "doc.md", Body: view.FileBodyDocument, Passages: parts},
	})
	g.Expect(err).NotTo(HaveOccurred())
	return docFromView(g, body)
}

// threadOf is the thread under the passage that covers a range, or nil.
func threadOf(parts []view.DocPart, first, last int32) *view.Thread {
	for _, part := range parts {
		passages := part.Fold
		if part.Passage != nil {
			passages = []view.DocPassage{*part.Passage}
		}
		for _, p := range passages {
			if p.First == first && p.Last == last {
				return p.Thread
			}
		}
	}
	return nil
}

func TestServer_Surface_review(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo, Agent: "spec/012-surface"}
	note := "Upload worker and its retry ladder\n\nThe second paragraph says what to look at."
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: note}).Review.ID

	round := f.roundAt(g, "/reviews/"+itoa(id))
	g.Expect(round.Title).To(Equal("Upload worker and its retry ladder"), "the full title, which app.css truncates")
	g.Expect([]string{round.Back, round.BackLabel}).To(Equal([]string{"/", "Queue"}))
	g.Expect(round.Waiting).To(Equal("now"))
	g.Expect(round.Abandoned).To(BeFalse())
	g.Expect(round.Decided).To(BeNil())
	g.Expect([]any{round.Round, round.Project, round.Agent}).To(Equal([]any{int32(1), "harbor", "spec/012-surface"}))
	// The brief's project is a link to its view.
	review, err := f.store.Review(t.Context(), id)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(round.ProjectHref).To(Equal(projectHref(review.ProjectID)))
	f.page(g, round.ProjectHref, http.StatusOK)
	g.Expect(round.ReviewTitle).To(Equal("Upload worker and its retry ladder"))
	g.Expect(round.Note).To(Equal([]string{"Upload worker and its retry ladder", "The second paragraph says what to look at."}))
	g.Expect([]string{round.Files, round.Added, round.Removed}).To(Equal([]string{"1 file", "1", "0"}))
	g.Expect(round.Against).To(BeNil(), "a first round has only its base")
	g.Expect(round.Acting).NotTo(BeNil())
	g.Expect(round.Acting.State).To(Equal("open"))
	g.Expect(round.Acting.Bar).NotTo(BeNil(), "an open round has the decision bar")

	g.Expect(cardPaths(round)).To(Equal([]string{"a.txt"}))
	body := f.bodiesOf(g, round)["a.txt"]
	g.Expect(body.Kind).To(Equal(view.FileBodyRows))
	g.Expect(rowLines(body.Rows)).To(Equal([]string{"hunk @@ -1 +1,2 @@", "context 1 one", "add 2 two"}))
}

func TestServer_Surface_reviewAbandoned(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	f.ok(g, t, http.MethodPost, "/agent/reviews/"+itoa(id)+"/abandon", api.AbandonBody{Caller: me}, &api.AbandonResult{})

	round := f.roundAt(g, "/reviews/"+itoa(id))
	g.Expect(round.Abandoned).To(BeTrue(), "a withdrawn review does not wait")
	g.Expect(round.Round).To(BeEquivalentTo(1))
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
	round := f.roundAt(g, "/reviews/"+itoa(id))
	g.Expect(round.Against).To(Equal(&view.Against{Previous: "Since round 1", Base: false}))
	g.Expect([]string{round.Files, round.Added, round.Removed}).To(Equal([]string{"1 file", "1", "0"}))
	g.Expect(round.Place).To(Equal(itoa(id) + ":2:"))

	// The whole review is measured from the base: two lines.
	whole := f.roundAt(g, "/reviews/"+itoa(id)+"?against=base")
	g.Expect(whole.Against).To(Equal(&view.Against{Previous: "Since round 1", Base: true}))
	g.Expect([]string{whole.Files, whole.Added, whole.Removed}).To(Equal([]string{"1 file", "2", "0"}))
	g.Expect(whole.Place).To(Equal(itoa(id) + ":2:base"))
	g.Expect(whole.Acting.Against).To(Equal("base"), "a removed row's quote needs the comparison")
}

func TestServer_Surface_reviewChangesRequested(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "Redo it.")

	round := f.roundAt(g, "/reviews/"+itoa(id))
	g.Expect(round.Decided).To(Equal(&view.Decided{
		Approved: false,
		Pill:     "Changes requested just now",
		Sentence: "The verdict went out just now.",
		Note:     []string{"Redo it."},
	}))
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

	round := f.roundAt(g, "/reviews/"+itoa(id))
	g.Expect(round.Decided).To(Equal(&view.Decided{
		Approved: true,
		Pill:     "Approved just now",
		Sentence: "3 comments and the verdict went out just now.",
		Note:     []string{"Good.", "Ship it."},
	}), "the late comment went out on its own")
	g.Expect(round.Acting.Bar).To(BeNil(), "a decided round has no decision bar")
	g.Expect(round.Acting.State).To(Equal("late"), "a decided round keeps one box, for a late comment")

	// A file's comments follow its diff, in line order.
	body := f.bodiesOf(g, round)["a.txt"]
	var onFile []string
	for _, thread := range body.Threads {
		onFile = append(onFile, fmt.Sprintf("line %d: %s", thread.Line, strings.Join(commentLines(thread.Comments), "; ")))
	}
	g.Expect(onFile).To(Equal([]string{"line 1: a.txt:1–2 | one\ntwo | Both lines.", "line 2: a.txt:2 | two | Why two?"}))

	// The review's own comments sit under the box a late comment is written
	// in, and the late one says so. Nothing released can change.
	g.Expect(commentLines(round.Acting.Comments)).To(Equal([]string{"This review | Overall fine.", "This review (after the verdict) | One more thing."}))
	var all []view.Comment
	all = append(all, round.Acting.Comments...)
	for _, thread := range body.Threads {
		all = append(all, thread.Comments...)
	}
	for _, c := range all {
		g.Expect(c.Editable).To(BeFalse(), "released comments cannot change: "+c.Body)
	}
	g.Expect(round.Replies).To(BeEmpty(), "nothing has been answered yet")

	// An approved round is the last, so a reply the agent writes to it can
	// show nowhere else.
	comments, err := f.store.Comments(t.Context(), roundID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(f.store.Reply(t.Context(), comments[0].ID, "Renamed it.")).To(Succeed())
	round = f.roundAt(g, "/reviews/"+itoa(id))
	g.Expect(round.Replies).To(HaveLen(1))
	g.Expect([]string{round.Replies[0].Where, round.Replies[0].Body}).To(Equal([]string{"a.txt:1–2", "Renamed it."}))
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
	round := f.roundAt(g, base)
	product := f.bodiesOf(g, round)["docs/product.md"]
	g.Expect(product.Kind).To(Equal(view.FileBodyDocument))
	doc := documentDoc(g, product.Passages)
	g.Expect(proseBlocks(doc)).To(Equal([]string{
		"kept: Product",
		"kept: Kept one.",
		"marked: New wording. | was: Old wording.",
		"kept: Stays between.",
		"removed: was: Removed passage.",
		"kept: Kept two.",
		"whole: Added at the end.",
	}))
	g.Expect(findAll(find(doc, "prose"), func(n *html.Node) bool { return attr(n, "id") != "" })).To(BeEmpty(), "two documents on one page could repeat an id")
	for _, card := range round.Cards {
		if card.Path == "docs/product.md" {
			g.Expect(card.RawDiff).To(Equal(base+"/files/docs/product.md?view=source"), "the Raw diff link")
		}
	}

	// The whole review compares with the base, which held the heading alone,
	// and every link keeps that comparison.
	whole := f.roundAt(g, base+"?against=base")
	product = f.bodiesOf(g, whole)["docs/product.md"]
	g.Expect(proseBlocks(documentDoc(g, product.Passages))).To(Equal([]string{
		"kept: Product",
		"whole: Kept one.",
		"whole: New wording.",
		"whole: Stays between.",
		"whole: Kept two.",
		"whole: Added at the end.",
	}))
	for _, card := range whole.Cards {
		if card.Path == "docs/product.md" {
			g.Expect(card.RawDiff).To(Equal(base + "/files/docs/product.md?against=base&view=source"))
		}
	}

	// The attack corpus leaves no script, no style, no image and no event
	// attribute in what the server marks up, and every link it kept stays
	// inside the round. An HTML block's placeholder links to Source, keeping
	// the comparison.
	rounds := map[string]view.Review{base: round, base + "?against=base": whole}
	for page, rv := range rounds {
		attack, ok := f.bodiesOf(g, rv)["docs/attack.md"]
		g.Expect(ok).To(BeTrue(), page)
		doc := documentDoc(g, attack.Passages)
		elements := findAll(doc, func(n *html.Node) bool {
			return n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "img" || n.Data == "svg" || n.Data == "iframe")
		})
		g.Expect(elements).To(BeEmpty(), page)
		g.Expect(eventAttributes(doc)).To(BeEmpty(), page)
		hrefs := documentLinks(find(doc, "prose"))
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

// A document's card shows what changed and folds the rest. Each list item is a
// passage, an ordered item keeps its number past a fold, and a passage with a
// comment stays out of the folds, its comment quoting the item alone. The
// file's own page shows the whole document.
func TestServer_Surface_reviewDocumentFolds(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	commitFile(g, repo, "docs/spec.md", "# Spec\n\nIntro.\n\n## Items\n\n- one\n- two\n- three\n- four\n- five\n- six\n- seven\n\n1. a\n2. b\n3. c\n4. d\n5. e\n")
	write(g, repo, "docs/spec.md", "# Spec\n\nIntro.\n\n## Items\n\n- one\n- two\n- three\n- 4th\n- five\n- six\n- seven\n\n1. a\n2. b\n3. c\n4. d\n5. e\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"docs"}, Note: "first"}).Review.ID
	base := "/reviews/" + itoa(id)
	req := view.CommentRequest{Round: 1, Level: view.CommentLevelPassage, Path: "docs/spec.md", Start: 16, End: 16, Body: "Why b?"}
	res := sendJSON(f.s.Surface(), http.MethodPost, commentsAPI(id), req)
	g.Expect(res.Code).To(Equal(http.StatusOK), res.Body.String())

	body := f.bodiesOf(g, f.roundAt(g, base))["docs/spec.md"]
	card := documentDoc(g, body.Passages)
	g.Expect(proseBlocks(card)).To(Equal([]string{
		"fold: 2 unchanged passages: Spec / Intro.",
		"kept: Items",
		"fold: 2 unchanged passages: one / two",
		"kept: three",
		"marked: 4th | was: four",
		"kept: five",
		"fold: 3 unchanged passages: six / seven / a",
		"kept: b",
		"fold: 3 unchanged passages: c / d / e",
	}))
	var starts []string
	for _, ol := range findAll(find(card, "prose"), isElement("ol")) {
		starts = append(starts, attr(ol, "start"))
	}
	g.Expect(starts).To(Equal([]string{"1", "2", "3", "4", "5"}))
	thread := threadOf(body.Passages, 16, 16)
	g.Expect(thread).NotTo(BeNil(), "the commented passage carries its thread")
	g.Expect(thread.ID).To(Equal(passageID("docs/spec.md", 16, 16)))
	g.Expect(thread.Comments).To(HaveLen(1))
	g.Expect([]any{thread.Comments[0].Where, thread.Comments[0].Quoted, thread.Comments[0].Body, thread.Comments[0].Editable}).To(Equal([]any{"docs/spec.md:16", "2. b", "Why b?", true}))

	// The file's own page shows every passage, with nothing folded.
	doc := f.page(g, base+"/files/docs/spec.md", http.StatusOK)
	g.Expect(findAll(doc, hasClass("fold"))).To(BeEmpty())
	g.Expect(proseBlocks(find(doc, "prose").Parent)).To(HaveLen(15))
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
		{
			page:   base + "/rounds/1?against=base",
			prefix: base + "/rounds/1",
			query:  "?against=base",
		},
	}
	for _, c := range pages {
		t.Run(c.page, func(t *testing.T) {
			g := NewWithT(t)
			body := f.bodiesOf(g, f.roundAt(g, c.page))["docs/product.md"]
			files := c.prefix + "/files/"
			g.Expect(documentLinks(find(documentDoc(g, body.Passages), "prose"))).To(Equal([]string{
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
	body := f.bodiesOf(g, f.roundAt(g, base))["docs/product.md"]
	prose := find(documentDoc(g, body.Passages), "prose")
	followed := map[string]string{}
	for _, a := range findAll(prose, isElement("a")) {
		href, _, _ := strings.Cut(attr(a, "href"), "#")
		if !strings.HasPrefix(href, "/") {
			continue
		}
		res := serveSurface(f.s.Surface(), http.MethodGet, "/api"+href)
		doc := viewDocument(g, f.s.Surface(), href, res.Code)
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
	current := f.roundAt(g, "/reviews/"+itoa(id))
	g.Expect(current.Earlier).To(Equal([]view.EarlierRound{{
		Href:  "/reviews/" + itoa(id) + "/rounds/1",
		Label: "Round 1 · changes requested",
		Age:   "just now",
	}}))
	g.Expect(current.Closed).To(BeEmpty())

	// The earlier round is closed, and lists its comments and what was done.
	earlier := f.roundAt(g, "/reviews/"+itoa(id)+"/rounds/1")
	g.Expect([]string{earlier.Title, earlier.Back, earlier.BackLabel}).To(Equal([]string{"Round 1", "/reviews/" + itoa(id), "Review"}))
	g.Expect(earlier.Decided).NotTo(BeNil())
	g.Expect(earlier.Decided.Pill).To(Equal("Changes requested"))
	g.Expect(earlier.Round).To(BeEquivalentTo(1))
	g.Expect(earlier.Closed).To(Equal("Closed just now. Round 2 is the current one."))
	g.Expect(earlier.Earlier).To(BeEmpty(), "an earlier round lists no earlier rounds")
	g.Expect(earlier.Acting).To(BeNil(), "nothing more can be said on a closed round")
	g.Expect(commentLines(earlier.Comments)).To(Equal([]string{"a.txt:2 | two | Name it.", "This review | Move the empty case."}))
	g.Expect(commentLines(earlier.Replies)).To(Equal([]string{"a.txt:2 | Named it."}))

	// The latest round under the rounds address is the review's own page.
	again := f.roundAt(g, "/reviews/"+itoa(id)+"/rounds/2")
	g.Expect([]string{again.Title, again.Back, again.BackLabel}).To(Equal([]string{"first", "/", "Queue"}))
	g.Expect(again.Earlier).NotTo(BeEmpty())

	missing := []string{"/reviews/" + itoa(id) + "/rounds/3", "/reviews/" + itoa(id) + "/rounds/0", "/reviews/" + itoa(id) + "/rounds/x", "/reviews/999/rounds/1"}
	for _, path := range missing {
		res := getView(f, "/api"+path, nil)
		g.Expect(res.Code).To(Equal(http.StatusNotFound), path)
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

	round := f.roundAt(g, "/reviews/"+itoa(id))
	g.Expect(round.Round).To(BeEquivalentTo(3))
	g.Expect(round.Decided.Pill).To(Equal("Approved 2h ago"))
	g.Expect(round.Decided.Sentence).To(Equal("The verdict went out 2 hours ago."))
	var rows []string
	for _, row := range round.Earlier {
		rows = append(rows, row.Label+" "+row.Age)
	}
	g.Expect(rows).To(Equal([]string{"Round 2 · changes requested 2h ago", "Round 1 · changes requested 2h ago"}))
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

	got := map[string]string{}
	for path, body := range f.bodiesOf(g, f.roundAt(g, "/reviews/"+itoa(id))) {
		g.Expect(body.Rows).To(BeEmpty(), path)
		g.Expect(body.Passages).To(BeEmpty(), path)
		line := string(body.Kind)
		if body.Rendered != nil && body.Rendered.Facts != nil {
			line += " " + body.Rendered.Facts.Kind + " " + body.Rendered.Facts.Size
		}
		got[path] = line
	}
	g.Expect(got).To(Equal(map[string]string{
		"a.bin":     "rendered Binary file 8 B",
		"a.txt":     "too_large",
		"binary.md": "rendered Binary file 11 B",
		"huge.md":   "too_large",
		"long.md":   "too_large",
	}))
	for _, card := range f.roundAt(g, "/reviews/"+itoa(id)).Cards {
		g.Expect(card.Whole).To(Equal("/reviews/"+itoa(id)+"/files/"+card.Path), "every card opens its whole file")
	}
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
	round := f.roundAt(g, base)
	g.Expect(round.Fresh).To(BeTrue())
	g.Expect(round.Files).To(Equal("1 file"))
	g.Expect(cardPaths(round)).To(Equal([]string{"specs/018.md"}))
	g.Expect(round.Cards[0].RawDiff).To(Equal(base + "/files/specs/018.md?view=source"))
	body := f.bodiesOf(g, round)["specs/018.md"]
	g.Expect(proseBlocks(documentDoc(g, body.Passages))).To(Equal([]string{"kept: Bulk import", "kept: Tagging one at a time.", "kept: Iteration 1"}))
	g.Expect(findAll(documentDoc(g, body.Passages), hasClass("fold"))).To(BeEmpty(), "a document with nothing to compare against shows whole")
	// With no card to hold it, the file's comment joins the review's own.
	g.Expect(commentLines(round.Comments)).To(Equal([]string{"specs/018.md | Name the manifest."}))
	g.Expect(round.Acting.Comments).To(BeEmpty())
	g.Expect(body.Comments).To(BeEmpty())

	// A document new since round 1, alone in its round, keeps the layout
	// beside the comparison control. The whole review holds both documents,
	// which puts each in a card.
	f.decide(g, t, id, store.VerdictChanges, "more")
	write(g, repo, "specs/019.md", "# Next\n")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
	round = f.roundAt(g, base)
	g.Expect(round.Fresh).To(BeTrue())
	g.Expect(round.Files).To(Equal("1 file"))
	g.Expect(round.Against).NotTo(BeNil(), "the comparison control sits beside the layout")
	g.Expect(round.Cards[0].RawDiff).To(Equal(base + "/files/specs/019.md?view=source"))
	whole := f.roundAt(g, base+"?against=base")
	g.Expect(whole.Fresh).To(BeFalse())
	g.Expect(whole.Files).To(Equal("2 files"))
	g.Expect(cardPaths(whole)).To(Equal([]string{"specs/018.md", "specs/019.md"}))
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

			round := f.roundAt(g, "/reviews/"+itoa(id))
			g.Expect(round.Fresh).To(Equal(c.fresh))
			g.Expect(round.Cards).To(HaveLen(len(c.paths)))
		})
	}
}

// A file that moved more than 500 lines holds its body behind a line naming
// its size and Load anyway, which fetches the body the cards route serves. At
// 500 the card is deferred as any other in a large review.
func TestServer_Surface_reviewHoldsAFilePastFiveHundred(t *testing.T) {
	counts := []int{500, 501}
	for _, n := range counts {
		t.Run(itoa(int64(n))+" lines", func(t *testing.T) {
			g := NewWithT(t)
			f := newFixture(g, t)
			repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
			write(g, repo, "a.txt", "one\n")
			write(g, repo, "d/lock.json", strings.Repeat("line\n", n))
			id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "lock"}).Review.ID

			round := f.roundAt(g, "/reviews/"+itoa(id))
			g.Expect(round.Cards).To(HaveLen(1))
			card := round.Cards[0]
			g.Expect(card.Body).To(BeNil())
			if n <= 500 {
				g.Expect(card.Held).To(BeEmpty())
				g.Expect(card.Deferred).To(BeTrue(), "a skeleton holds the place until the body is asked for")
				g.Expect(card.SkeletonLines).To(BeEquivalentTo(n))
				return
			}
			// A held card waits for the tap, so it never asks on its own.
			g.Expect(card.Deferred).To(BeFalse())
			g.Expect(card.Held).To(Equal("501"))
			g.Expect(card.HeldHint).To(Equal("Past the 500 a card shows on its own."))
			body := readBody(g, f, card.BodyHref)
			g.Expect(body.Kind).To(Equal(view.FileBodyRows))
			var lines int
			for _, row := range body.Rows {
				if row.Kind == view.CodeRowKindAdd {
					lines++
				}
			}
			g.Expect(lines).To(Equal(n))
		})
	}
}

// A late comment on a line sits under that line on a decided round, marked as
// after the verdict, with nothing to edit or delete, and the file keeps a slot
// for a late comment on the file as an open round does.
func TestServer_Surface_reviewLateLineComment(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	roundID := f.latestRound(g, t, id)
	f.addComment(g, t, store.NewComment{RoundID: roundID, Level: store.LevelLine, Path: "a.txt", StartLine: 2, EndLine: 2, Quoted: "two", Body: "Before."})
	f.decide(g, t, id, store.VerdictApprove, "")
	nextMillisecond()
	f.addComment(g, t, store.NewComment{RoundID: roundID, Level: store.LevelLine, Path: "a.txt", StartLine: 2, EndLine: 2, Quoted: "two", Body: "After."})

	body := f.bodiesOf(g, f.roundAt(g, "/reviews/"+itoa(id)))["a.txt"]
	g.Expect(body.Threads).To(HaveLen(1), "both comments sit under their line")
	thread := body.Threads[0]
	g.Expect(thread.ID).To(Equal(threadID("a.txt", 2)))
	g.Expect(commentLines(thread.Comments)).To(Equal([]string{"a.txt:2 | two | Before.", "a.txt:2 (after the verdict) | two | After."}))
	for _, c := range thread.Comments {
		g.Expect(c.Editable).To(BeFalse(), "released comments cannot change: "+c.Body)
	}
	g.Expect(body.Slot).To(Equal(slotID("a.txt")), "a late comment on the file has somewhere to open")
	g.Expect(body.ListID).To(Equal(listID(store.LevelFile, "a.txt")))
}

// A review past ten files lists them above the cards, each linking to its
// card, so a reader can jump to one. At ten files there is no list.
func TestServer_Surface_reviewListsFilesPastTen(t *testing.T) {
	counts := []int{10, 11}
	for _, n := range counts {
		t.Run(itoa(int64(n))+" files", func(t *testing.T) {
			g := NewWithT(t)
			f := newFixture(g, t)
			repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
			write(g, repo, "a.txt", "one\n")
			for i := range n {
				write(g, repo, fmt.Sprintf("d/%02d.txt", i), strings.Repeat("line\n", i+1))
			}
			id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"d"}, Note: "many"}).Review.ID

			round := f.roundAt(g, "/reviews/"+itoa(id))
			g.Expect(round.Listed).To(Equal(n > 10))
			g.Expect(round.Cards).To(HaveLen(n))
			for i, card := range round.Cards {
				g.Expect(card.Path).To(Equal(fmt.Sprintf("d/%02d.txt", i)))
				g.Expect(card.Added).To(Equal(strconv.Itoa(i + 1)))
				g.Expect(card.ID).To(Equal("file-"+pathKey(card.Path)), "a link names a card that is not its file's")
			}
		})
	}
}

// A repository moved or deleted after the round leaves nothing to compare
// against, and the round's capture still holds every file the agent changed.
// The page shows those as captured, and the review can still be decided.
func TestServer_Surface_reviewLostRepository(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	g.Expect(os.RemoveAll(filepath.Join(repo, ".git"))).To(Succeed())

	round := f.roundAt(g, "/reviews/"+itoa(id))
	g.Expect(round.Unmeasurable).To(Equal("The project's repository is gone, so the change cannot be measured."))
	g.Expect(round.Files).To(Equal("1 file"))
	g.Expect([]string{round.Added, round.Removed}).To(Equal([]string{"", ""}), "nothing was compared, so nothing is counted")
	g.Expect(round.Cards).To(HaveLen(1))
	g.Expect(round.Cards[0].Unmeasured).To(BeTrue())
	var lines []string
	for _, row := range f.bodiesOf(g, round)["a.txt"].Rows {
		g.Expect(row.Kind).NotTo(Equal(view.CodeRowKindAdd), "the file is shown as captured, with no change marked")
		var text strings.Builder
		for _, tok := range row.Tokens {
			text.WriteString(tok.Text)
		}
		lines = append(lines, text.String())
	}
	g.Expect(lines).To(Equal([]string{"one", "two"}))
	g.Expect(round.Acting.Bar).NotTo(BeNil(), "the review can still be decided")
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

	// The changed word is a mark around its role, and the keyword before it
	// is highlighted on its own.
	var marked []string
	for _, row := range f.bodiesOf(g, f.roundAt(g, "/reviews/"+itoa(id)))["x.go"].Rows {
		if row.Kind == view.CodeRowKindAdd || row.Kind == view.CodeRowKindDel {
			marked = append(marked, string(row.Kind)+" "+markedRow(row))
		}
	}
	g.Expect(marked).To(Equal([]string{
		"del \t[keyword return] {[number 1]}",
		"add \t[keyword return] {[number 2]}",
	}))
}

func TestServer_Surface_reviewModeChange(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	commitFile(g, repo, "deploy.sh", "#!/bin/sh\n")
	g.Expect(os.Chmod(filepath.Join(repo, "deploy.sh"), 0o700)).To(Succeed())
	g.Expect(os.MkdirAll(filepath.Join(repo, "docs"), 0o700)).To(Succeed())
	g.Expect(os.Symlink("v2/", filepath.Join(repo, "docs", "current"))).To(Succeed())
	write(g, repo, "docs/notes.txt", "a new text file\n")
	me := api.Caller{Dir: repo}
	paths := []string{"a.txt", "deploy.sh", "docs"}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: paths, Note: "first"}).Review.ID

	// lines is each card's mode line, by path, for the cards that have one.
	lines := func(round view.Review) map[string]string {
		out := map[string]string{}
		for path, body := range f.bodiesOf(g, round) {
			if body.Mode != "" {
				out[path] = body.Mode
			}
		}
		return out
	}
	// An execute bit with no byte changed, against the base, and a new
	// symlink. A new text file and an edited one carry no line.
	g.Expect(lines(f.roundAt(g, "/reviews/"+itoa(id)))).To(Equal(map[string]string{
		"deploy.sh":    "regular file (644) → executable file (755)",
		"docs/current": "new symlink",
	}))
	file := f.page(g, "/reviews/"+itoa(id)+"/files/deploy.sh", http.StatusOK)
	g.Expect(text(find(file, "modeline"))).To(Equal("regular file (644) → executable file (755)"))

	// A chmod between rounds is a file that moved, though no byte did.
	f.decide(g, t, id, store.VerdictChanges, "more")
	g.Expect(os.Chmod(filepath.Join(repo, "a.txt"), 0o700)).To(Succeed())
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
	g.Expect(lines(f.roundAt(g, "/reviews/"+itoa(id)))).To(Equal(map[string]string{"a.txt": "regular file (644) → executable file (755)"}))
}

func TestServer_Surface_reviewNotFound(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	paths := []string{"/reviews/999", "/reviews/abc", "/reviews/0"}
	for _, path := range paths {
		res := getView(f, "/api"+path, nil)
		g.Expect(res.Code).To(Equal(http.StatusNotFound), path)
		var missing view.NotFound
		g.Expect(json.Unmarshal(res.Body.Bytes(), &missing)).To(Succeed(), path)
		g.Expect(missing.Notice).To(BeNil(), path)
	}
}

func TestServer_Surface_reviewRendered(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	commitFile(g, repo, "icon.png", pngOf(g, 2, 3))
	commitFile(g, repo, "site/index.html", "<p>before</p>\n")
	write(g, repo, "icon.png", pngOf(g, 4, 5))
	write(g, repo, "site/index.html", "<p>after</p>\n")
	write(g, repo, "clip.mp4", "not really a video")
	write(g, repo, "paper.pdf", "%PDF-1.4\n")
	write(g, repo, "big.png", strings.Repeat("x", kind.MaxSend+1))
	write(g, repo, "data.zip", "PK\x03\x04")
	paths := []string{"icon.png", "site", "clip.mp4", "paper.pdf", "big.png", "data.zip"}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: paths, Note: "first", Views: map[string]string{"site/index.html": "rendered"}}).Review.ID
	raw := "/reviews/" + itoa(id) + "/raw/"

	// shown is each card as what it shows, by path.
	shown := func(round view.Review) map[string]string {
		out := map[string]string{}
		bodies := f.bodiesOf(g, round)
		for _, card := range round.Cards {
			body := bodies[card.Path]
			g.Expect(body.Kind).To(Equal(view.FileBodyRendered), card.Path)
			var parts []string
			if !card.Unmeasured {
				parts = append(parts, "counts +"+card.Added+" −"+card.Removed)
			}
			r := body.Rendered
			for _, side := range r.Sides {
				parts = append(parts, string(side.Element)+" "+side.Src)
			}
			if r.Frame != "" {
				frame := "iframe " + r.Frame
				if r.Sandboxed {
					frame += " allow-scripts"
				}
				parts = append(parts, frame)
			}
			if facts := r.Facts; facts != nil {
				line := "facts " + facts.Kind + facts.Size + facts.Hint
				if facts.Load != "" {
					line += "Load anyway"
				}
				parts = append(parts, line)
			}
			out[card.Path] = strings.Join(parts, " | ")
		}
		return out
	}
	want := map[string]string{
		"icon.png":        "img " + raw + "0/icon.png | img " + raw + "1/icon.png",
		"site/index.html": "counts +1 −1 | iframe " + raw + "1/site/index.html allow-scripts",
		"clip.mp4":        "video " + raw + "1/clip.mp4",
		"paper.pdf":       "iframe " + raw + "1/paper.pdf",
		"big.png":         "facts PNG image20.0 MB" + pastSendLimit + "Load anyway",
		"data.zip":        "facts ZIP archive4 B",
	}
	round := f.roundAt(g, "/reviews/"+itoa(id))
	g.Expect(shown(round)).To(Equal(want))

	// A deferred card's body, fetched on its first open, is the same.
	g.Expect(expectCardBodies(g, f, id, 0, "")).To(HaveLen(len(want)))
}

func TestServer_Surface_reviewSendsBodiesUpToItsLimits(t *testing.T) {
	cases := []struct {
		name  string
		files int
		lines int
		sent  bool
	}{
		{
			name:  "ten files",
			files: 10,
			lines: 1,
			sent:  true,
		},
		{
			name:  "eleven files",
			files: 11,
			lines: 1,
			sent:  false,
		},
		{
			name:  "three hundred lines",
			files: 1,
			lines: 300,
			sent:  true,
		},
		{
			name:  "three hundred and one lines",
			files: 1,
			lines: 301,
			sent:  false,
		},
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

			round := f.roundAt(g, "/reviews/"+itoa(id))
			g.Expect(round.Cards).To(HaveLen(c.files))
			for _, card := range round.Cards {
				// Past the thresholds a card sends no body with the round, and
				// asks for it at its own address once it nears the screen.
				g.Expect(card.Deferred).To(Equal(!c.sent), card.Path)
				g.Expect(card.Body == nil).To(Equal(!c.sent), card.Path)
			}
			if c.sent {
				return
			}
			// The skeleton holds the body's rows, one hunk header and each
			// line, so the body arriving moves nothing below it.
			for _, card := range round.Cards {
				g.Expect([]int32{card.SkeletonHunks, card.SkeletonLines}).To(Equal([]int32{1, int32(c.lines)}), card.Path)
			}
			body := readBody(g, f, round.Cards[0].BodyHref)
			var lines int
			for _, row := range body.Rows {
				if row.Kind == view.CodeRowKindAdd {
					lines++
				}
			}
			g.Expect(lines).To(Equal(c.lines))
			res := getView(f, "/api/reviews/"+itoa(id)+"/rounds/1/cards/d/missing.txt", nil)
			g.Expect(res.Code).To(Equal(http.StatusNotFound))
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

// page fetches a surface page and parses it, failing on any other status. The
// project, tree and file pages are React screens, so they are read through
// their views, and so is a review's round, whose template page no test parses.
func (f *fixture) page(g *WithT, path string, status int) *html.Node {
	g.THelper()
	if scopedPage(path) {
		return f.viewPage(g, path, status)
	}
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

// expectStoredTotals holds the review's totals to the size the store summed
// when the latest round was written.
func (f *fixture) expectStoredTotals(g *WithT, t *testing.T, reviewID int64) {
	g.THelper()
	review, err := f.store.Review(t.Context(), reviewID)
	g.Expect(err).NotTo(HaveOccurred())
	round, err := f.store.Round(t.Context(), reviewID, review.LatestRound)
	g.Expect(err).NotTo(HaveOccurred())
	view := f.roundAt(g, "/reviews/"+itoa(reviewID))
	got := fmt.Sprintf("%s +%s −%s", view.Files, view.Added, view.Removed)
	want := fmt.Sprintf("%s +%s −%s", count(round.Files, "file", "files"), thousands(round.Added), thousands(round.Removed))
	g.Expect(got).To(Equal(want), "round "+strconv.Itoa(round.Number))
	g.Expect(view.Cards).To(HaveLen(round.Files), "the files listed are the files counted")
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

// textWithout is n's text less every element named skip, as text reads it.
func textWithout(n *html.Node, skip string) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == skip {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// proseBlocks reads each block of the prose under n: how it is marked, the
// text it shows, and the text it replaced.
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
		for _, w := range findAll(block, hasClass("old")) {
			was = append(was, text(w))
		}
		words := findAll(block, func(n *html.Node) bool {
			return n.Type == html.ElementNode && (n.Data == "ins" || n.Data == "del") && attr(n, "class") == ""
		})
		changed := attr(block, "class") == "blk changed"
		switch {
		case changed && find(block, "whole") != nil:
			parts = append(parts, "whole: "+text(find(block, "whole")))
			if len(was) > 0 {
				parts = append(parts, "was: "+strings.Join(was, " / "))
			}
		case changed && len(was) > 0 && attr(block, "data-lines") == "":
			parts = append(parts, "removed: was: "+strings.Join(was, " / "))
		case changed && len(was) > 0:
			parts = append(parts, "struck: "+textWithout(block, "del"), "was: "+strings.Join(was, " / "))
		case changed && len(words) > 0:
			parts = append(parts, "marked: "+textWithout(block, "del"), "was: "+textWithout(block, "ins"))
		case changed:
			parts = append(parts, "changed: "+text(block))
		case attr(block, "class") == "blk":
			parts = append(parts, "kept: "+text(block))
		case block.Data == "div" && attr(block, "class") == "" && attr(block, "id") != "":
			parts = append(parts, "comments: "+text(block))
		case block.Data == "details" && attr(block, "class") == "fold":
			var held []string
			for c := block.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data != "summary" {
					held = append(held, text(c))
				}
			}
			parts = append(parts, "fold: "+text(findAll(block, isElement("summary"))[0])+": "+strings.Join(held, " / "))
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
