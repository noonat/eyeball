package capture

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"

	"github.com/noonat/eyeball/internal/blob"
	"github.com/noonat/eyeball/internal/store"
)

func TestReader_List(t *testing.T) {
	setup := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	dir := t.TempDir()
	inner := newRepo(setup, filepath.Join(dir, "inner"))
	inner.write(setup, "f.txt", "one\n")
	inner.commit(setup, "inner base")
	r := newRepo(setup, filepath.Join(dir, "project"))
	r.write(setup, ".gitignore", "private.txt\n")
	r.write(setup, "a.txt", "a\n")
	r.write(setup, "dir/b.txt", "b\n")
	r.write(setup, "dir/sub/c.txt", "c\n")
	r.write(setup, ":cdir/f.txt", "in a directory with a leading colon\n")
	r.write(setup, "gone.txt", "deleted by the agent\n")
	setup.Expect(os.Symlink("a.txt", filepath.Join(r.root, "link"))).To(Succeed())
	addSubmodule(setup, r, inner.root, "mod")
	base := r.commit(setup, "base")
	r.write(setup, "a.txt", "a, changed\n")
	r.write(setup, "dir/b.txt", "b, changed\n")
	r.write(setup, "new/n.txt", "added by the agent\n")
	r.write(setup, "private.txt", "a secret\n")
	setup.Expect(os.Remove(filepath.Join(r.root, "gone.txt"))).To(Succeed())
	blobs, _ := newBlobs(setup, t)
	req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	setup.Expect(err).NotTo(HaveOccurred())
	r.write(setup, "later.txt", "written after the round\n")
	st, err := Inspect(t.Context(), r.root, base, false)
	setup.Expect(err).NotTo(HaveOccurred())
	reader := st.Reader(blobs, req.Files)
	setup.Expect(reader.Listing()).To(Equal(ListingFrozen))

	// The base and the capture, never the disk: later.txt and the ignored
	// private.txt are on disk only. The submodule's gitlink belongs to
	// another repository, the link is a file, and the deletion is listed as
	// one.
	dirs := []struct {
		name string
		dir  string
		want []string
	}{
		{
			name: "the root",
			dir:  "",
			want: []string{".gitignore", ".gitmodules", ":cdir/", "a.txt", "dir/", "gone.txt (deleted)", "link", "new/"},
		},
		{
			name: "the root written as a slash",
			dir:  "/",
			want: []string{".gitignore", ".gitmodules", ":cdir/", "a.txt", "dir/", "gone.txt (deleted)", "link", "new/"},
		},
		{
			name: "one level of a directory",
			dir:  "dir",
			want: []string{"dir/b.txt", "dir/sub/"},
		},
		{
			name: "a directory with a trailing slash",
			dir:  "dir/sub/",
			want: []string{"dir/sub/c.txt"},
		},
		{
			name: "a directory with a leading colon",
			dir:  ":cdir",
			want: []string{":cdir/f.txt"},
		},
		{
			name: "a directory only the capture holds",
			dir:  "new",
			want: []string{"new/n.txt"},
		},
		{
			name: "a directory nothing holds",
			dir:  "nowhere",
			want: []string{},
		},
	}
	for _, c := range dirs {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := reader.List(t.Context(), c.dir)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(listed(got)).To(Equal(c.want))
		})
	}
}

func TestReader_List_captureAlone(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	// Each row leaves a repository holding disk.txt, which nothing captured,
	// and returns the state Inspect finds for it and the round's capture of
	// changed.txt.
	states := []struct {
		name  string
		build func(g *WithT, t *testing.T, blobs *blob.Store) (State, []store.File)
		live  Live
	}{
		{
			name: "an unborn branch",
			build: func(g *WithT, t *testing.T, blobs *blob.Store) (State, []store.File) {
				r := newRepo(g, t.TempDir())
				r.write(g, "changed.txt", "captured\n")
				req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Blobs: blobs})
				g.Expect(err).NotTo(HaveOccurred())
				r.write(g, "disk.txt", "on disk only\n")
				st, err := Inspect(t.Context(), r.root, "", false)
				g.Expect(err).NotTo(HaveOccurred())
				return st, req.Files
			},
			live: LiveIgnoreRules,
		},
		{
			name: "a gone base",
			build: func(g *WithT, t *testing.T, blobs *blob.Store) (State, []store.File) {
				r := newRepo(g, t.TempDir())
				r.write(g, "disk.txt", "tracked\n")
				base := r.commit(g, "base")
				r.write(g, "changed.txt", "captured\n")
				req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
				g.Expect(err).NotTo(HaveOccurred())
				st, err := Inspect(t.Context(), r.root, "0123456789012345678901234567890123456789", false)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(st.BaseGone).To(BeTrue())
				return st, req.Files
			},
			live: LiveIgnoreRules,
		},
		{
			name: "a lost repository",
			build: func(g *WithT, t *testing.T, blobs *blob.Store) (State, []store.File) {
				dir := t.TempDir()
				r := newRepo(g, filepath.Join(dir, "project"))
				r.write(g, "disk.txt", "tracked\n")
				base := r.commit(g, "base")
				r.write(g, "changed.txt", "captured\n")
				req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(os.Rename(filepath.Join(r.root, gitDir), filepath.Join(dir, "moved.git"))).To(Succeed())
				st, err := Inspect(t.Context(), r.root, base, false)
				g.Expect(err).NotTo(HaveOccurred())
				return st, req.Files
			},
			live: LiveLost,
		},
		{
			name: "a missing root",
			build: func(g *WithT, t *testing.T, blobs *blob.Store) (State, []store.File) {
				r := newRepo(g, t.TempDir())
				r.write(g, "disk.txt", "tracked\n")
				base := r.commit(g, "base")
				r.write(g, "changed.txt", "captured\n")
				req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(os.RemoveAll(r.root)).To(Succeed())
				st, err := Inspect(t.Context(), r.root, base, false)
				g.Expect(err).NotTo(HaveOccurred())
				return st, req.Files
			},
			live: LiveRootMissing,
		},
	}
	for _, c := range states {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			blobs, _ := newBlobs(g, t)
			st, files := c.build(g, t, blobs)
			g.Expect(st.Live).To(Equal(c.live))
			reader := st.Reader(blobs, files)

			// Listing the disk would list every file git ignores, or there is
			// no disk, so only what the agent changed is listed.
			g.Expect(reader.Listing()).To(Equal(ListingCapture))
			got, err := reader.List(t.Context(), "")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(listed(got)).To(Equal([]string{"changed.txt"}))
		})
	}
}

func TestReader_List_disk(t *testing.T) {
	setup := NewWithT(t)
	dir := t.TempDir()
	root := filepath.Join(dir, "project")
	writeFile(setup, root, Marker, "")
	writeFile(setup, root, "a.txt", "a\n")
	writeFile(setup, root, "gone.txt", "put back after the round deleted it\n")
	writeFile(setup, root, "real/sub/f.txt", "reached through a link\n")
	writeFile(setup, root, "vendor/z/ok.go", "the project's own\n")
	writeFile(setup, root, "vendor/y/.git/HEAD", "ref: refs/heads/main\n")
	writeFile(setup, root, "vendor/y/bar.go", "another repository's file\n")
	writeFile(setup, root, "vendor/locked/x.go", "behind a directory that cannot be searched\n")
	writeFile(setup, dir, "outside/secret.txt", "not the project's\n")
	setup.Expect(os.Symlink("a.txt", filepath.Join(root, "alink"))).To(Succeed())
	setup.Expect(os.Symlink("real", filepath.Join(root, "link"))).To(Succeed())
	setup.Expect(os.Symlink(filepath.Join(dir, "outside"), filepath.Join(root, "outlink"))).To(Succeed())
	setup.Expect(syscall.Mkfifo(filepath.Join(root, "fifo"), 0o600)).To(Succeed())
	locked := filepath.Join(root, "vendor", "locked")
	setup.Expect(os.Chmod(locked, 0)).To(Succeed())
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	files := []store.File{
		{Path: "both"},
		{Path: "both/x.txt", Digest: "captured"},
		{Path: "gone.txt"},
		{Path: "new/n.txt", Digest: "captured"},
	}
	blobs, _ := newBlobs(setup, t)
	st, err := Inspect(t.Context(), root, "", false)
	setup.Expect(err).NotTo(HaveOccurred())
	setup.Expect(st.Live).To(Equal(LiveOpen))
	reader := st.Reader(blobs, files)
	setup.Expect(reader.Listing()).To(Equal(ListingDisk))

	// A link of either kind is listed as a file, since a live read serves its
	// target text, and a FIFO is not listed at all. A deletion the round
	// records beats the disk having the file again.
	listings := []struct {
		name string
		dir  string
		want []string
		// perms says the row leans on vendor/locked's mode being enforced.
		perms bool
	}{
		{
			name: "the root",
			dir:  "",
			want: []string{Marker, "a.txt", "alink", "both/", "gone.txt (deleted)", "link", "new/", "outlink", "real/", "vendor/"},
		},
		{
			name:  "a directory holding another repository is skipped",
			dir:   "vendor",
			want:  []string{"vendor/z/"},
			perms: true,
		},
		{
			name: "a directory the disk lacks and the capture holds",
			dir:  "new",
			want: []string{"new/n.txt"},
		},
	}
	for _, c := range listings {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			// Root searches a directory whatever its mode, so chmod 0
			// denies nothing and vendor/locked/ is listed.
			if c.perms && os.Geteuid() == 0 {
				t.Skip("permission bits are not enforced for root")
			}
			got, err := reader.List(t.Context(), c.dir)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(listed(got)).To(Equal(c.want))
		})
	}

	// A file the round deleted at a name it also captured paths below lists
	// as a directory, the same way every time, whatever order the capture's
	// map gives.
	for i := 0; i < 20; i++ {
		got, err := reader.List(t.Context(), "")
		setup.Expect(err).NotTo(HaveOccurred())
		setup.Expect(listed(got)).To(ContainElement("both/"))
	}

	// The links lead inside the root, where os.Root would follow them, so the
	// check on each directory is what refuses these.
	refused := []struct {
		name   string
		dir    string
		reason string
	}{
		{
			name:   "a symlinked directory",
			dir:    "link",
			reason: "a symlink",
		},
		{
			name:   "a directory below a symlinked one",
			dir:    "link/sub",
			reason: "a symlink",
		},
		{
			name:   "a symlinked directory outside the root",
			dir:    "outlink",
			reason: "a symlink",
		},
		{
			name:   "a directory holding another repository",
			dir:    "vendor/y",
			reason: "holds a repository of its own",
		},
		{
			name:   "a file",
			dir:    "a.txt",
			reason: "not a directory",
		},
		{
			name:   "a directory nothing holds",
			dir:    "nowhere",
			reason: "nowhere",
		},
		{
			name:   "a climb out of the root",
			dir:    "../outside",
			reason: "not a path inside the project",
		},
	}
	for _, c := range refused {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := reader.List(t.Context(), c.dir)
			g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
			g.Expect(err.Error()).To(ContainSubstring(c.reason))
			g.Expect(got).To(BeEmpty())
		})
	}
}

func TestReader_List_fileAndDirectory(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	// The agent turned a name from one kind into the other, so the capture
	// records a deletion and a file at or under the same name. What exists as
	// of the round wins, and an address says what the listing says.
	swaps := []struct {
		name     string
		before   func(r *repo, g *WithT)
		after    func(r *repo, g *WithT)
		listed   string
		pathType PathType
	}{
		{
			name: "a file replaced by a directory",
			before: func(r *repo, g *WithT) {
				r.write(g, "a", "a file\n")
			},
			after: func(r *repo, g *WithT) {
				g.Expect(os.Remove(filepath.Join(r.root, "a"))).To(Succeed())
				r.write(g, "a/x", "a file in a directory\n")
			},
			listed:   "a/",
			pathType: PathDir,
		},
		{
			name: "a directory replaced by a file",
			before: func(r *repo, g *WithT) {
				r.write(g, "a/x", "a file in a directory\n")
			},
			after: func(r *repo, g *WithT) {
				g.Expect(os.RemoveAll(filepath.Join(r.root, "a"))).To(Succeed())
				r.write(g, "a", "a file\n")
			},
			listed:   "a",
			pathType: PathFile,
		},
	}
	for _, c := range swaps {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			r, blobs := readerRepo(g, t)
			c.before(r, g)
			base := r.commit(g, "base")
			c.after(r, g)
			req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
			g.Expect(err).NotTo(HaveOccurred())
			var captured []string
			for _, f := range req.Files {
				captured = append(captured, f.Path)
			}
			g.Expect(captured).To(ConsistOf("a", "a/x"), "the capture records both")
			st, err := Inspect(t.Context(), r.root, base, false)
			g.Expect(err).NotTo(HaveOccurred())
			reader := st.Reader(blobs, req.Files)

			got, err := reader.List(t.Context(), "")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(listed(got)).To(ContainElement(c.listed))
			pathType, _, err := reader.Locate(t.Context(), "a")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pathType).To(Equal(c.pathType))
		})
	}
}

func TestReader_List_ignoreRules(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	outer, notes := markerInCheckout(g, t.TempDir())
	outer.write(g, "notes/.gitignore", ":other.txt\nbuild/\n")
	outer.write(g, "notes/:other.txt", "ignored, with a leading colon\n")
	outer.write(g, "notes/:visible.txt", "not ignored, with a leading colon\n")
	outer.write(g, "notes/build/out", "built\n")
	outer.write(g, "notes/docs/d.md", "a document\n")
	g.Expect(os.Mkdir(filepath.Join(notes, "empty"), 0o700)).To(Succeed())
	blobs, _ := newBlobs(g, t)
	req, err := Freeze(t.Context(), Options{Round: 1, Root: notes, Paths: []string{"creds.txt"}, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	st, err := Inspect(t.Context(), notes, "", false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(st.Live).To(Equal(LiveIgnoreRules))
	reader := st.Reader(blobs, req.Files)
	g.Expect(reader.Listing()).To(Equal(ListingDisk))

	// What the checkout ignores is dropped, a directory included, and a
	// leading colon is not read as pathspec magic. creds.txt is ignored too,
	// but the round captured it, so it opens and is listed.
	got, err := reader.List(t.Context(), "")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(listed(got)).To(Equal([]string{Marker, ".gitignore", ":visible.txt", "creds.txt", "docs/", "empty/", "plan.md"}))
	uncaptured, err := st.Reader(blobs, nil).List(t.Context(), "")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(listed(uncaptured)).To(Equal([]string{Marker, ".gitignore", ":visible.txt", "docs/", "empty/", "plan.md"}))

	// A batch with nothing ignored in it is an answer, not a failure.
	got, err = reader.List(t.Context(), "docs")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(listed(got)).To(Equal([]string{"docs/d.md"}))

	// The ceiling stops git short of the checkout's .git, so check-ignore
	// exits 128. That is an error, never a reason to list the entries
	// unchecked. Asked before the reader is built, the root's check fails
	// instead, and the capture alone is listed.
	t.Setenv("GIT_CEILING_DIRECTORIES", outer.root)
	_, err = reader.List(t.Context(), "")
	g.Expect(errors.Is(err, ErrRulesUnreadable)).To(BeTrue())
	// A directory with nothing on disk to check asks git nothing, so it lists
	// empty even while the rules cannot be read. An empty check-ignore batch
	// exits 128 here, not 1, so the guard is not only saving a process.
	got, err = reader.List(t.Context(), "empty")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(BeEmpty())
	st, err = Inspect(t.Context(), notes, "", false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(st.Live).To(Equal(LiveRulesUnreadable))
	reader = st.Reader(blobs, req.Files)
	g.Expect(reader.Listing()).To(Equal(ListingCapture))
	got, err = reader.List(t.Context(), "")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(listed(got)).To(Equal([]string{"creds.txt"}))
}

func TestReader_List_refusedMarker(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	dir := t.TempDir()
	outer, notes := markerInCheckout(g, dir)
	ign := filepath.Join(outer.root, "ign")
	blobs, _ := newBlobs(g, t)

	// The checkout ignores ign as a whole. Per-path rules would drop every
	// entry from disk and leave the same captured file, so the source is
	// what tells the two apart.
	req, err := Freeze(t.Context(), Options{Round: 1, Root: ign, Paths: []string{"plan.md"}, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	st, err := Inspect(t.Context(), ign, "", false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(st.Live).To(Equal(LiveIgnored))
	reader := st.Reader(blobs, req.Files)
	g.Expect(reader.Listing()).To(Equal(ListingCapture))
	got, err := reader.List(t.Context(), "")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(listed(got)).To(Equal([]string{"plan.md"}))

	// The checkout's .git moved away from around a project seen inside it.
	req, err = Freeze(t.Context(), Options{Round: 1, Root: notes, Paths: []string{"plan.md"}, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(os.Rename(filepath.Join(outer.root, gitDir), filepath.Join(dir, "moved.git"))).To(Succeed())
	st, err = Inspect(t.Context(), notes, "", true)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(st.Live).To(Equal(LiveCheckoutGone))
	reader = st.Reader(blobs, req.Files)
	g.Expect(reader.Listing()).To(Equal(ListingCapture))
	got, err = reader.List(t.Context(), "")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(listed(got)).To(Equal([]string{"plan.md"}))
}

func TestReader_List_swapped(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	root := filepath.Join(dir, "project")
	writeFile(g, root, Marker, "")
	writeFile(g, root, "sub/x.txt", "the project's\n")
	writeFile(g, dir, "outside/secret.txt", "not the project's\n")
	blobs, _ := newBlobs(g, t)
	st, err := Inspect(t.Context(), root, "", false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(st.Live).To(Equal(LiveOpen))

	// list lists dir with fn run once the directories are checked, which is
	// where a local process racing the listing would change the disk.
	list := func(dir string, fn func()) ([]DirEntry, error) {
		reader := st.Reader(blobs, nil)
		reader.pause = func(step string) {
			if step == "checked" {
				fn()
			}
		}
		return reader.List(t.Context(), dir)
	}

	// A directory swapped for a link to outside after it was checked.
	// os.Root refuses the listing that now leaves the root.
	_, err = list("sub", func() {
		g.Expect(os.Rename(filepath.Join(root, "sub"), filepath.Join(dir, "sub.old"))).To(Succeed())
		g.Expect(os.Symlink(filepath.Join(dir, "outside"), filepath.Join(root, "sub"))).To(Succeed())
	})
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
	g.Expect(os.Remove(filepath.Join(root, "sub"))).To(Succeed())
	g.Expect(os.Rename(filepath.Join(dir, "sub.old"), filepath.Join(root, "sub"))).To(Succeed())

	// A .git appearing at the root of a project outside git is never listed.
	got, err := list("", func() {
		g.Expect(os.Mkdir(filepath.Join(root, gitDir), 0o700)).To(Succeed())
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(listed(got)).To(Equal([]string{Marker, "sub/"}))
}

func TestReader_ListSubdirs(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	r, blobs := readerRepo(g, t)
	r.write(g, "internal/doc.go", "package internal\n")
	r.write(g, "internal/storage/store.go", "package storage\n")
	r.write(g, "internal/upload/queue.go", "package upload\n")
	r.write(g, "internal/upload/worker.go", "package upload\n")
	r.write(g, "internal/upload/deep/d.go", "package deep\n")
	r.write(g, "internal/:cdir/base.txt", "unchanged, with a leading colon above it\n")
	base := r.commit(g, "base")
	r.write(g, "internal/upload/backoff.go", "package upload\n")
	r.write(g, "internal/:cdir/changed.txt", "added by the agent\n")
	req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	st, err := Inspect(t.Context(), r.root, base, false)
	g.Expect(err).NotTo(HaveOccurred())
	reader := st.Reader(blobs, req.Files)

	// Siblings share one ls-tree call, and each lists one level. The flag
	// that keeps a leading colon literal covers this call too.
	got, err := reader.ListSubdirs(t.Context(), "internal", []string{":cdir", "storage", "upload"})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(reader.lookups).To(Equal(1))
	g.Expect(listed(got[":cdir"])).To(Equal([]string{"internal/:cdir/base.txt", "internal/:cdir/changed.txt"}))
	g.Expect(listed(got["storage"])).To(Equal([]string{"internal/storage/store.go"}))
	g.Expect(listed(got["upload"])).To(Equal([]string{"internal/upload/backoff.go", "internal/upload/deep/", "internal/upload/queue.go", "internal/upload/worker.go"}))

	// The root's subdirectories, and a name the base lacks, which lists
	// nothing.
	got, err = reader.ListSubdirs(t.Context(), "", []string{"internal", "nowhere"})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(listed(got["internal"])).To(Equal([]string{"internal/:cdir/", "internal/doc.go", "internal/storage/", "internal/upload/"}))
	g.Expect(got["nowhere"]).To(BeEmpty())

	// A name is one segment below the directory, so no pathspec in the call
	// is the parent of another.
	bad := []string{"upload/deep", ".", "..", ".git"}
	for _, name := range bad {
		_, err := reader.ListSubdirs(t.Context(), "internal", []string{name})
		g.Expect(err).To(HaveOccurred(), name)
	}
}

func TestReader_ListSubdirs_disk(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	outer, notes := markerInCheckout(g, t.TempDir())
	outer.write(g, "notes/.gitignore", "*.log\n")
	outer.write(g, "notes/a/kept.md", "a document\n")
	outer.write(g, "notes/a/out.log", "ignored\n")
	outer.write(g, "notes/b/:other.md", "a document with a leading colon\n")
	outer.write(g, "notes/b/run.log", "ignored\n")
	outer.write(g, "notes/b/vendor/.git/HEAD", "ref: refs/heads/main\n")
	blobs, _ := newBlobs(g, t)
	st, err := Inspect(t.Context(), notes, "", false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(st.Live).To(Equal(LiveIgnoreRules))
	files := []store.File{{Path: "c/n.md", Digest: "captured"}}
	reader := st.Reader(blobs, files)

	// What the checkout ignores is dropped from every subdirectory, another
	// repository is skipped, and a subdirectory only the capture holds lists
	// what it holds.
	got, err := reader.ListSubdirs(t.Context(), "", []string{"a", "b", "c"})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(listed(got["a"])).To(Equal([]string{"a/kept.md"}))
	g.Expect(listed(got["b"])).To(Equal([]string{"b/:other.md"}))
	g.Expect(listed(got["c"])).To(Equal([]string{"c/n.md"}))

	// A failed batch is an error, never a reason to list the entries
	// unchecked.
	t.Setenv("GIT_CEILING_DIRECTORIES", outer.root)
	_, err = reader.ListSubdirs(t.Context(), "", []string{"a", "b"})
	g.Expect(errors.Is(err, ErrRulesUnreadable)).To(BeTrue())
}

// listed renders entries for an assertion: a directory with a trailing slash,
// and a deletion marked.
func listed(entries []DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		switch {
		case e.Dir:
			out = append(out, e.Path+"/")
		case e.Deleted:
			out = append(out, e.Path+" (deleted)")
		default:
			out = append(out, e.Path)
		}
	}
	return out
}
