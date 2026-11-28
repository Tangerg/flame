package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
)

func (s *Handler) PreviewSessionFork(ctx context.Context, in protocol.PreviewSessionForkRequest) (*protocol.SessionForkPreview, error) {
	spec, err := forkSpec(in.SessionID, in.FromRunID, "")
	if err != nil {
		return nil, err
	}
	preview, err := s.worktreeForks.Preview(ctx, spec)
	if err != nil {
		return nil, wireForkError(err, spec.ParentID)
	}
	history, tree := preview.History, preview.Checkout
	return &protocol.SessionForkPreview{
		Worktree:  protocol.ForkWorktreeConfirmation{ID: tree.ID, Digest: preview.Digest},
		FromRunID: history.RunID.String(), Messages: history.Messages, Runs: history.Runs,
		Source: protocol.WorkspaceRef{Path: tree.Source}, Destination: protocol.WorkspaceRef{Path: tree.Path},
		Head: tree.Head, Patch: tree.Patch,
	}, nil
}

func (s *Handler) replayFork(ctx context.Context, in protocol.ForkSessionRequest, call keyedCall) protocol.OperationResult {
	if in.Worktree == nil {
		return call.invokeAtomic(ctx, func(outcome atomicOutcome) error {
			spec, err := forkSpec(in.SessionID, in.FromRunID, in.Title)
			if err != nil {
				return outcome.commitFailure(ctx, err)
			}
			err = s.sessions.Fork(ctx, spec, outcome.sessionView())
			return outcome.settleRefusal(ctx, wireForkError(err, spec.ParentID))
		})
	}
	receipt := operation.ReplayClaim{Key: call.key, Fingerprint: call.fingerprint}
	payload, err := call.replay.InvokeRecoverable(ctx, call.key, call.fingerprint, call.admit, func(retained bool) error {
		if retained {
			return s.worktreeForks.Resume(ctx, receipt)
		}
		spec, err := forkSpec(in.SessionID, in.FromRunID, in.Title)
		if err != nil {
			return err
		}
		if in.Worktree.ID == "" {
			return protocol.NewFailure(protocol.ErrInvalidParams, "fork worktree identity is required")
		}
		approval := sessions.WorktreeForkApproval{ID: in.Worktree.ID, Digest: in.Worktree.Digest}
		return wireForkError(s.worktreeForks.Fork(ctx, spec, approval, receipt), spec.ParentID)
	}, encodeAcknowledgement)
	return call.result(ctx, payload, err)
}

func forkSpec(parent, from, title string) (sessions.ForkSpec, error) {
	sessionID, err := sessionParameter(parent)
	if err != nil {
		return sessions.ForkSpec{}, err
	}
	fromRunID, err := wireIdentity("fromRunId", from, identity.ParseOptionalRun)
	return sessions.ForkSpec{ParentID: sessionID, FromRunID: fromRunID, Title: title}, err
}

func wireForkError(err error, parent identity.SessionID) error {
	return wireSessionMutationErr(taskWorktreeError(wireBoundaryErr(err)), parent)
}

func EncodeWorktreeForkReceipt(child sessions.View) ([]byte, error) {
	method, _ := contract.lookup(sessionsFork)
	return encodeAtomicResult(method, new(presentSession(child)))
}
