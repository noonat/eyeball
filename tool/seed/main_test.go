package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/server"
	. "github.com/onsi/gomega"
	"golang.org/x/net/html"
)

func Test_linksResolve(t *testing.T) {
	g := NewWithT(t)
	s := seeded(g, t)
	srv := server.New(server.Config{Store: s.store, Blobs: s.blobs, Now: func() time.Time { return s.now }})
	c := &crawler{surface: srv.Surface(), mux: srv.SurfaceHandler()}
	c.crawl(g, "/")

	// Every seeded review, each earlier round, and every project was reached
	// from the queue, so the crawl rendered every page the seed made.
	projects, err := s.store.Projects(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(projects).To(HaveLen(4))
	for _, p := range projects {
		g.Expect(c.seen).To(HaveKey(fmt.Sprintf("/projects/%d", p.ID)), p.Name)
		reviews, err := s.store.ReviewsIn(t.Context(), p.ID)
		g.Expect(err).NotTo(HaveOccurred())
		for _, r := range reviews {
			g.Expect(c.seen).To(HaveKey(fmt.Sprintf("/reviews/%d", r.ReviewID)), r.Title)
			for n := 1; n < r.Round; n++ {
				g.Expect(c.seen).To(HaveKey(fmt.Sprintf("/reviews/%d/rounds/%d", r.ReviewID, n)), r.Title)
			}
		}
	}

	// The empty queue's project rows are links only when nothing waits.
	g.Expect(s.answerAll()).To(Succeed())
	c.crawl(g, "/")
	g.Expect(c.problems).To(BeEmpty())

	// Each kind of link the checks exist for was on some page, so a seed
	// that stopped writing one fails here rather than passing on less.
	g.Expect(c.hashed).To(BeNumerically(">", 0), "a tree row for a file with # in its name")
	g.Expect(c.hashedDocs).To(BeNumerically(">", 0), "a document's link to a file with # in its name")
	g.Expect(c.fragments).To(BeNumerically(">", 0), "a fragment-only link")
	g.Expect(c.absolute).To(HaveKey("https"))
	g.Expect(c.absolute).To(HaveKey("mailto"))
	g.Expect(c.sources).To(BeNumerically(">", 0), "an HTML block's link to Source")
	g.Expect(c.patterns).To(HaveKey("GET /projects/{project}/files/{path...}"))
	g.Expect(c.patterns).To(HaveKey("GET /reviews/{review}/rounds/{round}/files/{path...}"))
}

func Test_seedAnswersEverythingWaiting(t *testing.T) {
	g := NewWithT(t)
	s := seeded(g, t)
	g.Expect(s.answerAll()).To(Succeed())
	q, err := s.store.Queue(context.Background(), 5)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(q.Waiting).To(BeEmpty())
	g.Expect(q.Working).To(HaveLen(7))
}

func Test_seedRefusesADirectoryThatExists(t *testing.T) {
	g := NewWithT(t)
	dir, err := os.MkdirTemp("", "ebs-")
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	_, err = newSeed(context.Background(), dir)
	g.Expect(err).To(MatchError(ContainSubstring("exists")))
	entries, err := os.ReadDir(dir)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(entries).To(BeEmpty(), "the seed wrote into a directory it refused")
}

func Test_seedWritesEveryProjectState(t *testing.T) {
	g := NewWithT(t)
	s := seeded(g, t)
	projects, err := s.store.Projects(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	byName := map[string]string{}
	outside := 0
	for _, p := range projects {
		byName[p.Name] = p.Root
		reviews, err := s.store.ReviewsIn(t.Context(), p.ID)
		g.Expect(err).NotTo(HaveOccurred())
		for _, r := range reviews {
			round, err := s.store.Round(t.Context(), r.ReviewID, r.Round)
			g.Expect(err).NotTo(HaveOccurred())
			files, err := s.store.RoundFiles(t.Context(), round.ID)
			g.Expect(err).NotTo(HaveOccurred())
			for _, f := range files {
				if !f.InScope {
					outside++
				}
			}
		}
	}
	g.Expect(outside).To(BeNumerically(">", 0), "a file captured outside its review's paths")

	// A .git above the seed's directory would make scratch a marker project
	// inside a checkout, and the outside-git page would never be seen.
	scratch, err := capture.ReadWorkingCopy(t.Context(), byName["scratch"], false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(scratch.State.Live).To(Equal(capture.LiveOpen))
	g.Expect(scratch.Listed).To(BeFalse())

	clean, err := capture.ReadWorkingCopy(t.Context(), byName["field-notes"], false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(clean.Listed).To(BeTrue())
	g.Expect(clean.Entries).To(BeEmpty(), "field-notes committed its spec after the capture")

	harbor, err := capture.ReadWorkingCopy(t.Context(), byName["harbor"], false)
	g.Expect(err).NotTo(HaveOccurred())
	var changed []string
	for _, e := range harbor.Entries {
		changed = append(changed, e.Path)
	}
	g.Expect(changed).To(ContainElements("docs/architecture.md", "internal/storage/cache.go"), "written after every capture")

	_, err = os.Stat(filepath.Join(byName["eyeball"], "docs", "#retro.md"))
	g.Expect(err).NotTo(HaveOccurred())
}

func Test_seedWritesEveryQueueSection(t *testing.T) {
	g := NewWithT(t)
	s := seeded(g, t)
	q, err := s.store.Queue(context.Background(), 5)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(q.Waiting).To(HaveLen(4))
	g.Expect(q.Working).To(HaveLen(3))
	g.Expect(q.Settled).To(HaveLen(2))
	// Backdated as the tearouts' ages read: the longest wait is three hours.
	g.Expect(s.now.Sub(q.Waiting[0].RequestedAt)).To(BeNumerically("~", 3*time.Hour, time.Millisecond), "the store keeps milliseconds")
	g.Expect(q.Waiting[3].Round).To(Equal(2), "the product document waits on its second round")
}

// seeded is a seed written into a fresh directory with a short path, since
// the state directory's socket path has a length limit t.TempDir passes.
func seeded(g *WithT, t *testing.T) *seed {
	g.THelper()
	parent, err := os.MkdirTemp("", "ebs-")
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = os.RemoveAll(parent) })
	s, err := newSeed(context.Background(), filepath.Join(parent, "s"))
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = s.store.Close() })
	g.Expect(s.write()).To(Succeed())
	return s
}

// crawler renders surface pages, following every link to a page it has not
// rendered, and records each link that breaks a rule.
type crawler struct {
	surface http.Handler
	mux     *http.ServeMux
	// seen is every page the crawl reached, and answers what each address
	// answered, so a page linked from a hundred others renders once.
	seen    map[string]struct{}
	answers map[string]*httptest.ResponseRecorder
	// problems is every broken link, as a page and what is wrong.
	problems []string
	// hashed, hashedDocs, fragments, sources, absolute and patterns count
	// the links followed of each kind the checks are for.
	hashed, hashedDocs, fragments, sources int
	absolute                               map[string]int
	patterns                               map[string]int
}

// crawl renders the page at address and checks every link on it, following
// each one to a page not yet rendered. Each call starts afresh, since the
// store may have changed what every page says.
func (c *crawler) crawl(g *WithT, address string) {
	g.THelper()
	if c.absolute == nil {
		c.absolute, c.patterns = map[string]int{}, map[string]int{}
	}
	c.seen = map[string]struct{}{address: {}}
	c.answers = map[string]*httptest.ResponseRecorder{}
	g.Expect(c.get(address).Code).To(Equal(http.StatusOK), address)
	// Every page after the first is crawled because a link to it passed
	// check, which is where its status is held.
	queue := []string{address}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		res := c.get(at)
		doc, err := html.Parse(bytes.NewReader(res.Body.Bytes()))
		g.Expect(err).NotTo(HaveOccurred())
		base, err := url.Parse(at)
		g.Expect(err).NotTo(HaveOccurred())
		ids := idsIn(doc)
		for _, a := range linksIn(doc) {
			next, ok := c.check(base, ids, a)
			if !ok || next == "" {
				continue
			}
			if _, done := c.seen[next]; !done {
				c.seen[next] = struct{}{}
				queue = append(queue, next)
			}
		}
	}
}

// check holds one link to the rules, recording what it breaks, and returns
// the page to crawl next, empty for a link that leads to no page.
func (c *crawler) check(base *url.URL, ids map[string]struct{}, a *html.Node) (string, bool) {
	href := attr(a, "href")
	fail := func(format string, args ...any) (string, bool) {
		c.problems = append(c.problems, base.String()+": "+href+": "+fmt.Sprintf(format, args...))
		return "", false
	}
	u, err := url.Parse(href)
	if err != nil {
		return fail("does not parse: %v", err)
	}
	if u.Scheme != "" || u.Host != "" {
		switch u.Scheme {
		case "http", "https", "mailto":
		default:
			return fail("an absolute link with the scheme %q", u.Scheme)
		}
		if within(a, "prose") == nil {
			return fail("an absolute link outside a document")
		}
		c.absolute[u.Scheme]++
		return "", true
	}
	if strings.HasPrefix(href, "#") {
		if _, ok := ids[u.Fragment]; !ok {
			return fail("no id %q on the page", u.Fragment)
		}
		c.fragments++
		return "", true
	}

	target := base.ResolveReference(u)
	target.Fragment, target.RawFragment = "", ""
	address := target.RequestURI()
	req := httptest.NewRequest(http.MethodGet, "http://eyeball"+address, nil)
	_, pattern := c.mux.Handler(req)
	if pattern == "" {
		return fail("matches no surface route")
	}
	c.patterns[pattern]++
	res := c.get(address)
	if res.Code != http.StatusOK {
		return fail("answers %d", res.Code)
	}
	if !strings.HasPrefix(res.Header().Get("Content-Type"), "text/html") {
		return "", true
	}
	// A file's own bytes are the agent's page, not the surface's, so its
	// links are not the surface's to check and it carries no crumb.
	if strings.Contains(pattern, "/raw/") {
		return "", true
	}
	if within(a, "prose") != nil && strings.Contains(target.RawQuery, "view=source") {
		c.sources++
	}
	doc, err := html.Parse(bytes.NewReader(res.Body.Bytes()))
	if err != nil {
		return fail("the page does not parse: %v", err)
	}
	// A # left unescaped in a file's name makes the rest of the name a
	// fragment, which names no id on the page it lands on.
	if _, ok := idsIn(doc)[u.Fragment]; u.Fragment != "" && !ok {
		return fail("no id %q on the page it lands on", u.Fragment)
	}
	if !strings.Contains(pattern, "{path...}") {
		return address, true
	}

	// The page a file link lands on names that file in its crumb, so an
	// address cut short still fails where what it lands on answers 200.
	crumb := find(doc, "crumb")
	if crumb == nil {
		return fail("lands on a page with no crumb")
	}
	dir, name := text(find(crumb, "dir")), text(find(crumb, "name"))
	p := strings.TrimSuffix(req.URL.Path[strings.Index(req.URL.Path, "/files/")+len("/files/"):], "/")
	if strings.Contains(p, "#") && within(a, "prose") != nil {
		c.hashedDocs++
	} else if strings.Contains(p, "#") {
		c.hashed++
	}
	if p != "" && name != path.Base(p) {
		return fail("lands on %q, not %q", name, path.Base(p))
	}
	if parent := path.Dir(p); p != "" && parent != "." && !strings.HasSuffix(dir, parent+"/") {
		return fail("lands in %q, not %q", dir, parent)
	}
	if named := linkedName(base, a); named != "" && !strings.HasSuffix(dir+name, named) {
		return fail("is labeled %q and lands on %q", named, dir+name)
	}
	return address, true
}

// linkedName is the file a link names in its own text: a row in a tree or in
// a working copy, or the path heading the file card a link sits in. It is
// empty for a link whose text names no file, such as a back link.
func linkedName(base *url.URL, a *html.Node) string {
	if hasClass(a, "fileopen") {
		if details := within(a, "file"); details != nil {
			return text(find(details, "path"))
		}
	}
	if !hasClass(a, "treerow") {
		return ""
	}
	// The review page's rows are its earlier rounds and All files, which
	// name no file. A tree's rows and a working copy's do.
	if strings.Contains(base.Path, "/files/") || strings.HasPrefix(base.Path, "/projects/") {
		return text(find(a, "nm"))
	}
	return ""
}

// get is what the surface answers for address, asked once per crawl.
func (c *crawler) get(address string) *httptest.ResponseRecorder {
	if res, ok := c.answers[address]; ok {
		return res
	}
	res := httptest.NewRecorder()
	c.surface.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "http://eyeball"+address, nil))
	c.answers[address] = res
	return res
}

// linksIn is every element carrying an href, in document order.
func linksIn(n *html.Node) []*html.Node {
	var out []*html.Node
	if n.Type == html.ElementNode && attr(n, "href") != "" {
		out = append(out, n)
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		out = append(out, linksIn(child)...)
	}
	return out
}

// idsIn is every id on a page.
func idsIn(n *html.Node) map[string]struct{} {
	out := map[string]struct{}{}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if id := attr(n, "id"); id != "" {
			out[id] = struct{}{}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return out
}

// find is the first element at or below n carrying class, or nil.
func find(n *html.Node, class string) *html.Node {
	if n == nil {
		return nil
	}
	if hasClass(n, class) {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if found := find(child, class); found != nil {
			return found
		}
	}
	return nil
}

// within is the nearest element above n carrying class, or nil.
func within(n *html.Node, class string) *html.Node {
	for up := n.Parent; up != nil; up = up.Parent {
		if hasClass(up, class) {
			return up
		}
	}
	return nil
}

// hasClass reports whether an element's class attribute names class.
func hasClass(n *html.Node, class string) bool {
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

// attr is an element's attribute, empty where it has none.
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// text is the text below n with its whitespace collapsed, empty for nil.
func text(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}
