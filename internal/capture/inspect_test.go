package capture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"

	"github.com/noonat/eyeball/internal/store"
)

func TestBaseGone(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())
	r.write(g, "a.txt", "a\n")
	base := r.commit(g, "base")

	gone, err := BaseGone(t.Context(), r.root, base)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(gone).To(BeFalse())

	// An id never written is what a rebase and a garbage collection leave a
	// round holding, and cat-file -e exits 1 for it.
	gone, err = BaseGone(t.Context(), r.root, "0123456789012345678901234567890123456789")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(gone).To(BeTrue())
}

func TestBaseGone_notARepository(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	// The ceiling keeps git from finding a repository above the temporary
	// directory, so the call fails here on every machine.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	// cat-file exits 128 outside a repository. Reading that as gone would
	// send a broken repository to a live read of its whole working copy.
	gone, err := BaseGone(t.Context(), dir, "0123456789012345678901234567890123456789")
	g.Expect(err).To(HaveOccurred())
	g.Expect(gone).To(BeFalse())
}

func TestCheckoutAround(t *testing.T) {
	setup := NewWithT(t)
	dir := t.TempDir()
	outer := newRepo(setup, filepath.Join(dir, "outer"))
	writeFile(setup, outer.root, "notes/.eyeball", "")
	writeFile(setup, outer.root, "sub/plain.txt", "")
	both := newRepo(setup, filepath.Join(dir, "both"))
	writeFile(setup, both.root, Marker, "")
	writeFile(setup, dir, "alone/.eyeball", "")

	roots := []struct {
		name string
		root string
		want bool
	}{
		{
			name: "a marker with a .git above it",
			root: filepath.Join(outer.root, "notes"),
			want: true,
		},
		{
			name: "a marker beside a .git at the root",
			root: both.root,
			want: true,
		},
		{
			name: "a marker with no .git anywhere above",
			root: filepath.Join(dir, "alone"),
			want: false,
		},
		{
			name: "a git project with no marker",
			root: outer.root,
			want: false,
		},
		{
			name: "a directory with no marker inside a checkout",
			root: filepath.Join(outer.root, "sub"),
			want: false,
		},
	}
	for _, c := range roots {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := CheckoutAround(c.root)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(c.want))
		})
	}
}

func TestInspect(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	r := newRepo(g, filepath.Join(dir, "repo"))
	r.write(g, "a.txt", "a\n")
	base := r.commit(g, "base")
	writeFile(g, dir, "outside/.eyeball", "")

	states := []struct {
		name string
		root string
		base string
		want State
	}{
		{
			name: "a git project with its base",
			root: r.root,
			base: base,
			want: State{Root: r.root, Base: base, Live: LiveIgnoreRules},
		},
		{
			name: "a git project whose base is gone",
			root: r.root,
			base: "0123456789012345678901234567890123456789",
			want: State{Root: r.root, Base: "0123456789012345678901234567890123456789", Live: LiveIgnoreRules, BaseGone: true},
		},
		{
			name: "a git project asked about no base",
			root: r.root,
			base: "",
			want: State{Root: r.root, Live: LiveIgnoreRules},
		},
		{
			name: "a project outside git never seen in a checkout",
			root: filepath.Join(dir, "outside"),
			base: "",
			want: State{Root: filepath.Join(dir, "outside"), Live: LiveOpen},
		},
	}
	for _, c := range states {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := Inspect(t.Context(), c.root, c.base, false)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(c.want))
		})
	}
}

func TestInspect_baseUnreadable(t *testing.T) {
	g := NewWithT(t)
	r := newRepo(g, t.TempDir())

	// A base that is not an object name at all makes cat-file exit 128, and
	// with nothing saying which reader to build, that is an error.
	_, err := Inspect(t.Context(), r.root, "not-an-object", false)
	g.Expect(err).To(HaveOccurred())
}

func TestInspect_checkoutGone(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	dir := t.TempDir()
	outer, notes := markerInCheckout(g, dir)
	st := openStore(g, t)
	project, err := st.EnsureProject(t.Context(), notes, "notes")
	g.Expect(err).NotTo(HaveOccurred())
	blobs, _ := newBlobs(g, t)

	// Each run reads the flag back from the store and writes what Inspect
	// found, as a handler does. Nothing seeds the flag.
	inspect := func() State {
		g.THelper()
		project, err = st.Project(t.Context(), project.ID)
		g.Expect(err).NotTo(HaveOccurred())
		got, err := Inspect(t.Context(), notes, "", project.InsideCheckout)
		g.Expect(err).NotTo(HaveOccurred())
		if got.FoundCheckout {
			g.Expect(st.MarkInsideCheckout(t.Context(), project.ID)).To(Succeed())
		}
		return got
	}

	first := inspect()
	g.Expect(first.FoundCheckout).To(BeTrue())
	g.Expect(first.Live).To(Equal(LiveIgnoreRules))

	// With the outer .git moved away, what is left above the project is its
	// working files, and nothing on the disk says a repository was there. A
	// second run and a third both refuse, so the flag was not cleared by the
	// run that found nothing.
	g.Expect(os.Rename(filepath.Join(outer.root, gitDir), filepath.Join(dir, "moved.git"))).To(Succeed())
	second := inspect()
	g.Expect(second.FoundCheckout).To(BeFalse())
	g.Expect(second.Live).To(Equal(LiveCheckoutGone))
	third := inspect()
	g.Expect(third.Live).To(Equal(LiveCheckoutGone))

	_, _, err = third.Reader(blobs, nil).Open(t.Context(), "creds.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestInspect_ignoredMarkerProject(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	outer, _ := markerInCheckout(g, t.TempDir())
	ignored := filepath.Join(outer.root, "ign")
	blobs, _ := newBlobs(g, t)
	req, err := Freeze(t.Context(), Options{Round: 1, Root: ignored, Paths: []string{"plan.md"}, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	writeFile(g, ignored, "plan.md", "moved on since the round\n")

	got, err := Inspect(t.Context(), ignored, "", false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Live).To(Equal(LiveIgnored))
	g.Expect(got.FoundCheckout).To(BeTrue())

	// The outer checkout ignores the whole project, so nothing in it reads
	// live. What the round captured still opens.
	reader := got.Reader(blobs, req.Files)
	rc, source, err := reader.Open(t.Context(), "plan.md")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceRound))
	g.Expect(readAll(g, rc)).To(Equal("the plan\n"))
	_, _, err = reader.Open(t.Context(), "other.md")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestInspect_lostRepository(t *testing.T) {
	dir := t.TempDir()
	setup := NewWithT(t)

	// A git project inside an outer repository, with a base, whose own .git
	// was moved away. Without the lost check, cat-file asks the outer
	// repository, which lacks the base, and the project reads as a gone base
	// with live reads allowed under the outer repository's rules.
	outer := newRepo(setup, filepath.Join(dir, "outer"))
	outer.write(setup, "readme.txt", "outer\n")
	outer.commit(setup, "outer")
	nested := newRepo(setup, filepath.Join(outer.root, "nested"))
	nested.write(setup, "a.txt", "a\n")
	nestedBase := nested.commit(setup, "base")
	setup.Expect(os.Rename(filepath.Join(nested.root, gitDir), filepath.Join(dir, "nested.git"))).To(Succeed())

	// A branch with no commit records no base, so the test cannot depend on
	// one: with .git moved, the root holds neither entry FindRoot recorded it
	// for.
	unborn := newRepo(setup, filepath.Join(dir, "unborn"))
	unborn.write(setup, ".gitignore", "private.txt\n")
	unborn.write(setup, "private.txt", "ignored\n")
	setup.Expect(os.Rename(filepath.Join(unborn.root, gitDir), filepath.Join(dir, "unborn.git"))).To(Succeed())

	// A linked worktree whose git directory was removed. Every git call there
	// exits 128, so a base check would make every route an error.
	main := newRepo(setup, filepath.Join(dir, "main"))
	main.write(setup, "a.txt", "a\n")
	mainBase := main.commit(setup, "base")
	linked := filepath.Join(dir, "linked")
	main.git(setup, "worktree", "add", "--quiet", linked)
	setup.Expect(os.RemoveAll(filepath.Join(main.root, gitDir, "worktrees"))).To(Succeed())

	// A pointer git cannot parse is refused by git as an invalid gitfile.
	garbled := filepath.Join(dir, "garbled")
	writeFile(setup, garbled, gitDir, "not a pointer\n")

	// A .git link whose target is gone, which git walks past to whatever
	// repository is above.
	dangling := filepath.Join(outer.root, "dangling")
	setup.Expect(os.MkdirAll(dangling, 0o700)).To(Succeed())
	setup.Expect(os.Symlink(filepath.Join(dir, "nowhere.git"), filepath.Join(dangling, gitDir))).To(Succeed())

	roots := []struct {
		name string
		root string
		base string
		want Live
	}{
		{
			name: "a missing root",
			root: filepath.Join(dir, "nowhere"),
			base: nestedBase,
			want: LiveRootMissing,
		},
		{
			name: "a root that is a file",
			root: filepath.Join(dir, "garbled", gitDir),
			base: "",
			want: LiveRootMissing,
		},
		{
			name: "a root below a file",
			root: filepath.Join(dir, "garbled", gitDir, "sub"),
			base: "",
			want: LiveRootMissing,
		},
		{
			name: "a moved .git with a base",
			root: nested.root,
			base: nestedBase,
			want: LiveLost,
		},
		{
			name: "an unborn branch with its .git moved",
			root: unborn.root,
			base: "",
			want: LiveLost,
		},
		{
			name: "a worktree whose git directory is gone",
			root: linked,
			base: mainBase,
			want: LiveLost,
		},
		{
			name: "a .git link whose target is gone",
			root: dangling,
			base: "",
			want: LiveLost,
		},
		{
			name: "a .git file that is no pointer",
			root: garbled,
			base: "",
			want: LiveLost,
		},
	}
	for _, c := range roots {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := Inspect(t.Context(), c.root, c.base, false)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got.Live).To(Equal(c.want))
			g.Expect(got.BaseGone).To(BeFalse(), "a lost repository is never asked about its base")
		})
	}

	// The unborn branch's ignored file, which no rule is left to refuse once
	// its .git is gone.
	g := NewWithT(t)
	blobs, _ := newBlobs(g, t)
	got, err := Inspect(t.Context(), unborn.root, "", false)
	g.Expect(err).NotTo(HaveOccurred())
	_, _, err = got.Reader(blobs, nil).Open(t.Context(), "private.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestInspect_markerProject(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	outer, notes := markerInCheckout(g, t.TempDir())

	// notes is not ignored by the outer checkout, so each path is asked about
	// in turn.
	got, err := Inspect(t.Context(), notes, "", false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Live).To(Equal(LiveIgnoreRules))
	g.Expect(got.FoundCheckout).To(BeTrue())
	g.Expect(got.RulesErr).NotTo(HaveOccurred())

	// The ceiling stops git short of the .git the walk found, so check-ignore
	// exits 128. That is an answer saying the rules cannot be read, not a
	// reason to skip them.
	t.Setenv("GIT_CEILING_DIRECTORIES", outer.root)
	got, err = Inspect(t.Context(), notes, "", false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Live).To(Equal(LiveRulesUnreadable))
	g.Expect(got.FoundCheckout).To(BeTrue())
	g.Expect(got.RulesErr).To(HaveOccurred())
}

func TestInspect_markerWithBase(t *testing.T) {
	g := NewWithT(t)
	root := t.TempDir()
	writeFile(g, root, Marker, "")
	// Any git call fails with git off PATH, so an answer here was reached
	// from the filesystem alone.
	t.Setenv("PATH", "")

	// A marker project never records a base. A root holding both entries keeps
	// its marker after the .git is moved away, and a base says it was
	// captured as a git project, so it has lost its repository.
	got, err := Inspect(t.Context(), root, "0123456789012345678901234567890123456789", false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Live).To(Equal(LiveLost))
}

func TestInspect_relativeGitPointer(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	g.Expect(os.MkdirAll(filepath.Join(dir, "modules", "lib"), 0o700)).To(Succeed())
	writeFile(g, dir, "lib/.git", "gitdir: ../modules/lib\n")

	// A submodule writes its pointer relative to its own root. Read against
	// any other directory, every submodule would read as lost.
	got, err := Inspect(t.Context(), filepath.Join(dir, "lib"), "", false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Live).To(Equal(LiveIgnoreRules))
}

func TestInspect_unbornWithMarker(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	dir := t.TempDir()
	r := newRepo(g, filepath.Join(dir, "project"))
	r.write(g, Marker, "")
	r.write(g, ".gitignore", "private.txt\n")
	r.write(g, "private.txt", "a secret\n")
	st := openStore(g, t)
	project, err := st.EnsureProject(t.Context(), r.root, "project")
	g.Expect(err).NotTo(HaveOccurred())
	blobs, _ := newBlobs(g, t)

	// On a branch with no commit the round records no base, and the .git and
	// the marker sit side by side at the root. The sighting is written while
	// the .git is still there.
	first, err := Inspect(t.Context(), r.root, "", project.InsideCheckout)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(first.FoundCheckout).To(BeTrue())
	if first.FoundCheckout {
		g.Expect(st.MarkInsideCheckout(t.Context(), project.ID)).To(Succeed())
	}

	// Once the .git is moved the root holds only its marker, with nothing
	// above it, and no base says a repository was there. Only the flag does.
	g.Expect(os.Rename(filepath.Join(r.root, gitDir), filepath.Join(dir, "moved.git"))).To(Succeed())
	project, err = st.Project(t.Context(), project.ID)
	g.Expect(err).NotTo(HaveOccurred())
	second, err := Inspect(t.Context(), r.root, "", project.InsideCheckout)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(second.Live).To(Equal(LiveCheckoutGone))
	_, _, err = second.Reader(blobs, nil).Open(t.Context(), "private.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestLive_Refused(t *testing.T) {
	values := []struct {
		name string
		live Live
		want bool
	}{
		{
			name: "open",
			live: LiveOpen,
			want: false,
		},
		{
			name: "ignore rules",
			live: LiveIgnoreRules,
			want: false,
		},
		{
			name: "root missing",
			live: LiveRootMissing,
			want: true,
		},
		{
			name: "lost",
			live: LiveLost,
			want: true,
		},
		{
			name: "ignored",
			live: LiveIgnored,
			want: true,
		},
		{
			name: "checkout gone",
			live: LiveCheckoutGone,
			want: true,
		},
		{
			name: "rules unreadable",
			live: LiveRulesUnreadable,
			want: true,
		},
		{
			name: "a value nothing set",
			live: "",
			want: true,
		},
	}
	for _, c := range values {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(c.live.Refused()).To(Equal(c.want))
		})
	}
}

func TestState_Reader(t *testing.T) {
	g := NewWithT(t)
	r, blobs := readerRepo(g, t)
	base := r.commit(g, "base")
	r.write(g, "changed.txt", "captured\n")
	req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	r.write(g, "appeared-later.txt", "only on disk\n")

	// A gone base gives the reader no base, so a file the capture does not
	// hold reads live rather than failing on a commit git no longer has.
	gone, err := Inspect(t.Context(), r.root, "0123456789012345678901234567890123456789", false)
	g.Expect(err).NotTo(HaveOccurred())
	rc, source, err := gone.Reader(blobs, req.Files).Open(t.Context(), "untouched.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceWorking))
	g.Expect(readAll(g, rc)).To(Equal("untouched\n"))

	// A base that exists is read as before.
	kept, err := Inspect(t.Context(), r.root, base, false)
	g.Expect(err).NotTo(HaveOccurred())
	rc, source, err = kept.Reader(blobs, req.Files).Open(t.Context(), "untouched.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceBase))
	g.Expect(readAll(g, rc)).To(Equal("untouched\n"))
	rc, source, err = kept.Reader(blobs, req.Files).Open(t.Context(), "appeared-later.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceWorking))
	g.Expect(readAll(g, rc)).To(Equal("only on disk\n"))
}

func TestState_Reader_lostRepository(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	r := newRepo(g, filepath.Join(dir, "project"))
	r.write(g, ".gitignore", "private.txt\n")
	r.write(g, "untouched.txt", "untouched\n")
	base := r.commit(g, "base")
	r.write(g, "changed.txt", "captured\n")
	r.write(g, "private.txt", "a secret\n")
	blobs, _ := newBlobs(g, t)
	req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(os.Rename(filepath.Join(r.root, gitDir), filepath.Join(dir, "moved.git"))).To(Succeed())

	got, err := Inspect(t.Context(), r.root, base, false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Live).To(Equal(LiveLost))
	reader := got.Reader(blobs, req.Files)

	// The capture alone answers. private.txt was ignored by the repository
	// that is gone, and with no rule of its own left to refuse it, it would
	// otherwise read live.
	rc, source, err := reader.Open(t.Context(), "changed.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceRound))
	g.Expect(readAll(g, rc)).To(Equal("captured\n"))
	refused := []string{"private.txt", "untouched.txt"}
	for _, p := range refused {
		_, _, err = reader.Open(t.Context(), p)
		g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue(), p)
	}
}

func TestState_Reader_missingRoot(t *testing.T) {
	g := NewWithT(t)
	r, blobs := readerRepo(g, t)
	base := r.commit(g, "base")
	r.write(g, "changed.txt", "captured\n")
	req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(os.RemoveAll(r.root)).To(Succeed())

	got, err := Inspect(t.Context(), r.root, base, false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Live).To(Equal(LiveRootMissing))
	reader := got.Reader(blobs, req.Files)

	// The capture alone answers. There is no repository to read the base
	// from, so a file only the base held is not found rather than a git
	// failure.
	rc, source, err := reader.Open(t.Context(), "changed.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceRound))
	g.Expect(readAll(g, rc)).To(Equal("captured\n"))
	_, _, err = reader.Open(t.Context(), "untouched.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

func TestState_Reader_rulesUnreadable(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	outer, notes := markerInCheckout(g, t.TempDir())
	blobs, _ := newBlobs(g, t)
	req, err := Freeze(t.Context(), Options{Round: 1, Root: notes, Paths: []string{"plan.md"}, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	t.Setenv("GIT_CEILING_DIRECTORIES", outer.root)

	got, err := Inspect(t.Context(), notes, "", false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Live).To(Equal(LiveRulesUnreadable))
	reader := got.Reader(blobs, req.Files)

	// A failed ignore check refuses the one source the rules protect. The
	// captured file does not depend on them and still opens, and creds.txt,
	// which the outer checkout ignores, does not read live.
	rc, source, err := reader.Open(t.Context(), "plan.md")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceRound))
	g.Expect(readAll(g, rc)).To(Equal("the plan\n"))
	_, _, err = reader.Open(t.Context(), "creds.txt")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
}

// markerInCheckout builds a repository at dir/outer holding two marker
// projects: notes, which the repository does not ignore, and ign, which it
// ignores as a whole. Its .gitignore also names creds.txt anywhere. It returns
// the repository and the notes root.
func markerInCheckout(g *WithT, dir string) (*repo, string) {
	g.THelper()
	outer := newRepo(g, filepath.Join(dir, "outer"))
	outer.write(g, ".gitignore", "ign/\ncreds.txt\n")
	outer.commit(g, "base")
	notes := filepath.Join(outer.root, "notes")
	outer.write(g, "notes/"+Marker, "")
	outer.write(g, "notes/plan.md", "the plan\n")
	outer.write(g, "notes/creds.txt", "a secret\n")
	outer.write(g, "ign/"+Marker, "")
	outer.write(g, "ign/plan.md", "the plan\n")
	outer.write(g, "ign/other.md", "not captured\n")
	return outer, notes
}

// openStore opens a migrated store for a test that writes what Inspect found
// between runs, as a handler does.
func openStore(g *WithT, t *testing.T) *store.Store {
	g.THelper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "eyeball.db"))
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = st.Close() })
	return st
}
