package capture

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/store"
)

// Marker is the file a person creates to name a project that is not a git
// repository. It is read for its existence only.
const Marker = ".eyeball"

// ErrNoProject reports a working directory with no .git entry and no marker
// file anywhere above it.
var ErrNoProject = errors.New("not inside a project")

// ErrOutsideProject reports a review path that leaves the project root.
var ErrOutsideProject = errors.New("path is outside the project")

// Root is a project root found by walking up from a working directory, spelled
// two ways.
//
// Logical is the directory as the walk reached it, through whatever symlinks
// the caller's working directory runs through. Review paths are computed
// against it, because an agent builds an absolute path from the directory it
// sees. Resolved is the same directory with every symlink followed, and is the
// project's identity in the store: two shells reaching one repository by two
// spellings are one project.
type Root struct {
	// Logical is the root as reached from the working directory.
	Logical string
	// Resolved is the root with symlinks followed.
	Resolved string
}

// FindRoot walks up from dir, which must be absolute, to the nearest directory
// holding a .git entry or a marker file, whichever comes first.
//
// The nearest wins because a marker inside a repository is somebody naming a
// smaller project on purpose, and because a worktree or submodule holds a .git
// file of its own and is its own working copy.
func FindRoot(dir string) (Root, error) {
	if !filepath.IsAbs(dir) {
		return Root{}, errors.Newf("working directory %q is relative", dir)
	}
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		found, err := holdsEither(d)
		if err != nil {
			return Root{}, err
		}
		if found {
			resolved, err := filepath.EvalSymlinks(d)
			if err != nil {
				return Root{}, errors.Wrapf(err, "resolve %s", d)
			}
			return Root{Logical: d, Resolved: resolved}, nil
		}
		if d == filepath.Dir(d) {
			return Root{}, errors.Wrapf(ErrNoProject, "no %s or %s above %s; run from inside a git repository, or create %s at the project root", gitDir, Marker, dir, Marker)
		}
	}
}

// holdsEither reports whether d holds a .git entry or a marker file.
func holdsEither(d string) (bool, error) {
	names := []string{gitDir, Marker}
	for _, name := range names {
		found, err := holds(d, name)
		if err != nil || found {
			return found, err
		}
	}
	return false, nil
}

// ProjectName is what the surface displays for the project at root.
//
// A linked worktree is named after its repository, so every worktree of harbor
// displays as harbor. git says a root is a linked worktree when its git
// directory and its common directory differ. The common directory is then
// named for the repository: the directory holding it when it is .git, its
// parent when it is .bare, and otherwise its own name without a .git suffix,
// which is how a bare clone is named. Everywhere else, a main checkout, a
// submodule and a directory outside git, the name is the root's own. A
// submodule's common directory sits under the superproject's .git/modules, so
// naming it from there would call every submodule "modules".
func ProjectName(ctx context.Context, root string) (string, error) {
	own := filepath.Base(root)
	gitDirPath, common, found, err := gitDirs(ctx, root)
	if err != nil {
		return "", err
	}
	if !found {
		return own, nil
	}
	if gitDirPath == common {
		return own, nil
	}
	name := filepath.Base(common)
	switch name {
	case gitDir, ".bare":
		name = filepath.Base(filepath.Dir(common))
	default:
		name = strings.TrimSuffix(name, ".git")
	}
	if name == "" || name == "." || name == string(filepath.Separator) {
		return own, nil
	}
	return name, nil
}

// Repo is what git says about a working copy: the repository it belongs to,
// and whether it is that repository's main checkout.
type Repo struct {
	// Dir is the repository's common git directory with symlinks resolved, and
	// is empty for a working copy found by its marker file.
	Dir string
	// Main reports that the working copy is its repository's main checkout,
	// whose git directory is the common directory. A submodule is one too:
	// its common directory is its own, under the superproject's .git/modules.
	Main bool
}

// Repository asks git which repository the working copy at root belongs to.
//
// The common directory is the one answer git gives correctly in every layout:
// git worktree list names the git directory rather than the working copy as a
// --separate-git-dir repository's main entry, and does the same inside a
// submodule.
func Repository(ctx context.Context, root string) (Repo, error) {
	gitDirPath, common, found, err := gitDirs(ctx, root)
	if err != nil || !found {
		return Repo{}, err
	}
	resolved, err := filepath.EvalSymlinks(common)
	if err != nil {
		return Repo{}, errors.Wrapf(err, "resolve %s", common)
	}
	return Repo{Dir: resolved, Main: gitDirPath == common}, nil
}

// gitDirs reads a working copy's git directory and common directory, both
// absolute, and reports false for a root with no .git entry.
func gitDirs(ctx context.Context, root string) (gitDirPath, common string, found bool, err error) {
	if _, err := os.Lstat(filepath.Join(root, gitDir)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", false, nil
		}
		return "", "", false, errors.Wrapf(err, "look for %s in %s", gitDir, root)
	}
	out, err := run(ctx, root, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir")
	if err != nil {
		return "", "", false, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		return "", "", false, errors.Newf("rev-parse printed %d lines for two questions", len(lines))
	}
	return filepath.Clean(lines[0]), filepath.Clean(lines[1]), true, nil
}

// AgentName is the name an agent is known by: the one it gave explicitly, then
// the one in its environment, then its working copy's path with the home
// directory written as ~.
//
// The working copy is the project root, so an agent restarted in the same
// directory comes back under the same name, which is what identity has to
// survive. A branch name would read better in a queue and moves when the agent
// switches branches. home is resolved through symlinks by the caller, as the
// root is, so the ~ still applies where /home is itself a link.
func AgentName(explicit, fromEnv string, root Root, home string) string {
	if name := strings.TrimSpace(explicit); name != "" {
		return name
	}
	if name := strings.TrimSpace(fromEnv); name != "" {
		return name
	}
	if home != "" {
		if rel, err := filepath.Rel(home, root.Resolved); err == nil && !escapes(rel) {
			if rel == "." {
				return "~"
			}
			return "~/" + filepath.ToSlash(rel)
		}
	}
	return root.Resolved
}

// ReviewPath turns a path an agent named into one relative to the project
// root, slash separated.
//
// A relative path is joined to the agent's working directory, as a shell would
// read it. Either kind is then made relative to the logical root, the spelling
// the agent sees. An absolute path written in the resolved spelling is
// accepted too, since an agent may have built it from a resolved directory.
// Nothing in the path is resolved: a symlink inside the project is content to
// review, not a spelling of somewhere else.
func ReviewPath(root Root, cwd, p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", errors.Wrap(store.ErrInvalidPath, "a path cannot be empty")
	}
	full := p
	if !filepath.IsAbs(full) {
		full = filepath.Join(cwd, p)
	}
	full = filepath.Clean(full)
	bases := []string{root.Logical, root.Resolved}
	for _, base := range bases {
		rel, err := filepath.Rel(base, full)
		if err == nil && !escapes(rel) {
			return filepath.ToSlash(rel), nil
		}
	}
	return "", errors.Wrapf(ErrOutsideProject, "%s is outside the project at %s", p, root.Logical)
}

// escapes reports whether a relative path climbs out of the directory it is
// relative to.
func escapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel)
}
