package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	work "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/git"
)

type forkRegistration struct {
	Approval string `json:"approval"`
	Checkout string `json:"checkout"`
}

func (b *TaskWorktrees) PreviewFork(ctx context.Context, path, id string) (work.WorktreeFork, error) {
	var err error
	if id == "" {
		id, err = newTaskID()
		if err != nil {
			return work.WorktreeFork{}, err
		}
	}
	if !isTaskID(id) {
		return work.WorktreeFork{}, work.ErrTaskWorktreeInput
	}
	source, common, err := b.resolveTaskSource(ctx, path)
	if err != nil {
		return work.WorktreeFork{}, err
	}
	if source != path {
		return work.WorktreeFork{}, work.ErrTaskWorktreeInput
	}
	head, err := taskGitValue(ctx, source, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return work.WorktreeFork{}, err
	}
	patch, err := reviewableWorktreePatch(ctx, source, head)
	if err != nil {
		return work.WorktreeFork{}, err
	}
	return work.WorktreeFork{
		ID: id, Source: source, Repository: common, Head: head, Path: b.checkoutPath(id), Patch: string(patch),
	}, nil
}

func (b *TaskWorktrees) CloneFork(ctx context.Context, preview work.WorktreeFork, digest string) (work.TaskWorktree, bool, error) {
	checkout, err := b.openCheckout(preview.ID)
	switch {
	case errors.Is(err, work.ErrTaskWorktreeNotFound):
		record := taskRecord{
			ID: preview.ID, Source: preview.Source, CommonDir: preview.Repository, Base: preview.Head,
			Fork: &forkRegistration{Approval: digest},
		}
		task, err := b.registerTask(ctx, ctx, record, preview.Patch)
		return task, err == nil, err
	case err != nil:
		return work.TaskWorktree{}, false, err
	}
	saved := checkout.record
	if saved.Fork == nil || saved.Fork.Approval != digest || saved.Source != preview.Source ||
		saved.CommonDir != preview.Repository || saved.Base != preview.Head {
		return work.TaskWorktree{}, false, work.ErrTaskWorktreeChanged
	}
	task, err := checkout.Inspect(ctx)
	if err != nil {
		return work.TaskWorktree{}, false, err
	}
	if !task.Available || task.Path != preview.Path {
		return work.TaskWorktree{}, false, work.ErrTaskWorktreeChanged
	}
	head, err := taskGitValue(ctx, task.Path, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return work.TaskWorktree{}, false, err
	}
	if head != saved.Base {
		return work.TaskWorktree{}, false, work.ErrTaskWorktreeChanged
	}
	current, err := forkCheckoutDigest(ctx, task.Path, task.Base)
	if err != nil {
		return work.TaskWorktree{}, false, err
	}
	if current != saved.Fork.Checkout {
		return work.TaskWorktree{}, false, work.ErrTaskWorktreeChanged
	}
	return task, false, nil
}

func forkCheckoutDigest(ctx context.Context, path, base string) (string, error) {
	// Git can infer a rename after an untracked file becomes staged.
	patch, err := git.WorktreePatch(ctx, path, base, maxHandoffPatchBytes)
	if err != nil {
		return "", vcsError(err)
	}
	digest := sha256.Sum256(patch)
	return hex.EncodeToString(digest[:]), nil
}
