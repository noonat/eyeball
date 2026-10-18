package server

import (
	"net/http"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/store"
	. "github.com/onsi/gomega"
)

func TestServer_check(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	base := git(g, repo, "rev-parse", "main")
	me := api.Caller{Dir: repo}
	approved := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	f.decide(g, t, approved, store.VerdictApprove, "")
	commitFile(g, repo, "a.txt", "one\ntwo\n")
	target := git(g, repo, "rev-parse", "main")
	check := func(c api.Caller, from, to string) string {
		v := url.Values{"base": {from}, "new": {to}}
		return "/agent/check?" + query(c) + "&" + v.Encode()
	}

	// The tree the review approved, measured from its base, passes.
	var got api.CheckResult
	f.ok(g, t, http.MethodGet, check(me, base, target), nil, &got)
	g.Expect(got).To(Equal(api.CheckResult{Review: approved, Round: 1, Title: "first", Agent: got.Agent}))

	refusals := []struct {
		name   string
		caller api.Caller
		from   string
		to     string
		code   api.Code
		says   string
	}{
		{
			name:   "a base no approved review is measured from",
			caller: me,
			from:   target,
			to:     target,
			code:   api.CodeNotApproved,
			says:   "no approved review in " + repo + " has a latest round measured from " + target,
		},
		{
			name:   "a ref that names no commit",
			caller: me,
			from:   base,
			to:     "no-such-branch",
			code:   api.CodeBadCommit,
		},
	}
	for _, c := range refusals {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			_, e := f.refused(g, t, http.MethodGet, check(c.caller, c.from, c.to), nil)
			g.Expect(e.Code).To(Equal(c.code))
			g.Expect(e.Message).To(ContainSubstring(c.says))
		})
	}

	t.Run("one file differing", func(t *testing.T) {
		g := NewWithT(t)
		commitFile(g, repo, "a.txt", "one\ntwo\nthree\n")
		later := git(g, repo, "rev-parse", "main")
		_, e := f.refused(g, t, http.MethodGet, check(me, base, later), nil)
		g.Expect(e.Code).To(Equal(api.CodeTreeDiffers))
		g.Expect(e.Message).To(Equal("review " + itoa(approved) + " round 1: a.txt differs from the approved round"))
	})

	t.Run("an open review with the same tree", func(t *testing.T) {
		g := NewWithT(t)
		other := gitRepo(g, filepath.Join(t.TempDir(), "other"))
		from := git(g, other, "rev-parse", "main")
		f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: other}, Paths: []string{"a.txt"}, Note: "waiting"})
		commitFile(g, other, "a.txt", "one\ntwo\n")
		_, e := f.refused(g, t, http.MethodGet, check(api.Caller{Dir: other}, from, git(g, other, "rev-parse", "main")), nil)
		g.Expect(e.Code).To(Equal(api.CodeNotApproved))
	})

	t.Run("a repository eyeball has not seen is refused and not recorded", func(t *testing.T) {
		g := NewWithT(t)
		unseen := gitRepo(g, filepath.Join(t.TempDir(), "unseen"))
		head := git(g, unseen, "rev-parse", "main")
		before, err := f.store.Projects(t.Context())
		g.Expect(err).NotTo(HaveOccurred())
		_, e := f.refused(g, t, http.MethodGet, check(api.Caller{Dir: unseen}, head, head), nil)
		g.Expect(e.Code).To(Equal(api.CodeNotApproved))
		g.Expect(e.Message).To(Equal("no review has been opened in " + unseen))
		after, err := f.store.Projects(t.Context())
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(after).To(HaveLen(len(before)))
	})

	t.Run("a daemon that inherited another repository's GIT_DIR", func(t *testing.T) {
		// A daemon started from a git hook inherits the hook's environment,
		// where git sets GIT_DIR, and it keeps it for every project. Every git
		// call drops the repository-local variables, so the check still reads
		// this repository.
		g := NewWithT(t)
		elsewhere := gitRepo(g, filepath.Join(t.TempDir(), "elsewhere"))
		t.Setenv("GIT_DIR", filepath.Join(elsewhere, ".git"))
		t.Setenv("GIT_INDEX_FILE", filepath.Join(elsewhere, ".git", "index"))
		var got api.CheckResult
		f.ok(g, t, http.MethodGet, check(me, base, target), nil, &got)
		g.Expect(got.Review).To(Equal(approved))
	})

	t.Run("a check without its commits", func(t *testing.T) {
		g := NewWithT(t)
		_, e := f.refused(g, t, http.MethodGet, "/agent/check?"+query(me)+"&base="+base, nil)
		g.Expect(e.Code).To(Equal(api.CodeMissing))
	})
}
