package workspace

import (
	"context"
	"errors"
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
	target, err := w.backend.CloneFork(ctx, preview, digest)
	if err != nil {
		release()
		return "", nil, err
	}
	if target.Path != preview.Path {
		release()
		return "", nil, ErrTaskWorktreeChanged
	}
	releaseTarget, err := w.claim(target.Path)
	if err != nil {
		release()
		return "", nil, err
	}
	return target.Path, func() { releaseTarget(); release() }, nil
}
