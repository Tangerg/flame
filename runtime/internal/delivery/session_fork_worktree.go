package delivery

import (
	"context"
	"crypto/sha256"
	json "encoding/json/v2"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
)

func (s *Handler) PreviewSessionFork(ctx context.Context, in protocol.PreviewSessionForkRequest) (*protocol.SessionForkPreview, error) {
	parent, err := sessionParameter(in.SessionID)
	if err != nil {
		return nil, err
	}
	from, err := wireIdentity("fromRunId", in.FromRunID, identity.ParseOptionalRun)
	if err != nil {
		return nil, err
	}
	history, err := s.sessions.InspectFork(ctx, parent, from)
	if err != nil {
		return nil, wireSessionErr(wireBoundaryErr(err))
	}
	tree, err := s.taskWorktrees.PreviewFork(ctx, history.Path, "")
	if err != nil {
		return nil, taskWorktreeError(err)
	}
	digest := forkWorktreeDigest(in.SessionID, in.FromRunID, history, tree)
	if digest == "" {
		return nil, protocol.ErrWorkspaceUnavailable
	}
	return &protocol.SessionForkPreview{
		Worktree:  protocol.ForkWorktreeConfirmation{ID: tree.ID, Digest: digest},
		FromRunID: history.RunID.String(), Messages: history.Messages, Runs: history.Runs,
		Source: protocol.WorkspaceRef{Path: tree.Source}, Destination: protocol.WorkspaceRef{Path: tree.Path},
		Head: tree.Head, Patch: strings.ToValidUTF8(tree.Patch, "�"),
	}, nil
}

func (s *Handler) prepareForkWorkspace(in protocol.ForkSessionRequest) func(context.Context, sessions.ForkSource) (string, func(), error) {
	confirmation := *in.Worktree
	return func(ctx context.Context, history sessions.ForkSource) (string, func(), error) {
		path, release, err := s.taskWorktrees.PrepareFork(ctx, history.Path, confirmation.ID, confirmation.Digest, func(tree workspace.WorktreeFork) bool {
			return confirmation.Digest != "" && forkWorktreeDigest(in.SessionID, in.FromRunID, history, tree) == confirmation.Digest
		})
		return path, release, taskWorktreeError(err)
	}
}

func forkWorktreeDigest(session, from string, history sessions.ForkSource, tree workspace.WorktreeFork) string {
	patch := sha256.Sum256([]byte(tree.Patch))
	tree.Patch = ""
	// The snapshot consists only of validated identities, integers and paths.
	raw, err := json.Marshal(struct {
		Session, From string
		History       sessions.ForkSource
		Tree          workspace.WorktreeFork
		Patch         [32]byte
	}{session, from, history, tree, patch})
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
