package delivery

import (
	"context"
	"errors"
	"fmt"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// RollbackSession discards the runs after the kept boundary, truncating the
// session in place at a run granularity. Destructive: it
// truncates the conversation message log to the kept watermark, deletes the
// dropped Runs' durable items and records, clears dangling interrupts, and
// stops detached processes whose handles belonged to the discarded Session
// context. ToRunID is inclusive-keep (omit = clear to empty). Rejected with
// session_busy while a Run is in flight, and — for a file restore — while the
// boundary keeps a Run that would resume into the rewritten tree.
//
// The whole guarded operation — single-writer + working-tree admission, working
// tree restore, and durable truncation — belongs to the session use case. This
// method only decodes the wire intent and projects the result.
func (s *Handler) RollbackSession(ctx context.Context, in protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
	var response *protocol.RollbackSessionResponse
	err := s.rollbackSession(ctx, in, func(ctx context.Context, write func(context.Context) (sessions.RollbackResult, error)) error {
		result, err := write(ctx)
		if err != nil {
			return err
		}
		response = presentRollbackResult(result)
		return nil
	})
	return response, err
}

func (s *Handler) rollbackSession(ctx context.Context, in protocol.RollbackSessionRequest, complete sessions.RollbackCompletion) error {
	scope, err := rollbackScopeFromWire(in)
	if err != nil {
		return err
	}
	err = s.sessions.Rollback(ctx, sessions.RollbackSpec{
		SessionID: in.SessionID,
		ToRunID:   in.ToRunID,
		Scope:     scope,
	}, complete)
	if err != nil {
		return wireRollbackErr(err, in.SessionID)
	}
	return nil
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
