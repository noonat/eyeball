package capture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/noonat/eyeball/internal/store"
	. "github.com/onsi/gomega"
)

// whole is a review of every path the cases touch, claimed one by one, since
// a review cannot claim the project root itself.
var whole = []string{"a.txt", "b.txt", "gone.txt", "link", "lib", "docs", "notes"}

func TestMatch(t *testing.T) {
	cases := []struct {
		name string
		// paths are the review's paths.
		paths []string
		// work is the agent's change, captured as the approved round.
		work func(g *WithT, r *repo)
		// land turns the working tree into the commit checked, after capture.
		land func(g *WithT, r *repo)
		want string
	}{
		{
			name:  "a commit of exactly what was approved",
			paths: whole,
			work: func(g *WithT, r *repo) {
				r.write(g, "a.txt", "a, changed\n")
				r.write(g, "docs/new.txt", "new\n")
				g.Expect(os.Remove(filepath.Join(r.root, "gone.txt"))).To(Succeed())
			},
		},
		{
			name:  "a symlink retargeted",
			paths: whole,
			work: func(g *WithT, r *repo) {
				g.Expect(os.Remove(filepath.Join(r.root, "link"))).To(Succeed())
				g.Expect(os.Symlink("b.txt", filepath.Join(r.root, "link"))).To(Succeed())
			},
		},
		{
			name:  "one file differing",
			paths: whole,
			work:  func(g *WithT, r *repo) { r.write(g, "a.txt", "a, approved\n") },
			land:  func(g *WithT, r *repo) { r.write(g, "a.txt", "a, changed after\n") },
			want:  "a.txt differs from the approved round",
		},
		{
			name:  "a path the round did not capture",
			paths: whole,
			work:  func(g *WithT, r *repo) { r.write(g, "a.txt", "a, approved\n") },
			land:  func(g *WithT, r *repo) { r.write(g, "extra.txt", "slipped in\n") },
			want:  "extra.txt changes in the new tree and is not in the approved round",
		},
		{
			name:  "a captured deletion the new tree keeps",
			paths: whole,
			work: func(g *WithT, r *repo) {
				g.Expect(os.Remove(filepath.Join(r.root, "gone.txt"))).To(Succeed())
			},
			land: func(g *WithT, r *repo) { r.write(g, "gone.txt", "gone\n") },
			want: "the approved round deletes gone.txt, and the new tree keeps it",
		},
		{
			name:  "a deletion the round did not approve",
			paths: whole,
			work:  func(g *WithT, r *repo) { r.write(g, "b.txt", "b, approved\n") },
			land: func(g *WithT, r *repo) {
				g.Expect(os.Remove(filepath.Join(r.root, "b.txt"))).To(Succeed())
			},
			want: "the new tree deletes b.txt, and the approved round keeps it",
		},
		{
			name:  "an in-scope stray the new tree lacks",
			paths: whole,
			work: func(g *WithT, r *repo) {
				r.write(g, "a.txt", "a, approved\n")
				r.write(g, "notes/stray.log", "scratch\n")
			},
			land: func(g *WithT, r *repo) {
				g.Expect(os.RemoveAll(filepath.Join(r.root, "notes"))).To(Succeed())
			},
			want: "the approved round holds notes/stray.log, and the new tree does not",
		},
		{
			name:  "an approved change the new tree left out",
			paths: whole,
			work: func(g *WithT, r *repo) {
				r.write(g, "a.txt", "a, approved\n")
				r.write(g, "b.txt", "b, approved\n")
			},
			land: func(g *WithT, r *repo) { r.write(g, "b.txt", "b\n") },
			want: "the approved round changes b.txt, and the new tree does not",
		},
		{
			name:  "an out-of-scope change the new tree makes",
			paths: []string{"lib"},
			work: func(g *WithT, r *repo) {
				r.write(g, "lib/x.go", "package lib // changed\n")
				r.write(g, "a.txt", "a, outside the review\n")
			},
			want: "a.txt changes in the new tree and is outside the approved review's paths",
		},
		{
			name:  "an executable approved as one",
			paths: whole,
			work: func(g *WithT, r *repo) {
				r.write(g, "lib/run.sh", "#!/bin/sh\n")
				g.Expect(os.Chmod(filepath.Join(r.root, "lib/run.sh"), 0o700)).To(Succeed())
			},
		},
		{
			name:  "an execute bit set after approval",
			paths: whole,
			work:  func(g *WithT, r *repo) { r.write(g, "a.txt", "a, approved\n") },
			land: func(g *WithT, r *repo) {
				g.Expect(os.Chmod(filepath.Join(r.root, "a.txt"), 0o700)).To(Succeed())
			},
			want: "a.txt is an executable file in the new tree and a regular file in the approved round",
		},
		{
			name:  "a file turned into a symlink with the same bytes",
			paths: whole,
			work:  func(g *WithT, r *repo) { r.write(g, "a.txt", "b.txt") },
			land: func(g *WithT, r *repo) {
				g.Expect(os.Remove(filepath.Join(r.root, "a.txt"))).To(Succeed())
				g.Expect(os.Symlink("b.txt", filepath.Join(r.root, "a.txt"))).To(Succeed())
			},
			want: "a.txt is a symlink in the new tree and a regular file in the approved round",
		},
		{
			name:  "a symlink turned into a file with the same bytes",
			paths: whole,
			work: func(g *WithT, r *repo) {
				g.Expect(os.Remove(filepath.Join(r.root, "link"))).To(Succeed())
				g.Expect(os.Symlink("b.txt", filepath.Join(r.root, "link"))).To(Succeed())
			},
			land: func(g *WithT, r *repo) {
				g.Expect(os.Remove(filepath.Join(r.root, "link"))).To(Succeed())
				r.write(g, "link", "b.txt")
			},
			want: "link is a regular file in the new tree and a symlink in the approved round",
		},
		{
			name:  "an out-of-scope capture the new tree leaves alone",
			paths: []string{"lib"},
			work: func(g *WithT, r *repo) {
				r.write(g, "lib/x.go", "package lib // changed\n")
				r.write(g, "a.txt", "a, outside the review\n")
			},
			land: func(g *WithT, r *repo) { r.write(g, "a.txt", "a\n") },
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			r, base := matchRepo(g, t)
			c.work(g, r)
			files := approve(g, t, r, base, c.paths)
			if c.land != nil {
				c.land(g, r)
			}
			target := r.commit(g, "work")
			got, err := Match(t.Context(), r.root, base, target, files)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(c.want))
		})
	}
}

func TestMatch_mergeCommit(t *testing.T) {
	// A merge commit and the fast-forward it could have been hold the same
	// tree, so both match the round that approved the branch.
	g := NewWithT(t)
	r, base := matchRepo(g, t)
	r.git(g, "checkout", "--quiet", "-b", "work")
	r.write(g, "a.txt", "a, on the branch\n")
	files := approve(g, t, r, base, whole)
	r.git(g, "add", "-A")
	r.git(g, "commit", "--quiet", "--no-verify", "-m", "work")
	r.git(g, "checkout", "--quiet", "main")
	r.git(g, "merge", "--quiet", "--no-ff", "--no-edit", "--no-verify", "work")
	merge := r.head()
	g.Expect(r.git(g, "show", "--no-patch", "--format=%P", merge)).To(ContainSubstring(" "), "a merge commit has two parents")

	got, err := Match(t.Context(), r.root, base, merge, files)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(BeEmpty())
	got, err = Match(t.Context(), r.root, base, r.git(g, "rev-parse", "work"), files)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(BeEmpty())
}

func TestMatch_replaceRefs(t *testing.T) {
	// git replace swaps one object for another in every read that honors
	// refs/replace. A commit the branch moves to must be compared as itself,
	// not as the approved commit an agent swapped in for it.
	g := NewWithT(t)
	r, base := matchRepo(g, t)
	r.write(g, "a.txt", "a, approved\n")
	files := approve(g, t, r, base, whole)
	approved := r.commit(g, "approved")
	r.write(g, "a.txt", "a, not approved\n")
	evil := r.commit(g, "evil")

	r.git(g, "replace", evil, approved)
	got, err := Match(t.Context(), r.root, base, evil, files)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal("a.txt differs from the approved round"), "a replaced commit")
	r.git(g, "replace", "-d", evil)

	r.git(g, "replace", r.git(g, "rev-parse", evil+":a.txt"), r.git(g, "rev-parse", approved+":a.txt"))
	got, err = Match(t.Context(), r.root, base, evil, files)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal("a.txt differs from the approved round"), "a replaced blob")
}

func TestMatch_roundWithoutModes(t *testing.T) {
	// A round taken before modes were kept is read as having captured regular
	// files, so a regular file still matches and an executable fails closed.
	g := NewWithT(t)
	r, base := matchRepo(g, t)
	r.write(g, "a.txt", "a, approved\n")
	r.write(g, "lib/run.sh", "#!/bin/sh\n")
	g.Expect(os.Chmod(filepath.Join(r.root, "lib/run.sh"), 0o700)).To(Succeed())
	files := approve(g, t, r, base, whole)
	for i := range files {
		files[i].Mode = ""
	}
	target := r.commit(g, "work")
	got, err := Match(t.Context(), r.root, base, target, files)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal("lib/run.sh is an executable file in the new tree and a regular file in the approved round"))
}

func TestMatch_submodule(t *testing.T) {
	// No capture records a submodule, so a commit that adds one never matches,
	// and the sentence says why rather than calling it uncaptured.
	g := NewWithT(t)
	other := newRepo(g, filepath.Join(t.TempDir(), "other"))
	other.write(g, "x.txt", "x\n")
	other.commit(g, "other")
	r, base := matchRepo(g, t)
	addSubmodule(g, r, other.root, "lib/vendored")
	files := approve(g, t, r, base, append([]string{".gitmodules"}, whole...))
	target := r.commit(g, "work")
	got, err := Match(t.Context(), r.root, base, target, files)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal("lib/vendored is a submodule, which no round captures"))
}

func TestResolveCommit(t *testing.T) {
	g := NewWithT(t)
	r, base := matchRepo(g, t)
	got, err := ResolveCommit(t.Context(), r.root, "main")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal(base))
	_, err = ResolveCommit(t.Context(), r.root, "no-such-branch")
	g.Expect(err).To(MatchError(ErrBadCommit))
	tree := r.git(g, "rev-parse", "main^{tree}")
	_, err = ResolveCommit(t.Context(), r.root, tree)
	g.Expect(err).To(MatchError(ErrBadCommit), "a tree is not a commit")
}

// matchRepo is a repository with a committed base for a round to be taken
// against: files to change, one to delete, a symlink and a directory a review
// can name alone.
func matchRepo(g *WithT, t *testing.T) (*repo, string) {
	g.THelper()
	r := newRepo(g, filepath.Join(t.TempDir(), "harbor"))
	r.write(g, "a.txt", "a\n")
	r.write(g, "b.txt", "b\n")
	r.write(g, "gone.txt", "gone\n")
	r.write(g, "lib/x.go", "package lib\n")
	g.Expect(os.Symlink("a.txt", filepath.Join(r.root, "link"))).To(Succeed())
	return r, r.commit(g, "base")
}

// approve captures the working tree against base as a first round, marking
// each file in scope as the store would.
func approve(g *WithT, t *testing.T, r *repo, base string, paths []string) []store.RoundFile {
	g.THelper()
	blobs, _ := newBlobs(g, t)
	req, err := Freeze(t.Context(), Options{Root: r.root, Base: base, Paths: paths, Round: 1, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	files := make([]store.RoundFile, 0, len(req.Files))
	for _, f := range req.Files {
		files = append(files, store.RoundFile{File: f, InScope: store.Covers(paths, f.Path)})
	}
	return files
}
