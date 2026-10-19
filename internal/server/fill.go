package server

import (
	"context"
	"os"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/store"
)

// FillRepositories asks git which repository each project belongs to, for the
// projects written before a project was a repository, and merges two that turn
// out to share one. It runs before the agent socket serves a request: a request
// from a new worktree finds its project by repository, and would register a
// second project if its main checkout's old row had none yet.
//
// A project whose root is gone cannot be asked, keeps no repository, and stays
// a project of its own. One that fails is logged and left for the next start,
// since a single unreadable directory should not keep the daemon down.
func (s *Server) FillRepositories(ctx context.Context) {
	projects, err := s.db.s.ProjectsWithoutRepository(ctx)
	if err != nil {
		s.log.Warn("fill repositories", "err", err)
		return
	}
	for _, p := range projects {
		if err := s.fillRepository(ctx, p); err != nil {
			s.log.Warn("fill repository", "root", p.Root, "err", err)
		}
	}
}

// fillRepository asks git about one project's root and records the answer.
func (s *Server) fillRepository(ctx context.Context, p store.Project) error {
	if _, err := os.Stat(p.Root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return errors.Wrapf(err, "look at %s", p.Root)
	}
	repo, err := capture.Repository(ctx, p.Root)
	if err != nil || repo.Dir == "" {
		return err
	}
	copy, err := s.db.EnsureWorkingCopy(ctx, p.ID, p.Root)
	if err != nil {
		return err
	}
	if repo.Main {
		if err := s.db.MarkMain(ctx, copy.ID); err != nil {
			return err
		}
	}
	other, err := s.db.ProjectByRepository(ctx, repo.Dir)
	if errors.Is(err, store.ErrNotFound) {
		return s.db.SetRepository(ctx, p.ID, repo.Dir)
	}
	if err != nil {
		return err
	}
	// The project holding the main checkout keeps its id, so the links to it
	// that a person is most likely to have kept still open it.
	if !repo.Main {
		return s.db.MergeProjects(ctx, p.ID, other.ID)
	}
	if err := s.db.MergeProjects(ctx, other.ID, p.ID); err != nil {
		return err
	}
	return s.db.SetRepository(ctx, p.ID, repo.Dir)
}
