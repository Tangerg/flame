package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	work "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/git/process"
)

const maxHandoff = 512 << 10

func (b *TaskWorktrees) Preview(ctx context.Context, task work.TaskWorktree) (work.WorktreeHandoff, error) {
	head, err := readHandoffHead(ctx, task.Source)
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	if head.commit != task.Base {
		return work.WorktreeHandoff{}, work.ErrTaskWorktreeChanged
	}
	record, err := b.load(task.ID)
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	current, err := taskCommonDir(ctx, task.Source)
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	if current != record.Common {
		return work.WorktreeHandoff{}, work.ErrTaskWorktreeChanged
	}
	status, err := taskGitBytes(ctx, task.Source, 0, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	if len(status) > 0 {
		return work.WorktreeHandoff{}, fmt.Errorf("%w: destination has local changes", work.ErrTaskWorktreeChanged)
	}
	patch, err := taskPatch(ctx, task)
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	digest := sha256.New()
	fmt.Fprintf(digest, "%q\n%q\n%q\n%q\n", task.ID, task.Source, head.reference, head.commit)
	_, _ = digest.Write(patch)
	return work.WorktreeHandoff{
		Worktree: task, Branch: head.branch(), Head: head.commit, Digest: hex.EncodeToString(digest.Sum(nil)), Patch: string(patch),
	}, nil
}

type handoffHead struct {
	commit    string
	reference string
}

func (h handoffHead) branch() *string {
	if h.reference == "" {
		return nil
	}
	return new(strings.TrimPrefix(h.reference, "refs/heads/"))
}

// Switching branches can keep the current commit, so both come from one Git worktree entry.
func readHandoffHead(ctx context.Context, checkout string) (handoffHead, error) {
	raw, err := taskGitBytes(ctx, checkout, 0, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return handoffHead{}, errors.Join(work.ErrTaskWorktreeChanged, err)
	}
	for entry := range bytes.SplitSeq(raw, []byte{0, 0}) {
		path, fields, _ := bytes.Cut(entry, []byte{0})
		if string(path) != "worktree "+checkout {
			continue
		}
		var head handoffHead
		for field := range bytes.SplitSeq(fields, []byte{0}) {
			name, value, _ := strings.Cut(string(field), " ")
			switch name {
			case "HEAD":
				head.commit = value
			case "branch":
				head.reference = value
			}
		}
		return head, nil
	}
	return handoffHead{}, fmt.Errorf("%w: the destination is missing from Git's worktree list", work.ErrTaskWorktreeChanged)
}

func (b *TaskWorktrees) Apply(ctx context.Context, preview work.WorktreeHandoff) error {
	if preview.Patch == "" {
		return nil
	}
	result, err := process.ApplyIndexPatch(ctx, preview.Worktree.Source, preview.Patch)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%w: git apply exited %d: %s", work.ErrTaskWorktreeChanged, result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return nil
}

func taskPatch(ctx context.Context, task work.TaskWorktree) ([]byte, error) {
	patch, err := taskGitBytes(ctx, task.Path, 0, "diff", "--binary", "--no-ext-diff", "--no-textconv", "--no-color", task.Base, "--")
	if err != nil {
		return nil, err
	}
	if len(patch) > maxHandoff {
		return nil, work.ErrVCSResultTooLarge
	}
	untracked, err := taskGitBytes(ctx, task.Path, 0, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	for name := range bytes.SplitSeq(untracked, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		if !filepath.IsLocal(string(name)) {
			return nil, work.ErrPathOutsideRoot
		}
		delta, err := taskGitBytes(ctx, task.Path, 1, "diff", "--no-index", "--binary", "--no-ext-diff", "--no-textconv", "--no-color", "--", os.DevNull, string(name))
		if err != nil {
			return nil, err
		}
		if len(delta) > maxHandoff-len(patch) {
			return nil, work.ErrVCSResultTooLarge
		}
		patch = append(patch, delta...)
	}
	return patch, nil
}
