package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/diff"
	"github.com/noonat/eyeball/internal/store"
	. "github.com/onsi/gomega"
	"golang.org/x/net/html"
)

func TestServer_Surface_project(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	f := newFixture(g, t)
	h := newHarbor(g, t, f)

	doc := f.page(g, projectHref(h.project), http.StatusOK)
	g.Expect(barText(doc)).To(Equal("Queueharbor"))
	g.Expect(attr(find(doc, "back"), "href")).To(Equal("/"))
	g.Expect(text(find(find(doc, "brief"), "agent"))).To(Equal(h.root), "the root, written with ~ where it is under home")
	g.Expect(text(find(find(doc, "brief"), "title"))).To(Equal("harbor"))
	g.Expect(notices(find(doc, "brief").Parent)).To(Equal([]string{"info: Everything changed on disk right now, live. Read only: a verdict belongs to a review."}))
	g.Expect(headings(doc)).To(Equal([]string{"Open reviews 4", "Working copy 10 files"}))

	// The working copy's rows, in path order. A file an open review covers is
	// marked and goes to that review's file page. Any other is bold with its
	// counts against HEAD, and goes to the project file page. The approved
	// review's file is unclaimed. The ignored file is not listed, and neither
	// is the file inside a directory that has become a repository.
	files := projectFileHref(h.project, 0, "")
	g.Expect(workingRows(doc)).To(Equal([]string{
		"bold a.txt +1 " + files + "a.txt",
		"bold done/x.txt +1 −1 " + files + "done/x.txt",
		"bold gone.txt −2 " + files + "gone.txt",
		"plain k1/x.txt In review /reviews/" + itoa(h.k1) + "/files/k1/x.txt",
		"plain k2/x.txt In review /reviews/" + itoa(h.k2) + "/files/k2/x.txt",
		"bold link +1 −1 " + files + "link",
		"bold notes/new.md +2 " + files + "notes/new.md",
		"bold q#1.txt +1 " + files + "q%231.txt",
		"plain w1/x.txt In review /reviews/" + itoa(h.w1) + "/files/w1/x.txt",
		"plain w2/x.txt In review /reviews/" + itoa(h.w2) + "/files/w2/x.txt",
	}))
	// A claimed row's link lands on its review's file page.
	f.page(g, "/reviews/"+itoa(h.k1)+"/files/k1/x.txt", http.StatusOK)
}

func TestServer_Surface_projectBlobsUntouched(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	root, err := filepath.EvalSymlinks(repo)
	g.Expect(err).NotTo(HaveOccurred())
	// Opened through the store, so nothing has frozen a round and the blob
	// store starts empty.
	project, err := f.store.EnsureProject(t.Context(), root, "harbor")
	g.Expect(err).NotTo(HaveOccurred())

	doc := f.page(g, projectHref(project.ID), http.StatusOK)
	g.Expect(workingRows(doc)).To(Equal([]string{"bold a.txt +1 " + projectFileHref(project.ID, 0, "a.txt")}))
	f.page(g, projectFileHref(project.ID, 0, "a.txt"), http.StatusOK)
	f.page(g, projectFileHref(project.ID, 0, "a.txt")+"?view=plain", http.StatusOK)

	var stored []string
	err = filepath.WalkDir(filepath.Join(f.state, "blobs"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			stored = append(stored, p)
		}
		return err
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(stored).To(BeEmpty())
}

func TestServer_Surface_projectCapped(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	for i := range 204 {
		write(g, repo, fmt.Sprintf("new/%03d.txt", i), "new\n")
	}
	root, err := filepath.EvalSymlinks(repo)
	g.Expect(err).NotTo(HaveOccurred())
	project, err := f.store.EnsureProject(t.Context(), root, "harbor")
	g.Expect(err).NotTo(HaveOccurred())

	doc := f.page(g, projectHref(project.ID), http.StatusOK)
	g.Expect(headings(doc)).To(Equal([]string{"Working copy 205 files"}))
	rows := workingRows(doc)
	g.Expect(rows).To(HaveLen(maxWorkingRows))
	g.Expect(rows[0]).To(Equal("bold a.txt +1 " + projectFileHref(project.ID, 0, "a.txt")))
	g.Expect(rows[maxWorkingRows-1]).To(Equal("bold new/198.txt +1 " + projectFileHref(project.ID, 0, "new/198.txt")))
	g.Expect(notices(find(doc, "section"))).To(Equal([]string{
		"info: Everything changed on disk right now, live. Read only: a verdict belongs to a review.",
		"info: The first 200 of 205 files are listed.",
	}))

	// The cap limits the rows that are read, not what the listing holds.
	page := f.page(g, projectFileHref(project.ID, 0, "new/203.txt"), http.StatusOK)
	g.Expect(text(find(page, "toolbar"))).To(Equal("+1 −0 on disk MarkedPlain"))
}

func TestServer_Surface_projectFile(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	f := newFixture(g, t)
	h := newHarbor(g, t, f)
	href := projectFileHref(h.project, 0, "a.txt")

	doc := f.page(g, href, http.StatusOK)
	g.Expect(barText(doc)).To(Equal("harbora.txt"))
	g.Expect(attr(find(doc, "back"), "href")).To(Equal(projectHref(h.project)))
	g.Expect(find(find(doc, "crumb"), "dir")).To(BeNil())
	g.Expect(text(find(find(doc, "crumb"), "name"))).To(Equal("a.txt"))
	g.Expect(notices(doc)).To(Equal([]string{"warn: Live, not from any round. The file as it is on disk now, compared with HEAD. It moves while an agent works."}))
	g.Expect(text(find(doc, "toolbar"))).To(Equal("+1 −0 on disk MarkedPlain"))
	g.Expect(segmentLinks(doc)).To(Equal([]string{
		"Marked " + href + "?view=marked current",
		"Plain " + href + "?view=plain",
	}))
	g.Expect(tableRows(doc)).To(Equal([]string{"1one", "add 2+two"}))

	doc = f.page(g, href+"?view=plain", http.StatusOK)
	g.Expect(segmentLinks(doc)).To(Equal([]string{
		"Marked " + href + "?view=marked",
		"Plain " + href + "?view=plain current",
	}))
	g.Expect(tableRows(doc)).To(Equal([]string{"1one", "2two"}))

	// The path is cleaned before it is looked up, as on a review's file
	// page, and an escaped name reaches its file.
	doc = f.page(g, href+"/", http.StatusOK)
	g.Expect(text(find(find(doc, "crumb"), "name"))).To(Equal("a.txt"))
	doc = f.page(g, projectFileHref(h.project, 0, "q#1.txt"), http.StatusOK)
	g.Expect(text(find(find(doc, "crumb"), "name"))).To(Equal("q#1.txt"))

	// A markdown file answers what moved on disk, so it gets Marked and
	// Plain, and is never rendered as a document.
	md := projectFileHref(h.project, 0, "notes/new.md")
	doc = f.page(g, md, http.StatusOK)
	g.Expect(text(find(find(doc, "crumb"), "dir"))).To(Equal("notes/"))
	g.Expect(find(doc, "prose")).To(BeNil())
	g.Expect(segmentLinks(doc)).To(Equal([]string{
		"Marked " + md + "?view=marked current",
		"Plain " + md + "?view=plain",
	}))
	g.Expect(tableRows(doc)).To(Equal([]string{"add 1+# New", "add 2+A paragraph."}))
	doc = f.page(g, md+"?view=source", http.StatusOK)
	g.Expect(find(doc, "prose")).To(BeNil())

	// A file gone from disk shows its removed lines, and the deleted box in
	// Plain, where there is nothing on disk to show.
	gone := projectFileHref(h.project, 0, "gone.txt")
	doc = f.page(g, gone, http.StatusOK)
	g.Expect(tableRows(doc)).To(Equal([]string{"del 1−removed", "del 2−from disk"}))
	doc = f.page(g, gone+"?view=plain", http.StatusOK)
	g.Expect(text(find(doc, "empty"))).To(Equal("DeletedThis file is gone from disk."))
	g.Expect(find(doc, "file")).To(BeNil())

	// A changed link compares its target text.
	doc = f.page(g, projectFileHref(h.project, 0, "link"), http.StatusOK)
	g.Expect(tableRows(doc)).To(Equal([]string{"del 1−old-target", "add 1+new-target"}))
}

func TestServer_Surface_projectFileCountsMatchTheRow(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	f := newFixture(g, t)
	h := newHarbor(g, t, f)

	// The row's counts and the page's are one comparison, so they agree for
	// every unclaimed file.
	doc := f.page(g, projectHref(h.project), http.StatusOK)
	rows := findAll(find(doc, "tree"), hasClass("inscope"))
	g.Expect(rows).To(HaveLen(6))
	for _, row := range rows {
		page := f.page(g, attr(row, "href"), http.StatusOK)
		counts := strings.TrimSuffix(text(find(find(page, "toolbar"), "counts")), " on disk")
		g.Expect(spelled(counts)).To(Equal(text(find(row, "counts"))), text(find(row, "nm")))
	}
}

func TestServer_Surface_projectFileNotFound(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	f := newFixture(g, t)
	h := newHarbor(g, t, f)
	files := projectFileHref(h.project, 0, "")

	// Only a path the working copy lists answers. An unchanged tracked file,
	// an ignored one and one the live read refuses are not listed.
	paths := []string{
		files + "keep.txt",
		files + "q%231.txt.missing",
		files + "private.txt",
		files + "vendor/x/a.txt",
		files + "nothing.txt",
		files,
		files + ".git/config",
		files + "a.txt%00",
		files + "%2e%2e/%2e%2e/etc/passwd",
		"/projects/999/files/a.txt",
		"/projects/abc/files/a.txt",
		"/projects/999",
		"/projects/abc",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			g := NewWithT(t)
			doc := f.page(g, p, http.StatusNotFound)
			g.Expect(text(find(doc, "empty"))).To(ContainSubstring("There is no page at this address."))
		})
	}
}

func TestServer_Surface_projectFileTooLarge(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "big.txt", strings.Repeat("x", diff.MaxBytes+1))
	write(g, repo, "bin.dat", "a\x00b")
	root, err := filepath.EvalSymlinks(repo)
	g.Expect(err).NotTo(HaveOccurred())
	project, err := f.store.EnsureProject(t.Context(), root, "harbor")
	g.Expect(err).NotTo(HaveOccurred())

	doc := f.page(g, projectFileHref(project.ID, 0, "big.txt"), http.StatusOK)
	g.Expect(text(find(doc, "toobig"))).To(Equal("Too large to show as a diff."))
	g.Expect(find(doc, "toolbar")).To(BeNil())
	doc = f.page(g, projectFileHref(project.ID, 0, "bin.dat"), http.StatusOK)
	g.Expect(text(find(doc, "toobig"))).To(Equal("Binary file, not shown as a diff."))
	g.Expect(find(doc, "toolbar")).To(BeNil())

	// The rows carry no counts for either.
	doc = f.page(g, projectHref(project.ID), http.StatusOK)
	g.Expect(workingRows(doc)).To(Equal([]string{
		"bold a.txt +1 " + projectFileHref(project.ID, 0, "a.txt"),
		"bold big.txt " + projectFileHref(project.ID, 0, "big.txt"),
		"bold bin.dat " + projectFileHref(project.ID, 0, "bin.dat"),
	}))
}

func TestServer_Surface_projectFromAReview(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	f := newFixture(g, t)
	dir := t.TempDir()
	outer, notes := markerInCheckout(g, dir)
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: notes}, Paths: []string{"plan.md"}, Note: "first"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "more")
	g.Expect(os.Rename(filepath.Join(outer, ".git"), filepath.Join(dir, "moved.git"))).To(Succeed())

	// The review's header links to the project view, which reads the project
	// by its id, flag included, so it still refuses after the .git moved.
	review := f.page(g, "/reviews/"+itoa(id), http.StatusOK)
	href := attr(find(find(review, "who"), "bit"), "href")
	g.Expect(href).To(Equal(projectHref(1)))
	doc := f.page(g, href, http.StatusOK)
	g.Expect(notices(doc)).To(Equal([]string{"warn: Live browsing is unavailable. The checkout the project sat inside has lost its repository."}))
	g.Expect(text(find(doc, "empty"))).To(Equal("Cannot be listedNothing on disk is read for this project. Reviews still work, and the files their rounds captured still open."))
}

func TestServer_Surface_projectMarksCheckout(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	outer := gitRepo(g, filepath.Join(t.TempDir(), "outer"))
	write(g, outer, "notes/.eyeball", "")
	root, err := filepath.EvalSymlinks(filepath.Join(outer, "notes"))
	g.Expect(err).NotTo(HaveOccurred())
	// Opened through the store, as tool/seed opens one, so the request path
	// that also writes the flag never runs.
	project, err := f.store.EnsureProject(t.Context(), root, "notes")
	g.Expect(err).NotTo(HaveOccurred())
	before, err := f.store.Project(t.Context(), project.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(before.InsideCheckout).To(BeFalse())

	f.page(g, projectHref(project.ID), http.StatusOK)
	after, err := f.store.Project(t.Context(), project.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(after.InsideCheckout).To(BeTrue())
}

func TestServer_Surface_projectOpenReviews(t *testing.T) {
	g := NewWithT(t)
	q := newQueueFixture(g, t)
	harbor := q.project(g, t, "/home/a/src/harbor", "harbor")
	other := q.project(g, t, "/home/a/src/eyeball", "eyeball")

	// Each group's order differs from the store's newest request first, and
	// from the review ids, so only sorting puts them right.
	k1 := q.open(g, t, harbor, "agent-k1", "k1", "k1")
	q.advance(time.Minute)
	k2 := q.open(g, t, harbor, "agent-k2", "k2", "k2")
	q.advance(time.Minute)
	w1 := q.open(g, t, harbor, "agent-w1", "w1", "w1")
	q.advance(time.Minute)
	w2 := q.open(g, t, harbor, "agent-w2", "w2", "w2")
	q.advance(time.Minute)
	done := q.open(g, t, harbor, "agent-done", "done", "done")
	q.open(g, t, other, "agent-x", "elsewhere", "x")
	q.advance(time.Minute)
	q.decide(g, t, k2, store.VerdictChanges)
	q.advance(10 * time.Minute)
	q.decide(g, t, k1, store.VerdictChanges)
	q.decide(g, t, done, store.VerdictApprove)
	q.advance(5 * time.Minute)

	// Waiting before working, the longest wait first and then the latest
	// verdict first, as the queue orders each. A working row's age runs from
	// its verdict. The project is left out of each row, since the page is the
	// project, and an approved review is not open.
	res := serveSurface(q.s.Surface(), http.MethodGet, projectHref(harbor))
	g.Expect(res.Code).To(Equal(http.StatusOK))
	doc := parse(g, res.Body.String())
	g.Expect(headings(doc)[0]).To(Equal("Open reviews 4"))
	type shape struct {
		Class, Href, Agent, Age, Title, Meta string
		Project                              bool
	}
	var got []shape
	for _, r := range findAll(doc, hasClass("row")) {
		got = append(got, shape{
			Class:   attr(r, "class"),
			Href:    attr(r, "href"),
			Agent:   text(find(r, "agent")),
			Age:     text(find(r, "age")),
			Title:   text(find(r, "title")),
			Meta:    text(find(r, "meta")),
			Project: find(r, "project") != nil,
		})
	}
	g.Expect(got).To(Equal([]shape{
		{
			Class: "row waiting",
			Href:  "/reviews/" + itoa(w1),
			Agent: "agent-w1",
			Age:   "18m",
			Title: "w1",
			Meta:  "Round 11 file+1 −0",
		},
		{
			Class: "row waiting",
			Href:  "/reviews/" + itoa(w2),
			Agent: "agent-w2",
			Age:   "17m",
			Title: "w2",
			Meta:  "Round 11 file+1 −0",
		},
		{
			Class: "row working",
			Href:  "/reviews/" + itoa(k1),
			Agent: "agent-k1",
			Age:   "5m",
			Title: "k1",
			Meta:  "Changes requested, round 1",
		},
		{
			Class: "row working",
			Href:  "/reviews/" + itoa(k2),
			Agent: "agent-k2",
			Age:   "15m",
			Title: "k2",
			Meta:  "Changes requested, round 1",
		},
	}))
}

func TestServer_Surface_projectStates(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	cannot := "Cannot be listedWhat changed on disk is a question only git answers. Reviews still work; this view does not."
	refused := "Cannot be listedNothing on disk is read for this project. Reviews still work, and the files their rounds captured still open."
	cases := []struct {
		name    string
		setup   func(g *WithT, t *testing.T, f *fixture, dir string) int64
		notices []string
		empty   string
	}{
		{
			name: "a clean tree with no open review",
			setup: func(g *WithT, t *testing.T, f *fixture, dir string) int64 {
				return storeProject(g, t, f, cleanRepo(g, dir), "harbor")
			},
			empty: "Nothing changed on diskEverything is committed.",
		},
		{
			name: "a clean tree with one open review",
			setup: func(g *WithT, t *testing.T, f *fixture, dir string) int64 {
				project := storeProject(g, t, f, cleanRepo(g, dir), "harbor")
				storeReview(g, t, f, project, "one")
				return project
			},
			empty: "Nothing changed on diskEverything the open review covers is committed.",
		},
		{
			name: "a clean tree with two open reviews",
			setup: func(g *WithT, t *testing.T, f *fixture, dir string) int64 {
				project := storeProject(g, t, f, cleanRepo(g, dir), "harbor")
				storeReview(g, t, f, project, "one")
				storeReview(g, t, f, project, "two")
				return project
			},
			empty: "Nothing changed on diskEverything the open reviews cover is committed.",
		},
		{
			name: "a project outside git",
			setup: func(g *WithT, t *testing.T, f *fixture, dir string) int64 {
				write(g, dir, "scratch/.eyeball", "")
				write(g, dir, "scratch/a.txt", "a\n")
				return storeProject(g, t, f, filepath.Join(dir, "scratch"), "scratch")
			},
			notices: []string{"warn: No git repository here. A round has nothing to diff against, so a first request will show whole files."},
			empty:   cannot,
		},
		{
			name: "a marker project inside a checkout that does not ignore it",
			setup: func(g *WithT, t *testing.T, f *fixture, dir string) int64 {
				_, notes := markerInCheckout(g, dir)
				return storeProject(g, t, f, notes, "notes")
			},
			notices: []string{"warn: No repository of its own. A marker file names this project inside another checkout. A round has nothing to diff against, so a first request will show whole files."},
			empty:   cannot,
		},
		{
			name: "a missing root",
			setup: func(g *WithT, t *testing.T, f *fixture, dir string) int64 {
				return storeProject(g, t, f, filepath.Join(dir, "gone"), "gone")
			},
			empty: "Cannot be listedThe project's root, " + "{dir}/gone" + ", no longer exists.",
		},
		{
			name: "a lost repository",
			setup: func(g *WithT, t *testing.T, f *fixture, dir string) int64 {
				repo := cleanRepo(g, dir)
				g.Expect(os.Rename(filepath.Join(repo, ".git"), filepath.Join(dir, "moved.git"))).To(Succeed())
				return storeProject(g, t, f, repo, "harbor")
			},
			empty: "Cannot be listedThe project's repository is gone: its root no longer holds the .git it had.",
		},
		{
			name: "a marker project the checkout ignores",
			setup: func(g *WithT, t *testing.T, f *fixture, dir string) int64 {
				outer, _ := markerInCheckout(g, dir)
				return storeProject(g, t, f, filepath.Join(outer, "ign"), "ign")
			},
			notices: []string{"warn: Live browsing is unavailable. The checkout around the project ignores it."},
			empty:   refused,
		},
		{
			name: "a marker project whose checkout lost its .git",
			setup: func(g *WithT, t *testing.T, f *fixture, dir string) int64 {
				outer, notes := markerInCheckout(g, dir)
				project := storeProject(g, t, f, notes, "notes")
				g.Expect(f.store.MarkInsideCheckout(t.Context(), project)).To(Succeed())
				g.Expect(os.Rename(filepath.Join(outer, ".git"), filepath.Join(dir, "moved.git"))).To(Succeed())
				return project
			},
			notices: []string{"warn: Live browsing is unavailable. The checkout the project sat inside has lost its repository."},
			empty:   refused,
		},
		{
			name: "a marker project whose rules cannot be read",
			setup: func(g *WithT, t *testing.T, f *fixture, dir string) int64 {
				outer, notes := markerInCheckout(g, dir)
				t.Setenv("GIT_CEILING_DIRECTORIES", outer)
				return storeProject(g, t, f, notes, "notes")
			},
			notices: []string{"warn: Live browsing is unavailable. Git could not say which files it ignores."},
			empty:   refused,
		},
		{
			name: "a repository whose HEAD names no commit",
			setup: func(g *WithT, t *testing.T, f *fixture, dir string) int64 {
				repo := cleanRepo(g, dir)
				write(g, repo, "a.txt", "changed on disk\n")
				write(g, repo, ".git/HEAD", "0000000000000000000000000000000000000000\n")
				return storeProject(g, t, f, repo, "harbor")
			},
			empty: "Cannot be listedThe repository's HEAD names no commit, so git has nothing to compare the disk with.",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			f := newFixture(g, t)
			dir, err := filepath.EvalSymlinks(t.TempDir())
			g.Expect(err).NotTo(HaveOccurred())
			project := c.setup(g, t, f, dir)

			doc := f.page(g, projectHref(project), http.StatusOK)
			g.Expect(notices(doc)).To(Equal(c.notices))
			// The page writes only a .git it found, so a second view reads
			// the same as the first.
			doc = f.page(g, projectHref(project), http.StatusOK)
			g.Expect(notices(doc)).To(Equal(c.notices))
			g.Expect(text(find(doc, "empty"))).To(Equal(strings.ReplaceAll(c.empty, "{dir}", dir)))
			g.Expect(headings(doc)).To(ContainElement("Working copy"), "nothing is listed, so nothing is counted")
			g.Expect(find(doc, "tree")).To(BeNil())
			// No file of the project answers the project file route.
			f.page(g, projectFileHref(project, 0, "a.txt"), http.StatusNotFound)
		})
	}
}

// harbor is a project with four open reviews, one approved, and changes on
// disk that no review has claimed.
type harbor struct {
	project        int64
	root           string
	w1, w2, k1, k2 int64
}

// newHarbor builds harbor: w1 and w2 waiting, w1 the longer, k1 and k2
// working, k1 decided last, and done approved. On disk beyond those, a.txt is
// modified, gone.txt deleted, link pointed elsewhere, notes/new.md new,
// private.txt ignored, and vendor/x/a.txt modified inside a directory that
// has since become a repository.
func newHarbor(g *WithT, t *testing.T, f *fixture) harbor {
	g.THelper()
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, ".gitignore", "private.txt\n")
	write(g, repo, "gone.txt", "removed\nfrom disk\n")
	write(g, repo, "vendor/x/a.txt", "committed\n")
	g.Expect(os.Symlink("old-target", filepath.Join(repo, "link"))).To(Succeed())
	claims := []string{"w1", "w2", "k1", "k2", "done"}
	for _, c := range claims {
		write(g, repo, c+"/x.txt", "before\n")
	}
	git(g, repo, "add", "-A")
	git(g, repo, "reset", "--quiet", "--", "a.txt")
	tree := git(g, repo, "write-tree")
	commit := git(g, repo, "commit-tree", tree, "-p", "main", "-m", "more")
	git(g, repo, "update-ref", "refs/heads/main", commit)
	g.Expect(os.Remove(filepath.Join(repo, "gone.txt"))).To(Succeed())
	g.Expect(os.Remove(filepath.Join(repo, "link"))).To(Succeed())
	g.Expect(os.Symlink("new-target", filepath.Join(repo, "link"))).To(Succeed())
	write(g, repo, "notes/new.md", "# New\nA paragraph.\n")
	write(g, repo, "private.txt", "a secret\n")
	write(g, repo, "q#1.txt", "a name to escape\n")
	write(g, repo, "vendor/x/a.txt", "edited\n")
	git(g, filepath.Join(repo, "vendor", "x"), "init", "--quiet", "--template=")

	ids := map[string]int64{}
	for _, c := range claims {
		write(g, repo, c+"/x.txt", "after\n")
		me := api.Caller{Dir: repo, Agent: "agent-" + c}
		ids[c] = f.request(g, t, api.RequestBody{Caller: me, Paths: []string{c}, Note: c}).Review.ID
	}
	f.decide(g, t, ids["done"], store.VerdictApprove, "")
	f.decide(g, t, ids["k2"], store.VerdictChanges, "more")
	f.decide(g, t, ids["k1"], store.VerdictChanges, "more")
	review, err := f.store.Review(t.Context(), ids["w1"])
	g.Expect(err).NotTo(HaveOccurred())
	root, err := filepath.EvalSymlinks(repo)
	g.Expect(err).NotTo(HaveOccurred())
	return harbor{project: review.ProjectID, root: root, w1: ids["w1"], w2: ids["w2"], k1: ids["k1"], k2: ids["k2"]}
}

// cleanRepo is a repository under dir with everything committed.
func cleanRepo(g *WithT, dir string) string {
	g.THelper()
	repo := gitRepo(g, filepath.Join(dir, "harbor"))
	git(g, repo, "add", "-A")
	tree := git(g, repo, "write-tree")
	commit := git(g, repo, "commit-tree", tree, "-p", "main", "-m", "all")
	git(g, repo, "update-ref", "refs/heads/main", commit)
	return repo
}

// storeProject registers a project through the store, which is how tool/seed
// registers one, with no request to capture.
func storeProject(g *WithT, t *testing.T, f *fixture, root, name string) int64 {
	g.THelper()
	project, err := f.store.EnsureProject(t.Context(), root, name)
	g.Expect(err).NotTo(HaveOccurred())
	return project.ID
}

// storeReview opens a review through the store, claiming one path, whose
// round no page here reads.
func storeReview(g *WithT, t *testing.T, f *fixture, projectID int64, claim string) {
	g.THelper()
	_, _, err := f.store.OpenReview(t.Context(), store.NewReview{
		ProjectID: projectID,
		Agent:     "agent-" + claim,
		Title:     claim,
		Paths:     []string{claim},
		Request: store.Request{
			Note:  "look",
			Files: []store.File{{Path: claim, Digest: strings.Repeat("a", 64), Size: 1}},
		},
	})
	g.Expect(err).NotTo(HaveOccurred())
}

// headings is the text of every section heading on a page.
func headings(doc *html.Node) []string {
	var out []string
	for _, h := range findAll(doc, isElement("h2")) {
		if hasClass("title")(h) {
			continue
		}
		out = append(out, text(h))
	}
	return out
}

// workingRows is each row of the working copy, as bold or plain, its path,
// its pill or counts, and its link.
func workingRows(doc *html.Node) []string {
	var out []string
	for _, row := range findAll(doc, hasClass("treerow")) {
		weight := "plain"
		if hasClass("inscope")(row) {
			weight = "bold"
		}
		label := text(find(row, "nm"))
		if extra := text(find(row, "pill")) + text(find(row, "counts")); extra != "" {
			label += " " + extra
		}
		out = append(out, weight+" "+label+" "+attr(row, "href"))
	}
	return out
}

// spelled is a toolbar's counts as a row writes them, which leaves out a side
// that moved nothing.
func spelled(counts string) string {
	plus, minus, _ := strings.Cut(counts, " ")
	var parts []string
	if plus != "+0" {
		parts = append(parts, plus)
	}
	if minus != "−0" {
		parts = append(parts, minus)
	}
	return strings.Join(parts, " ")
}

// A project with a main checkout and a worktree on disk shows a section for
// each, named by its directory with what it is and where under the name. A
// file is in review only in the working copy its review came from, and the
// worktree's rows open its own files.
func TestServer_Surface_projectWorkingCopies(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	g.Expect(err).NotTo(HaveOccurred())
	main := gitRepo(g, filepath.Join(dir, "harbor"))
	inMain := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: main, Agent: "a"}, Paths: []string{"a.txt"}, Note: "main"}).Review.ID
	worktree := filepath.Join(dir, "harbor-retry")
	git(g, main, "worktree", "add", "--quiet", "-b", "retry", worktree)
	write(g, worktree, "a.txt", "one\nretry\n")
	write(g, worktree, "b.txt", "new\n")
	inWorktree := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: worktree, Agent: "a"}, Paths: []string{"b.txt"}, Note: "retry"}).Review.ID
	review, err := f.store.Review(t.Context(), inMain)
	g.Expect(err).NotTo(HaveOccurred())
	copies, err := f.store.WorkingCopies(t.Context(), review.ProjectID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(copies).To(HaveLen(2))
	retry := copies[1]

	doc := f.page(g, projectHref(review.ProjectID), http.StatusOK)
	var headings, hints []string
	for _, h := range findAll(doc, func(n *html.Node) bool { return n.Data == "h2" }) {
		headings = append(headings, text(h))
	}
	for _, h := range findAll(doc, hasClass("hint")) {
		hints = append(hints, text(h))
	}
	g.Expect(headings).To(Equal([]string{"harbor", "Open reviews 2", "harbor 1 file", "harbor-retry 2 files"}))
	g.Expect(hints).To(Equal([]string{"Main checkout, " + main, "Worktree, " + worktree}))
	g.Expect(workingRows(doc)).To(Equal([]string{
		"plain a.txt In review " + fileHref(inMain, 1, true, "a.txt"),
		"bold a.txt +1 " + projectFileHref(review.ProjectID, retry.ID, "a.txt"),
		"plain b.txt In review " + fileHref(inWorktree, 1, true, "b.txt"),
	}))

	// The worktree's row opens the worktree's file, and a copy the project
	// does not have is not found.
	page := f.page(g, projectFileHref(review.ProjectID, retry.ID, "a.txt"), http.StatusOK)
	g.Expect(text(page)).To(ContainSubstring("retry"))
	// The page's own Marked and Plain links keep the copy.
	for _, link := range findAll(find(page, "segmented"), func(n *html.Node) bool { return n.Data == "a" }) {
		f.page(g, attr(link, "href"), http.StatusOK)
	}
	g.Expect(findAll(find(page, "segmented"), func(n *html.Node) bool { return n.Data == "a" })).To(HaveLen(2))
	other, err := f.store.EnsureProject(t.Context(), dir, "other")
	g.Expect(err).NotTo(HaveOccurred())
	elsewhere, err := f.store.EnsureWorkingCopy(t.Context(), other.ID, other.Root)
	g.Expect(err).NotTo(HaveOccurred())
	f.page(g, projectFileHref(review.ProjectID, elsewhere.ID, "a.txt"), http.StatusNotFound)
	f.page(g, projectHref(review.ProjectID)+"/files/a.txt?copy=x", http.StatusNotFound)

	// Once the worktree is removed, the main checkout is drawn alone, as a
	// project with one working copy always was.
	g.Expect(os.RemoveAll(worktree)).To(Succeed())
	doc = f.page(g, projectHref(review.ProjectID), http.StatusOK)
	headings = nil
	for _, h := range findAll(doc, func(n *html.Node) bool { return n.Data == "h2" }) {
		headings = append(headings, text(h))
	}
	g.Expect(headings).To(Equal([]string{"harbor", "Open reviews 2", "Working copy 1 file"}))
	g.Expect(findAll(doc, hasClass("hint"))).To(BeEmpty())

	// A worktree left alone after its main checkout is removed is named for
	// what it is, rather than drawn as the project's root.
	git(g, main, "worktree", "prune")
	git(g, main, "worktree", "add", "--quiet", "-b", "again", worktree)
	write(g, worktree, "c.txt", "new\n")
	f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: worktree, Agent: "a"}, Paths: []string{"c.txt"}, Note: "again"})
	g.Expect(os.Rename(main, main+"-moved")).To(Succeed())
	doc = f.page(g, projectHref(review.ProjectID), http.StatusOK)
	var lone []string
	for _, h := range findAll(doc, hasClass("hint")) {
		lone = append(lone, text(h))
	}
	g.Expect(lone).To(Equal([]string{"Worktree, " + worktree}))
}
