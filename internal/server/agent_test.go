package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/store"
	. "github.com/onsi/gomega"
)

func Test_defaultTitle(t *testing.T) {
	long := strings.Repeat("word ", 20)
	cases := []struct {
		name string
		note string
		want string
	}{
		{
			name: "the first line that is not blank",
			note: "\n \nSummary line\nbody",
			want: "Summary line",
		},
		{
			name: "cut at a word boundary",
			note: long,
			want: strings.TrimSpace(strings.Repeat("word ", 14)),
		},
		{
			name: "cut on a character boundary",
			note: "x" + strings.Repeat("é", 50),
			want: "x" + strings.Repeat("é", 35),
		},
		{
			name: "blank throughout",
			note: " \n\t\n",
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got := defaultTitle(c.note)
			g.Expect(got).To(Equal(c.want))
			g.Expect(len(got)).To(BeNumerically("<=", titleLimit))
			g.Expect(utf8.ValidString(got)).To(BeTrue())
		})
	}
}

func Test_everySentinelHasAMapping(t *testing.T) {
	g := NewWithT(t)
	mapped := map[string]struct{}{}
	for _, m := range mappings {
		mapped[m.name] = struct{}{}
	}
	// Parsing the source is the only way to enumerate a package's variables.
	// A list written here by hand would pass when a sentinel was added there
	// and forgotten.
	packages := []string{"store", "capture"}
	var found []string
	for _, pkg := range packages {
		fset := token.NewFileSet()
		sources, err := filepath.Glob(filepath.Join("..", pkg, "*.go"))
		g.Expect(err).NotTo(HaveOccurred())
		for _, source := range sources {
			if strings.HasSuffix(source, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, source, nil, 0)
			g.Expect(err).NotTo(HaveOccurred())
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.VAR {
					continue
				}
				for _, spec := range gen.Specs {
					for _, name := range spec.(*ast.ValueSpec).Names {
						if strings.HasPrefix(name.Name, "Err") && name.IsExported() {
							found = append(found, pkg+"."+name.Name)
						}
					}
				}
			}
		}
	}
	g.Expect(found).NotTo(BeEmpty(), "the parse found nothing, so it checked nothing")
	for _, name := range found {
		g.Expect(mapped).To(HaveKey(name), "no refusal mapping for "+name)
	}
}

func Test_failureCarriesNoRawError(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	g.Expect(f.store.Close()).To(Succeed())

	status, refusal := f.refused(g, t, http.MethodGet, "/agent/status?"+query(api.Caller{Dir: repo}), nil)
	g.Expect(status).To(Equal(http.StatusInternalServerError))
	g.Expect(refusal.Code).To(Equal(api.CodeInternal))
	g.Expect(refusal.Message).To(Equal("the daemon failed to answer"))
	g.Expect(refusal.Message + refusal.Fix).NotTo(ContainSubstring("sql"))
}

func Test_hubSignalsAndForgets(t *testing.T) {
	g := NewWithT(t)
	h := newHub()
	ch, unsubscribe := h.subscribe()
	g.Expect(h.count()).To(Equal(1))

	// Two publishes with nobody draining coalesce into one signal, and
	// neither blocks.
	h.publish()
	h.publish()
	g.Expect(ch).To(Receive())
	g.Expect(ch).NotTo(Receive())

	unsubscribe()
	g.Expect(h.count()).To(BeZero())
	h.publish()
	g.Expect(ch).NotTo(Receive())
}

func Test_markCheckoutReturnsTheStoredFlag(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	outer := gitRepo(g, filepath.Join(t.TempDir(), "outer"))
	write(g, outer, "notes/.eyeball", "")
	root, err := filepath.EvalSymlinks(filepath.Join(outer, "notes"))
	g.Expect(err).NotTo(HaveOccurred())
	project, err := f.store.EnsureProject(t.Context(), root, "notes")
	g.Expect(err).NotTo(HaveOccurred())

	// The rest of the request reads the project it is handed, so it has to
	// say what the store now holds.
	got, err := f.s.markCheckout(t.Context(), project)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.InsideCheckout).To(BeTrue())
}

func Test_markingACheckoutDoesNotPublish(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	project, err := f.store.EnsureProject(t.Context(), "/tmp/notes", "notes")
	g.Expect(err).NotTo(HaveOccurred())
	ch, unsubscribe := f.s.hub.subscribe()
	defer unsubscribe()

	// A page view writes the flag, so a publish here would wake every waiting
	// agent on a GET. The bounded wait covers a publish that arrives late.
	g.Expect(f.s.db.MarkInsideCheckout(t.Context(), project.ID)).To(Succeed())
	g.Consistently(ch, 100*time.Millisecond).ShouldNot(Receive())
	got, err := f.store.Project(t.Context(), project.ID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.InsideCheckout).To(BeTrue())

	// The subscription is live, so the silence above was not a deaf channel.
	f.s.hub.publish()
	g.Expect(ch).To(Receive())
}

func Test_routesMatchOnTheirOwnListenerOnly(t *testing.T) {
	s := New(Config{})
	handlers := map[side]*http.ServeMux{
		sideAgent:   s.AgentHandler(),
		sideSurface: s.SurfaceHandler(),
	}
	for _, rt := range routes {
		t.Run(rt.pattern, func(t *testing.T) {
			g := NewWithT(t)
			method, path, _ := strings.Cut(rt.pattern, " ")
			path = strings.NewReplacer("{review}", "1", "{project}", "1", "{comment}", "1", "{round}", "1", "{path...}", "docs/a.md", "{$}", "").Replace(path)
			req, err := http.NewRequest(method, "http://eyeball"+path, nil)
			g.Expect(err).NotTo(HaveOccurred())
			for which, mux := range handlers {
				_, pattern := mux.Handler(req)
				if which == rt.side {
					g.Expect(pattern).To(Equal(rt.pattern))
				} else {
					g.Expect(pattern).To(BeEmpty(), "a route reachable from the other listener")
				}
			}
		})
	}
}

func TestServer_abandonedReviewOwesNoVerdict(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "more")

	// The verdict was never received, and then the agent withdrew the review.
	// A wait on it now returns abandoned and stamps no verdict, so listing it
	// as owed would list it forever.
	f.ok(g, t, http.MethodPost, "/agent/reviews/"+itoa(id)+"/abandon", api.AbandonBody{Caller: me}, &api.AbandonResult{})
	var got api.StatusResult
	f.ok(g, t, http.MethodGet, "/agent/status?"+query(me), nil, &got)
	g.Expect(got.Reviews).To(BeEmpty())
}

func TestServer_asksForANextRound(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	first := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"})
	id := first.Review.ID

	// Asking again before a verdict is refused before anything is captured.
	// The new content's blob is the evidence: a capture would have written it.
	write(g, repo, "a.txt", "content no round has seen\n")
	_, refusal := f.refused(g, t, http.MethodPost, "/agent/requests", api.RequestBody{Caller: me, Review: id, Note: "again"})
	g.Expect(refusal.Code).To(Equal(api.CodeAwaitingVerdict))
	g.Expect(refusal.Fix).To(ContainSubstring("eyeball wait"))
	sum := sha256.Sum256([]byte("content no round has seen\n"))
	has, err := f.blobs.Has(hex.EncodeToString(sum[:]))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(has).To(BeFalse(), "a refused round captured content")

	f.decide(g, t, id, store.VerdictChanges, "say more")
	got := f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "rewrote it"})
	g.Expect(got.Review.Round).To(Equal(2))
	g.Expect(got.Review.State).To(Equal(api.StateWaiting))
	g.Expect(got.Size).To(Equal(api.Size{Files: 1, Added: 1, Removed: 2}))
}

// A main checkout and its worktree are one project, and each round records the
// working copy it came from. A bare clone's worktree and a submodule are
// repositories of their own.
func TestServer_findsAWorktreeInItsRepository(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	g.Expect(err).NotTo(HaveOccurred())
	main := gitRepo(g, filepath.Join(dir, "harbor"))
	worktree := filepath.Join(dir, "harbor-retry")
	git(g, main, "worktree", "add", "--quiet", "-b", "retry", worktree)
	write(g, worktree, "a.txt", "one\nretry\n")
	projectOf := func(reviewID int64) int64 {
		review, err := f.store.Review(t.Context(), reviewID)
		g.Expect(err).NotTo(HaveOccurred())
		return review.ProjectID
	}
	copyOf := func(reviewID int64) store.WorkingCopy {
		round, err := f.store.Round(t.Context(), reviewID, 1)
		g.Expect(err).NotTo(HaveOccurred())
		c, err := f.store.WorkingCopyByID(t.Context(), round.WorkingCopyID)
		g.Expect(err).NotTo(HaveOccurred())
		return c
	}

	inMain := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: main, Agent: "a"}, Paths: []string{"a.txt"}, Note: "main"})
	g.Expect(copyOf(inMain.Review.ID).Root).To(Equal(main))
	g.Expect(copyOf(inMain.Review.ID).Main).To(BeTrue())

	// The main checkout was seen first, so it is the project's root, and a
	// round that fell back to the root would record it instead of the
	// worktree. The same path is a different change in each copy, so the
	// main checkout's claim does not refuse it.
	inWorktree := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: worktree, Agent: "a"}, Paths: []string{"a.txt"}, Note: "retry"})
	g.Expect(projectOf(inWorktree.Review.ID)).To(Equal(projectOf(inMain.Review.ID)))
	g.Expect(copyOf(inWorktree.Review.ID).Root).To(Equal(worktree), "a round reads the working copy it was captured from")
	g.Expect(copyOf(inWorktree.Review.ID).Main).To(BeFalse())

	// A project first seen from a worktree moves its root to the main
	// checkout once a request comes from there.
	other := gitRepo(g, filepath.Join(dir, "dock"))
	otherWorktree := filepath.Join(dir, "dock-retry")
	git(g, other, "worktree", "add", "--quiet", "-b", "retry", otherWorktree)
	write(g, otherWorktree, "a.txt", "one\nretry\n")
	first := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: otherWorktree, Agent: "a"}, Paths: []string{"a.txt"}, Note: "retry"})
	f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: other, Agent: "a"}, Paths: []string{"a.txt"}, Note: "main"})
	project, err := f.store.Project(t.Context(), projectOf(first.Review.ID))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(project.Root).To(Equal(other))

	bare := filepath.Join(dir, "shared.git")
	git(g, main, "clone", "--quiet", "--bare", main, bare)
	fromBare := filepath.Join(dir, "shared-wt")
	git(g, bare, "worktree", "add", "--quiet", fromBare)
	write(g, fromBare, "a.txt", "one\nbare\n")
	inBare := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: fromBare, Agent: "a"}, Paths: []string{"a.txt"}, Note: "bare"})
	g.Expect(projectOf(inBare.Review.ID)).NotTo(Equal(projectOf(inMain.Review.ID)), "a clone is another repository")

	git(g, main, "-c", "protocol.file.allow=always", "submodule", "--quiet", "add", bare, "vendored")
	sub := filepath.Join(main, "vendored")
	write(g, sub, "a.txt", "one\nsub\n")
	inSub := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: sub, Agent: "a"}, Paths: []string{"a.txt"}, Note: "sub"})
	g.Expect(projectOf(inSub.Review.ID)).NotTo(Equal(projectOf(inMain.Review.ID)))
	g.Expect(projectOf(inSub.Review.ID)).NotTo(Equal(projectOf(inBare.Review.ID)))
}

func TestServer_handsOverLateComments(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	unread := f.comment(g, t, id, "in round one's batch")
	f.decide(g, t, id, store.VerdictChanges, "")
	write(g, repo, "a.txt", "three\n")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})

	// Round one's batch was never acknowledged, and round two waits. That
	// comment is not late, so wait blocks rather than returning it now.
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	status, _ := callNoFail(f, ctx, waitPath(id, me))
	g.Expect(status).To(BeZero(), "wait returned while round two still waits")

	// A comment written on round one after its verdict is late, and wakes a
	// wait that is already blocked, coming out on its own while round two
	// waits.
	done := f.blockedWait(g, t, id, me)
	late := addToRound(g, t, f, id, 1, "after the verdict")
	var got api.WaitResult
	g.Eventually(done, 5*time.Second).Should(Receive(&got))
	g.Expect(got.Kind).To(Equal(api.KindComments))
	g.Expect(ids(got.Comments)).To(Equal([]int64{late.ID}))
	g.Expect(got.Next).To(ContainSubstring("still waiting"))

	// Acknowledged, it is not owed any more, and wait blocks again.
	f.ok(g, t, http.MethodPost, "/agent/reviews/"+itoa(id)+"/receipt", api.ReceiptBody{Caller: me, Comments: []int64{late.ID}}, &api.ReceiptResult{})
	ctx2, cancel2 := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel2()
	status, _ = callNoFail(f, ctx2, waitPath(id, me))
	g.Expect(status).To(BeZero(), "an acknowledged late comment was handed over again")

	// Once round two is decided, its batch carries round one's unread
	// comments as well, because no later wait would show them.
	f.decide(g, t, id, store.VerdictApprove, "")
	got = f.wait(g, t, id, me)
	g.Expect(got.Kind).To(Equal(api.KindVerdict))
	g.Expect(ids(got.Comments)).To(Equal([]int64{unread.ID}), "the late one was acknowledged above")
	g.Expect(got.Review.State).To(Equal(api.StateApproved))
}

func TestServer_marksAMarkerProjectInsideACheckout(t *testing.T) {
	projects := []struct {
		name     string
		checkout bool
		known    bool
		want     bool
	}{
		{
			name:     "a new project inside a checkout",
			checkout: true,
			known:    false,
			want:     true,
		},
		{
			name:     "a known project left unflagged",
			checkout: true,
			known:    true,
			want:     true,
		},
		{
			name:     "a project with no checkout around it",
			checkout: false,
			known:    false,
			want:     false,
		},
	}
	for _, c := range projects {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			f := newFixture(g, t)
			dir := filepath.Join(t.TempDir(), "outer")
			if c.checkout {
				gitRepo(g, dir)
			}
			notes := filepath.Join(dir, "notes")
			write(g, notes, ".eyeball", "")
			write(g, notes, "n.md", "a note\n")
			root, err := filepath.EvalSymlinks(notes)
			g.Expect(err).NotTo(HaveOccurred())
			if c.known {
				// Registered before the store kept the flag, so it reads 0
				// and the request reuses the row.
				known, err := f.store.EnsureProject(t.Context(), root, "notes")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(known.InsideCheckout).To(BeFalse())
			}

			f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: notes, Agent: "agent-1"}, Paths: []string{"n.md"}, Note: "a note"})
			got, err := f.store.ProjectByRoot(t.Context(), root)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got.InsideCheckout).To(Equal(c.want))
		})
	}
}

// A directory made again at the root of a removed working copy belongs to that
// copy's old project, so the old project is merged into the one git now names.
func TestServer_mergesTheProjectOfARecreatedRoot(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	g.Expect(err).NotTo(HaveOccurred())
	main := gitRepo(g, filepath.Join(dir, "harbor"))
	worktree := filepath.Join(dir, "harbor-retry")

	// The old project at the worktree's root was written before projects
	// had repositories, and its directory was gone when the daemon started.
	old, err := f.store.EnsureProject(t.Context(), worktree, "harbor")
	g.Expect(err).NotTo(HaveOccurred())
	_, _, err = f.store.OpenReview(t.Context(), store.NewReview{
		ProjectID: old.ID,
		Agent:     "a",
		Title:     "old work",
		Paths:     []string{"a.txt"},
		Request:   store.Request{Note: "old", Files: []store.File{{Path: "a.txt"}}},
	})
	g.Expect(err).NotTo(HaveOccurred())

	f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: main, Agent: "a"}, Paths: []string{"a.txt"}, Note: "main"})

	git(g, main, "worktree", "add", "--quiet", "-b", "retry", worktree)
	write(g, worktree, "b.txt", "new\n")
	got := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: worktree, Agent: "a"}, Paths: []string{"b.txt"}, Note: "again"})

	review, err := f.store.Review(t.Context(), got.Review.ID)
	g.Expect(err).NotTo(HaveOccurred())
	rows, err := f.store.ReviewsIn(t.Context(), review.ProjectID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(rows).To(HaveLen(3), "the old project's review moved into the repository's project")
	_, err = f.store.Project(t.Context(), old.ID)
	g.Expect(err).To(MatchError(store.ErrNotFound))
}

func TestServer_namesTheAgentFromTheCallersHome(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	home := t.TempDir()
	repo := gitRepo(g, filepath.Join(home, "src", "harbor"))

	// The daemon's home is another directory: it is whichever agent started
	// it, and the name must not change when a different one does.
	got := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo, Home: home}, Paths: []string{"a.txt"}, Note: "n"})
	g.Expect(got.Review.Agent).To(Equal("~/src/harbor"))

	var status api.StatusResult
	f.ok(g, t, http.MethodGet, "/agent/status?"+query(api.Caller{Dir: repo, Home: home}), nil, &status)
	g.Expect(status.Agent).To(Equal("~/src/harbor"))
	g.Expect(statusIDs(status.Reviews)).To(Equal([]int64{got.Review.ID}))
}

func TestServer_opensAReview(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo, Agent: "agent-1"}

	got := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "\n  Add a second line\n\nmore detail"})
	g.Expect(got.Review.ID).NotTo(BeZero())
	g.Expect(got.Review.Title).To(Equal("Add a second line"))
	g.Expect(got.Review.State).To(Equal(api.StateWaiting))
	g.Expect(got.Review.Round).To(Equal(1))
	g.Expect(got.Review.Agent).To(Equal("agent-1"))
	g.Expect(got.Review.Project.Name).To(Equal("harbor"))
	g.Expect(got.Size).To(Equal(api.Size{Files: 1, Added: 1}))
	g.Expect(got.Next).To(ContainSubstring("eyeball wait"))
	round, err := f.store.Round(t.Context(), got.Review.ID, 1)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(round.Note).To(Equal("Add a second line\n\nmore detail"))

	// A relative path from a subdirectory is read the way a shell reads it.
	write(g, repo, "docs/b.md", "new\n")
	sub := api.Caller{Dir: filepath.Join(repo, "docs"), Agent: "agent-1"}
	second := f.request(g, t, api.RequestBody{Caller: sub, Paths: []string{"b.md"}, Note: "a doc"})
	g.Expect(second.Review.Paths).To(Equal([]string{"docs/b.md"}))
}

func TestServer_ownership(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	dir := t.TempDir()
	repo := gitRepo(g, filepath.Join(dir, "harbor"))
	elsewhere := gitRepo(g, filepath.Join(dir, "field-notes"))
	me := api.Caller{Dir: repo, Agent: "agent-1"}
	review := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"})
	id := review.Review.ID
	f.comment(g, t, id, "a point")
	f.decide(g, t, id, store.VerdictChanges, "")
	released, err := f.store.Released(t.Context(), id)
	g.Expect(err).NotTo(HaveOccurred())
	commentID := released[0].ID

	callers := []struct {
		name   string
		caller api.Caller
		code   api.Code
	}{
		{
			name:   "another project",
			caller: api.Caller{Dir: elsewhere, Agent: "agent-1"},
			code:   api.CodeWrongProject,
		},
		{
			name:   "another agent",
			caller: api.Caller{Dir: repo, Agent: "agent-2"},
			code:   api.CodeWrongAgent,
		},
	}
	for _, c := range callers {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			idPath := "/agent/reviews/" + itoa(id)
			calls := []struct {
				method string
				path   string
				body   any
			}{
				{
					method: http.MethodPost,
					path:   "/agent/requests",
					body:   api.RequestBody{Caller: c.caller, Review: id, Note: "n"},
				},
				{
					method: http.MethodGet,
					path:   waitPath(id, c.caller),
					body:   nil,
				},
				{
					method: http.MethodPost,
					path:   idPath + "/receipt",
					body:   api.ReceiptBody{Caller: c.caller, Comments: []int64{commentID}},
				},
				{
					method: http.MethodPost,
					path:   idPath + "/abandon",
					body:   api.AbandonBody{Caller: c.caller},
				},
				{
					method: http.MethodPost,
					path:   "/agent/comments/" + itoa(commentID) + "/reply",
					body:   api.ReplyBody{Caller: c.caller, Body: "x"},
				},
			}
			for _, call := range calls {
				status, refusal := f.refused(g, t, call.method, call.path, call.body)
				g.Expect(status).To(Equal(http.StatusForbidden), call.path)
				g.Expect(refusal.Code).To(Equal(c.code), call.path)
			}
		})
	}

	// None of them changed anything.
	c, err := f.store.Released(t.Context(), id)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(c[0].Delivered).To(BeZero())
	g.Expect(c[0].Reply).To(BeEmpty())
	got, err := f.store.Review(t.Context(), id)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.State).To(Equal(store.StateOpen))
	g.Expect(got.LatestRound).To(Equal(1))
}

func TestServer_refusesAClaimedPath(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo, Agent: "agent-1"}
	held := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"})
	again := api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "again"}

	_, refusal := f.refused(g, t, http.MethodPost, "/agent/requests", again)
	g.Expect(refusal.Code).To(Equal(api.CodePathClaimed))
	g.Expect(refusal.Fix).To(ContainSubstring("eyeball wait"), "the holder is waiting")

	f.decide(g, t, held.Review.ID, store.VerdictChanges, "more")
	_, refusal = f.refused(g, t, http.MethodPost, "/agent/requests", again)
	g.Expect(refusal.Fix).To(ContainSubstring("--review"), "the holder is decided")

	other := again
	other.Agent = "agent-2"
	_, refusal = f.refused(g, t, http.MethodPost, "/agent/requests", other)
	g.Expect(refusal.Fix).To(ContainSubstring(`--agent "agent-1"`), "the holder belongs to another agent")
}

func TestServer_refusesBadRequests(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	cases := []struct {
		name string
		body api.RequestBody
		code api.Code
	}{
		{
			name: "no note",
			body: api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: " \n "},
			code: api.CodeMissing,
		},
		{
			name: "no paths",
			body: api.RequestBody{Caller: me, Note: "n"},
			code: api.CodeMissing,
		},
		{
			name: "paths with a review",
			body: api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Review: 1, Note: "n"},
			code: api.CodeUsage,
		},
		{
			name: "a base with a review",
			body: api.RequestBody{Caller: me, Base: "HEAD", Review: 1, Note: "n"},
			code: api.CodeUsage,
		},
		{
			name: "a path that matches nothing",
			body: api.RequestBody{Caller: me, Paths: []string{"a.txt", "a.tx"}, Note: "n"},
			code: api.CodePathUnmatched,
		},
		{
			name: "nothing in scope",
			body: api.RequestBody{Caller: me, Paths: []string{"keep.txt"}, Note: "n"},
			code: api.CodeNothingInScope,
		},
		{
			name: "a path outside the project",
			body: api.RequestBody{Caller: me, Paths: []string{"../elsewhere"}, Note: "n"},
			code: api.CodeOutsideProject,
		},
		{
			name: "a base that names nothing",
			body: api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Base: "no-such-ref", Note: "n"},
			code: api.CodeBadBase,
		},
		{
			name: "a base outside git",
			body: api.RequestBody{Caller: api.Caller{Dir: marked(g, t)}, Paths: []string{"notes.md"}, Base: "HEAD", Note: "n"},
			code: api.CodeBadBase,
		},
		{
			name: "no project",
			body: api.RequestBody{Caller: api.Caller{Dir: t.TempDir()}, Paths: []string{"a.txt"}, Note: "n"},
			code: api.CodeNoProject,
		},
		{
			name: "a relative working directory",
			body: api.RequestBody{Caller: api.Caller{Dir: "harbor"}, Paths: []string{"a.txt"}, Note: "n"},
			code: api.CodeBadRequest,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			_, refusal := f.refused(g, t, http.MethodPost, "/agent/requests", c.body)
			g.Expect(refusal.Code).To(Equal(c.code), refusal.Message)
			g.Expect(refusal.Fix).NotTo(BeEmpty())
		})
	}
}

func TestServer_reply(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	c := f.comment(g, t, id, "a point")
	path := "/agent/comments/" + itoa(c.ID) + "/reply"

	_, refusal := f.refused(g, t, http.MethodPost, path, api.ReplyBody{Caller: me, Body: "done"})
	g.Expect(refusal.Code).To(Equal(api.CodeNotReleased))

	f.decide(g, t, id, store.VerdictChanges, "")
	var got api.ReplyResult
	f.ok(g, t, http.MethodPost, path, api.ReplyBody{Caller: me, Body: "done"}, &got)
	g.Expect(got.Comment.Reply).To(Equal("done"))
	g.Expect(got.Comment.ID).To(Equal(c.ID))
}

func TestServer_seesAVerdictBetweenReadAndBlock(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID

	// The verdict lands after the waiter has read an undecided round and
	// before it blocks. A waiter that subscribed only now would miss the
	// signal and never return.
	var once sync.Once
	hook := func() {
		once.Do(func() { f.decide(g, t, id, store.VerdictApprove, "") })
	}
	f.s.afterRead.Store(&hook)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	status, _ := callNoFail(f, ctx, waitPath(id, me))
	g.Expect(status).To(Equal(http.StatusOK))
}

func TestServer_show(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo, Agent: "agent-1"}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	c := f.comment(g, t, id, "a point")
	f.decide(g, t, id, store.VerdictApprove, "good")

	// No caller at all: show reads any review and marks nothing delivered.
	var got api.ShowResult
	f.ok(g, t, http.MethodGet, "/agent/reviews/"+itoa(id), nil, &got)
	g.Expect(got.Review.State).To(Equal(api.StateApproved))
	g.Expect(got.Rounds).To(HaveLen(1))
	g.Expect(got.Rounds[0].Verdict).To(Equal(api.VerdictApprove))
	g.Expect(got.Rounds[0].VerdictNote).To(Equal("good"))
	g.Expect(got.Rounds[0].DecidedAt).NotTo(BeNil())
	g.Expect(got.Files).To(HaveLen(1))
	g.Expect(got.Files[0].Path).To(Equal("a.txt"))
	g.Expect(got.Files[0].Digest).To(HaveLen(64))
	released, err := f.store.Released(t.Context(), id)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(released[0].ID).To(Equal(c.ID))
	g.Expect(released[0].Delivered).To(BeZero())
}

func TestServer_status(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "b.txt", "b\n")
	write(g, repo, "c.txt", "c\n")
	me := api.Caller{Dir: repo, Agent: "agent-1"}
	waiting := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "waiting"}).Review.ID
	approved := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"b.txt"}, Note: "approved"}).Review.ID
	f.decide(g, t, approved, store.VerdictApprove, "")
	write(g, repo, "d.txt", "d\n")
	unread := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"d.txt"}, Note: "approved, unread"}).Review.ID
	f.comment(g, t, unread, "read this")
	f.decide(g, t, unread, store.VerdictApprove, "")
	theirs := api.Caller{Dir: repo, Agent: "agent-2"}
	f.request(g, t, api.RequestBody{Caller: theirs, Paths: []string{"c.txt"}, Note: "theirs"})

	// An approval stays listed until the agent has received it, even with no
	// comment to read: a wait killed before it printed the verdict must not
	// leave the approval reachable only by an id the agent forgot.
	var got api.StatusResult
	f.ok(g, t, http.MethodGet, "/agent/status?"+query(me), nil, &got)
	g.Expect(got.Agent).To(Equal("agent-1"))
	g.Expect(got.Daemon.PID).To(Equal(42))
	g.Expect(got.Project.Name).To(Equal("harbor"))
	g.Expect(statusIDs(got.Reviews)).To(Equal([]int64{unread, approved, waiting}))
	g.Expect(got.Next).To(ContainSubstring(fmt.Sprintf("review %d has a verdict not yet read", approved)))

	f.ok(g, t, http.MethodPost, "/agent/reviews/"+itoa(approved)+"/receipt", api.ReceiptBody{Caller: me, Round: 1}, &api.ReceiptResult{})
	f.ok(g, t, http.MethodGet, "/agent/status?"+query(me), nil, &got)
	g.Expect(statusIDs(got.Reviews)).To(Equal([]int64{unread, waiting}), "a received approval with nothing unread is not listed")
	g.Expect(got.Reviews[0].State).To(Equal(api.StateApproved))
	g.Expect(got.Reviews[0].Undelivered).To(Equal(1))
	g.Expect(got.Reviews[1].State).To(Equal(api.StateWaiting))
	g.Expect(got.Others).To(Equal(1))
	g.Expect(got.Next).To(ContainSubstring("status --all"))

	var all api.StatusResult
	f.ok(g, t, http.MethodGet, "/agent/status?"+query(me)+"&all=1", nil, &all)
	g.Expect(all.Reviews).To(HaveLen(4))
	g.Expect(all.Next).To(ContainSubstring(`--agent "agent-2"`), "a suggestion for another agent's review names that agent")
}

func TestServer_views(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "site/page.html", "<p>page</p>\n")
	write(g, repo, "site/icon.png", "\x89PNG\r\n\x1a\n")
	me := api.Caller{Dir: filepath.Join(repo, "site")}

	refusals := []struct {
		name  string
		views map[string]string
		says  string
	}{
		{
			name:  "a path the round does not hold",
			views: map[string]string{"nope.html": "rendered"},
			says:  "site/nope.html is not a file this round holds",
		},
		{
			name:  "a source view of an image",
			views: map[string]string{"icon.png": "source"},
			says:  `site/icon.png has no "source" view`,
		},
		{
			name:  "a rendering of code",
			views: map[string]string{"../a.txt": "rendered"},
			says:  `a.txt has no "rendered" view`,
		},
		{
			name:  "a choice that is not one",
			views: map[string]string{"page.html": "page"},
			says:  `site/page.html has no "page" view`,
		},
		{
			name:  "a path leaving the project",
			views: map[string]string{"../../x.html": "rendered"},
		},
	}
	for _, c := range refusals {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			_, e := f.refused(g, t, http.MethodPost, "/agent/requests", api.RequestBody{Caller: me, Paths: []string{"."}, Note: "first", Views: c.views})
			if c.says == "" {
				g.Expect(e.Code).To(Equal(api.CodeOutsideProject))
				return
			}
			g.Expect(e.Code).To(Equal(api.CodeBadView))
			g.Expect(e.Message).To(ContainSubstring(c.says))
			_, err := f.store.Review(t.Context(), 1)
			g.Expect(err).To(MatchError(store.ErrNotFound), "a refused view stores nothing")
		})
	}

	// The view is stored per round, keyed by the path from the project root,
	// and the next round does not carry it.
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"."}, Note: "first", Views: map[string]string{"page.html": "rendered"}}).Review.ID
	views := func(round int64) map[string]string {
		files, err := f.store.RoundFiles(t.Context(), round)
		g.Expect(err).NotTo(HaveOccurred())
		out := map[string]string{}
		for _, file := range files {
			out[file.Path] = file.View
		}
		return out
	}
	g.Expect(views(f.latestRound(g, t, id))).To(Equal(map[string]string{"a.txt": "", "site/icon.png": "", "site/page.html": "page"}))
	f.decide(g, t, id, store.VerdictChanges, "more")
	write(g, repo, "site/page.html", "<p>page, again</p>\n")
	_, e := f.refused(g, t, http.MethodPost, "/agent/requests", api.RequestBody{Caller: me, Review: id, Note: "second", Views: map[string]string{"icon.png": "source"}})
	g.Expect(e.Code).To(Equal(api.CodeBadView), "a next round checks its views too")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
	g.Expect(views(f.latestRound(g, t, id))).To(HaveKeyWithValue("site/page.html", ""))
}

func TestServer_waitAnswersWhenStopping(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID

	type answer struct {
		status int
		body   []byte
	}
	done := make(chan answer, 1)
	go func() {
		status, body := callNoFail(f, t.Context(), waitPath(id, me))
		done <- answer{status, body}
	}()
	g.Eventually(f.s.hub.count, 5*time.Second).Should(Equal(1))
	f.cancel()
	var got answer
	g.Eventually(done, 5*time.Second).Should(Receive(&got))
	g.Expect(got.status).To(Equal(http.StatusServiceUnavailable))
	g.Expect(string(got.body)).To(ContainSubstring(string(api.CodeDaemonStopping)))
}

func TestServer_waitEndsOnAbandon(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	f.decide(g, t, id, store.VerdictChanges, "more")

	write(g, repo, "a.txt", "three\n")
	f.request(g, t, api.RequestBody{Caller: me, Review: id, Note: "second"})
	done := f.blockedWait(g, t, id, me)
	var abandoned api.AbandonResult
	f.ok(g, t, http.MethodPost, "/agent/reviews/"+itoa(id)+"/abandon", api.AbandonBody{Caller: me}, &abandoned)
	g.Expect(abandoned.Review.State).To(Equal(api.StateAbandoned))
	var got api.WaitResult
	g.Eventually(done, 5*time.Second).Should(Receive(&got))
	g.Expect(got.Kind).To(Equal(api.KindAbandoned))

	// A review withdrawn right after a verdict, with no later round, has a
	// decided latest round, and abandoned still wins.
	write(g, repo, "b.txt", "b\n")
	other := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"b.txt"}, Note: "other"}).Review.ID
	f.decide(g, t, other, store.VerdictChanges, "more")
	f.ok(g, t, http.MethodPost, "/agent/reviews/"+itoa(other)+"/abandon", api.AbandonBody{Caller: me}, &abandoned)
	got = f.wait(g, t, other, me)
	g.Expect(got.Kind).To(Equal(api.KindAbandoned))
}

func TestServer_waitsForTheVerdict(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	me := api.Caller{Dir: repo}
	id := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID

	// A wait blocks until the verdict, and a comment written before it is
	// not handed over early.
	done := f.blockedWait(g, t, id, me)
	f.comment(g, t, id, "before the verdict")
	g.Consistently(done, 300*time.Millisecond).ShouldNot(Receive())
	f.decide(g, t, id, store.VerdictChanges, "tighten it")
	var got api.WaitResult
	g.Eventually(done, 5*time.Second).Should(Receive(&got))
	g.Expect(got.Kind).To(Equal(api.KindVerdict))
	g.Expect(got.Verdict).To(Equal(api.VerdictChanges))
	g.Expect(got.Note).To(Equal("tighten it"))
	g.Expect(got.DecidedAt).NotTo(BeNil())
	g.Expect(got.Comments).To(HaveLen(1))
	g.Expect(got.Review.State).To(Equal(api.StateChangesRequested))
	g.Expect(got.Next).To(ContainSubstring("--review"))

	// Waiting again returns the same batch, delivered or not.
	f.ok(g, t, http.MethodPost, "/agent/reviews/"+itoa(id)+"/receipt", api.ReceiptBody{Caller: me, Comments: []int64{got.Comments[0].ID}}, &api.ReceiptResult{})
	released, err := f.store.Released(t.Context(), id)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(released[0].Delivered).NotTo(BeZero(), "the receipt stamped it")
	again := f.wait(g, t, id, me)
	g.Expect(again.Comments).To(HaveLen(1))
}

// marked builds a project outside git, named by a marker file, with one file.
func marked(g *WithT, t *testing.T) string {
	g.THelper()
	dir := filepath.Join(t.TempDir(), "notes")
	write(g, dir, ".eyeball", "")
	write(g, dir, "notes.md", "a note\n")
	return dir
}

// callNoFail sends a GET and reports a status of zero when the context ended
// before an answer, which is what a test asserting that a wait blocks reads.
func callNoFail(f *fixture, ctx context.Context, path string) (int, []byte) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://eyeball"+path, nil)
	if err != nil {
		return 0, nil
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return 0, nil
	}
	defer func() { _ = resp.Body.Close() }()
	body := make([]byte, 0, 512)
	buf := make([]byte, 512)
	for {
		n, err := resp.Body.Read(buf)
		body = append(body, buf[:n]...)
		if err != nil {
			break
		}
	}
	return resp.StatusCode, body
}

// addToRound writes a comment on one numbered round through the wrapper.
func addToRound(g *WithT, t *testing.T, f *fixture, reviewID int64, number int, body string) store.Comment {
	g.THelper()
	round, err := f.store.Round(t.Context(), reviewID, number)
	g.Expect(err).NotTo(HaveOccurred())
	c, err := f.s.db.AddComment(t.Context(), store.NewComment{RoundID: round.ID, Level: store.LevelReview, Body: body})
	g.Expect(err).NotTo(HaveOccurred())
	return c
}

// ids lists comment ids in order.
func ids(in []api.Comment) []int64 {
	out := make([]int64, 0, len(in))
	for _, c := range in {
		out = append(out, c.ID)
	}
	return out
}

// statusIDs lists status rows' review ids in order.
func statusIDs(in []api.StatusRow) []int64 {
	out := make([]int64, 0, len(in))
	for _, r := range in {
		out = append(out, r.ID)
	}
	return out
}

// itoa writes an id for a URL.
func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
