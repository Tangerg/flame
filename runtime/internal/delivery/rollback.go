package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

const toRunIDField = "toRunId"

func (s *Handler) RollbackSession(ctx context.Context, in protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
	restoreType, err := rollbackTypeFromWire(in)
	if err != nil {
		return nil, err
	}
	if restoreType != protocol.RestoreHistory {
		result, err := s.rollbackWorkspace(ctx, in, restoreType, sessions.WorkspaceRollbackReceipt{})
		if err != nil {
			return nil, err
		}
		return presentRollbackResult(result), nil
	}
	var result sessions.RollbackResult
	if err := s.rollbackHistory(ctx, in, directReply(&result)); err != nil {
		return nil, err
	}
	return presentRollbackResult(result), nil
}

func (s *Handler) rollbackHistory(ctx context.Context, in protocol.RollbackSessionRequest, complete operation.Reply[sessions.RollbackResult]) error {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return parseErr
	}
	toRunID, err := wireIdentity(toRunIDField, in.ToRunID, identity.ParseOptionalRun)
	if err != nil {
		return err
	}
	if err := s.sessions.RollbackHistory(ctx, sessionID, toRunID, complete); err != nil {
		return wireRollbackErr(err, sessionID)
	}
	return nil
}

func (s *Handler) rollbackWorkspace(ctx context.Context, in protocol.RollbackSessionRequest, restoreType protocol.RestoreType, receipt sessions.WorkspaceRollbackReceipt) (sessions.RollbackResult, error) {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return sessions.RollbackResult{}, parseErr
	}
	toRunID, err := wireIdentity(toRunIDField, in.ToRunID, identity.ParseRun)
	if err != nil {
		return sessions.RollbackResult{}, err
	}
	result, err := s.sessions.RollbackWorkspace(ctx, sessions.WorkspaceRollbackSpec{
		SessionID: sessionID, ToRunID: toRunID,
		RestoreHistory: restoreType == protocol.RestoreBoth, Receipt: receipt,
	})
	if err != nil {
		return sessions.RollbackResult{}, wireRollbackErr(err, sessionID)
	}
	return result, nil
}

func (s *Handler) replayRollback(ctx context.Context, in protocol.RollbackSessionRequest, call keyedCall) protocol.OperationResult {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return protocol.OperationResult{}
	}
	restoreType, refusal := rollbackTypeFromWire(in)
	if refusal != nil || restoreType == protocol.RestoreHistory {
		return call.invokeAtomic(ctx, func(outcome atomicOutcome) error {
			if refusal != nil {
				return outcome.commitFailure(ctx, refusal)
			}
			reply := atomicReply(outcome.complete, func(result sessions.RollbackResult) ([]byte, error) {
				return encodeAtomicResult(outcome.method, presentRollbackResult(result))
			})
			return outcome.settleRefusal(ctx, s.rollbackHistory(ctx, in, reply))
		})
	}
	payload, err := call.replay.InvokeRecoverable(ctx, call.key, call.fingerprint, func() ([]byte, error) {
		receipt := sessions.WorkspaceRollbackReceipt{Key: call.key, Fingerprint: call.fingerprint}
		result, err := s.rollbackWorkspace(ctx, in, restoreType, receipt)
		if errors.Is(err, sessions.ErrRollbackRecoveryPending) {
			return nil, errors.Join(operation.ErrOutcomePending, err)
		}
		if err != nil {
			return nil, err
		}
		return EncodeWorkspaceRollbackReceipt(sessionID, result, nil)
	}, func(err error) ([]byte, error) {
		return encodeStoredOutcome(failed(projectError(err)))
	})
	return call.result(ctx, payload, err)
}

func EncodeWorkspaceRollbackReceipt(sessionID identity.SessionID, result sessions.RollbackResult, cause error) ([]byte, error) {
	if cause != nil {
		return encodeStoredOutcome(failed(projectError(wireRollbackErr(cause, sessionID))))
	}
	method, _ := contract.lookup(sessionsRollback)
	return encodeAtomicResult(method, presentRollbackResult(result))
}

func rollbackTypeFromWire(in protocol.RollbackSessionRequest) (protocol.RestoreType, error) {
	switch in.RestoreType {
	case "", protocol.RestoreHistory:
		return protocol.RestoreHistory, nil
	case protocol.RestoreFiles, protocol.RestoreBoth:
		if in.ToRunID == "" {
			return "", protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("restoreType %q requires toRunId", in.RestoreType))
		}
		return in.RestoreType, nil
	default:
		return "", protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown restoreType %q", in.RestoreType))
	}
}

func presentRollbackResult(result sessions.RollbackResult) *protocol.RollbackSessionResponse {
	droppedRuns := make([]protocol.DroppedRun, 0, len(result.Dropped))
	for _, dropped := range result.Dropped {
		input := make([]protocol.ContentBlock, len(dropped.UserInput))
		for i, block := range dropped.UserInput {
			input[i] = presentContent(block)
		}
		droppedRuns = append(droppedRuns, protocol.DroppedRun{Run: presentRunSummary(dropped.Run), UserInput: input})
	}
	sess := presentSession(result.Session)
	return &protocol.RollbackSessionResponse{Session: &sess, DroppedRuns: droppedRuns}
}

func wireRollbackErr(err error, sessionID identity.SessionID) error {
	switch {
	case errors.Is(err, sessions.ErrRollbackRecoveryPending):
		return protocol.NewFailure(protocol.ErrInternalError, "file rollback recovery is still pending; retain the original request for recovery", err)
	case errors.Is(err, sessions.ErrCheckpointRestoreIncomplete):
		return err
	case errors.Is(err, rundomain.ErrSessionBusy):
		return protocol.NewFailure(protocol.ErrSessionBusy, fmt.Sprintf("session %q has a run in flight", sessionID))
	case errors.Is(err, sessions.ErrWorkspaceMutationPending):
		return protocol.NewFailure(protocol.ErrSessionBusy, fmt.Sprintf("session %q has an unfinished file rollback that must be recovered first", sessionID))
	case errors.Is(err, sessions.ErrCheckpointUnavailable):
		return protocol.ErrCheckpointUnavailable
	case errors.Is(err, sessions.ErrCheckpointConflict):
		return protocol.NewFailure(protocol.ErrCheckpointConflict, "file restore was refused to preserve unarchived workspace material; move the conflicting files or directories before trying again", err)
	case errors.Is(err, transcript.ErrRunNotFound):
		return protocol.ErrRunNotFound
	case errors.Is(err, transcript.ErrNotRoot):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	default:
		return wireSessionErr(err)
	}
}

func wireBoundaryErr(err error) error {
	switch {
	case errors.Is(err, transcript.ErrRunNotFound):
		return protocol.ErrRunNotFound
	case errors.Is(err, transcript.ErrNotRoot):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	default:
		return err
	}
}
