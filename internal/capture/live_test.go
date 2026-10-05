package capture

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"
)

func Test_inSubmodule(t *testing.T) {
	messages := []struct {
		name   string
		stderr string
		want   bool
	}{
		{
			name:   "git's message for a path in a submodule",
			stderr: "fatal: Pathspec './mod/m.go' is in submodule 'mod'\n",
			want:   true,
		},
		{
			name:   "the same message translated",
			stderr: "Schwerwiegend: Pfadspezifikation './mod/m.go' befindet sich in Submodul 'mod'\n",
			want:   false,
		},
		{
			name:   "another fatal error",
			stderr: "fatal: .git/index: index file smaller than expected\n",
			want:   false,
		},
		{
			name:   "another error naming a path that holds the phrase",
			stderr: "fatal: unable to read './a' is in submodule 'b': Permission denied\n",
			want:   false,
		},
		{
			name:   "another error about a pathspec",
			stderr: "fatal: Pathspec './x' did not match any file\n",
			want:   false,
		},
		{
			name:   "a path beyond a symlink",
			stderr: "fatal: pathspec './linkdir/doc.go' is beyond a symbolic link\n",
			want:   false,
		},
	}
	for _, c := range messages {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(inSubmodule(c.stderr)).To(Equal(c.want))
		})
	}
}

func Test_missing(t *testing.T) {
	setup := NewWithT(t)
	dir := t.TempDir()
	project := filepath.Join(dir, "project")
	writeFile(setup, project, "go.mod", "module x\n")
	writeFile(setup, dir, "outside/x", "not the project's\n")
	setup.Expect(os.Symlink("loop", filepath.Join(project, "loop"))).To(Succeed())
	setup.Expect(os.Symlink(filepath.Join(dir, "outside"), filepath.Join(project, "outlink"))).To(Succeed())
	root, err := os.OpenRoot(project)
	setup.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = root.Close() })

	// lstat reads a path through the root, as the live read does, so each
	// error below is the one a typed address would meet.
	lstat := func(name string) error {
		_, err := root.Lstat(name)
		setup.Expect(err).To(HaveOccurred(), name)
		return err
	}
	errs := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "a path that does not exist",
			err:  lstat("nope"),
			want: true,
		},
		{
			name: "a path below a file",
			err:  lstat("go.mod/x"),
			want: true,
		},
		{
			name: "a 300-byte segment",
			err:  lstat(strings.Repeat("a", 300)),
			want: true,
		},
		{
			name: "a path through a link to itself",
			err:  lstat("loop/x"),
			want: true,
		},
		{
			name: "a path through a link to outside the root",
			err:  lstat("outlink/x"),
			want: true,
		},
		{
			name: "a climb out of the root",
			err:  lstat("../outside/x"),
			want: true,
		},
		{
			name: "a permission failure",
			err:  &fs.PathError{Op: "lstat", Path: "x", Err: syscall.EACCES},
			want: false,
		},
		{
			name: "a permission failure on a path named like the escape",
			err:  &fs.PathError{Op: "lstat", Path: escapeText, Err: syscall.EACCES},
			want: false,
		},
		{
			name: "an error with no path",
			err:  errors.New(escapeText),
			want: false,
		},
	}
	for _, c := range errs {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(missing(c.err)).To(Equal(c.want), c.err.Error())
		})
	}
}
