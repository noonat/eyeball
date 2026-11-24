package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/noonat/eyeball/internal/store"
	"github.com/noonat/eyeball/internal/view"
	. "github.com/onsi/gomega"
	"golang.org/x/net/html"
)

// A waiting row quotes the note's first paragraph, split as the review page
// splits the letter, so a note with blank lines around its paragraphs reads the
// same in both places.
func Test_queueRowGist(t *testing.T) {
	g := NewWithT(t)
	s := &Server{}
	row := s.queueRow(store.ReviewRow{Note: "\n\n  First paragraph,\non two lines.\r\n\r\nSecond paragraph.\n"}, "1m")
	g.Expect(row.Gist).To(Equal("First paragraph,\non two lines."))
	g.Expect(paragraphs("\n\n  First paragraph,\non two lines.\r\n\r\nSecond paragraph.\n")[0]).To(Equal(row.Gist))
	g.Expect(s.queueRow(store.ReviewRow{}, "1m").Gist).To(BeEmpty(), "a row with no note quotes nothing")

	// A review opened without a title is titled with its note's first line,
	// so the row quotes from the line after it rather than repeat the title.
	note := "Upload worker and its retry ladder\n\nThe second paragraph says what to look at."
	g.Expect(s.queueRow(store.ReviewRow{Note: note, Title: defaultTitle(note)}, "1m").Gist).To(Equal("The second paragraph says what to look at."))
	two := "Upload worker\nand its retry ladder\n\nMore."
	g.Expect(s.queueRow(store.ReviewRow{Note: two, Title: defaultTitle(two)}, "1m").Gist).To(Equal("and its retry ladder"))
	g.Expect(s.queueRow(store.ReviewRow{Note: "Upload worker", Title: "Upload worker"}, "1m").Gist).To(BeEmpty(), "a note that is only its title quotes nothing")
	g.Expect(s.queueRow(store.ReviewRow{Note: note, Title: "Retry ladder"}, "1m").Gist).To(Equal("Upload worker and its retry ladder"), "a title of its own leaves the note whole")
}

func Test_tilde(t *testing.T) {
	g := NewWithT(t)
	s := &Server{home: "/home/a"}
	g.Expect(s.tilde("/home/a/src/eyeball")).To(Equal("~/src/eyeball"))
	g.Expect(s.tilde("/home/a")).To(Equal("~"))
	g.Expect(s.tilde("/home/ab/src")).To(Equal("/home/ab/src"), "a sibling of home is not under it")
	g.Expect(s.tilde("/srv/x")).To(Equal("/srv/x"))
	g.Expect((&Server{}).tilde("/home/a/x")).To(Equal("/home/a/x"))
}

// A queue with nothing in it sends null sections, as the generated types say.
func TestServer_queueViewEmpty(t *testing.T) {
	g := NewWithT(t)
	q := newQueueFixture(g, t)
	body := serveSurface(q.s.Surface(), http.MethodGet, "/api/").Body.String()
	g.Expect(body).To(ContainSubstring(`"waiting": null`))
	g.Expect(body).To(ContainSubstring(`"waiting_count": 0`))
}

// With nothing waiting the queue still lists what an agent holds. The view
// sends no project list, since the project view is reached from a review's
// header.
func TestServer_queueViewHoldsWorkingWithNothingWaiting(t *testing.T) {
	g := NewWithT(t)
	q := newQueueFixture(g, t)
	eyeball := q.project(g, t, "/home/a/src/eyeball", "eyeball")
	// field-notes has no review, so only a list of projects would name it.
	q.project(g, t, "/srv/field-notes", "field-notes")
	harbor := q.project(g, t, "/home/a/src/harbor", "harbor")
	q.decide(g, t, q.open(g, t, eyeball, "one", "first", "a"), store.VerdictChanges)
	q.open(g, t, eyeball, "two", "second", "b")
	q.decide(g, t, 2, store.VerdictChanges)
	q.open(g, t, harbor, "three", "third", "c")
	q.decide(g, t, 3, store.VerdictChanges)
	q.advance(5 * time.Minute)

	got := getQueueView(g, q)
	g.Expect(got.Waiting).To(BeNil())
	g.Expect(got.WaitingCount).To(BeZero())
	g.Expect(got.Working).To(HaveLen(3))
	g.Expect(got.Settled).To(BeNil())
}

// The tail of Settled is for finding a recent verdict, not a history, so it
// holds the newest five.
func TestServer_queueViewKeepsFiveSettled(t *testing.T) {
	g := NewWithT(t)
	q := newQueueFixture(g, t)
	project := q.project(g, t, "/home/a/src/eyeball", "eyeball")
	for i := range 6 {
		id := q.open(g, t, project, "agent", "approved "+strconv.Itoa(i), "claim"+strconv.Itoa(i))
		q.advance(time.Minute)
		q.decide(g, t, id, store.VerdictApprove)
	}
	var titles []string
	for _, row := range getQueueView(g, q).Settled {
		titles = append(titles, row.Title)
	}
	g.Expect(titles).To(Equal([]string{"approved 5", "approved 4", "approved 3", "approved 2", "approved 1"}))
}

// A row says where it came from, what it is called, how long it has been in its
// section, and what its latest round moved, each as the page prints it.
func TestServer_queueViewReadsEachRow(t *testing.T) {
	g := NewWithT(t)
	q := newQueueFixture(g, t)
	eyeball := q.project(g, t, "/home/a/src/eyeball", "eyeball")
	harbor := q.project(g, t, "/home/a/src/harbor", "harbor")
	// Settled first, so it is the oldest: approved two hours ago.
	settled := q.open(g, t, harbor, "agent-s", "Delta sync watermark", "s")
	q.advance(time.Hour)
	q.decide(g, t, settled, store.VerdictApprove)
	q.advance(time.Hour)
	// Working: changes requested 30 minutes ago.
	working := q.open(g, t, eyeball, "spec/014-queue", "Queue screen markup", "w")
	q.advance(30 * time.Minute)
	q.decide(g, t, working, store.VerdictChanges)
	q.advance(18 * time.Minute)
	// Waiting: asked for 12 minutes ago.
	q.open(g, t, eyeball, "spec/012-surface", "Product doc, second pass", "a")
	q.advance(12 * time.Minute)

	got := getQueueView(g, q)
	g.Expect(got.Waiting).To(Equal([]view.Row{{
		ID:      3,
		Project: "eyeball",
		Agent:   "spec/012-surface",
		Title:   "Product doc, second pass",
		Age:     "12m",
		Round:   1,
		Files:   "1 file",
		Added:   "1",
		Removed: "0",
		Gist:    "look",
	}}))
	g.Expect(got.Working).To(HaveLen(1))
	g.Expect(got.Working[0]).To(HaveField("Age", "30m"))
	g.Expect(got.Working[0]).To(HaveField("Title", "Queue screen markup"))
	g.Expect(got.Settled).To(HaveLen(1))
	g.Expect(got.Settled[0]).To(HaveField("Age", "2h"))
	g.Expect(got.Settled[0]).To(HaveField("Project", "harbor"))
}

// An id is sent as a string, since the generator types it as one.
func TestServer_queueViewSendsIDsAsStrings(t *testing.T) {
	g := NewWithT(t)
	q := newQueueFixture(g, t)
	q.open(g, t, q.project(g, t, "/home/a/src/eyeball", "eyeball"), "a", "first", "a")
	body := serveSurface(q.s.Surface(), http.MethodGet, "/api/").Body.String()
	g.Expect(body).To(ContainSubstring(`"id": "1"`))
}

// A section holds the rows the store put there, in the store's order, and the
// eye's count is the waiting section's length. A section filled from the wrong
// rows would show a review under the wrong heading.
func TestServer_queueViewSendsTheStoresRows(t *testing.T) {
	g := NewWithT(t)
	q := newQueueFixture(g, t)
	project := q.project(g, t, "/home/a/src/eyeball", "eyeball")
	settled := q.open(g, t, project, "s", "settled", "s")
	q.advance(time.Hour)
	q.decide(g, t, settled, store.VerdictApprove)
	working := q.open(g, t, project, "w", "working", "w")
	q.advance(time.Hour)
	q.decide(g, t, working, store.VerdictChanges)
	q.open(g, t, project, "a", "waiting a", "a")
	q.advance(time.Minute)
	q.open(g, t, project, "b", "waiting b", "b")

	want, err := q.store.Queue(t.Context(), settledShown)
	g.Expect(err).NotTo(HaveOccurred())
	got := getQueueView(g, q)
	g.Expect(viewIDs(got.Waiting)).To(Equal(reviewIDs(want.Waiting)))
	g.Expect(viewIDs(got.Working)).To(Equal(reviewIDs(want.Working)))
	g.Expect(viewIDs(got.Settled)).To(Equal(reviewIDs(want.Settled)))
	g.Expect(got.Waiting).To(HaveLen(2))
	g.Expect(got.Working).To(HaveLen(1))
	g.Expect(got.Settled).To(HaveLen(1))
	g.Expect(got.WaitingCount).To(Equal(int32(2)))
	g.Expect(got.Waiting[0].Title).To(Equal("waiting a"))
	g.Expect(got.Waiting[0].Age).To(Equal("1m"))
	g.Expect(got.Waiting[0].Project).To(Equal("eyeball"))
}

// The view's stamp is what the stream sends for the same rows, through every
// change of state. The app refetches when the two stamps differ, so a stamp that
// disagreed with the stream's would refetch forever or never.
func TestServer_queueViewStampMatchesTheStream(t *testing.T) {
	g := NewWithT(t)
	q := newQueueFixture(g, t)
	project := q.project(g, t, "/home/a/src/eyeball", "eyeball")
	stamps := map[string]bool{}
	check := func() {
		g.THelper()
		got := getQueueView(g, q)
		want, err := q.s.queueStamp(t.Context())
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got.Stamp).To(Equal(want))
		g.Expect(stamps).NotTo(HaveKey(got.Stamp), "each state has a stamp of its own")
		stamps[got.Stamp] = true
	}
	check()
	first := q.open(g, t, project, "a", "first", "a")
	check()
	q.advance(time.Minute)
	q.decide(g, t, first, store.VerdictChanges)
	check()
	q.advance(time.Minute)
	q.decide(g, t, q.open(g, t, project, "b", "second", "b"), store.VerdictApprove)
	check()
}

// A count past a thousand is written with a separator.
func TestServer_queueViewWritesLargeCountsWithSeparators(t *testing.T) {
	g := NewWithT(t)
	q := newQueueFixture(g, t)
	project := q.project(g, t, "/home/a/src/eyeball", "eyeball")
	q.openSized(g, t, project, "agent", "big", "a", 1284, 903)
	row := getQueueView(g, q).Waiting[0]
	g.Expect(row.Added).To(Equal("1,284"))
	g.Expect(row.Removed).To(Equal("903"))
}

// The shell holds what every load needs and nothing the store changes: the
// head the shell writes, the React bundle and an element to draw into.
func TestServer_Surface_appShell(t *testing.T) {
	g := NewWithT(t)
	q := newQueueFixture(g, t)
	q.open(g, t, q.project(g, t, "/home/a/src/eyeball", "eyeball"), "a", "first", "a")
	q.s.daemon.Build = "the-build"
	res := serveSurface(q.s.Surface(), http.MethodGet, "/")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Header().Get("Content-Type")).To(Equal("text/html; charset=utf-8"))
	doc := parse(g, res.Body.String())

	metas := map[string]string{}
	for _, m := range findAll(doc, func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "meta" }) {
		metas[attr(m, "name")] = attr(m, "content")
	}
	g.Expect(metas).To(HaveKeyWithValue("eyeball-build", "the-build"))
	g.Expect(metas).To(HaveKey("theme-color"))

	var hrefs, srcs []string
	for _, n := range findAll(doc, func(n *html.Node) bool { return n.Type == html.ElementNode && (n.Data == "link" || n.Data == "script") }) {
		if n.Data == "link" {
			hrefs = append(hrefs, attr(n, "rel")+" "+attr(n, "href"))
		} else {
			srcs = append(srcs, attr(n, "src"))
		}
	}
	g.Expect(hrefs).To(ContainElements("stylesheet /static/app.css?b="+q.s.buildToken(), "stylesheet /static/shell.css?b="+q.s.buildToken(), "manifest /manifest.webmanifest"))
	g.Expect(srcs).To(Equal([]string{"/static/app.js?b=" + q.s.buildToken()}))
	g.Expect(findAll(doc, func(n *html.Node) bool { return attr(n, "id") == "root" })).To(HaveLen(1))
	g.Expect(res.Body.String()).NotTo(ContainSubstring("data-stamp"), "the app takes its stamp from the view")
	g.Expect(res.Body.String()).NotTo(ContainSubstring(`class="row`), "the shell draws no rows")

	js := serveSurface(q.s.Surface(), http.MethodGet, "/static/app.js")
	g.Expect(js.Code).To(Equal(http.StatusOK))
	g.Expect(js.Header().Get("Content-Type")).To(Equal("text/javascript; charset=utf-8"))
	g.Expect(js.Body.Len()).To(BeNumerically(">", 100000))
}

// queueFixture is a surface over a store whose clock the test moves.
type queueFixture struct {
	s     *Server
	store *store.Store
	at    time.Time
	// base is a temporary directory every project root is made under, so
	// the roots exist and the queue lists them. A root reads as a path
	// outside it, such as /home/a/src/eyeball.
	base string
}

// newQueueFixture opens a store and a server sharing one hand-moved clock.
func newQueueFixture(g *WithT, t *testing.T) *queueFixture {
	g.THelper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	g.Expect(err).NotTo(HaveOccurred())
	q := &queueFixture{at: time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC), base: base}
	clock := func() time.Time { return q.at }
	st, err := store.OpenWithClock(t.Context(), filepath.Join(base, "eyeball.db"), clock)
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = st.Close() })
	q.store = st
	q.s = New(Config{Store: st, Home: q.path("/home/a"), Now: clock})
	return q
}

// advance moves the clock on.
func (q *queueFixture) advance(d time.Duration) {
	q.at = q.at.Add(d)
}

// path is where the fixture keeps a root that reads as p.
func (q *queueFixture) path(p string) string {
	return filepath.Join(q.base, p)
}

// project registers a project at a root it makes under the fixture's base.
func (q *queueFixture) project(g *WithT, t *testing.T, root, name string) int64 {
	g.THelper()
	g.Expect(os.MkdirAll(q.path(root), 0o700)).To(Succeed())
	p, err := q.store.EnsureProject(t.Context(), q.path(root), name)
	g.Expect(err).NotTo(HaveOccurred())
	return p.ID
}

// open opens a review whose one round changed one line of one file.
func (q *queueFixture) open(g *WithT, t *testing.T, projectID int64, agent, title, claim string) int64 {
	g.THelper()
	return q.openSized(g, t, projectID, agent, title, claim, 1, 0)
}

// openSized opens a review whose one round moved added and removed lines.
func (q *queueFixture) openSized(g *WithT, t *testing.T, projectID int64, agent, title, claim string, added, removed int) int64 {
	g.THelper()
	review, _, err := q.store.OpenReview(t.Context(), store.NewReview{
		ProjectID: projectID,
		Agent:     agent,
		Title:     title,
		Paths:     []string{claim},
		Request: store.Request{
			Note:    "look",
			Files:   []store.File{{Path: claim, Digest: strings.Repeat("a", 64), Size: 1}},
			Changes: []store.Change{{Path: claim, Added: added, Removed: removed}},
		},
	})
	g.Expect(err).NotTo(HaveOccurred())
	return review.ID
}

// decide gives a verdict on a review's latest round.
func (q *queueFixture) decide(g *WithT, t *testing.T, reviewID int64, verdict store.Verdict) {
	g.THelper()
	review, err := q.store.Review(t.Context(), reviewID)
	g.Expect(err).NotTo(HaveOccurred())
	round, err := q.store.Round(t.Context(), reviewID, review.LatestRound)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(q.store.Decide(t.Context(), round.ID, verdict, "a note")).To(Succeed())
}

// parse parses a page.
func parse(g *WithT, body string) *html.Node {
	g.THelper()
	// The template review page prints these two for its script. A test about a
	// round reads the round's view instead, so the page is never parsed.
	g.Expect(body).NotTo(SatisfyAny(ContainSubstring(` data-stamp="`), ContainSubstring(` data-review="`)), "the template review page is read through its view")
	doc, err := html.Parse(strings.NewReader(body))
	g.Expect(err).NotTo(HaveOccurred())
	return doc
}

// hasClass matches an element carrying class.
func hasClass(class string) func(*html.Node) bool {
	return func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return false
		}
		for _, c := range strings.Fields(attr(n, "class")) {
			if c == class {
				return true
			}
		}
		return false
	}
}

// find is the first element under n carrying class, or nil.
func find(n *html.Node, class string) *html.Node {
	if n == nil {
		return nil
	}
	found := findAll(n, hasClass(class))
	if len(found) == 0 {
		return nil
	}
	return found[0]
}

// findAll is every element under n that match accepts, in document order,
// without looking inside a match.
func findAll(n *html.Node, match func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if match(c) {
			out = append(out, c)
			continue
		}
		out = append(out, findAll(c, match)...)
	}
	return out
}

// attr is an element's attribute, or empty.
func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// text is the text under n with runs of whitespace closed up, or empty for
// nil.
// barText is the top bar's text without its eye, which is a count rather
// than a label, so a test about the back link and the title reads only those.
func barText(doc *html.Node) string {
	bar := find(doc, "topbar")
	if bar == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && slices.Contains(strings.Fields(attr(n, "class")), "eye") {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(bar)
	return strings.Join(strings.Fields(b.String()), " ")
}

func text(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
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

// getQueueView reads the queue's JSON the way the app does.
func getQueueView(g *WithT, q *queueFixture) view.Queue {
	g.THelper()
	res := serveSurface(q.s.Surface(), http.MethodGet, "/api/")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	g.Expect(res.Header().Get("Content-Type")).To(Equal("application/json"))
	var got view.Queue
	g.Expect(json.Unmarshal(res.Body.Bytes(), &got)).To(Succeed())
	return got
}

// viewIDs is the review ids of a section, in order.
func viewIDs(rows []view.Row) []int64 {
	var out []int64
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

// reviewIDs is the review ids of store rows, in order.
func reviewIDs(rows []store.ReviewRow) []int64 {
	var out []int64
	for _, r := range rows {
		out = append(out, r.ReviewID)
	}
	return out
}
