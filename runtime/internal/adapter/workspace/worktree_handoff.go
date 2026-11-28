package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	work "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/git"
	"github.com/Tangerg/flame/runtime/internal/infra/git/process"
)

const maxHandoffPatchBytes = 512 << 10

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
	if current != record.CommonDir {
		return work.WorktreeHandoff{}, work.ErrTaskWorktreeChanged
	}
	status, err := taskGitBytes(ctx, task.Source, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	if len(status) > 0 {
		return work.WorktreeHandoff{}, fmt.Errorf("%w: destination has local changes", work.ErrTaskWorktreeChanged)
	}
	patch, err := git.WorktreePatch(ctx, task.Path, task.Base, maxHandoffPatchBytes)
	if err != nil {
		return work.WorktreeHandoff{}, vcsError(err)
	}
	if !utf8.Valid(patch) {
		return work.WorktreeHandoff{}, work.ErrHandoffPatchEncoding
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
	raw, err := taskGitBytes(ctx, checkout, "worktree", "list", "--porcelain", "-z")
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
