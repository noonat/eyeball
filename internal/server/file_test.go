package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/diff"
	"github.com/noonat/eyeball/internal/store"
	. "github.com/onsi/gomega"
	"golang.org/x/net/html"
)

func Test_escapePath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{path: "a#b.md", want: "a%23b.md"},
		{path: "q?.md", want: "q%3F.md"},
		{path: "50%.md", want: "50%25.md"},
		{path: "dir with space/x.md", want: "dir%20with%20space/x.md"},
		{path: "internal/upload/worker.go", want: "internal/upload/worker.go"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(escapePath(c.path)).To(Equal(c.want))
		})
	}
}

func TestServer_Surface_file(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	var lines []string
	for i := 1; i <= 12; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	commitFile(g, repo, "src/long.txt", strings.Join(lines, "\n")+"\n")
	lines[5] = "line 6, changed"
	write(g, repo, "src/long.txt", strings.Join(lines, "\n")+"\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"src"}, Note: "first"}).Review.ID

	doc := f.page(g, "/reviews/"+itoa(id)+"/files/src/long.txt", http.StatusOK)
	g.Expect(text(find(doc, "topbar"))).To(Equal("Reviewlong.txt"))
	g.Expect(attr(find(doc, "back"), "href")).To(Equal("/reviews/" + itoa(id)))
	g.Expect(text(find(find(doc, "crumb"), "dir"))).To(Equal("src/"))
	g.Expect(text(find(find(doc, "crumb"), "name"))).To(Equal("long.txt"))
	g.Expect(find(doc, "notice")).To(BeNil(), "a file the review changes carries no banner")
	g.Expect(text(find(doc, "toolbar"))).To(Equal("+1 −1 in this round MarkedPlain"))
	links := segmentLinks(doc)
	g.Expect(links).To(Equal([]string{
		"Marked /reviews/" + itoa(id) + "/files/src/long.txt?view=marked current",
		"Plain /reviews/" + itoa(id) + "/files/src/long.txt?view=plain",
	}))
	// The whole file, with the change marked in place and no hunk header.
	got := tableRows(doc)
	g.Expect(got).To(HaveLen(13))
	g.Expect(got[0]).To(Equal("1line 1"))
	g.Expect(got[5]).To(Equal("del 6−line 6"))
	g.Expect(got[6]).To(Equal("add 6+line 6, changed"))
	g.Expect(got[12]).To(Equal("12line 12"))

	// Plain is the file as the agent left it, with no tint and no removed
	// line.
	doc = f.page(g, "/reviews/"+itoa(id)+"/files/src/long.txt?view=plain", http.StatusOK)
	g.Expect(segmentLinks(doc)).To(Equal([]string{
		"Marked /reviews/" + itoa(id) + "/files/src/long.txt?view=marked",
		"Plain /reviews/" + itoa(id) + "/files/src/long.txt?view=plain current",
	}))
	got = tableRows(doc)
	g.Expect(got).To(HaveLen(12))
	g.Expect(got[5]).To(Equal("6line 6, changed"))
}

func TestServer_Surface_fileAgainst(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "keep.txt", "changed in round one\n")
	write(g, repo, "context.txt", "outside the paths, left alone\n")
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt", "keep.txt"}, Note: "first"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "more")
	write(g, repo, "a.txt", "one\ntwo\nthree\n")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
	base := "/reviews/" + itoa(id)

	// The file page counts what the review page's card counts, under either
	// comparison, and says which it is.
	againsts := []struct {
		query   string
		counted string
	}{
		{query: "", counted: "in this round"},
		{query: "?against=base", counted: "in this review"},
	}
	for _, c := range againsts {
		t.Run("against "+c.query, func(t *testing.T) {
			g := NewWithT(t)
			card := ""
			for _, file := range findAll(f.page(g, base+c.query, http.StatusOK), hasClass("file")) {
				if text(find(file, "path")) == "a.txt" {
					card = text(find(file, "counts"))
				}
			}
			g.Expect(card).NotTo(BeEmpty())
			doc := f.page(g, base+"/files/a.txt"+c.query, http.StatusOK)
			g.Expect(text(find(find(doc, "toolbar"), "counts"))).To(Equal(card + " " + c.counted))
		})
	}

	// Every link keeps the comparison the page was opened with.
	doc := f.page(g, base+"/files/a.txt?against=base", http.StatusOK)
	g.Expect(segmentLinks(doc)).To(Equal([]string{
		"Marked " + base + "/files/a.txt?against=base&view=marked current",
		"Plain " + base + "/files/a.txt?against=base&view=plain",
	}))
	g.Expect(attr(find(doc, "back"), "href")).To(Equal(base + "?against=base"))
	doc = f.page(g, base+"/files/a.txt?against=base&view=plain", http.StatusOK)
	g.Expect(segmentLinks(doc)[1]).To(HaveSuffix(" current"))
	g.Expect(attr(find(doc, "back"), "href")).To(Equal(base + "?against=base"))

	// A file the review changed in round one and left alone since has
	// nothing to mark against round one, and says so in place of counts.
	doc = f.page(g, base+"/files/keep.txt", http.StatusOK)
	g.Expect(text(find(doc, "toolbar"))).To(Equal("Not changed since round 1"))
	g.Expect(find(doc, "segmented")).To(BeNil())
	g.Expect(tableRows(doc)).To(Equal([]string{"1changed in round one"}))
	doc = f.page(g, base+"/files/keep.txt?against=base", http.StatusOK)
	g.Expect(text(find(doc, "toolbar"))).To(Equal("+1 −1 in this review MarkedPlain"))
	// A file outside the review's paths that did not move has no toolbar at
	// all.
	doc = f.page(g, base+"/files/context.txt", http.StatusOK)
	g.Expect(find(doc, "toolbar")).To(BeNil())
	g.Expect(tableRows(doc)).To(Equal([]string{"1outside the paths, left alone"}))

	// An earlier round's page stays on that round, and goes back to it.
	doc = f.page(g, base+"/rounds/1/files/a.txt", http.StatusOK)
	g.Expect(text(find(doc, "topbar"))).To(Equal("Round 1a.txt"))
	g.Expect(attr(find(doc, "back"), "href")).To(Equal(base + "/rounds/1"))
	g.Expect(segmentLinks(doc)[0]).To(Equal("Marked " + base + "/rounds/1/files/a.txt?view=marked current"))
	g.Expect(tableRows(doc)).To(Equal([]string{"1one", "add 2+two"}))
	doc = f.page(g, base+"/rounds/1/files/a.txt?against=base", http.StatusOK)
	g.Expect(attr(find(doc, "back"), "href")).To(Equal(base + "/rounds/1?against=base"))

	// The current round answers under the round's address as well.
	doc = f.page(g, base+"/rounds/2/files/a.txt", http.StatusOK)
	g.Expect(text(find(doc, "topbar"))).To(Equal("Reviewa.txt"))
}

func TestServer_Surface_fileBanners(t *testing.T) {
	t.Run("each source on each round", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		f := newFixture(g, t)
		repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
		commitFile(g, repo, "lib.txt", "library\n")
		write(g, repo, "other.txt", "context\n")
		me := api.Caller{Dir: repo}
		id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt", "keep.txt"}, Note: "first"}).Review.ID
		f.decide(g, t, id, store.VerdictChanges, "more")
		write(g, repo, "other.txt", "more context\n")
		f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
		write(g, repo, "later.txt", "written after both rounds\n")
		base := "/reviews/" + itoa(id)

		pages := []struct {
			path    string
			notices []string
			rows    []string
		}{
			{
				path:    "/files/other.txt",
				notices: []string{"info: Not part of this review. Changed by the agent outside the review's paths, shown as it was when round 2 was taken."},
				rows:    []string{"del 1−context", "add 1+more context"},
			},
			{
				path:    "/rounds/1/files/other.txt",
				notices: []string{"info: Not part of this review. Changed by the agent outside the review's paths, shown as it was when round 1 was taken."},
				rows:    []string{"add 1+context"},
			},
			{
				path:    "/files/keep.txt",
				notices: []string{"info: Unchanged by this review. Unchanged since the commit round 1 was taken from."},
				rows:    []string{"1unchanged"},
			},
			{
				path:    "/rounds/1/files/lib.txt",
				notices: []string{"info: Not part of this review. Unchanged since the commit round 1 was taken from."},
				rows:    []string{"1library"},
			},
			{
				path:    "/files/later.txt",
				notices: []string{"warn: Live, not from this round. Nothing captured this file, so it is shown as it is on disk now and may have moved."},
				rows:    []string{"1written after both rounds"},
			},
			{
				path:    "/rounds/1/files/later.txt",
				notices: []string{"warn: Live, not from this round. Nothing captured this file, so it is shown as it is on disk now and may have moved."},
				rows:    []string{"1written after both rounds"},
			},
		}
		for _, c := range pages {
			t.Run(c.path, func(t *testing.T) {
				g := NewWithT(t)
				doc := f.page(g, base+c.path, http.StatusOK)
				g.Expect(notices(doc)).To(Equal(c.notices))
				g.Expect(tableRows(doc)).To(Equal(c.rows))
			})
		}
		// A context file goes back to its round until the tree exists.
		doc := f.page(g, base+"/rounds/1/files/lib.txt", http.StatusOK)
		g.Expect(text(find(doc, "back"))).To(Equal("Round 1"))
		g.Expect(attr(find(doc, "back"), "href")).To(Equal(base + "/rounds/1"))
	})

	t.Run("a gone base", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		f := newFixture(g, t)
		repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
		id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
		dropBase(g, repo)

		// Nothing is compared, so there are no counts and no control, and the
		// captured file shows plain.
		doc := f.page(g, "/reviews/"+itoa(id)+"/files/a.txt", http.StatusOK)
		g.Expect(notices(doc)).To(BeEmpty())
		g.Expect(text(find(doc, "toolbar"))).To(Equal("The commit this review started from is gone, so the change cannot be measured."))
		g.Expect(find(doc, "segmented")).To(BeNil())
		g.Expect(tableRows(doc)).To(Equal([]string{"1one", "2two"}))
		// A file the capture does not hold reads live, and says why.
		doc = f.page(g, "/reviews/"+itoa(id)+"/files/keep.txt", http.StatusOK)
		g.Expect(notices(doc)).To(Equal([]string{"warn: Live, not from this round. The commit round 1 was taken from is gone, so it is shown as it is on disk now and may have moved."}))
		g.Expect(find(doc, "toolbar")).To(BeNil())
	})

	t.Run("a lost repository", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		f := newFixture(g, t)
		dir := t.TempDir()
		repo := gitRepo(g, filepath.Join(dir, "harbor"))
		commitFile(g, repo, ".gitignore", "private.txt\n")
		write(g, repo, "private.txt", "a secret\n")
		write(g, repo, "other.txt", "context\n")
		write(g, repo, "fresh/new.txt", "a directory the capture holds\n")
		id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
		g.Expect(os.Rename(filepath.Join(repo, ".git"), filepath.Join(dir, "moved.git"))).To(Succeed())
		lost := "warn: Only what this round captured can be shown. The project's repository is gone from its root."

		doc := f.page(g, "/reviews/"+itoa(id)+"/files/a.txt", http.StatusOK)
		g.Expect(notices(doc)).To(Equal([]string{lost}))
		g.Expect(text(find(doc, "toolbar"))).To(Equal("The project's repository is gone, so the change cannot be measured."))
		g.Expect(find(doc, "segmented")).To(BeNil())
		g.Expect(tableRows(doc)).To(Equal([]string{"1one", "2two"}))
		doc = f.page(g, "/reviews/"+itoa(id)+"/files/other.txt", http.StatusOK)
		g.Expect(notices(doc)).To(Equal([]string{lost, "info: Not part of this review. Changed by the agent outside the review's paths, shown as it was when round 1 was taken."}))

		// The formerly ignored file has no rule left to refuse it, and the
		// base cannot be read, so neither opens.
		refused := []string{"private.txt", "keep.txt"}
		for _, p := range refused {
			doc = f.page(g, "/reviews/"+itoa(id)+"/files/"+p, http.StatusNotFound)
			g.Expect(notices(doc)).To(Equal([]string{lost}), p)
			g.Expect(text(find(doc, "empty"))).To(ContainSubstring("There is no page at this address."), p)
		}
		// The tree's root and a directory are the plain 404 page until the
		// tree answers them. No live read was asked for, so nothing was
		// refused.
		directories := []string{"", "fresh"}
		for _, p := range directories {
			doc = f.page(g, "/reviews/"+itoa(id)+"/files/"+p, http.StatusNotFound)
			g.Expect(notices(doc)).To(BeEmpty(), p)
		}
	})

	t.Run("a missing root", func(t *testing.T) {
		g := NewWithT(t)
		f := newFixture(g, t)
		repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
		id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
		g.Expect(os.RemoveAll(repo)).To(Succeed())
		root, err := filepath.EvalSymlinks(filepath.Dir(repo))
		g.Expect(err).NotTo(HaveOccurred())
		missing := "warn: Only what this round captured can be shown. The project's root, " + filepath.Join(root, "harbor") + ", no longer exists."

		doc := f.page(g, "/reviews/"+itoa(id)+"/files/a.txt", http.StatusOK)
		g.Expect(notices(doc)).To(Equal([]string{missing}))
		g.Expect(text(find(doc, "toolbar"))).To(Equal("The project's root is gone, so the change cannot be measured."))
		g.Expect(tableRows(doc)).To(Equal([]string{"1one", "2two"}))
		doc = f.page(g, "/reviews/"+itoa(id)+"/files/keep.txt", http.StatusNotFound)
		g.Expect(notices(doc)).To(Equal([]string{missing}))
	})

	t.Run("outside git", func(t *testing.T) {
		g := NewWithT(t)
		f := newFixture(g, t)
		root := filepath.Join(t.TempDir(), "notes")
		write(g, root, ".eyeball", "")
		write(g, root, "plan.md", "the plan\n")
		id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: root}, Paths: []string{"plan.md"}, Note: "first"}).Review.ID
		write(g, root, "later.md", "written later\n")

		doc := f.page(g, "/reviews/"+itoa(id)+"/files/later.md", http.StatusOK)
		g.Expect(notices(doc)).To(Equal([]string{"warn: Live, not from this round. The project is outside git, so it is shown as it is on disk now and may have moved."}))
		doc = f.page(g, "/reviews/"+itoa(id)+"/files/plan.md", http.StatusOK)
		g.Expect(notices(doc)).To(BeEmpty())
		g.Expect(text(find(doc, "toolbar"))).To(Equal("+1 −0 in this round MarkedPlain"))
	})
}

func TestServer_Surface_fileColonNamedBase(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	commitFile(g, repo, ":weird.txt", "leading colon\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID

	// The disk holds the same file, so only the banner tells a lookup that
	// read the colon as pathspec magic, and fell through to the disk, from
	// one that found it in the base.
	doc := f.page(g, "/reviews/"+itoa(id)+"/files/:weird.txt", http.StatusOK)
	g.Expect(notices(doc)).To(Equal([]string{"info: Not part of this review. Unchanged since the commit round 1 was taken from."}))
	g.Expect(tableRows(doc)).To(Equal([]string{"1leading colon"}))
}

func TestServer_Surface_fileDeleted(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	commitFile(g, repo, "lib.txt", "library\nsecond line\n")
	g.Expect(os.Remove(filepath.Join(repo, "keep.txt"))).To(Succeed())
	g.Expect(os.Remove(filepath.Join(repo, "lib.txt"))).To(Succeed())
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt", "keep.txt"}, Note: "first"}).Review.ID
	base := "/reviews/" + itoa(id)

	// Marked shows every old line as removed, and Plain says it is gone.
	doc := f.page(g, base+"/files/keep.txt", http.StatusOK)
	g.Expect(notices(doc)).To(BeEmpty())
	g.Expect(text(find(doc, "toolbar"))).To(Equal("+0 −1 in this round MarkedPlain"))
	g.Expect(tableRows(doc)).To(Equal([]string{"del 1−unchanged"}))
	doc = f.page(g, base+"/files/keep.txt?view=plain", http.StatusOK)
	g.Expect(text(find(doc, "empty"))).To(Equal("DeletedThe agent deleted this file as of round 1."))
	g.Expect(find(doc, "code")).To(BeNil())

	// Deleted outside the review's paths, it says so.
	doc = f.page(g, base+"/files/lib.txt", http.StatusOK)
	g.Expect(notices(doc)).To(Equal([]string{"info: Not part of this review. Deleted by the agent outside the review's paths, as of round 1."}))
	g.Expect(tableRows(doc)).To(Equal([]string{"del 1−library", "del 2−second line"}))

	// Still deleted a round later, it did not move against that round, so it
	// shows the box with no control. Against the base it moved.
	f.decide(g, t, id, store.VerdictChanges, "more")
	write(g, repo, "a.txt", "one\ntwo\nthree\n")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
	doc = f.page(g, base+"/files/keep.txt", http.StatusOK)
	g.Expect(find(doc, "toolbar")).To(BeNil())
	g.Expect(text(find(doc, "empty"))).To(Equal("DeletedThe agent deleted this file as of round 2."))
	doc = f.page(g, base+"/files/keep.txt?against=base", http.StatusOK)
	g.Expect(text(find(doc, "toolbar"))).To(Equal("+0 −1 in this review MarkedPlain"))
	g.Expect(tableRows(doc)).To(Equal([]string{"del 1−unchanged"}))
	doc = f.page(g, base+"/files/lib.txt", http.StatusOK)
	g.Expect(notices(doc)).To(Equal([]string{"info: Not part of this review. Deleted by the agent outside the review's paths, as of round 2."}))
	g.Expect(text(find(doc, "empty"))).To(Equal("DeletedThe agent deleted this file as of round 2."))
}

func TestServer_Surface_fileEscapedNames(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	names := []string{"a#b.txt", "q?.txt", "50%.txt", "with space/x.txt"}
	for _, name := range names {
		write(g, repo, name, "content of "+name+"\n")
	}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: names, Note: "first"}).Review.ID

	// The card's link is the escaped address, and following it lands on a
	// page naming the whole file, not a prefix cut short at a # or a ?.
	links := map[string]string{}
	for _, file := range findAll(f.page(g, "/reviews/"+itoa(id), http.StatusOK), hasClass("file")) {
		links[text(find(file, "path"))] = attr(find(find(file, "filelinks"), "btn"), "href")
	}
	g.Expect(links).To(Equal(map[string]string{
		"a#b.txt":          "/reviews/" + itoa(id) + "/files/a%23b.txt",
		"q?.txt":           "/reviews/" + itoa(id) + "/files/q%3F.txt",
		"50%.txt":          "/reviews/" + itoa(id) + "/files/50%25.txt",
		"with space/x.txt": "/reviews/" + itoa(id) + "/files/with%20space/x.txt",
	}))
	for name, href := range links {
		doc := f.page(g, href, http.StatusOK)
		g.Expect(text(find(find(doc, "crumb"), "name"))).To(Equal(filepath.Base(name)), href)
		g.Expect(segmentLinks(doc)[1]).To(HavePrefix("Plain "+href+"?view=plain"), href)
	}
}

func TestServer_Surface_fileMarksCheckout(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	outer := gitRepo(g, filepath.Join(t.TempDir(), "outer"))
	write(g, outer, "notes/.eyeball", "")
	write(g, outer, "notes/n.md", "a note\n")
	root, err := filepath.EvalSymlinks(filepath.Join(outer, "notes"))
	g.Expect(err).NotTo(HaveOccurred())
	// Opened through the store, as tool/seed opens one, so the request path
	// that also writes the flag never runs.
	project, err := f.store.EnsureProject(t.Context(), root, "notes")
	g.Expect(err).NotTo(HaveOccurred())
	req, err := capture.Freeze(t.Context(), capture.Options{Root: root, Paths: []string{"n.md"}, Round: 1, Blobs: f.blobs})
	g.Expect(err).NotTo(HaveOccurred())
	req.Note = "a note"
	review, _, err := f.store.OpenReview(t.Context(), store.NewReview{ProjectID: project.ID, Agent: "agent-1", Title: "Notes", Paths: []string{"n.md"}, Request: req})
	g.Expect(err).NotTo(HaveOccurred())
	before, err := f.store.Project(t.Context(), project.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(before.InsideCheckout).To(BeFalse())

	f.page(g, "/reviews/"+itoa(review.ID)+"/files/n.md", http.StatusOK)
	after, err := f.store.Project(t.Context(), project.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(after.InsideCheckout).To(BeTrue())
}

func TestServer_Surface_fileNotFound(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	commitFile(g, repo, "d/x.txt", "in a directory\n")
	write(g, repo, "fresh/new.txt", "a directory the capture holds\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt", "fresh"}, Note: "first"}).Review.ID
	g.Expect(os.Symlink("loop", filepath.Join(repo, "loop"))).To(Succeed())
	base := "/reviews/" + itoa(id)

	paths := []string{
		base + "/files/",
		base + "/files/.git/config",
		base + "/files/d/.git/config",
		base + "/files/%2e%2e/%2e%2e/etc/passwd",
		base + "/files/..%2F..%2Fetc%2Fpasswd",
		base + "/files/a/../../x",
		base + "/files/a%00b",
		base + "/files/nothing.txt",
		base + "/files/a.txt/x",
		base + "/files/" + strings.Repeat("x", 300),
		base + "/files/loop/x",
		// A directory, until the tree answers one.
		base + "/files/d",
		base + "/files/fresh",
		base + "/rounds/2/files/a.txt",
		base + "/rounds/x/files/a.txt",
		"/reviews/999/files/a.txt",
		"/reviews/abc/files/a.txt",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			g := NewWithT(t)
			doc := f.page(g, p, http.StatusNotFound)
			g.Expect(text(find(doc, "empty"))).To(ContainSubstring("There is no page at this address."))
			g.Expect(find(doc, "notice")).To(BeNil(), "a live read was allowed, so nothing was refused")
		})
	}
}

func TestServer_Surface_fileRefusedLive(t *testing.T) {
	t.Run("a failed ignore check", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		f := newFixture(g, t)
		repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
		id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
		write(g, repo, "later.txt", "written later\n")
		// A truncated index makes check-ignore exit 128.
		write(g, repo, ".git/index", "garbage\n")

		doc := f.page(g, "/reviews/"+itoa(id)+"/files/later.txt", http.StatusNotFound)
		g.Expect(notices(doc)).To(Equal([]string{"warn: Nothing on disk can be shown. Git could not say which files it ignores."}))
		doc = f.page(g, "/reviews/"+itoa(id)+"/files/a.txt", http.StatusOK)
		g.Expect(tableRows(doc)).To(Equal([]string{"1one", "add 2+two"}))
	})

	t.Run("rules unreadable for a marker project", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		f := newFixture(g, t)
		outer, notes := markerInCheckout(g, t.TempDir())
		id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: notes}, Paths: []string{"plan.md"}, Note: "first"}).Review.ID
		// Before the ceiling, the outer checkout's rules refuse creds.txt and
		// let other.md through.
		doc := f.page(g, "/reviews/"+itoa(id)+"/files/creds.txt", http.StatusNotFound)
		g.Expect(notices(doc)).To(BeEmpty())
		f.page(g, "/reviews/"+itoa(id)+"/files/other.md", http.StatusOK)

		// The ceiling stops git short of the outer checkout, so its rules
		// cannot be read, and nothing on disk opens.
		t.Setenv("GIT_CEILING_DIRECTORIES", outer)
		unreadable := []string{"warn: Nothing on disk can be shown. Git could not say which files it ignores."}
		doc = f.page(g, "/reviews/"+itoa(id)+"/files/creds.txt", http.StatusNotFound)
		g.Expect(notices(doc)).To(Equal(unreadable))
		doc = f.page(g, "/reviews/"+itoa(id)+"/files/other.md", http.StatusNotFound)
		g.Expect(notices(doc)).To(Equal(unreadable))
		f.page(g, "/reviews/"+itoa(id)+"/files/plan.md", http.StatusOK)
	})

	t.Run("a disowned marker project", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		f := newFixture(g, t)
		outer, _ := markerInCheckout(g, t.TempDir())
		ign := filepath.Join(outer, "ign")
		id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: ign}, Paths: []string{"plan.md"}, Note: "first"}).Review.ID

		doc := f.page(g, "/reviews/"+itoa(id)+"/files/other.md", http.StatusNotFound)
		g.Expect(notices(doc)).To(Equal([]string{"warn: Only what this round captured can be shown. The checkout around the project ignores it."}))
		doc = f.page(g, "/reviews/"+itoa(id)+"/files/plan.md", http.StatusOK)
		g.Expect(notices(doc)).To(BeEmpty())
	})

	t.Run("a moved outer .git", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		f := newFixture(g, t)
		dir := t.TempDir()
		outer, notes := markerInCheckout(g, dir)
		id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: notes}, Paths: []string{"plan.md"}, Note: "first"}).Review.ID
		g.Expect(os.Rename(filepath.Join(outer, ".git"), filepath.Join(dir, "moved.git"))).To(Succeed())

		gone := []string{"warn: Only what this round captured can be shown. The checkout the project sat inside has lost its repository."}
		paths := []string{"creds.txt", "other.md"}
		for _, p := range paths {
			doc := f.page(g, "/reviews/"+itoa(id)+"/files/"+p, http.StatusNotFound)
			g.Expect(notices(doc)).To(Equal(gone), p)
		}
		f.page(g, "/reviews/"+itoa(id)+"/files/plan.md", http.StatusOK)
	})
}

func TestServer_Surface_fileTooLargeOrBinary(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	huge := strings.Repeat("x", diff.MaxBytes) + "\n"
	commitFile(g, repo, "base.bin", "\x00\x02binary in the base")
	commitFile(g, repo, "huge.txt", huge)
	commitFile(g, repo, "shrunk.txt", huge)
	write(g, repo, "big.txt", strings.Repeat("x\n", 20001))
	write(g, repo, "a.bin", "\x00\x01binary")
	write(g, repo, "shrunk.txt", "small now\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"big.txt", "a.bin", "shrunk.txt"}, Note: "big"}).Review.ID

	pages := []struct {
		path string
		want string
	}{
		{path: "big.txt", want: "Too large to show as a diff."},
		{path: "a.bin", want: "Binary file, not shown as a diff."},
		{path: "base.bin", want: "Binary file, not shown as a diff."},
		{path: "huge.txt", want: "Too large to show as a diff."},
		{path: "shrunk.txt", want: "Too large to show as a diff."},
	}
	for _, c := range pages {
		t.Run(c.path, func(t *testing.T) {
			g := NewWithT(t)
			doc := f.page(g, "/reviews/"+itoa(id)+"/files/"+c.path, http.StatusOK)
			g.Expect(text(find(doc, "toobig"))).To(Equal(c.want))
			g.Expect(find(doc, "code")).To(BeNil())
		})
	}
}

func TestServer_Surface_reviewWholeFileLink(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "more")
	write(g, repo, "a.txt", "one\ntwo\nthree\n")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
	base := "/reviews/" + itoa(id)

	// Each card links its file's page, keeping the page's comparison, and an
	// earlier round's card stays on that round.
	pages := []struct {
		page string
		want string
	}{
		{page: base, want: base + "/files/a.txt"},
		{page: base + "?against=base", want: base + "/files/a.txt?against=base"},
		{page: base + "/rounds/1", want: base + "/rounds/1/files/a.txt"},
		{page: base + "/rounds/1?against=base", want: base + "/rounds/1/files/a.txt?against=base"},
	}
	for _, c := range pages {
		t.Run(c.page, func(t *testing.T) {
			g := NewWithT(t)
			link := find(find(f.page(g, c.page, http.StatusOK), "file"), "filelinks")
			g.Expect(text(link)).To(Equal("Whole file"))
			g.Expect(attr(find(link, "btn"), "href")).To(Equal(c.want))
			g.Expect(hasClass("small")(find(link, "btn"))).To(BeTrue())
		})
	}
}

// commitFile writes one file and records it, alone, on main. Anything else
// changed in the working tree stays uncommitted.
func commitFile(g *WithT, repo, path, content string) {
	g.THelper()
	write(g, repo, path, content)
	git(g, repo, "--literal-pathspecs", "add", "--", path)
	tree := git(g, repo, "write-tree")
	commit := git(g, repo, "commit-tree", tree, "-p", "main", "-m", "add "+path)
	git(g, repo, "update-ref", "refs/heads/main", commit)
}

// dropBase moves main to a commit with no parent and deletes the commit it
// pointed at, as a rebase and a garbage collection would.
func dropBase(g *WithT, repo string) {
	g.THelper()
	old := git(g, repo, "rev-parse", "main")
	tree := git(g, repo, "write-tree")
	commit := git(g, repo, "commit-tree", tree, "-m", "rewritten")
	git(g, repo, "update-ref", "refs/heads/main", commit)
	g.Expect(os.Remove(filepath.Join(repo, ".git", "objects", old[:2], old[2:]))).To(Succeed())
}

// markerInCheckout builds a checkout holding two marker projects: notes,
// which the checkout's rules apply to, and ign, which the checkout ignores. It
// returns the checkout's root and notes's resolved root.
func markerInCheckout(g *WithT, dir string) (string, string) {
	g.THelper()
	outer := gitRepo(g, filepath.Join(dir, "outer"))
	commitFile(g, outer, ".gitignore", "ign/\ncreds.txt\n")
	write(g, outer, "notes/.eyeball", "")
	write(g, outer, "notes/plan.md", "the plan\n")
	write(g, outer, "notes/creds.txt", "a secret\n")
	write(g, outer, "notes/other.md", "not captured\n")
	write(g, outer, "ign/.eyeball", "")
	write(g, outer, "ign/plan.md", "the plan\n")
	write(g, outer, "ign/other.md", "not captured\n")
	resolved, err := filepath.EvalSymlinks(outer)
	g.Expect(err).NotTo(HaveOccurred())
	return resolved, filepath.Join(resolved, "notes")
}

// notices is each banner on a page, as warn or info and its text.
func notices(doc *html.Node) []string {
	var out []string
	for _, n := range findAll(doc, hasClass("notice")) {
		kind := "info"
		if hasClass("warn")(n) {
			kind = "warn"
		}
		out = append(out, kind+": "+text(n))
	}
	return out
}

// tableRows is each row of a page's code table, as its class and its text.
func tableRows(doc *html.Node) []string {
	var out []string
	for _, tr := range findAll(doc, func(n *html.Node) bool { return n.Data == "tr" }) {
		out = append(out, strings.TrimSpace(attr(tr, "class")+" "+text(tr)))
	}
	return out
}

// segmentLinks is each link of a page's segmented control, as its text, its
// address, and whether it is the current one.
func segmentLinks(doc *html.Node) []string {
	control := find(doc, "segmented")
	if control == nil {
		return nil
	}
	var out []string
	for _, a := range findAll(control, func(n *html.Node) bool { return n.Data == "a" }) {
		link := text(a) + " " + attr(a, "href")
		if attr(a, "aria-current") == "true" {
			link += " current"
		}
		out = append(out, link)
	}
	return out
}
