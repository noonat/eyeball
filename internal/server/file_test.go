package server

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/diff"
	"github.com/noonat/eyeball/internal/kind"
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
		{path: "\\evil.com", want: "%5Cevil.com"},
		{path: "docs/x\"y.md", want: "docs/x%22y.md"},
		{path: "", want: ""},
		{path: "internal/upload/worker.go", want: "internal/upload/worker.go"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(escapePath(c.path)).To(Equal(c.want))
		})
	}
}

func Test_everyRefusalHasANotice(t *testing.T) {
	// "future" stands for a refusing value added to capture after this was
	// written. treeNotice dereferences what refusalNotice returns for any
	// value Refused accepts, so a nil there would be a panic and a 500.
	generic := "Nothing on disk is read for this project."
	values := []capture.Live{
		capture.LiveOpen,
		capture.LiveIgnoreRules,
		capture.LiveRootMissing,
		capture.LiveLost,
		capture.LiveIgnored,
		capture.LiveCheckoutGone,
		capture.LiveRulesUnreadable,
		capture.Live("future"),
	}
	for _, live := range values {
		t.Run(string(live), func(t *testing.T) {
			g := NewWithT(t)
			st := capture.State{Live: live}
			n := refusalNotice(st, nil, "~/src/x")
			if !live.Refused() {
				g.Expect(n).To(BeNil())
				return
			}
			g.Expect(n).NotTo(BeNil())
			g.Expect(n.Warn).To(BeTrue())
			if live == "future" {
				g.Expect(n.Text).To(Equal(generic))
			} else {
				g.Expect(n.Text).NotTo(Equal(generic), "a known value has a reason of its own")
			}
			tree := treeNotice(st, capture.ListingCapture, "~/src/x")
			g.Expect(tree.Text).To(HavePrefix(n.Text))
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
	g.Expect(barText(doc)).To(Equal("Reviewlong.txt"))
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
	g.Expect(barText(doc)).To(Equal("Round 1a.txt"))
	g.Expect(attr(find(doc, "back"), "href")).To(Equal(base + "/rounds/1"))
	g.Expect(segmentLinks(doc)[0]).To(Equal("Marked " + base + "/rounds/1/files/a.txt?view=marked current"))
	g.Expect(tableRows(doc)).To(Equal([]string{"1one", "add 2+two"}))
	doc = f.page(g, base+"/rounds/1/files/a.txt?against=base", http.StatusOK)
	g.Expect(attr(find(doc, "back"), "href")).To(Equal(base + "/rounds/1?against=base"))

	// The current round answers under the round's address as well.
	doc = f.page(g, base+"/rounds/2/files/a.txt", http.StatusOK)
	g.Expect(barText(doc)).To(Equal("Reviewa.txt"))
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
		// A file the review changes goes back to its round, and any other
		// file to its directory in the tree, keeping the round and the
		// comparison.
		backs := []struct {
			path  string
			label string
			href  string
		}{
			{path: "/files/a.txt", label: "Review", href: base},
			{
				path:  "/rounds/1/files/a.txt?against=base",
				label: "Round 1",
				href:  base + "/rounds/1?against=base",
			},
			{
				path:  "/rounds/1/files/lib.txt",
				label: "Files",
				href:  base + "/rounds/1/files/",
			},
			{
				path:  "/files/other.txt?against=base",
				label: "Files",
				href:  base + "/files/?against=base",
			},
			{path: "/files/later.txt", label: "Files", href: base + "/files/"},
		}
		for _, c := range backs {
			doc := f.page(g, base+c.path, http.StatusOK)
			g.Expect(text(find(doc, "back"))).To(Equal(c.label), c.path)
			g.Expect(attr(find(doc, "back"), "href")).To(Equal(c.href), c.path)
		}
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
		// A path CleanPath refuses is the plain 404 page. No live read was
		// asked for, so nothing was refused.
		doc = f.page(g, "/reviews/"+itoa(id)+"/files/.git/config", http.StatusNotFound)
		g.Expect(notices(doc)).To(BeEmpty())
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
		g.Expect(text(find(doc, "toolbar"))).To(Equal("+1 −0 in this round DocumentSource"))
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

func TestServer_Surface_fileDocument(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "a.txt", "one\n")
	commitFile(g, repo, "docs/product.md", "# Product\n\nFirst.\n")
	commitFile(g, repo, "docs/keep.md", "# Keep\n")
	commitFile(g, repo, "docs/gone.md", "# Gone\n\nA passage.\n")
	commitFile(g, repo, "docs/guide.md", "# Guide\n\n## Part two\n")
	write(g, repo, "docs/product.md", "# Product\n\nFirst, reworded.\n\n## Part two\n\nSee [part two](#part-two) and [the guide](guide.md).\n\n<div>\nraw\n</div>\n")
	write(g, repo, "docs/keep.md", "# Keep\n\nChanged in round one.\n")
	me := api.Caller{Dir: repo}
	paths := []string{"docs/product.md", "docs/keep.md", "docs/gone.md"}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: paths, Note: "first"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "more")
	write(g, repo, "docs/product.md", "# Product\n\nFirst, reworded again.\n\n## Part two\n\nSee [part two](#part-two) and [the guide](guide.md).\n\n<div>\nraw\n</div>\n")
	g.Expect(os.Remove(filepath.Join(repo, "docs", "gone.md"))).To(Succeed())
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
	base := "/reviews/" + itoa(id)
	product := base + "/files/docs/product.md"

	// A markdown file opens on its document, with heading ids, its counts,
	// and Document and Source in place of Marked and Plain.
	doc := f.page(g, product, http.StatusOK)
	g.Expect(text(find(doc, "toolbar"))).To(Equal("+1 −1 in this round DocumentSource"))
	g.Expect(segmentLinks(doc)).To(Equal([]string{
		"Document " + product + "?view=document current",
		"Source " + product + "?view=source",
	}))
	g.Expect(find(doc, "file")).To(BeNil())
	prose := find(doc, "prose")
	g.Expect(hasClass("page")(prose.Parent)).To(BeTrue())
	g.Expect(proseBlocks(prose.Parent)).To(Equal([]string{
		"kept: Product",
		"marked: First, reworded again. | was: First, reworded.",
		"kept: Part two",
		"kept: See part two and the guide.",
		"kept: This block is HTML, which is not rendered. Read it in Source.",
	}))
	var ids []string
	for _, h := range findAll(prose, func(n *html.Node) bool { return attr(n, "id") != "" }) {
		ids = append(ids, h.Data+"#"+attr(h, "id"))
	}
	g.Expect(ids).To(Equal([]string{"h1#product", "h2#part-two"}))
	g.Expect(documentLinks(prose)).To(Equal([]string{"#part-two", base + "/files/docs/guide.md", product + "?view=source"}))

	// Source is the whole file with the change marked, and Marked and Plain,
	// a code file's views, open the document.
	doc = f.page(g, product+"?view=source", http.StatusOK)
	g.Expect(segmentLinks(doc)[1]).To(HaveSuffix(" current"))
	g.Expect(find(doc, "prose")).To(BeNil())
	g.Expect(tableRows(doc)).To(ContainElements("del 3−First, reworded.", "add 3+First, reworded again."))
	views := []string{"plain", "marked", "document", "nonsense"}
	for _, view := range views {
		doc := f.page(g, product+"?view="+view, http.StatusOK)
		g.Expect(find(doc, "prose")).NotTo(BeNil(), view)
		g.Expect(segmentLinks(doc)[0]).To(HaveSuffix(" current"), view)
	}

	// Every link keeps the comparison, and an earlier round's stays on it.
	doc = f.page(g, product+"?against=base", http.StatusOK)
	g.Expect(segmentLinks(doc)).To(Equal([]string{
		"Document " + product + "?against=base&view=document current",
		"Source " + product + "?against=base&view=source",
	}))
	g.Expect(documentLinks(find(doc, "prose"))).To(Equal([]string{"#part-two", base + "/files/docs/guide.md?against=base", product + "?against=base&view=source"}))
	doc = f.page(g, base+"/rounds/1/files/docs/product.md?against=base", http.StatusOK)
	earlier := base + "/rounds/1/files/docs/product.md"
	g.Expect(segmentLinks(doc)).To(Equal([]string{
		"Document " + earlier + "?against=base&view=document current",
		"Source " + earlier + "?against=base&view=source",
	}))
	g.Expect(documentLinks(find(doc, "prose"))).To(Equal([]string{"#part-two", base + "/rounds/1/files/docs/guide.md?against=base", earlier + "?against=base&view=source"}))

	// A document that did not move says so, keeps both views, and marks
	// nothing.
	doc = f.page(g, base+"/files/docs/keep.md", http.StatusOK)
	g.Expect(text(find(doc, "toolbar"))).To(Equal("Not changed since round 1 DocumentSource"))
	g.Expect(proseBlocks(find(doc, "prose").Parent)).To(Equal([]string{"kept: Keep", "kept: Changed in round one."}))
	doc = f.page(g, base+"/files/docs/keep.md?view=source", http.StatusOK)
	g.Expect(tableRows(doc)).To(Equal([]string{"1# Keep", "2", "3Changed in round one."}))

	// A file opened for context has both views and no counts.
	doc = f.page(g, base+"/files/docs/guide.md", http.StatusOK)
	g.Expect(notices(doc)).To(Equal([]string{"info: Not part of this review. Unchanged since the commit round 1 was taken from."}))
	g.Expect(text(find(doc, "toolbar"))).To(Equal("DocumentSource"))
	g.Expect(proseBlocks(find(doc, "prose").Parent)).To(Equal([]string{"kept: Guide", "kept: Part two"}))
	g.Expect(findAll(find(doc, "prose"), func(n *html.Node) bool { return attr(n, "id") == "part-two" })).To(HaveLen(1))
	doc = f.page(g, base+"/files/docs/guide.md?view=source", http.StatusOK)
	g.Expect(tableRows(doc)).To(Equal([]string{"1# Guide", "2", "3## Part two"}))
	write(g, repo, "later.md", "# Later\n")
	doc = f.page(g, base+"/files/later.md", http.StatusOK)
	g.Expect(text(find(doc, "toolbar"))).To(Equal("DocumentSource"))
	g.Expect(proseBlocks(find(doc, "prose").Parent)).To(Equal([]string{"kept: Later"}))

	// A deleted document shows its removed blocks, and Source its removed
	// lines. Still deleted a round on, it has one view and no control.
	doc = f.page(g, base+"/files/docs/gone.md", http.StatusOK)
	g.Expect(text(find(doc, "toolbar"))).To(Equal("+0 −3 in this round DocumentSource"))
	g.Expect(proseBlocks(find(doc, "prose").Parent)).To(Equal([]string{"removed: was: Gone / A passage."}))
	doc = f.page(g, base+"/files/docs/gone.md?view=source", http.StatusOK)
	g.Expect(tableRows(doc)).To(Equal([]string{"del 1−# Gone", "del 2−", "del 3−A passage."}))
	f.decide(g, t, id, store.VerdictChanges, "more")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "third"})
	doc = f.page(g, base+"/files/docs/gone.md", http.StatusOK)
	g.Expect(find(doc, "toolbar")).To(BeNil())
	g.Expect(text(find(doc, "empty"))).To(Equal("DeletedThe agent deleted this file as of round 3."))
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
		links[text(find(file, "path"))] = attr(find(file, "fileopen"), "href")
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

// A file opened for context is read live from the working copy its round came
// from. The main checkout is the project's root, and the file exists only in
// the worktree, so reading the project's root would answer not found.
func TestServer_Surface_fileFromItsWorkingCopy(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	g.Expect(err).NotTo(HaveOccurred())
	main := gitRepo(g, filepath.Join(dir, "harbor"))
	f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: main, Agent: "a"}, Paths: []string{"a.txt"}, Note: "main"})
	worktree := filepath.Join(dir, "harbor-retry")
	git(g, main, "worktree", "add", "--quiet", "-b", "retry", worktree)
	write(g, worktree, "a.txt", "one\nretry\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: worktree, Agent: "a"}, Paths: []string{"a.txt"}, Note: "retry"}).Review.ID
	// Written after the request, so the round did not capture it and the page
	// has to read it live.
	write(g, worktree, "context.txt", "only in the worktree\n")

	doc := f.page(g, "/reviews/"+itoa(id)+"/files/context.txt", http.StatusOK)
	g.Expect(text(doc)).To(ContainSubstring("only in the worktree"))
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
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	g.Expect(os.Symlink("loop", filepath.Join(repo, "loop"))).To(Succeed())
	write(g, repo, "locked/x.txt", "behind a directory that cannot be searched\n")
	locked := filepath.Join(repo, "locked")
	g.Expect(os.Chmod(locked, 0)).To(Succeed())
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	base := "/reviews/" + itoa(id)

	paths := []string{
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
		// The tree leaves out a directory it cannot search, so an address
		// below one names nothing, as a directory nothing holds does.
		base + "/files/locked/x.txt",
		base + "/files/nowhere/",
		base + "/rounds/2/files/a.txt",
		base + "/rounds/x/files/a.txt",
		"/reviews/999/files/a.txt",
		"/reviews/abc/files/a.txt",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			g := NewWithT(t)
			// Root searches a directory whatever its mode, so chmod 0
			// denies nothing and locked/x.txt opens live.
			if strings.Contains(p, "/locked/") && os.Geteuid() == 0 {
				t.Skip("permission bits are not enforced for root")
			}
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

		// A path the disk lacks gets the same banner, since git is asked
		// before the disk is believed.
		paths := []string{"later.txt", "nothing.txt", strings.Repeat("x", 300)}
		for _, p := range paths {
			doc := f.page(g, "/reviews/"+itoa(id)+"/files/"+p, http.StatusNotFound)
			g.Expect(notices(doc)).To(Equal([]string{"warn: Nothing can be read live from disk. Git could not say which files it ignores."}), p)
		}
		doc := f.page(g, "/reviews/"+itoa(id)+"/files/a.txt", http.StatusOK)
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
		unreadable := []string{"warn: Nothing can be read live from disk. Git could not say which files it ignores."}
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

func TestServer_Surface_fileRendered(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	commitFile(g, repo, "icon.png", pngOf(g, 2, 3))
	commitFile(g, repo, "gone.png", pngOf(g, 1, 1))
	commitFile(g, repo, "badge.svg", "<svg xmlns='http://www.w3.org/2000/svg'/>\n")
	commitFile(g, repo, "site/index.html", "<p>before</p>\n")
	commitFile(g, repo, "paper.pdf", "%PDF-1.4 before\n")
	write(g, repo, "icon.png", pngOf(g, 4, 5))
	write(g, repo, "new.png", pngOf(g, 6, 7))
	g.Expect(os.Remove(filepath.Join(repo, "gone.png"))).To(Succeed())
	write(g, repo, "badge.svg", "<svg xmlns='http://www.w3.org/2000/svg'><rect/></svg>\n")
	write(g, repo, "site/index.html", "<p>after</p>\n")
	write(g, repo, "paper.pdf", "%PDF-1.4 after\n")
	write(g, repo, "clip.mp4", "not really a video")
	write(g, repo, "big.png", strings.Repeat("x", kind.MaxSend+1))
	write(g, repo, "data.zip", "PK\x03\x04")
	paths := []string{"icon.png", "new.png", "gone.png", "badge.svg", "site", "paper.pdf", "clip.mp4", "big.png", "data.zip"}
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: paths, Note: "first"}).Review.ID
	base := "/reviews/" + itoa(id) + "/files/"
	raw := "/reviews/" + itoa(id) + "/raw/"

	// sides is each frame of a before-over-after as its caption and source.
	sides := func(doc *html.Node) []string {
		var out []string
		for _, p := range findAll(doc, hasClass("preview")) {
			media := findAll(p, func(n *html.Node) bool { return n.Data == "img" || n.Data == "video" || n.Data == "audio" })
			g.Expect(media).To(HaveLen(1))
			out = append(out, attr(p, "class")+" | "+text(find(p, "cap"))+" | "+media[0].Data+" "+attr(media[0], "src"))
		}
		return out
	}
	frame := func(doc *html.Node) *html.Node {
		frames := findAll(doc, func(n *html.Node) bool { return n.Data == "iframe" })
		if len(frames) == 0 {
			return nil
		}
		g.Expect(frames).To(HaveLen(1))
		return frames[0]
	}

	t.Run("a changed image, before over after", func(t *testing.T) {
		g := NewWithT(t)
		doc := f.page(g, base+"icon.png", http.StatusOK)
		g.Expect(sides(doc)).To(Equal([]string{
			"preview was | Before2 × 3 · " + bytesOf(pngOf(g, 2, 3)) + " | img " + raw + "0/icon.png",
			"preview now | After4 × 5 · " + bytesOf(pngOf(g, 4, 5)) + " | img " + raw + "1/icon.png",
		}))
		g.Expect(find(doc, "code")).To(BeNil())
	})
	t.Run("a new image", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(sides(f.page(g, base+"new.png", http.StatusOK))).To(Equal([]string{"preview now | New6 × 7 · " + bytesOf(pngOf(g, 6, 7)) + " | img " + raw + "1/new.png"}))
	})
	t.Run("a deleted image shows the side that had it", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(sides(f.page(g, base+"gone.png", http.StatusOK))).To(Equal([]string{"preview was | Deleted1 × 1 · " + bytesOf(pngOf(g, 1, 1)) + " | img " + raw + "0/gone.png"}))
	})
	t.Run("a recording plays", func(t *testing.T) {
		g := NewWithT(t)
		doc := f.page(g, base+"clip.mp4", http.StatusOK)
		g.Expect(sides(doc)).To(Equal([]string{"preview now | NewMP4 video · 18 B | video " + raw + "1/clip.mp4"}))
		video := findAll(doc, func(n *html.Node) bool { return n.Data == "video" })[0]
		g.Expect(attr(video, "preload")).To(Equal("metadata"))
		g.Expect(video.Attr).To(ContainElement(html.Attribute{Key: "playsinline"}))
	})
	t.Run("a pdf shows in an unsandboxed frame", func(t *testing.T) {
		g := NewWithT(t)
		doc := f.page(g, base+"paper.pdf", http.StatusOK)
		fr := frame(doc)
		g.Expect(attr(fr, "src")).To(Equal(raw + "1/paper.pdf"))
		g.Expect(fr.Attr).NotTo(ContainElement(HaveField("Key", "sandbox")))
		g.Expect(text(find(doc, "cap"))).To(Equal("PDF document15 BOpen"))
		g.Expect(attr(find(doc, "open"), "href")).To(Equal(raw + "1/paper.pdf"))
	})
	t.Run("a page opens as source", func(t *testing.T) {
		g := NewWithT(t)
		doc := f.page(g, base+"site/index.html", http.StatusOK)
		g.Expect(frame(doc)).To(BeNil())
		g.Expect(find(doc, "code")).NotTo(BeNil())
		g.Expect(segmentLinks(doc)).To(Equal([]string{
			"Page " + base + "site/index.html?view=page",
			"Source " + base + "site/index.html?view=source current",
		}))
	})
	t.Run("a page in its Page view is sandboxed", func(t *testing.T) {
		g := NewWithT(t)
		doc := f.page(g, base+"site/index.html?view=page", http.StatusOK)
		fr := frame(doc)
		g.Expect(attr(fr, "sandbox")).To(Equal("allow-scripts"))
		g.Expect(attr(fr, "src")).To(Equal(raw + "1/site/index.html"))
		g.Expect(find(doc, "code")).To(BeNil())
		g.Expect(text(find(doc, "notice"))).To(ContainSubstring("Runs with no access to this surface."))
		g.Expect(text(find(doc, "toolbar"))).To(ContainSubstring("+1 −1"))
		g.Expect(segmentLinks(doc)).To(Equal([]string{
			"Page " + base + "site/index.html?view=page current",
			"Source " + base + "site/index.html?view=source",
		}))
	})
	t.Run("svg in its Image view", func(t *testing.T) {
		g := NewWithT(t)
		doc := f.page(g, base+"badge.svg?view=image", http.StatusOK)
		g.Expect(sides(doc)).To(Equal([]string{
			"preview was | Before42 B | img " + raw + "0/badge.svg",
			"preview now | After54 B | img " + raw + "1/badge.svg",
		}))
		g.Expect(frame(doc)).To(BeNil(), "an svg never runs in a frame")
		g.Expect(segmentLinks(doc)).To(Equal([]string{
			"Image " + base + "badge.svg?view=image current",
			"Source " + base + "badge.svg?view=source",
		}))
	})
	t.Run("past the send limit", func(t *testing.T) {
		g := NewWithT(t)
		doc := f.page(g, base+"big.png", http.StatusOK)
		g.Expect(text(find(doc, "filefacts"))).To(Equal("PNG image20.0 MB" + pastSendLimit + "Load anyway"))
		g.Expect(findAll(doc, func(n *html.Node) bool { return n.Data == "img" })).To(BeEmpty())
		g.Expect(attr(find(doc, "load"), "href")).To(Equal(base + "big.png?load=1"))

		// Load anyway shows it, asking the raw route for it past the limit.
		doc = f.page(g, base+"big.png?load=1", http.StatusOK)
		g.Expect(sides(doc)).To(Equal([]string{"preview now | New20.0 MB | img " + raw + "1/big.png?load=1"}))
	})
	t.Run("a file with no rendering", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(text(find(f.page(g, base+"data.zip", http.StatusOK), "filefacts"))).To(Equal("ZIP archive4 B"))
	})

	// The agent's choice is the page's default at that round, and the
	// segmented control still offers the other view.
	f.decide(g, t, id, store.VerdictChanges, "more")
	write(g, repo, "site/index.html", "<p>again</p>\n")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second", Views: map[string]string{"site/index.html": "rendered"}})
	doc := f.page(g, base+"site/index.html", http.StatusOK)
	g.Expect(attr(frame(doc), "src")).To(Equal(raw + "2/site/index.html"))
	g.Expect(segmentLinks(doc)).To(Equal([]string{
		"Page " + base + "site/index.html?view=page current",
		"Source " + base + "site/index.html?view=source",
	}))
	g.Expect(frame(f.page(g, base+"site/index.html?view=source", http.StatusOK))).To(BeNil())
	g.Expect(frame(f.page(g, "/reviews/"+itoa(id)+"/rounds/1/files/site/index.html", http.StatusOK))).To(BeNil(), "the first round has no choice")
}

func TestServer_Surface_fileTooLargeOrBinary(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	huge := strings.Repeat("x", diff.MaxBytes) + "\n"
	commitFile(g, repo, "base.bin", "\x00\x02binary in the base")
	commitFile(g, repo, "huge.txt", huge)
	commitFile(g, repo, "shrunk.txt", huge)
	commitFile(g, repo, "base.md", "# Binary\n\x00\x02")
	commitFile(g, repo, "huge.md", huge)
	write(g, repo, "big.txt", strings.Repeat("x\n", 20001))
	write(g, repo, "a.bin", "\x00\x01binary")
	write(g, repo, "shrunk.txt", "small now\n")
	// A document falls back as a code file does, past the line limit too,
	// where each block is a line of its own.
	write(g, repo, "long.md", strings.Repeat("# a\n", 20001))
	write(g, repo, "binary.md", "# Binary\n\x00\x01")
	write(g, repo, "bytes.md", huge)
	paths := []string{"big.txt", "a.bin", "shrunk.txt", "long.md", "binary.md", "bytes.md"}
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: paths, Note: "big"}).Review.ID
	write(g, repo, "live.md", huge)
	write(g, repo, "live-binary.md", "# Live\n\x00\x03")

	pages := []struct {
		path  string
		want  string
		facts string
	}{
		{path: "big.txt", want: "Too large to show as a diff."},
		{path: "a.bin", facts: "Binary file8 B"},
		{path: "base.bin", facts: "Binary file20 B"},
		{path: "huge.txt", want: "Too large to show as a diff."},
		{path: "shrunk.txt", want: "Too large to show as a diff."},
		{path: "long.md", want: "Too large to show as a diff."},
		{path: "binary.md", facts: "Binary file11 B"},
		{path: "bytes.md", want: "Too large to show as a diff."},
		{path: "base.md", facts: "Binary file11 B"},
		{path: "huge.md", want: "Too large to show as a diff."},
		{path: "live.md", want: "Too large to show as a diff."},
		{path: "live-binary.md", facts: "Binary file9 B"},
	}
	for _, c := range pages {
		t.Run(c.path, func(t *testing.T) {
			g := NewWithT(t)
			views := []string{"", "?view=source"}
			for _, view := range views {
				doc := f.page(g, "/reviews/"+itoa(id)+"/files/"+c.path+view, http.StatusOK)
				if c.facts != "" {
					g.Expect(text(find(doc, "filefacts"))).To(Equal(c.facts), view)
				} else {
					g.Expect(text(find(doc, "toobig"))).To(Equal(c.want), view)
				}
				g.Expect(find(doc, "code")).To(BeNil(), view)
				g.Expect(find(doc, "prose")).To(BeNil(), view)
				g.Expect(find(doc, "segmented")).To(BeNil(), view)
			}
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
		{
			page: base + "/rounds/1?against=base",
			want: base + "/rounds/1/files/a.txt?against=base",
		},
	}
	for _, c := range pages {
		t.Run(c.page, func(t *testing.T) {
			g := NewWithT(t)
			link := find(find(f.page(g, c.page, http.StatusOK), "file"), "fileopen")
			g.Expect(attr(link, "aria-label")).To(Equal("Whole file"))
			g.Expect(attr(link, "href")).To(Equal(c.want))
			g.Expect(link.Parent.Data).To(Equal("summary"), "the link sits in the card's header")
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

// documentLinks is the href of every link in a document, in order.
func documentLinks(prose *html.Node) []string {
	var out []string
	for _, a := range findAll(prose, isElement("a")) {
		out = append(out, attr(a, "href"))
	}
	return out
}

// bytesOf is a content's size as a caption writes it.
func bytesOf(content string) string {
	return bytesText(int64(len(content)))
}

// pngOf is a blank PNG of a size, encoded.
func pngOf(g *WithT, w, h int) string {
	g.THelper()
	var b bytes.Buffer
	g.Expect(png.Encode(&b, image.NewGray(image.Rect(0, 0, w, h)))).To(Succeed())
	return b.String()
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
