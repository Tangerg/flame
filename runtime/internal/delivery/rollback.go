package delivery

import (
	"context"
	"errors"
	"fmt"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func (s *Handler) RollbackSession(ctx context.Context, in protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
	restoreType, err := rollbackTypeFromWire(in)
	if err != nil {
		return nil, err
	}
	if restoreType != protocol.RestoreHistory {
		result, err := s.rollbackWorkspace(ctx, in, restoreType, sessions.WorkspaceRollbackReceipt{}, nil)
		if err != nil {
			return nil, err
		}
		return presentRollbackResult(result), nil
	}
	var response *protocol.RollbackSessionResponse
	err = s.rollbackHistory(ctx, in, func(ctx context.Context, write func(context.Context) (sessions.RollbackResult, error)) error {
		result, err := write(ctx)
		if err != nil {
			return err
		}
		response = presentRollbackResult(result)
		return nil
	})
	return response, err
}

func (s *Handler) rollbackHistory(ctx context.Context, in protocol.RollbackSessionRequest, complete sessions.RollbackCompletion) error {
	if err := s.sessions.RollbackHistory(ctx, in.SessionID, in.ToRunID, complete); err != nil {
		return wireRollbackErr(err, in.SessionID)
	}
	return nil
}

func (s *Handler) rollbackWorkspace(ctx context.Context, in protocol.RollbackSessionRequest, restoreType protocol.RestoreType, receipt sessions.WorkspaceRollbackReceipt, encode sessions.RollbackReceiptEncoder) (sessions.RollbackResult, error) {
	result, err := s.sessions.RollbackWorkspace(ctx, sessions.WorkspaceRollbackSpec{
		SessionID: in.SessionID, ToRunID: in.ToRunID,
		RestoreHistory: restoreType == protocol.RestoreBoth, Receipt: receipt,
	}, encode)
	if err != nil {
		return sessions.RollbackResult{}, wireRollbackErr(err, in.SessionID)
	}
	return result, nil
}

func (e *Endpoint) replayRollback(ctx context.Context, s *Handler, method *Method, in protocol.RollbackSessionRequest, key, fingerprint string) ([]byte, error) {
	restoreType, err := rollbackTypeFromWire(in)
	if err != nil || restoreType == protocol.RestoreHistory {
		return e.idempotency.InvokeAtomic(ctx, key, fingerprint, func(complete *operation.AtomicCompletion) error {
			if err != nil {
				return commitAtomicFailure(ctx, complete, err)
			}
			return s.commitRollbackOutcome(ctx, method, in, complete)
		})
	}
	return e.idempotency.InvokeRecoverable(ctx, key, fingerprint, func() ([]byte, error) {
		receipt := sessions.WorkspaceRollbackReceipt{Key: key, Fingerprint: fingerprint}
		var payload []byte
		_, err := s.rollbackWorkspace(ctx, in, restoreType, receipt, func(sessionID string, result sessions.RollbackResult, cause error) ([]byte, error) {
			encoded, err := EncodeWorkspaceRollbackReceipt(sessionID, result, cause)
			payload = encoded
			return encoded, err
		})
		if errors.Is(err, sessions.ErrRollbackRecoveryPending) {
			return nil, errors.Join(operation.ErrOutcomePending, err)
		}
		if len(payload) != 0 {
			return payload, nil
		}
		return nil, err
	}, func(err error) ([]byte, error) {
		return encodeStoredOutcome(failed(projectError(err)))
	})
}

func EncodeWorkspaceRollbackReceipt(sessionID string, result sessions.RollbackResult, cause error) ([]byte, error) {
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

// wireRollbackErr maps session rollback errors onto their wire
// errors (the boundary sentinels are already wire-mapped inside the resolver).
func wireRollbackErr(err error, sessionID string) error {
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
