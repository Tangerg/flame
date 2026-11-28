package workspace

import (
	"context"
)

type WorktreeFork struct {
	ID, Source, Head, Path, Patch string
	Repository                    string
}

type ForkWriteReceipt struct{ Refusal []byte }

func (w *TaskWorktrees) PreviewFork(ctx context.Context, source, id string) (WorktreeFork, error) {
	path, err := w.scope.ResolveRoot(source)
	if err != nil {
		return WorktreeFork{}, err
	}
	return w.backend.PreviewFork(ctx, path, id)
}

func (w *TaskWorktrees) PrepareFork(ctx context.Context, source, id string, approve func(WorktreeFork) error) (WorktreeFork, func(), error) {
	path, err := w.scope.ResolveRoot(source)
	if err != nil {
		return WorktreeFork{}, nil, err
	}
	release, err := w.claim(path)
	if err != nil {
		return WorktreeFork{}, nil, err
	}
	preview, err := w.backend.PreviewFork(ctx, path, id)
	if err != nil {
		release()
		return WorktreeFork{}, nil, err
	}
	if err := approve(preview); err != nil {
		release()
		return WorktreeFork{}, nil, err
	}
	releaseTarget, err := w.claim(preview.Path)
	if err != nil {
		release()
		return WorktreeFork{}, nil, err
	}
	done := func() { releaseTarget(); release() }
	if err := w.backend.RequireVacantCheckout(preview.ID); err != nil {
		done()
		return WorktreeFork{}, nil, err
	}
	return preview, done, nil
}
