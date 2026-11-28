package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	work "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/git/process"
	"github.com/google/uuid"
)

const forkCheckoutCommitted = "checkout_registered"

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

func (b *TaskWorktrees) RequireVacantCheckout(id string) error {
	for _, path := range []string{b.checkoutPath(id), filepath.Join(b.root, taskRecordName(id))} {
		if _, err := os.Lstat(path); err == nil {
			return work.ErrTaskWorktreeChanged
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (b *TaskWorktrees) WriteFork(ctx context.Context, id uuid.UUID, fork work.WorktreeFork) error {
	start, err := b.prepare(writeFork, fork)
	if err != nil {
		return err
	}
	return start(ctx, id, []byte(forkCheckoutCommitted))
}

func (b *TaskWorktrees) ReadForkReceipt(id uuid.UUID) (work.ForkWriteReceipt, bool, error) {
	payload, err := b.ReadReceipt(id)
	if err != nil {
		return work.ForkWriteReceipt{}, false, err
	}
	if bytes.Equal(payload, []byte(forkCheckoutCommitted)) {
		return work.ForkWriteReceipt{}, true, nil
	}
	return work.ForkWriteReceipt{Refusal: payload}, len(payload) != 0, nil
}

func writeForkCheckout(ctx context.Context, fork work.WorktreeFork, index string) error {
	if !isTaskID(fork.ID) || !validTaskSHA(fork.Head) || filepath.Base(fork.Path) != fork.ID ||
		!filepath.IsAbs(fork.Path) || !filepath.IsAbs(fork.Repository) || !filepath.IsAbs(fork.Source) {
		return work.ErrTaskWorktreeInput
	}
	// An inapplicable patch must fail before Git registers a checkout.
	tree, err := process.PrepareCheckoutTree(ctx, fork.Repository, fork.Head, fork.Patch, index)
	if err != nil {
		return errors.Join(work.ErrTaskWorktreeChanged, err)
	}
	directory := filepath.Dir(fork.Path)
	if err := os.MkdirAll(directory, filesystem.PrivateDirectoryMode); err != nil {
		return err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	if _, err := taskGitValue(ctx, fork.Repository, "worktree", "add", "--detach", "--quiet", "--no-checkout", "--", fork.Path, fork.Head); err != nil {
		return errors.Join(work.ErrVCSUnavailable, err)
	}
	// Keep the approved HEAD while Git populates the index and working tree
	// from the prepared tree. A later failure leaves the admitted write unconfirmed.
	if _, err := taskGitValue(ctx, fork.Path, "read-tree", "--reset", "-u", tree); err != nil {
		return errors.Join(operation.ErrOutcomePending, err)
	}
	if err := stampTaskCustody(ctx, fork.Path, fork.ID); err != nil {
		return errors.Join(operation.ErrOutcomePending, err)
	}
	record := taskRecord{ID: fork.ID, Source: fork.Source, CommonDir: fork.Repository, Base: fork.Head}
	if err := writeTaskRecord(root, record); err != nil {
		return fmt.Errorf("workspace: publish fork checkout: %w", errors.Join(operation.ErrOutcomePending, err))
	}
	return nil
}
