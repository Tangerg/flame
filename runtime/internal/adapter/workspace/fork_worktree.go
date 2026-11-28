package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	work "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

func newTaskID() string {
	var key [16]byte
	rand.Read(key[:])
	return hex.EncodeToString(key[:])
}

func (b *TaskWorktrees) PreviewFork(ctx context.Context, path, id string) (work.WorktreeFork, error) {
	if id == "" {
		id = newTaskID()
	}
	if !isTaskID(id) {
		return work.WorktreeFork{}, work.ErrTaskWorktreeInput
	}
	top, err := taskGitValue(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return work.WorktreeFork{}, errors.Join(work.ErrVCSUnavailable, err)
	}
	source, err := filepath.EvalSymlinks(top)
	if err != nil {
		return work.WorktreeFork{}, err
	}
	if source != path {
		return work.WorktreeFork{}, work.ErrTaskWorktreeInput
	}
	common, err := taskCommonDir(ctx, source)
	if err != nil {
		return work.WorktreeFork{}, err
	}
	head, err := taskGitValue(ctx, source, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return work.WorktreeFork{}, err
	}
	patch, err := taskPatch(ctx, work.TaskWorktree{Path: source, Base: head})
	if err != nil {
		return work.WorktreeFork{}, err
	}
	root := b.root
	if physical, err := filepath.EvalSymlinks(root); err == nil {
		root = physical
	} else if !errors.Is(err, os.ErrNotExist) {
		return work.WorktreeFork{}, err
	} else {
		physical, err := filepath.EvalSymlinks(filepath.Dir(root))
		if err != nil {
			return work.WorktreeFork{}, err
		}
		root = filepath.Join(physical, filepath.Base(root))
	}
	if relative, err := filepath.Rel(source, root); err == nil && filepath.IsLocal(relative) {
		return work.WorktreeFork{}, work.ErrTaskWorktreeInput
	}
	return work.WorktreeFork{ID: id, Source: source, Repository: common, Head: head,
		Path: filepath.Join(root, id, taskCheckout), Patch: string(patch)}, nil
}

func (b *TaskWorktrees) CloneFork(ctx context.Context, preview work.WorktreeFork, digest string) (work.TaskWorktree, error) {
	wanted := taskRecord{ID: preview.ID, Source: preview.Source, Common: preview.Repository, Base: preview.Head, ForkDigest: digest}
	saved, err := b.load(preview.ID)
	switch {
	case errors.Is(err, work.ErrTaskWorktreeNotFound):
		return b.createTaskRecord(ctx, wanted, preview.Patch)
	case err != nil:
		return work.TaskWorktree{}, err
	case saved.ForkDigest != digest || saved.Source != wanted.Source || saved.Common != wanted.Common || saved.Base != wanted.Base:
		return work.TaskWorktree{}, work.ErrTaskWorktreeChanged
	}
	task, err := b.inspect(ctx, saved)
	if err != nil {
		return work.TaskWorktree{}, err
	}
	if !task.Available || task.Path != preview.Path {
		return work.TaskWorktree{}, work.ErrTaskWorktreeChanged
	}
	head, err := taskGitValue(ctx, task.Path, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return work.TaskWorktree{}, err
	}
	if head != saved.Base {
		return work.TaskWorktree{}, work.ErrTaskWorktreeChanged
	}
	patch, err := taskPatch(ctx, task)
	if err != nil {
		return work.TaskWorktree{}, err
	}
	if !slices.Equal(forkPatchParts(string(patch)), forkPatchParts(preview.Patch)) {
		return work.TaskWorktree{}, work.ErrTaskWorktreeChanged
	}
	return task, nil
}

// Applying the patch stages additions. Git reports staged and untracked files
// in a different order even when the complete set of changes matches.
func forkPatchParts(patch string) []string {
	parts := strings.Split(patch, "\ndiff --git ")
	// Normalize the leading Git marker so the first entry is comparable with later entries.
	for i := range parts {
		parts[i] = strings.TrimSuffix(strings.TrimPrefix(parts[i], "diff --git "), "\n")
	}
	slices.Sort(parts)
	return parts
}
