package capture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"
)

func TestReader_Locate(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	r, blobs := readerRepo(g, t)
	r.write(g, ".gitignore", "build/\nprivate.txt\n")
	r.write(g, "internal/doc.go", "package internal\n")
	r.write(g, "internal/upload/worker.go", "package upload\n")
	r.write(g, "gone.txt", "deleted by the agent\n")
	r.write(g, ":weird.txt", "leading colon\n")
	r.write(g, "go.mod", "module x\n")
	base := r.commit(g, "base")
	r.write(g, "changed.txt", "captured\n")
	r.write(g, "fresh/new.go", "package fresh\n")
	g.Expect(os.Remove(filepath.Join(r.root, "gone.txt"))).To(Succeed())
	req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	// After the capture: the directory the capture holds gone from disk, the
	// deleted file back, an ignored directory and file, a directory, a file
	// and a link only the disk holds, another repository, a link to a
	// directory, and a link that loops.
	g.Expect(os.RemoveAll(filepath.Join(r.root, "fresh"))).To(Succeed())
	g.Expect(os.Symlink("internal", filepath.Join(r.root, "linkdir"))).To(Succeed())
	g.Expect(os.Symlink("later.txt", filepath.Join(r.root, "link.txt"))).To(Succeed())
	r.write(g, "gone.txt", "back again\n")
	r.write(g, "build/out", "built\n")
	r.write(g, "private.txt", "a secret\n")
	r.write(g, "later/x.txt", "written after the round\n")
	r.write(g, "later.txt", "written after the round\n")
	r.write(g, "vendor/y/.git/HEAD", "ref: refs/heads/main\n")
	g.Expect(os.Symlink("loop", filepath.Join(r.root, "loop"))).To(Succeed())
	r.write(g, "locked/x.txt", "behind a directory that cannot be searched\n")
	locked := filepath.Join(r.root, "locked")
	g.Expect(os.Chmod(locked, 0)).To(Succeed())
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	st, err := Inspect(t.Context(), r.root, base, false)
	g.Expect(err).NotTo(HaveOccurred())
	reader := st.Reader(blobs, req.Files)
	paths := []struct {
		path     string
		pathType PathType
		source   Source
	}{
		{path: "gone.txt", pathType: PathDeleted, source: SourceRound},
		{path: "changed.txt", pathType: PathFile, source: SourceRound},
		{path: "fresh", pathType: PathDir},
		{path: "untouched.txt", pathType: PathFile, source: SourceBase},
		{path: ":weird.txt", pathType: PathFile, source: SourceBase},
		{path: "internal", pathType: PathDir},
		{path: "internal/upload", pathType: PathDir},
		{path: "internal/upload/worker.go", pathType: PathFile, source: SourceBase},
		{path: "later", pathType: PathDir},
		{path: "later.txt", pathType: PathFile, source: SourceWorking},
		{path: "link.txt", pathType: PathFile, source: SourceWorking},
		// An ignored file is found here, and the live read that follows is
		// what refuses it.
		{path: "private.txt", pathType: PathFile, source: SourceWorking},
	}
	for _, c := range paths {
		t.Run(c.path, func(t *testing.T) {
			g := NewWithT(t)
			pathType, source, err := reader.Locate(t.Context(), c.path)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pathType).To(Equal(c.pathType))
			g.Expect(source).To(Equal(c.source))
		})
	}

	missing := []string{
		"nothing.txt",
		"build",
		"vendor/y",
		"vendor/y/a.go",
		".git/config",
		"../outside",
		"a\x00b",
		"go.mod/x",
		strings.Repeat("x", 300),
		"loop/x",
		"linkdir/upload",
		"linkdir/doc.go",
		// The listing leaves out a directory it cannot search, so a path
		// below one is not found rather than a failure.
		"locked/x.txt",
		"locked/sub/y.txt",
	}
	for _, p := range missing {
		t.Run("not found: "+p, func(t *testing.T) {
			g := NewWithT(t)
			_, _, err := reader.Locate(t.Context(), p)
			g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue(), "%v", err)
		})
	}
}

func TestReader_Locate_gitlink(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	inner := newRepo(g, filepath.Join(dir, "inner"))
	inner.write(g, "f.txt", "one\n")
	inner.commit(g, "inner base")
	r := newRepo(g, filepath.Join(dir, "outer"))
	r.write(g, "a.txt", "a\n")
	addSubmodule(g, r, inner.root, "sub")
	base := r.commit(g, "base")
	// With its .git file gone, the checkout is a plain directory on disk, so
	// only the base's answer keeps it from being read as one.
	g.Expect(os.Remove(filepath.Join(r.root, "sub", gitDir))).To(Succeed())
	blobs, _ := newBlobs(g, t)
	st, err := Inspect(t.Context(), r.root, base, false)
	g.Expect(err).NotTo(HaveOccurred())

	// The base holds a gitlink, whose contents belong to another repository,
	// and its checkout on disk is not read in its place.
	_, _, err = st.Reader(blobs, nil).Locate(t.Context(), "sub")
	g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue(), "%v", err)
}

func TestReader_Locate_oneLookup(t *testing.T) {
	g := NewWithT(t)
	r, blobs := readerRepo(g, t)
	for i := range 20 {
		r.write(g, "internal/"+itoa(i)+".go", "package internal\n")
	}
	base := r.commit(g, "base")
	st, err := Inspect(t.Context(), r.root, base, false)
	g.Expect(err).NotTo(HaveOccurred())
	reader := st.Reader(blobs, nil)

	// A directory is named by one ls-tree call, and a file found in the base
	// is read without asking again.
	pathType, _, err := reader.Locate(t.Context(), "internal")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(pathType).To(Equal(PathDir))
	g.Expect(reader.lookups).To(Equal(1))
	_, source, err := reader.Locate(t.Context(), "internal/7.go")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceBase))
	rc, _, err := reader.Frozen(t.Context(), "internal/7.go")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(readAll(g, rc)).To(Equal("package internal\n"))
	g.Expect(reader.lookups).To(Equal(2))
}

func TestReader_Locate_refused(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	r := newRepo(g, filepath.Join(dir, "project"))
	r.write(g, ".gitignore", "private.txt\n")
	base := r.commit(g, "base")
	r.write(g, "changed.txt", "captured\n")
	blobs, _ := newBlobs(g, t)
	req, err := Freeze(t.Context(), Options{Round: 1, Root: r.root, Base: base, Blobs: blobs})
	g.Expect(err).NotTo(HaveOccurred())
	r.write(g, "later/x.txt", "only on disk\n")
	r.write(g, "private.txt", "a secret\n")
	g.Expect(os.Rename(filepath.Join(r.root, gitDir), filepath.Join(dir, "moved.git"))).To(Succeed())

	st, err := Inspect(t.Context(), r.root, base, false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(st.Live).To(Equal(LiveLost))
	reader := st.Reader(blobs, req.Files)

	// The capture still answers, and nothing on disk does.
	pathType, source, err := reader.Locate(t.Context(), "changed.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(pathType).To(Equal(PathFile))
	g.Expect(source).To(Equal(SourceRound))
	refused := []string{"private.txt", "later", "later/x.txt"}
	for _, p := range refused {
		_, _, err = reader.Locate(t.Context(), p)
		g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue(), p)
		g.Expect(errors.Is(err, ErrRulesUnreadable)).To(BeFalse(), p)
	}
}

func TestReader_Locate_rulesUnreadable(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	r, blobs := readerRepo(g, t)
	base := r.commit(g, "base")
	r.write(g, "later/x.txt", "only on disk\n")
	// A truncated index makes check-ignore exit 128.
	g.Expect(os.WriteFile(filepath.Join(r.root, gitDir, "index"), []byte("garbage\n"), 0o600)).To(Succeed())
	st, err := Inspect(t.Context(), r.root, base, false)
	g.Expect(err).NotTo(HaveOccurred())

	// A directory is asked about here, and so is a path the disk lacks, so
	// the failure surfaces here, with its reason. A file found on disk is
	// left to the live read, which asks.
	asked := []string{"later", "nothing.txt"}
	for _, p := range asked {
		_, _, err = st.Reader(blobs, nil).Locate(t.Context(), p)
		g.Expect(errors.Is(err, ErrNotFound)).To(BeTrue(), p)
		g.Expect(errors.Is(err, ErrRulesUnreadable)).To(BeTrue(), p)
	}
	_, source, err := st.Reader(blobs, nil).Locate(t.Context(), "later/x.txt")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(source).To(Equal(SourceWorking))
}
