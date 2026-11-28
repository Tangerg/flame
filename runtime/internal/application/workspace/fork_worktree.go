package workspace

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
)

type WorktreeFork struct {
	ID, Source, Head, Path, Patch string
	Repository                    string
}

func (w *TaskWorktrees) PreviewFork(ctx context.Context, source, id string) (WorktreeFork, error) {
	path, err := w.scope.ResolveRoot(source)
	if err != nil {
		return WorktreeFork{}, err
	}
	return w.backend.PreviewFork(ctx, path, id)
}

func (w *TaskWorktrees) PrepareFork(ctx context.Context, source, id, digest string, matches func(WorktreeFork) bool) (string, func(), error) {
	path, err := w.scope.ResolveRoot(source)
	if err != nil {
		return "", nil, err
	}
	release, err := w.claim(path)
	if err != nil {
		return "", nil, err
	}
	preview, err := w.backend.PreviewFork(ctx, path, id)
	if err != nil || !matches(preview) {
		release()
		return "", nil, errors.Join(ErrTaskWorktreeChanged, err)
	}
	releaseTarget, err := w.claim(preview.Path)
	if err != nil {
		release()
		return "", nil, err
	}
	done := func() { releaseTarget(); release() }
	target, registered, err := w.backend.CloneFork(ctx, preview, digest)
	if err != nil {
		done()
		return "", nil, err
	}
	if target.Path != preview.Path {
		done()
		return "", nil, ErrTaskWorktreeChanged
	}
	if registered {
		w.invalidations.Notify(invalidation.Notice{Resource: invalidation.TaskWorktrees})
	}
	return target.Path, done, nil
}
