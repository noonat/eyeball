package capture

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/gomega"
)

// fixtureEnv keeps a fixture repository out of the developer's git
// configuration. A global commit template, a hooks path or commit signing
// turned on would otherwise decide what a test sees, and the failure would
// appear on one machine and not another.
//
// None of this applies to capture itself, which reads the agent's repository
// through the agent's own git and its own configuration on purpose.
var fixtureEnv = []string{
	"GIT_CONFIG_GLOBAL=/dev/null",
	"GIT_CONFIG_SYSTEM=/dev/null",
	"GIT_AUTHOR_NAME=eyeball test",
	"GIT_AUTHOR_EMAIL=test@example.invalid",
	"GIT_COMMITTER_NAME=eyeball test",
	"GIT_COMMITTER_EMAIL=test@example.invalid",
}

// repo is a git repository built for one test.
type repo struct {
	root string
}

// newRepo initializes a repository at dir, which has no commit yet.
//
// The init template is disabled and commits are made with plumbing, so nothing
// a developer has configured takes part in what the tests below see.
func newRepo(g *WithT, dir string, initArgs ...string) *repo {
	g.THelper()
	g.Expect(os.MkdirAll(dir, 0o700)).To(Succeed())
	r := &repo{root: dir}
	args := []string{"init", "--quiet", "--template=", "-b", "main"}
	r.git(g, append(args, initArgs...)...)
	return r
}

// write puts content at a slash-separated path under the repository root,
// creating the directories above it.
func (r *repo) write(g *WithT, path, content string) {
	g.THelper()
	full := filepath.Join(r.root, filepath.FromSlash(path))
	g.Expect(os.MkdirAll(filepath.Dir(full), 0o700)).To(Succeed())
	g.Expect(os.WriteFile(full, []byte(content), 0o600)).To(Succeed())
}

// commit stages everything in the working tree and records it on main,
// returning the new commit id.
//
// write-tree and commit-tree rather than `git commit`, so no hook runs and no
// template is read. The parent is whatever main already points at, and there is
// none on the first call.
func (r *repo) commit(g *WithT, message string) string {
	g.THelper()
	r.git(g, "add", "-A")
	tree := r.git(g, "write-tree")
	args := []string{"commit-tree", tree, "-m", message}
	if parent := r.head(); parent != "" {
		args = append(args, "-p", parent)
	}
	commit := r.git(g, args...)
	r.git(g, "update-ref", "refs/heads/main", commit)
	return commit
}

// head is the commit main points at, and is empty on a branch that has none.
func (r *repo) head() string {
	out, err := r.tryGit("rev-parse", "--verify", "--quiet", "refs/heads/main")
	if err != nil {
		return ""
	}
	return out
}

// git runs one git command in the repository and returns its trimmed output,
// failing the test if the command does not succeed.
func (r *repo) git(g *WithT, args ...string) string {
	g.THelper()
	out, err := r.tryGit(args...)
	g.Expect(err).NotTo(HaveOccurred())
	return out
}

// tryGit runs one git command and hands back whatever it did, for the calls
// that are expected to fail.
func (r *repo) tryGit(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", r.root}, args...)...)
	cmd.Env = append(os.Environ(), fixtureEnv...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
