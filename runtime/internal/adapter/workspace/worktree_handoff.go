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

func (t *taskCheckout) PreviewHandoff(ctx context.Context) (work.WorktreeHandoff, error) {
	registration, err := t.checkoutRegistration(ctx)
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	if registration == nil {
		return work.WorktreeHandoff{}, work.ErrTaskWorktreeNotFound
	}
	source := t.record.Source
	head, err := readHandoffHead(ctx, source)
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	if head.commit != t.record.Base {
		return work.WorktreeHandoff{}, work.ErrTaskWorktreeChanged
	}
	current, err := taskCommonDir(ctx, source)
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	if current != t.record.CommonDir {
		return work.WorktreeHandoff{}, work.ErrTaskWorktreeChanged
	}
	status, err := taskGitBytes(ctx, source, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	if len(status) > 0 {
		return work.WorktreeHandoff{}, fmt.Errorf("%w: destination has local changes", work.ErrTaskWorktreeChanged)
	}
	patch, err := reviewableWorktreePatch(ctx, t.path, t.record.Base)
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	digest := sha256.New()
	fmt.Fprintf(digest, "%q\n%q\n%q\n%q\n", t.record.ID, source, head.reference, head.commit)
	_, _ = digest.Write(patch)
	return work.WorktreeHandoff{
		Worktree: t.view(registration), Branch: head.branch(), Head: head.commit, Digest: hex.EncodeToString(digest.Sum(nil)), Patch: string(patch),
	}, nil
}

func reviewableWorktreePatch(ctx context.Context, checkout, base string) ([]byte, error) {
	patch, err := git.WorktreePatch(ctx, checkout, base, maxHandoffPatchBytes)
	if err != nil {
		return nil, vcsError(err)
	}
	if !utf8.Valid(patch) {
		return nil, work.ErrWorktreePatchEncoding
	}
	return patch, nil
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

func (t *taskCheckout) PrepareHandoff(ctx context.Context, digest string) (func(context.Context) (work.WorktreeHandoff, error), error) {
	preview, err := t.PreviewHandoff(ctx)
	if err != nil {
		return nil, err
	}
	if preview.Digest != digest {
		return nil, work.ErrTaskWorktreeChanged
	}
	return func(write context.Context) (work.WorktreeHandoff, error) {
		if err := applyWorktreePatch(write, t.record.Source, preview.Patch); err != nil {
			return work.WorktreeHandoff{}, err
		}
		return preview, nil
	}, nil
}

func applyWorktreePatch(ctx context.Context, checkout, patch string) error {
	if patch == "" {
		return nil
	}
	result, err := process.ApplyIndexPatch(ctx, checkout, patch)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%w: git apply exited %d: %s", work.ErrTaskWorktreeChanged, result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return nil
}
