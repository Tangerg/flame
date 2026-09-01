package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/application/agent/mutation"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

type deletionRuntime interface {
	DeleteSession(context.Context, agent.DeleteSession) error
	GetSession(context.Context, string) (agent.SessionSnapshot, error)
}

// DeletionResult binds settlement to the exact durable runtime command.
type DeletionResult struct {
	Request agent.DeleteSession
	Outcome mutation.Outcome
}

// Delete stages or resumes one deletion intent, then converges its runtime
// outcome without modifying local authoring state. The caller must apply the
// returned result at its presentation boundary with ConfirmDeletion or
// RejectDeletion.
func Delete(
	ctx context.Context,
	runtime deletionRuntime,
	authoring *workbench.Store,
	sessionID string,
	policy commandreplay.Policy,
	backoff retry.Backoff,
) (DeletionResult, error) {
	if authoring == nil {
		return DeletionResult{}, errors.New("CLI workbench is unavailable")
	}
	if err := runtimeprotocol.ValidateSessionID(sessionID); err != nil {
		return DeletionResult{}, err
	}
	pending, exists := authoring.PendingSessionDeletion(sessionID)
	fresh := !exists
	if exists && pending.Phase == workbench.SessionDeletionConfirmed {
		return DeletionResult{Request: pending.Request(), Outcome: mutation.Confirmed}, nil
	}
	request := pending.Request()
	if !exists {
		commandID, err := agent.NewCommandID()
		if err != nil {
			return DeletionResult{}, fmt.Errorf("create session deletion identity: %w", err)
		}
		request = agent.DeleteSession{CommandID: commandID, SessionID: sessionID}
		replay, err := policy.NewGuard()
		if err != nil {
			return DeletionResult{}, err
		}
		if err := authoring.StageSessionDeletion(request, replay); err != nil {
			return DeletionResult{}, fmt.Errorf("stage session deletion: %w", err)
		}
		pending, exists = authoring.PendingSessionDeletion(sessionID)
		if !exists {
			return DeletionResult{}, errors.New("staged session deletion is absent")
		}
	}
	if fresh {
		outcome, err := settleDeletion(ctx, runtime, request, pending.Replay, policy, backoff, true)
		return DeletionResult{Request: request, Outcome: outcome}, err
	}
	if !policy.Replayable(pending.Replay) {
		outcome, err := resolveExpired(ctx, runtime, pending.SessionID, pending.Replay, policy)
		return DeletionResult{Request: request, Outcome: outcome}, err
	}
	outcome, err := settleDeletion(ctx, runtime, request, pending.Replay, policy, backoff, false)
	return DeletionResult{Request: request, Outcome: outcome}, err
}

// settleDeletion observes the authoritative runtime outcome. A successful
// delete may still return a post-commit cleanup error, so an authoritative
// not-found read confirms a missing acknowledgement.
func settleDeletion(
	ctx context.Context,
	runtime deletionRuntime,
	request agent.DeleteSession,
	replay commandreplay.Guard,
	policy commandreplay.Policy,
	backoff retry.Backoff,
	fresh bool,
) (mutation.Outcome, error) {
	admit := mutation.ReplayAdmission(policy, replay)
	if fresh {
		admit = mutation.FreshReplayAdmission(policy, replay)
	}
	_, err := mutation.ConfirmAdmitted(ctx, backoff, admit, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, runtime.DeleteSession(ctx, request)
	})
	if err == nil || errors.Is(err, agent.ErrSessionNotFound) {
		return mutation.Confirmed, nil
	}
	if errors.Is(err, mutation.ErrReplayGuaranteeUnavailable) {
		outcome, resolveErr := resolveExpired(ctx, runtime, request.SessionID, replay, policy)
		if outcome != mutation.Unknown {
			return outcome, resolveErr
		}
		return mutation.Unknown, errors.Join(
			fmt.Errorf("delete session outcome is unknown: %w", err), resolveErr,
		)
	}
	if mutation.OutcomeUnknown(err) {
		return mutation.Unknown, fmt.Errorf("delete session outcome is unknown: %w", err)
	}
	_, readErr := runtime.GetSession(ctx, request.SessionID)
	if errors.Is(readErr, agent.ErrSessionNotFound) {
		return mutation.Confirmed, nil
	}
	if readErr != nil {
		return mutation.Unknown, errors.Join(
			fmt.Errorf("delete session: %w", err),
			fmt.Errorf("read deletion outcome: %w", readErr),
		)
	}
	return mutation.Rejected, err
}

// ConfirmDeletion upgrades a prepared command to a durable tombstone and retires all
// local state. It is idempotent for an already-confirmed cleanup record.
func ConfirmDeletion(authoring *workbench.Store, result DeletionResult) error {
	pending, exists := authoring.PendingSessionDeletion(result.Request.SessionID)
	if exists && pending.Phase == workbench.SessionDeletionPrepared {
		return authoring.ConfirmSessionDeletion(result.Request.SessionID, result.Request.CommandID)
	}
	return authoring.RetireSessionState(result.Request.SessionID)
}

// RejectDeletion removes only the exact prepared intent after a definitive refusal.
func RejectDeletion(authoring *workbench.Store, result DeletionResult) error {
	return authoring.RejectSessionDeletion(result.Request.SessionID, result.Request.CommandID)
}

// RecoverDeletions settles every journal before any session draft is made visible.
func RecoverDeletions(
	ctx context.Context,
	runtime deletionRuntime,
	authoring *workbench.Store,
	policy commandreplay.Policy,
	backoff retry.Backoff,
) error {
	for _, pending := range authoring.PendingSessionDeletions() {
		if pending.Phase == workbench.SessionDeletionConfirmed {
			if err := authoring.RetireSessionState(pending.SessionID); err != nil {
				return err
			}
			continue
		}
		result := DeletionResult{Request: pending.Request()}
		if !policy.Replayable(pending.Replay) {
			outcome, err := resolveExpired(ctx, runtime, pending.SessionID, pending.Replay, policy)
			result.Outcome = outcome
			switch outcome {
			case mutation.Confirmed:
				if confirmErr := ConfirmDeletion(authoring, result); confirmErr != nil {
					return confirmErr
				}
			case mutation.Rejected:
				if rejectErr := RejectDeletion(authoring, result); rejectErr != nil {
					return rejectErr
				}
			case mutation.Unknown:
				return fmt.Errorf("recover session deletion %s: %w", pending.SessionID, err)
			}
			continue
		}
		outcome, err := settleDeletion(ctx, runtime, result.Request, pending.Replay, policy, backoff, false)
		result.Outcome = outcome
		switch outcome {
		case mutation.Confirmed:
			if confirmErr := ConfirmDeletion(authoring, result); confirmErr != nil {
				return confirmErr
			}
		case mutation.Rejected:
			if rejectErr := RejectDeletion(authoring, result); rejectErr != nil {
				return errors.Join(err, rejectErr)
			}
		case mutation.Unknown:
			return err
		}
	}
	return nil
}

func resolveExpired(
	ctx context.Context,
	runtime deletionRuntime,
	sessionID string,
	replay commandreplay.Guard,
	policy commandreplay.Policy,
) (mutation.Outcome, error) {
	if !policy.SameStore(replay) {
		return mutation.Unknown, errors.New("session deletion belongs to another runtime")
	}
	_, err := runtime.GetSession(ctx, sessionID)
	if errors.Is(err, agent.ErrSessionNotFound) {
		return mutation.Confirmed, nil
	}
	if err != nil {
		return mutation.Unknown, fmt.Errorf("read deletion outcome: %w", err)
	}
	return mutation.Rejected, nil
}
