package server

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/noonat/eyeball/internal/store"
	. "github.com/onsi/gomega"
	"golang.org/x/net/html"
)

func Test_emptyQueueSentence(t *testing.T) {
	cases := []struct {
		name     string
		projects []store.ProjectRow
		want     string
	}{
		{
			name:     "nobody at work",
			projects: []store.ProjectRow{{Project: store.Project{Name: "harbor"}}},
			want:     "No agents are working.",
		},
		{
			name:     "one agent",
			projects: []store.ProjectRow{{Project: store.Project{Name: "harbor"}, Agents: 1}},
			want:     "One agent is working in harbor.",
		},
		{
			name:     "two agents in one project",
			projects: []store.ProjectRow{{Project: store.Project{Name: "eyeball"}, Agents: 2}, {Project: store.Project{Name: "idle"}}},
			want:     "Two agents are working in eyeball.",
		},
		{
			name: "agents across three projects",
			projects: []store.ProjectRow{
				{Project: store.Project{Name: "eyeball"}, Agents: 2},
				{Project: store.Project{Name: "field-notes"}, Agents: 1},
				{Project: store.Project{Name: "harbor"}, Agents: 1},
			},
			want: "Four agents are working across eyeball, field-notes and harbor.",
		},
		{
			name:     "more agents than a sentence spells",
			projects: []store.ProjectRow{{Project: store.Project{Name: "a"}, Agents: 6}, {Project: store.Project{Name: "b"}, Agents: 6}},
			want:     "12 agents are working across a and b.",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			s := &Server{}
			g.Expect(s.emptyQueue(c.projects).Sentence).To(Equal(c.want))
		})
	}
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

func TestServer_Surface_emptyQueue(t *testing.T) {
	g := NewWithT(t)
	q := newQueueFixture(g, t)
	eyeball := q.project(g, t, "/home/a/src/eyeball", "eyeball")
	fieldNotes := q.project(g, t, "/srv/field-notes", "field-notes")
	harbor := q.project(g, t, "/home/a/src/harbor", "harbor")
	working := q.open(g, t, eyeball, "one", "first", "a")
	q.decide(g, t, working, store.VerdictChanges)
	q.open(g, t, eyeball, "two", "second", "b")
	q.decide(g, t, 2, store.VerdictChanges)
	q.open(g, t, harbor, "three", "third", "c")
	q.decide(g, t, 3, store.VerdictChanges)
	q.advance(5 * time.Minute)

	res := serveSurface(q.s.Surface(), http.MethodGet, "/")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	doc := parse(g, res.Body.String())

	g.Expect(text(find(doc, "topbar"))).To(Equal("eyeball"), "no waiting count with nothing waiting")
	empty := find(doc, "empty")
	g.Expect(empty).NotTo(BeNil())
	g.Expect(text(empty)).To(Equal("Nothing waitingThree agents are working across eyeball and harbor."))

	var headings []string
	for _, h := range findAll(doc, func(n *html.Node) bool { return n.Data == "h2" }) {
		headings = append(headings, text(h))
	}
	// Working still follows the empty state when it has rows.
	g.Expect(headings).To(Equal([]string{"Projects 3", "Working 3"}))

	projects := findAll(find(doc, "section"), hasClass("row"))
	type shape struct {
		Element, Href, Project, Activity, Root string
	}
	var got []shape
	for _, p := range projects {
		got = append(got, shape{Element: p.Data, Href: attr(p, "href"), Project: text(find(p, "project")), Activity: text(find(p, "age")), Root: text(find(p, "agent"))})
	}
	// Each row is a link to its project's view.
	g.Expect(got).To(Equal([]shape{
		{Element: "a", Href: projectHref(eyeball), Project: "eyeball", Activity: "2 agents", Root: "~/src/eyeball"},
		{Element: "a", Href: projectHref(fieldNotes), Project: "field-notes", Activity: "Idle", Root: "/srv/field-notes"},
		{Element: "a", Href: projectHref(harbor), Project: "harbor", Activity: "1 agent", Root: "~/src/harbor"},
	}))
}

func TestServer_Surface_queue(t *testing.T) {
	g := NewWithT(t)
	q := newQueueFixture(g, t)
	eyeball := q.project(g, t, "/home/a/src/eyeball", "eyeball")
	harbor := q.project(g, t, "/home/a/src/harbor", "harbor")

	// Settled first, so it is the oldest: approved two hours ago.
	settled := q.open(g, t, harbor, "agent-s", "Delta sync watermark", "s")
	q.advance(time.Hour)
	q.decide(g, t, settled, store.VerdictApprove)
	q.advance(time.Hour)
	// Working: changes requested 30 minutes ago, on a round asked for an hour
	// before that.
	working := q.open(g, t, eyeball, "spec/014-queue", "Queue screen markup", "w")
	q.advance(30 * time.Minute)
	q.decide(g, t, working, store.VerdictChanges)
	q.advance(18 * time.Minute)
	// Waiting: asked for 12 minutes ago.
	q.open(g, t, eyeball, "spec/012-surface", "Product doc, second pass", "a")
	q.advance(12 * time.Minute)

	res := serveSurface(q.s.Surface(), http.MethodGet, "/")
	g.Expect(res.Code).To(Equal(http.StatusOK))
	doc := parse(g, res.Body.String())

	g.Expect(text(find(doc, "topbar"))).To(Equal("eyeball1 waiting"))
	var headings []string
	for _, h := range findAll(doc, func(n *html.Node) bool { return n.Data == "h2" }) {
		headings = append(headings, text(h))
	}
	g.Expect(headings).To(Equal([]string{"Waiting on you 1", "Working 1", "Settled 1"}))

	rows := findAll(doc, hasClass("row"))
	g.Expect(rows).To(HaveLen(3))
	type shape struct {
		Class, Href, Project, Agent, Age, Title, Meta string
	}
	var got []shape
	for _, r := range rows {
		got = append(got, shape{
			Class:   attr(r, "class"),
			Href:    attr(r, "href"),
			Project: text(find(r, "project")),
			Agent:   text(find(r, "agent")),
			Age:     text(find(r, "age")),
			Title:   text(find(r, "title")),
			Meta:    text(find(r, "meta")),
		})
	}
	g.Expect(got).To(Equal([]shape{
		{Class: "row waiting", Href: "/reviews/3", Project: "eyeball", Agent: "spec/012-surface", Age: "12m", Title: "Product doc, second pass", Meta: "Round 11 file+1 −0"},
		{Class: "row working", Href: "/reviews/2", Project: "eyeball", Agent: "spec/014-queue", Age: "30m", Title: "Queue screen markup", Meta: "Changes requested, round 1"},
		{Class: "row approved", Href: "/reviews/1", Project: "harbor", Agent: "agent-s", Age: "2h", Title: "Delta sync watermark", Meta: "ApprovedRound 1"},
	}))
	g.Expect(find(doc, "empty")).To(BeNil(), "something is waiting, so the queue is not empty")
}

func TestServer_Surface_queueKeepsFiveSettled(t *testing.T) {
	g := NewWithT(t)
	q := newQueueFixture(g, t)
	project := q.project(g, t, "/home/a/src/eyeball", "eyeball")
	for i := range 6 {
		id := q.open(g, t, project, "agent", "approved "+strconv.Itoa(i), "claim"+strconv.Itoa(i))
		q.advance(time.Minute)
		q.decide(g, t, id, store.VerdictApprove)
	}

	res := serveSurface(q.s.Surface(), http.MethodGet, "/")
	doc := parse(g, res.Body.String())
	rows := findAll(doc, hasClass("approved"))
	var titles []string
	for _, r := range rows {
		if attr(r, "class") == "row approved" {
			titles = append(titles, text(find(r, "title")))
		}
	}
	// The newest five, newest first. The tail is for finding a recent
	// verdict, not a history.
	g.Expect(titles).To(Equal([]string{"approved 5", "approved 4", "approved 3", "approved 2", "approved 1"}))
}

func TestServer_Surface_queueWritesLargeCountsWithSeparators(t *testing.T) {
	g := NewWithT(t)
	q := newQueueFixture(g, t)
	project := q.project(g, t, "/home/a/src/eyeball", "eyeball")
	q.openSized(g, t, project, "agent", "big", "a", 1284, 903)

	doc := parse(g, serveSurface(q.s.Surface(), http.MethodGet, "/").Body.String())
	g.Expect(text(find(find(doc, "row"), "counts"))).To(Equal("+1,284 \u2212903"))
}

// queueFixture is a surface over a store whose clock the test moves.
type queueFixture struct {
	s     *Server
	store *store.Store
	at    time.Time
}

// newQueueFixture opens a store and a server sharing one hand-moved clock.
func newQueueFixture(g *WithT, t *testing.T) *queueFixture {
	g.THelper()
	q := &queueFixture{at: time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)}
	clock := func() time.Time { return q.at }
	st, err := store.OpenWithClock(t.Context(), filepath.Join(t.TempDir(), "eyeball.db"), clock)
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = st.Close() })
	q.store = st
	q.s = New(Config{Store: st, Home: "/home/a", Now: clock})
	return q
}

// advance moves the clock on.
func (q *queueFixture) advance(d time.Duration) {
	q.at = q.at.Add(d)
}

// project registers a project.
func (q *queueFixture) project(g *WithT, t *testing.T, root, name string) int64 {
	g.THelper()
	p, err := q.store.EnsureProject(t.Context(), root, name)
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
