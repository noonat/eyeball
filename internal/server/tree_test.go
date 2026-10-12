package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/store"
	. "github.com/onsi/gomega"
	"golang.org/x/net/html"
)

func TestServer_Surface_reviewAllFiles(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "more")
	write(g, repo, "a.txt", "one\ntwo\nthree\n")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
	base := "/reviews/" + itoa(id)

	// The row opens the tree's root at the page's round, keeping the page's
	// comparison.
	pages := []struct {
		page string
		name string
		href string
	}{
		{page: base, name: "All files", href: base + "/files/"},
		{page: base + "?against=base", name: "All files", href: base + "/files/?against=base"},
		{page: base + "/rounds/1", name: "All files", href: base + "/rounds/1/files/"},
		{page: base + "/rounds/1?against=base", name: "All files", href: base + "/rounds/1/files/?against=base"},
	}
	for _, c := range pages {
		t.Run(c.page, func(t *testing.T) {
			g := NewWithT(t)
			var all []string
			for _, row := range findAll(f.page(g, c.page, http.StatusOK), hasClass("treerow")) {
				if strings.HasPrefix(text(find(row, "nm")), "All files") {
					all = append(all, text(find(row, "nm"))+" "+attr(row, "href"))
				}
			}
			g.Expect(all).To(Equal([]string{c.name + " " + c.href}))
		})
	}
}

func TestServer_Surface_tree(t *testing.T) {
	t.Run("one level", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		f := newFixture(g, t)
		repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
		committed := map[string]string{
			".gitignore":                   "build/\n",
			":cdir/f.txt":                  "in a directory with a leading colon\n",
			"internal/doc.go":              "package internal\n",
			"internal/migrations/0001.sql": "create table t;\n",
			"internal/outbox/gone.go":      "package outbox\n",
			"internal/storage/store.go":    "package storage\n",
			"internal/upload/old.go":       "package upload\n// old\n",
			"internal/upload/queue.go":     "package upload\n",
			"internal/upload/worker.go":    "package upload\n// v1\n",
		}
		for p, content := range committed {
			commitFile(g, repo, p, content)
		}
		write(g, repo, "internal/upload/worker.go", "package upload\n// v2\n")
		write(g, repo, "internal/upload/backoff.go", "package upload\n\nconst tries = 3\n")
		write(g, repo, "internal/doc.go", "package internal\n// changed outside the review's paths\n")
		write(g, repo, "internal/storage/store.go", "package storage\n// changed outside the review's paths\n")
		g.Expect(os.Remove(filepath.Join(repo, "internal/upload/old.go"))).To(Succeed())
		g.Expect(os.Remove(filepath.Join(repo, "internal/outbox/gone.go"))).To(Succeed())
		me := api.Caller{Dir: repo}
		id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"internal/upload"}, Note: "first"}).Review.ID
		write(g, repo, "build/out", "ignored\n")
		write(g, repo, "later/x.txt", "written after the round\n")
		base := "/reviews/" + itoa(id)

		// One level, directories first, with the subdirectory holding what
		// the review changes expanded in place. Bold is the scope flag: a
		// file captured outside the review's paths is not bold, nor is a file
		// inside them the review left alone, and a deletion the review makes
		// is listed with what it removed.
		frozen := "info: The project as the agent had it. Files this review changes are in bold."
		internal := []string{
			"treerow migrations/ " + base + "/files/internal/migrations/",
			"treerow outbox/ " + base + "/files/internal/outbox/",
			"treerow storage/ " + base + "/files/internal/storage/",
			"treerow upload/ " + base + "/files/internal/upload/",
			"treerow indent inscope backoff.go +3 " + base + "/files/internal/upload/backoff.go",
			"treerow indent inscope old.go −2 " + base + "/files/internal/upload/old.go",
			"treerow indent queue.go " + base + "/files/internal/upload/queue.go",
			"treerow indent inscope worker.go +1 −1 " + base + "/files/internal/upload/worker.go",
			"treerow doc.go " + base + "/files/internal/doc.go",
		}
		addresses := []string{"/files/internal/", "/files/internal"}
		for _, p := range addresses {
			doc := f.page(g, base+p, http.StatusOK)
			g.Expect(barText(doc)).To(Equal("FilesFiles"), p)
			g.Expect(attr(find(doc, "back"), "href")).To(Equal(base+"/files/"), p)
			g.Expect(text(find(doc, "crumb"))).To(Equal("harbor / internal"), p)
			g.Expect(notices(doc)).To(Equal([]string{frozen}), p)
			g.Expect(treeRows(doc)).To(Equal(internal), p)
		}

		// The frozen notice has no heading, as the tearout's has none. A
		// subdirectory holding only changes outside the review's paths is
		// not expanded.
		doc := f.page(g, base+"/files/internal/", http.StatusOK)
		g.Expect(findAll(find(doc, "notice"), func(n *html.Node) bool { return n.Data == "strong" })).To(BeEmpty())

		// The root goes back to the round, and its crumb is the project. The
		// mux sends the address without its slash there.
		res := serveSurface(f.s.Surface(), http.MethodGet, base+"/files")
		g.Expect(res.Code).To(Equal(http.StatusTemporaryRedirect))
		g.Expect(res.Header().Get("Location")).To(Equal(base + "/files/"))
		doc = f.page(g, base+"/files/", http.StatusOK)
		g.Expect(barText(doc)).To(Equal("ReviewFiles"))
		g.Expect(attr(find(doc, "back"), "href")).To(Equal(base))
		g.Expect(text(find(doc, "crumb"))).To(Equal("harbor"))
		g.Expect(find(find(doc, "crumb"), "dir")).To(BeNil())
		g.Expect(treeRows(doc)).To(Equal([]string{
			"treerow :cdir/ " + base + "/files/:cdir/",
			"treerow internal/ " + base + "/files/internal/",
			"treerow indent migrations/ " + base + "/files/internal/migrations/",
			"treerow indent outbox/ " + base + "/files/internal/outbox/",
			"treerow indent storage/ " + base + "/files/internal/storage/",
			"treerow indent upload/ " + base + "/files/internal/upload/",
			"treerow indent doc.go " + base + "/files/internal/doc.go",
			"treerow .gitignore " + base + "/files/.gitignore",
			"treerow a.txt " + base + "/files/a.txt",
			"treerow keep.txt " + base + "/files/keep.txt",
		}))

		// A directory deeper down goes back to its parent, by name, and a
		// leading colon is not read as pathspec magic.
		doc = f.page(g, base+"/files/internal/upload/", http.StatusOK)
		g.Expect(text(find(doc, "back"))).To(Equal("internal"))
		g.Expect(attr(find(doc, "back"), "href")).To(Equal(base + "/files/internal/"))
		g.Expect(text(find(doc, "crumb"))).To(Equal("harbor / internal/upload"))
		doc = f.page(g, base+"/files/:cdir/", http.StatusOK)
		g.Expect(treeRows(doc)).To(Equal([]string{"treerow f.txt " + base + "/files/:cdir/f.txt"}))

		// Every deletion in outbox is outside the review's paths, so it lists
		// in its parent and opens on the empty box. So does a directory made
		// after the round, which nothing captured.
		emptied := []string{"/files/internal/outbox/", "/files/later/"}
		for _, p := range emptied {
			doc = f.page(g, base+p, http.StatusOK)
			g.Expect(notices(doc)).To(Equal([]string{frozen}), p)
			g.Expect(find(doc, "tree")).To(BeNil(), p)
			g.Expect(text(find(doc, "empty"))).To(Equal("Nothing hereNothing is here as of round 1."), p)
		}

		// An ignored directory and a path that names nothing are the 404
		// page.
		missing := []string{"/files/build/", "/files/build", "/files/nowhere/"}
		for _, p := range missing {
			doc = f.page(g, base+p, http.StatusNotFound)
			g.Expect(text(find(doc, "empty"))).To(ContainSubstring("There is no page at this address."), p)
			g.Expect(notices(doc)).To(BeEmpty(), p)
		}

		// Every link keeps the comparison, and the counts are the whole
		// review's either way.
		doc = f.page(g, base+"/files/internal/?against=base", http.StatusOK)
		g.Expect(attr(find(doc, "back"), "href")).To(Equal(base + "/files/?against=base"))
		rows := treeRows(doc)
		g.Expect(rows[0]).To(Equal("treerow migrations/ " + base + "/files/internal/migrations/?against=base"))
		g.Expect(rows[7]).To(Equal("treerow indent inscope worker.go +1 −1 " + base + "/files/internal/upload/worker.go?against=base"))
		// A context file goes back to its directory, keeping it too.
		doc = f.page(g, base+"/files/internal/upload/queue.go?against=base", http.StatusOK)
		g.Expect(text(find(doc, "back"))).To(Equal("upload"))
		g.Expect(attr(find(doc, "back"), "href")).To(Equal(base + "/files/internal/upload/?against=base"))

		// A second round adds a file. The tree at round 1 is round 1's own
		// capture, and stays on that round. The current tree counts the whole
		// review, so a file changed in round 1 and left alone since keeps its
		// counts.
		f.decide(g, t, id, store.VerdictChanges, "more")
		write(g, repo, "internal/upload/retry.go", "package upload\n")
		f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
		doc = f.page(g, base+"/files/internal/upload/", http.StatusOK)
		g.Expect(treeRows(doc)).To(Equal([]string{
			"treerow inscope backoff.go +3 " + base + "/files/internal/upload/backoff.go",
			"treerow inscope old.go −2 " + base + "/files/internal/upload/old.go",
			"treerow queue.go " + base + "/files/internal/upload/queue.go",
			"treerow inscope retry.go +1 " + base + "/files/internal/upload/retry.go",
			"treerow inscope worker.go +1 −1 " + base + "/files/internal/upload/worker.go",
		}))
		doc = f.page(g, base+"/rounds/1/files/internal/upload/", http.StatusOK)
		g.Expect(attr(find(doc, "back"), "href")).To(Equal(base + "/rounds/1/files/internal/"))
		g.Expect(treeRows(doc)).To(Equal([]string{
			"treerow inscope backoff.go +3 " + base + "/rounds/1/files/internal/upload/backoff.go",
			"treerow inscope old.go −2 " + base + "/rounds/1/files/internal/upload/old.go",
			"treerow queue.go " + base + "/rounds/1/files/internal/upload/queue.go",
			"treerow inscope worker.go +1 −1 " + base + "/rounds/1/files/internal/upload/worker.go",
		}))
		doc = f.page(g, base+"/rounds/1/files/", http.StatusOK)
		g.Expect(text(find(doc, "back"))).To(Equal("Round 1"))
		g.Expect(attr(find(doc, "back"), "href")).To(Equal(base + "/rounds/1"))
		doc = f.page(g, base+"/rounds/2/files/", http.StatusOK)
		g.Expect(text(find(doc, "back"))).To(Equal("Review"))
	})

	t.Run("outside git", func(t *testing.T) {
		g := NewWithT(t)
		f := newFixture(g, t)
		root := filepath.Join(t.TempDir(), "notes")
		write(g, root, ".eyeball", "")
		write(g, root, "plan.md", "the plan\n")
		id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: root}, Paths: []string{"plan.md"}, Note: "first"}).Review.ID
		write(g, root, "later.md", "written later\n")
		base := "/reviews/" + itoa(id)

		// The disk is listed as it is now, beside the capture, and the notice
		// says so.
		doc := f.page(g, base+"/files/", http.StatusOK)
		g.Expect(notices(doc)).To(Equal([]string{"warn: Live, not from this round. Files nothing captured are listed as they are on disk now, and may have moved. Files this review changes are in bold."}))
		g.Expect(treeRows(doc)).To(Equal([]string{
			"treerow .eyeball " + base + "/files/.eyeball",
			"treerow later.md " + base + "/files/later.md",
			"treerow inscope plan.md +1 " + base + "/files/plan.md",
		}))
		// A path the disk lacks is the plain 404 page. Outside git there are
		// no ignore rules to ask about.
		doc = f.page(g, base+"/files/nowhere/", http.StatusNotFound)
		g.Expect(notices(doc)).To(BeEmpty())
	})

	t.Run("a gone base", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		f := newFixture(g, t)
		repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
		commitFile(g, repo, "d/x.txt", "deleted outside the review's paths\n")
		g.Expect(os.RemoveAll(filepath.Join(repo, "d"))).To(Succeed())
		id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
		dropBase(g, repo)
		base := "/reviews/" + itoa(id)
		gone := "warn: Only the files this round captured are listed. The commit round 1 was taken from is gone, so any other file opens only by its address, as it is on disk now. Files this review changes are in bold."

		// The capture alone, and nothing to count against.
		doc := f.page(g, base+"/files/", http.StatusOK)
		g.Expect(notices(doc)).To(Equal([]string{gone}))
		g.Expect(treeRows(doc)).To(Equal([]string{
			"treerow d/ " + base + "/files/d/",
			"treerow inscope a.txt " + base + "/files/a.txt",
		}))
		// A directory holding only a deletion is a directory to the address
		// as it is to the listing, with no base to ask.
		doc = f.page(g, base+"/files/d/", http.StatusOK)
		g.Expect(notices(doc)).To(Equal([]string{gone}))
		g.Expect(text(find(doc, "empty"))).To(Equal("Nothing hereNothing is here as of round 1."))
	})

	t.Run("an unborn branch", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		f := newFixture(g, t)
		repo := filepath.Join(t.TempDir(), "harbor")
		g.Expect(os.MkdirAll(repo, 0o700)).To(Succeed())
		git(g, repo, "init", "--quiet", "--template=", "-b", "main")
		write(g, repo, "a.txt", "one\n")
		write(g, repo, "lib/b.txt", "two\n")
		id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
		base := "/reviews/" + itoa(id)

		// With no commit, every file was captured, so the capture is the
		// project as the agent had it.
		doc := f.page(g, base+"/files/", http.StatusOK)
		g.Expect(notices(doc)).To(Equal([]string{"info: The project as the agent had it. Files this review changes are in bold."}))
		g.Expect(treeRows(doc)).To(Equal([]string{
			"treerow lib/ " + base + "/files/lib/",
			"treerow inscope a.txt +1 " + base + "/files/a.txt",
		}))
	})

	t.Run("a lost repository", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		f := newFixture(g, t)
		dir := t.TempDir()
		repo := gitRepo(g, filepath.Join(dir, "harbor"))
		write(g, repo, "other.txt", "context\n")
		write(g, repo, "fresh/new.txt", "a directory the capture holds\n")
		id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
		g.Expect(os.Rename(filepath.Join(repo, ".git"), filepath.Join(dir, "moved.git"))).To(Succeed())
		base := "/reviews/" + itoa(id)
		lost := "warn: Only what this round captured can be shown. The project's repository is gone from its root. Files this review changes are in bold."

		// The capture alone, with one banner, and nothing to count against.
		doc := f.page(g, base+"/files/", http.StatusOK)
		g.Expect(notices(doc)).To(Equal([]string{lost}))
		g.Expect(treeRows(doc)).To(Equal([]string{
			"treerow fresh/ " + base + "/files/fresh/",
			"treerow inscope a.txt " + base + "/files/a.txt",
			"treerow other.txt " + base + "/files/other.txt",
		}))
		doc = f.page(g, base+"/files/fresh/", http.StatusOK)
		g.Expect(notices(doc)).To(Equal([]string{lost}))
		g.Expect(treeRows(doc)).To(Equal([]string{"treerow new.txt " + base + "/files/fresh/new.txt"}))
	})

	t.Run("a marker project the checkout ignores", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		f := newFixture(g, t)
		outer, _ := markerInCheckout(g, t.TempDir())
		id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: filepath.Join(outer, "ign")}, Paths: []string{"plan.md"}, Note: "first"}).Review.ID
		base := "/reviews/" + itoa(id)

		doc := f.page(g, base+"/files/", http.StatusOK)
		g.Expect(notices(doc)).To(Equal([]string{"warn: Only what this round captured can be shown. The checkout around the project ignores it. Files this review changes are in bold."}))
		g.Expect(treeRows(doc)).To(Equal([]string{"treerow inscope plan.md +1 " + base + "/files/plan.md"}))
	})

	t.Run("a marker project whose checkout lost its .git", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		f := newFixture(g, t)
		dir := t.TempDir()
		outer, notes := markerInCheckout(g, dir)
		id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: notes}, Paths: []string{"plan.md"}, Note: "first"}).Review.ID
		g.Expect(os.Rename(filepath.Join(outer, ".git"), filepath.Join(dir, "moved.git"))).To(Succeed())
		base := "/reviews/" + itoa(id)

		doc := f.page(g, base+"/files/", http.StatusOK)
		g.Expect(notices(doc)).To(Equal([]string{"warn: Only what this round captured can be shown. The checkout the project sat inside has lost its repository. Files this review changes are in bold."}))
		g.Expect(treeRows(doc)).To(Equal([]string{"treerow inscope plan.md +1 " + base + "/files/plan.md"}))
	})

	t.Run("a marker project's failed ignore batch", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		f := newFixture(g, t)
		dir := t.TempDir()
		inner := gitRepo(g, filepath.Join(dir, "inner"))
		git(g, inner, "add", "-A")
		git(g, inner, "commit", "--quiet", "--no-verify", "-m", "inner")
		outer, notes := markerInCheckout(g, dir)
		git(g, outer, "-c", "protocol.file.allow=always", "submodule", "add", "--quiet", inner, "notes/mod")
		git(g, outer, "commit", "--quiet", "--no-verify", "-m", "add the submodule")
		git(g, outer, "submodule", "deinit", "--quiet", "--force", "notes/mod")
		write(g, outer, "notes/mod/m.go", "written into a submodule with no checkout\n")
		id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: notes}, Paths: []string{"plan.md"}, Note: "first"}).Review.ID
		base := "/reviews/" + itoa(id)

		// The project's own check passes, so the root lists from disk, but
		// git exits 128 on the batch for the submodule's directory. The
		// entries are never shown unchecked: the capture alone is listed, and
		// the banner says git could not answer.
		doc := f.page(g, base+"/files/", http.StatusOK)
		g.Expect(notices(doc)[0]).To(HavePrefix("warn: Live, not from this round."))
		g.Expect(treeRows(doc)).To(ContainElement("treerow mod/ " + base + "/files/mod/"))
		doc = f.page(g, base+"/files/mod/", http.StatusOK)
		g.Expect(notices(doc)).To(Equal([]string{"warn: Nothing can be read live from disk. Git could not say which files it ignores. Files this review changes are in bold."}))
		g.Expect(find(doc, "tree")).To(BeNil())
		g.Expect(text(find(doc, "empty"))).To(Equal("Nothing hereNothing is here as of round 1."))
	})
}

func TestServer_Surface_treeCapped(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	for i := range 1001 {
		write(g, repo, fmt.Sprintf("full/%04d.txt", i), "x\n")
		write(g, repo, fmt.Sprintf("over/%04d.txt", i), "x\n")
	}
	g.Expect(os.Remove(filepath.Join(repo, "full", "1000.txt"))).To(Succeed())
	git(g, repo, "add", "-A")
	tree := git(g, repo, "write-tree")
	commit := git(g, repo, "commit-tree", tree, "-p", "main", "-m", "many files")
	git(g, repo, "update-ref", "refs/heads/main", commit)
	write(g, repo, "a.txt", "one\ntwo\nthree\n")
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	base := "/reviews/" + itoa(id)

	// A thousand entries are all listed. One more is past the cap, which a
	// notice counts.
	doc := f.page(g, base+"/files/full/", http.StatusOK)
	g.Expect(treeRows(doc)).To(HaveLen(1000))
	g.Expect(notices(doc)).To(HaveLen(1))
	doc = f.page(g, base+"/files/over/", http.StatusOK)
	rows := treeRows(doc)
	g.Expect(rows).To(HaveLen(1000))
	g.Expect(rows[999]).To(HavePrefix("treerow 0999.txt "))
	g.Expect(notices(doc)).To(Equal([]string{
		"info: The project as the agent had it. Files this review changes are in bold.",
		"info: The first 1,000 of 1,001 entries are listed.",
	}))
}

// treeRows is each row of a tree page, as its class, its name with a slash
// for a directory, its counts, and its address.
func treeRows(doc *html.Node) []string {
	var out []string
	for _, row := range findAll(doc, hasClass("treerow")) {
		name := text(find(row, "nm"))
		if find(row, "i-folder") != nil {
			name += "/"
		}
		fields := []string{attr(row, "class"), name}
		if counts := text(find(row, "counts")); counts != "" {
			fields = append(fields, counts)
		}
		out = append(out, strings.Join(append(fields, attr(row, "href")), " "))
	}
	return out
}
