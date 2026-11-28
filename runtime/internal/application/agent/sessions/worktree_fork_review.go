package sessions

import (
	"context"
	"crypto/sha256"
	json "encoding/json/v2"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/workspace"
)

type WorktreeForkApproval struct {
	ID     string
	Digest string
}

type WorktreeForkPreview struct {
	History  ForkSource
	Checkout workspace.WorktreeFork
	Digest   string
}

func (f *WorktreeForks) Preview(ctx context.Context, spec ForkSpec) (WorktreeForkPreview, error) {
	basis, err := f.sessions.readForkBasis(ctx, spec.ParentID, spec.FromRunID)
	if err != nil {
		return WorktreeForkPreview{}, err
	}
	history := basis.source()
	checkout, err := f.worktrees.PreviewFork(ctx, history.Path, "")
	if err != nil {
		return WorktreeForkPreview{}, err
	}
	digest, err := forkReviewDigest(spec, history, checkout)
	if err != nil {
		return WorktreeForkPreview{}, err
	}
	return WorktreeForkPreview{History: history, Checkout: checkout, Digest: digest}, nil
}

func forkReviewDigest(spec ForkSpec, history ForkSource, tree workspace.WorktreeFork) (string, error) {
	patch := sha256.Sum256([]byte(tree.Patch))
	tree.Patch = ""
	raw, err := json.Marshal(struct {
		Session, From string
		History       ForkSource
		Tree          workspace.WorktreeFork
		Patch         [32]byte
	}{spec.ParentID.String(), spec.FromRunID.String(), history, tree, patch})
	if err != nil {
		return "", fmt.Errorf("sessions: encode worktree fork review: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}
