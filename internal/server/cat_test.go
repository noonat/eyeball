package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/store"
	. "github.com/onsi/gomega"
)

// catPath is the request a cat makes.
func catPath(c api.Caller, base, path string) string {
	v := url.Values{"base": {base}, "path": {path}}
	return "/agent/cat?" + query(c) + "&" + v.Encode()
}

// catBytes is the answer to a cat that must succeed: its content, after the
// digest and the size are checked against it.
func catBytes(g *WithT, t *testing.T, f *fixture, c api.Caller, base, path string) []byte {
	g.THelper()
	var got api.CatResult
	f.ok(g, t, http.MethodGet, catPath(c, base, path), nil, &got)
	sum := sha256.Sum256(got.Content)
	g.Expect(got.Digest).To(Equal(hex.EncodeToString(sum[:])))
	g.Expect(got.Size).To(Equal(int64(len(got.Content))))
	return got.Content
}

// approvedAgain opens a review of a.txt measured from base, which holds
// whatever the working tree holds now, and approves it.
func approvedAgain(g *WithT, t *testing.T, f *fixture, repo, base string) int64 {
	g.THelper()
	id := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt"}, Base: base, Note: "again"}).Review.ID
	f.decide(g, t, id, store.VerdictApprove, "")
	return id
}

func TestServer_cat(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	base := git(g, repo, "rev-parse", "main")
	me := api.Caller{Dir: repo}
	approved := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	f.decide(g, t, approved, store.VerdictApprove, "")

	t.Run("the approved bytes, by a relative and an absolute path", func(t *testing.T) {
		g := NewWithT(t)
		spellings := []string{"a.txt", filepath.Join(repo, "a.txt")}
		for _, p := range spellings {
			g.Expect(string(catBytes(g, t, f, me, base, p))).To(Equal("one\ntwo\n"), p)
		}
		var got api.CatResult
		f.ok(g, t, http.MethodGet, catPath(me, base, "a.txt"), nil, &got)
		g.Expect(got.Review).To(Equal(approved))
		g.Expect(got.Round).To(Equal(1))
		g.Expect(got.Path).To(Equal("a.txt"))
	})

	t.Run("bytes the working tree has since lost are still the approved ones", func(t *testing.T) {
		g := NewWithT(t)
		write(g, repo, "a.txt", "changed after approval\n")
		defer write(g, repo, "a.txt", "one\ntwo\n")
		g.Expect(string(catBytes(g, t, f, me, base, "a.txt"))).To(Equal("one\ntwo\n"))
	})

	refusals := []struct {
		name   string
		caller api.Caller
		base   string
		path   string
		code   api.Code
		says   string
	}{
		{name: "a file the review did not capture", caller: me, base: base, path: "keep.txt", code: api.CodeNotCaptured, says: "captured keep.txt"},
		{name: "a ref that names no commit", caller: me, base: "no-such-branch", path: "a.txt", code: api.CodeBadCommit},
		{name: "a path outside the project", caller: me, base: base, path: "/etc/passwd", code: api.CodeOutsideProject},
	}
	for _, c := range refusals {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			_, e := f.refused(g, t, http.MethodGet, catPath(c.caller, c.base, c.path), nil)
			g.Expect(e.Code).To(Equal(c.code))
			g.Expect(e.Message).To(ContainSubstring(c.says))
		})
	}

	t.Run("a base no approved review is measured from", func(t *testing.T) {
		g := NewWithT(t)
		moved := gitRepo(g, filepath.Join(t.TempDir(), "moved"))
		approvedAgain(g, t, f, moved, git(g, moved, "rev-parse", "main"))
		commitFile(g, moved, "b.txt", "later\n")
		_, e := f.refused(g, t, http.MethodGet, catPath(api.Caller{Dir: moved}, git(g, moved, "rev-parse", "main"), "a.txt"), nil)
		g.Expect(e.Code).To(Equal(api.CodeNotCaptured))
		g.Expect(e.Message).To(ContainSubstring("a latest round measured from"))
	})

	t.Run("a request without its base or path", func(t *testing.T) {
		g := NewWithT(t)
		_, e := f.refused(g, t, http.MethodGet, "/agent/cat?"+query(me)+"&base="+base, nil)
		g.Expect(e.Code).To(Equal(api.CodeMissing))
		_, e = f.refused(g, t, http.MethodGet, "/agent/cat?"+query(me)+"&path=a.txt", nil)
		g.Expect(e.Code).To(Equal(api.CodeMissing))
	})

	t.Run("a review still waiting does not answer", func(t *testing.T) {
		g := NewWithT(t)
		other := gitRepo(g, filepath.Join(t.TempDir(), "other"))
		from := git(g, other, "rev-parse", "main")
		f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: other}, Paths: []string{"a.txt"}, Note: "waiting"})
		_, e := f.refused(g, t, http.MethodGet, catPath(api.Caller{Dir: other}, from, "a.txt"), nil)
		g.Expect(e.Code).To(Equal(api.CodeNotCaptured))
	})

	t.Run("two approved reviews holding different bytes are ambiguous", func(t *testing.T) {
		g := NewWithT(t)
		twice := gitRepo(g, filepath.Join(t.TempDir(), "twice"))
		from := git(g, twice, "rev-parse", "main")
		first := approvedAgain(g, t, f, twice, from)
		write(g, twice, "a.txt", "one\nthree\n")
		second := approvedAgain(g, t, f, twice, from)
		_, e := f.refused(g, t, http.MethodGet, catPath(api.Caller{Dir: twice}, from, "a.txt"), nil)
		g.Expect(e.Code).To(Equal(api.CodeAmbiguous))
		g.Expect(e.Message).To(ContainSubstring("reviews " + itoa(second) + " and " + itoa(first)))
	})

	t.Run("two approved reviews holding the same bytes are one answer", func(t *testing.T) {
		g := NewWithT(t)
		same := gitRepo(g, filepath.Join(t.TempDir(), "same"))
		from := git(g, same, "rev-parse", "main")
		approvedAgain(g, t, f, same, from)
		approvedAgain(g, t, f, same, from)
		g.Expect(string(catBytes(g, t, f, api.Caller{Dir: same}, from, "a.txt"))).To(Equal("one\ntwo\n"))
	})

	t.Run("a file captured outside the review's paths does not answer", func(t *testing.T) {
		g := NewWithT(t)
		scoped := gitRepo(g, filepath.Join(t.TempDir(), "scoped"))
		write(g, scoped, "b.txt", "not claimed\n")
		from := git(g, scoped, "rev-parse", "main")
		id := approvedAgain(g, t, f, scoped, from)
		// The case means nothing unless the round did capture the file, out of
		// scope.
		var shown api.ShowResult
		f.ok(g, t, http.MethodGet, "/agent/reviews/"+itoa(id), nil, &shown)
		var outside []bool
		for _, file := range shown.Files {
			if file.Path == "b.txt" {
				outside = append(outside, !file.InScope)
			}
		}
		g.Expect(outside).To(Equal([]bool{true}))
		_, e := f.refused(g, t, http.MethodGet, catPath(api.Caller{Dir: scoped}, from, "b.txt"), nil)
		g.Expect(e.Code).To(Equal(api.CodeNotCaptured))
	})

	t.Run("a review that deleted the file does not make another's bytes ambiguous", func(t *testing.T) {
		g := NewWithT(t)
		gone := gitRepo(g, filepath.Join(t.TempDir(), "gone"))
		from := git(g, gone, "rev-parse", "main")
		g.Expect(os.Remove(filepath.Join(gone, "a.txt"))).To(Succeed())
		approvedAgain(g, t, f, gone, from)
		write(g, gone, "a.txt", "one\ntwo\n")
		approvedAgain(g, t, f, gone, from)
		g.Expect(string(catBytes(g, t, f, api.Caller{Dir: gone}, from, "a.txt"))).To(Equal("one\ntwo\n"))
	})

	t.Run("a repository eyeball has not seen is refused and not recorded", func(t *testing.T) {
		g := NewWithT(t)
		unseen := gitRepo(g, filepath.Join(t.TempDir(), "unseen"))
		head := git(g, unseen, "rev-parse", "main")
		before, err := f.store.Projects(t.Context())
		g.Expect(err).NotTo(HaveOccurred())
		_, e := f.refused(g, t, http.MethodGet, catPath(api.Caller{Dir: unseen}, head, "a.txt"), nil)
		g.Expect(e.Code).To(Equal(api.CodeNotCaptured))
		g.Expect(e.Message).To(Equal("no review has been opened in " + unseen))
		after, err := f.store.Projects(t.Context())
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(after).To(HaveLen(len(before)))
	})
}

// TestServer_checkAcceptsAnUnreferencedCommit pins what a gate that allows a
// change to one file builds on: check reads commits and not refs, so a commit no
// ref names, whose tree is the merged tree with one blob put back to the
// approved bytes, matches the approved review where the merged commit does not.
func TestServer_checkAcceptsAnUnreferencedCommit(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	base := git(g, repo, "rev-parse", "main")
	me := api.Caller{Dir: repo}
	approved := f.request(g, t, api.RequestBody{Caller: me, Paths: []string{"a.txt"}, Note: "first"}).Review.ID
	f.decide(g, t, approved, store.VerdictApprove, "")

	// The merged commit holds a.txt as it was changed after the approval.
	commitFile(g, repo, "a.txt", "one\ntwo\nafter the approval\n")
	merged := git(g, repo, "rev-parse", "main")
	check := func(to string) string {
		v := url.Values{"base": {base}, "new": {to}}
		return "/agent/check?" + query(me) + "&" + v.Encode()
	}
	_, e := f.refused(g, t, http.MethodGet, check(merged), nil)
	g.Expect(e.Code).To(Equal(api.CodeTreeDiffers), "the merged commit differs, so the swap below has something to fix")

	// The approved bytes, put back through a temporary index, so no ref and no
	// index of the repository moves.
	approvedBytes := catBytes(g, t, f, me, base, "a.txt")
	g.Expect(strings.TrimSpace(string(approvedBytes))).To(Equal("one\ntwo"))
	blobFile := filepath.Join(t.TempDir(), "approved")
	g.Expect(os.WriteFile(blobFile, approvedBytes, 0o600)).To(Succeed())
	blob := git(g, repo, "hash-object", "-w", blobFile)
	index := filepath.Join(t.TempDir(), "index")
	t.Setenv("GIT_INDEX_FILE", index)
	git(g, repo, "read-tree", merged)
	git(g, repo, "update-index", "--cacheinfo", "100644,"+blob+",a.txt")
	tree := git(g, repo, "write-tree")
	swapped := git(g, repo, "commit-tree", tree, "-p", merged, "-m", "the merged tree with the approved a.txt")
	t.Setenv("GIT_INDEX_FILE", "")

	g.Expect(git(g, repo, "for-each-ref", "--contains", swapped)).To(BeEmpty(), "no ref names the commit")
	var got api.CheckResult
	f.ok(g, t, http.MethodGet, check(swapped), nil, &got)
	g.Expect(got.Review).To(Equal(approved))
}
