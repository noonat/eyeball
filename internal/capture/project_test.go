package capture

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/blob"
	"github.com/noonat/eyeball/internal/store"
	. "github.com/onsi/gomega"
)

func Test_gitIgnoresAnotherRepositorysGitDir(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	mine := newRepo(g, filepath.Join(dir, "mine"))
	mine.write(g, "mine.txt", "a\n")
	mine.commit(g, "base")
	mine.write(g, "mine.txt", "b\n")
	other := newRepo(g, filepath.Join(dir, "other"))
	// Different content, so no object id exists in both repositories and a
	// read through the wrong one fails rather than finding the same blob.
	other.write(g, "other.txt", "other base\n")
	other.commit(g, "base")
	other.write(g, "other.txt", "other edit\n")

	// A daemon started from inside a git hook inherits these, and without the
	// scrub every capture would read the other repository.
	t.Setenv("GIT_DIR", filepath.Join(other.root, ".git"))
	t.Setenv("GIT_WORK_TREE", other.root)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other.root, ".git", "index"))

	base, err := ResolveBase(t.Context(), mine.root, "")
	g.Expect(err).NotTo(HaveOccurred())
	got, err := Changed(t.Context(), mine.root, base)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]Entry{{Path: "mine.txt"}}))

	// The base content is streamed by a second git call, which has to be
	// scrubbed as well.
	blobs, err := blob.Open(t.TempDir())
	g.Expect(err).NotTo(HaveOccurred())
	rc, source, err := NewReader(mine.root, base, blobs, nil).Frozen(t.Context(), "mine.txt")
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { _ = rc.Close() }()
	content, err := io.ReadAll(rc)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceBase))
	g.Expect(string(content)).To(Equal("a\n"))
}

func Test_scrubEnvKeepsWhatBelongsToTheUser(t *testing.T) {
	g := NewWithT(t)
	env := []string{
		"GIT_DIR=/x/.git",
		"GIT_CONFIG_GLOBAL=/home/a/.config/git/alt",
		"GIT_EXEC_PATH=/opt/git/libexec",
		"GIT_INDEX_FILE=/x/.git/index",
		"HOME=/home/a",
	}
	got := scrubEnv(env, localVars())
	g.Expect(got).To(Equal([]string{
		"GIT_CONFIG_GLOBAL=/home/a/.config/git/alt",
		"GIT_EXEC_PATH=/opt/git/libexec",
		"HOME=/home/a",
	}))
}

func TestAgentName(t *testing.T) {
	root := Root{Logical: "/link/harbor", Resolved: "/home/a/src/harbor"}
	cases := []struct {
		name     string
		explicit string
		env      string
		root     Root
		home     string
		want     string
	}{
		{
			name:     "the flag wins",
			explicit: "spec/012-surface",
			env:      "from-env",
			root:     root,
			home:     "/home/a",
			want:     "spec/012-surface",
		},
		{
			name: "then the environment",
			env:  "  from-env ",
			root: root,
			home: "/home/a",
			want: "from-env",
		},
		{
			name: "then the resolved root under home",
			root: root,
			home: "/home/a",
			want: "~/src/harbor",
		},
		{
			name: "a root outside home keeps its path",
			root: Root{Logical: "/srv/x", Resolved: "/srv/x"},
			home: "/home/a",
			want: "/srv/x",
		},
		{
			name: "a sibling of home is not under it",
			root: Root{Logical: "/home/ab/x", Resolved: "/home/ab/x"},
			home: "/home/a",
			want: "/home/ab/x",
		},
		{
			name: "home itself",
			root: Root{Logical: "/home/a", Resolved: "/home/a"},
			home: "/home/a",
			want: "~",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(AgentName(c.explicit, c.env, c.root, c.home)).To(Equal(c.want))
		})
	}
}

func TestFindRoot(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	r := newRepo(g, filepath.Join(dir, "harbor"))
	deep := filepath.Join(r.root, "internal", "store")
	g.Expect(os.MkdirAll(deep, 0o700)).To(Succeed())

	got, err := FindRoot(deep)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal(Root{Logical: r.root, Resolved: r.root}))
}

func TestFindRoot_markerInsideARepository(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	r := newRepo(g, filepath.Join(dir, "harbor"))
	docs := filepath.Join(r.root, "docs")
	g.Expect(os.MkdirAll(filepath.Join(docs, "a"), 0o700)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(docs, Marker), nil, 0o600)).To(Succeed())

	got, err := FindRoot(filepath.Join(docs, "a"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Logical).To(Equal(docs), "the nearest of the two wins")
}

func TestFindRoot_noProject(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	_, err := FindRoot(dir)
	g.Expect(errors.Is(err, ErrNoProject)).To(BeTrue())
	g.Expect(err.Error()).To(ContainSubstring(Marker))

	_, err = FindRoot("relative/dir")
	g.Expect(err).To(HaveOccurred())
}

func TestFindRoot_symlinkedDirectory(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	r := newRepo(g, filepath.Join(dir, "real", "harbor"))
	g.Expect(os.MkdirAll(filepath.Join(r.root, "sub"), 0o700)).To(Succeed())
	link := filepath.Join(dir, "link")
	g.Expect(os.Symlink(r.root, link)).To(Succeed())

	got, err := FindRoot(filepath.Join(link, "sub"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal(Root{Logical: link, Resolved: r.root}))
}

func TestFindRoot_symlinkedDirectoryWithReviewPath(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	r := newRepo(g, filepath.Join(dir, "real", "harbor"))
	g.Expect(os.MkdirAll(filepath.Join(r.root, "sub"), 0o700)).To(Succeed())
	link := filepath.Join(dir, "link")
	g.Expect(os.Symlink(r.root, link)).To(Succeed())
	cwd := filepath.Join(link, "sub")

	root, err := FindRoot(cwd)
	g.Expect(err).NotTo(HaveOccurred())
	relative, err := ReviewPath(root, cwd, "a.go")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(relative).To(Equal("sub/a.go"))
	absolute, err := ReviewPath(root, cwd, filepath.Join(cwd, "b.go"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(absolute).To(Equal("sub/b.go"))
}

func TestProjectName(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	r := newRepo(g, filepath.Join(dir, "harbor"))
	r.write(g, "a.txt", "a\n")
	r.commit(g, "base")

	plain := filepath.Join(dir, "notes")
	g.Expect(os.MkdirAll(plain, 0o700)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(plain, Marker), nil, 0o600)).To(Succeed())

	linked := filepath.Join(dir, "harbor-012")
	r.git(g, "worktree", "add", "--quiet", linked)

	bare := &repo{root: filepath.Join(dir, "shared.git")}
	r.git(g, "clone", "--quiet", "--bare", r.root, bare.root)
	fromBare := filepath.Join(dir, "shared-wt")
	bare.git(g, "worktree", "add", "--quiet", fromBare)

	dotBare := &repo{root: filepath.Join(dir, "proj", ".bare")}
	r.git(g, "clone", "--quiet", "--bare", r.root, dotBare.root)
	fromDotBare := filepath.Join(dir, "proj", "main")
	dotBare.git(g, "worktree", "add", "--quiet", fromDotBare)

	super := newRepo(g, filepath.Join(dir, "super"))
	super.git(g, "-c", "protocol.file.allow=always", "submodule", "--quiet", "add", r.root, "vendored")
	sub := filepath.Join(super.root, "vendored")

	cases := []struct {
		name string
		root string
		want string
	}{
		{
			name: "a main checkout",
			root: r.root,
			want: "harbor",
		},
		{
			name: "a directory outside git",
			root: plain,
			want: "notes",
		},
		{
			name: "a linked worktree is named for its repository",
			root: linked,
			want: "harbor",
		},
		{
			name: "a worktree of a bare clone loses the .git suffix",
			root: fromBare,
			want: "shared",
		},
		{
			name: "a worktree of a .bare directory takes its parent's name",
			root: fromDotBare,
			want: "proj",
		},
		{
			name: "a submodule keeps its own name",
			root: sub,
			want: "vendored",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := ProjectName(t.Context(), c.root)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(c.want))
		})
	}
}

func TestReviewPath(t *testing.T) {
	root := Root{Logical: "/link/harbor", Resolved: "/real/harbor"}
	cases := []struct {
		name string
		cwd  string
		path string
		want string
	}{
		{
			name: "relative to a subdirectory",
			cwd:  "/link/harbor/internal",
			path: "store/a.go",
			want: "internal/store/a.go",
		},
		{
			name: "relative, climbing within the root",
			cwd:  "/link/harbor/internal",
			path: "../docs",
			want: "docs",
		},
		{
			name: "absolute through the link",
			cwd:  "/link/harbor/internal",
			path: "/link/harbor/docs/product.md",
			want: "docs/product.md",
		},
		{
			name: "absolute in the resolved spelling",
			cwd:  "/link/harbor",
			path: "/real/harbor/docs",
			want: "docs",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := ReviewPath(root, c.cwd, c.path)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(c.want))
		})
	}

	_, err := ReviewPath(root, "/link/harbor", " ")
	g := NewWithT(t)
	g.Expect(errors.Is(err, store.ErrInvalidPath)).To(BeTrue())

	outside := []string{"../../elsewhere", "/srv/other/x", "/link/harbor-other/x"}
	for _, p := range outside {
		t.Run("refuses "+p, func(t *testing.T) {
			g := NewWithT(t)
			_, err := ReviewPath(root, "/link/harbor/internal", p)
			g.Expect(errors.Is(err, ErrOutsideProject)).To(BeTrue())
		})
	}
}
