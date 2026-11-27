package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/retry"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type deletionRuntime interface {
	DeleteSession(context.Context, conversation.DeleteSession) error
	GetSession(context.Context, string) (conversation.SessionSnapshot, error)
}

type ConfirmedDeletion struct {
	LocalCleanupError error
}

type deletionSettlement struct {
	outcome mutation.Outcome
	failure error
	cleanup error
}

func Delete(
	ctx context.Context,
	runtime deletionRuntime,
	store *workbench.Store,
	sessionID string,
	policy mutation.ReplayPolicy,
	backoff retry.Backoff,
) (ConfirmedDeletion, error) {
	if store == nil {
		return ConfirmedDeletion{}, workbench.ErrUnavailable
	}
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return ConfirmedDeletion{}, err
	}
	pending, exists := store.PendingSessionDeletion(sessionID)
	if !exists {
		replayGuard, err := policy.NewGuard()
		if err != nil {
			return ConfirmedDeletion{}, err
		}
		request := conversation.DeleteSession{CommandID: mutation.NewCommandID(), SessionID: sessionID}
		pending, err = store.StageSessionDeletion(request, replayGuard)
		if err != nil {
			return ConfirmedDeletion{}, fmt.Errorf("stage session deletion: %w", err)
		}
	}
	settled := settlePendingDeletion(ctx, runtime, store, pending, policy, backoff)
	switch settled.outcome {
	case mutation.Confirmed:
		return ConfirmedDeletion{LocalCleanupError: settled.cleanup}, nil
	case mutation.Unknown:
		return ConfirmedDeletion{}, fmt.Errorf("session deletion outcome is unknown (original request saved for recovery): %w", settled.failure)
	default:
		return ConfirmedDeletion{}, errors.Join(settled.failure, settled.cleanup)
	}
}

func settlePendingDeletion(
	ctx context.Context,
	runtime deletionRuntime,
	store *workbench.Store,
	pending workbench.PendingSessionDeletion,
	policy mutation.ReplayPolicy,
	backoff retry.Backoff,
) (settled deletionSettlement) {
	if policy.Replayable(pending.Replay) {
		settled.outcome, settled.failure = settleDeletion(ctx, runtime, store, pending.Request(), pending.Replay, policy, backoff)
	} else {
		settled.outcome, settled.failure = resolveExpired(ctx, runtime, pending.SessionID, pending.Replay, policy)
	}
	switch settled.outcome {
	case mutation.Confirmed:
		settled.cleanup = store.ConfirmSessionDeletion(pending.SessionID, pending.CommandID)
	case mutation.Rejected:
		settled.cleanup = store.RejectSessionDeletion(pending.SessionID, pending.CommandID)
	}
	return settled
}

func settleDeletion(
	ctx context.Context,
	runtime deletionRuntime,
	store *workbench.Store,
	request conversation.DeleteSession,
	replayGuard authoring.ReplayGuard,
	policy mutation.ReplayPolicy,
	backoff retry.Backoff,
) (mutation.Outcome, error) {
	_, err := mutation.ConfirmAdmitted(ctx, backoff, mutation.ReplayAdmission(policy, replayGuard), func(ctx context.Context) (struct{}, error) {
		if err := store.DispatchReady(); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, runtime.DeleteSession(ctx, request)
	})
	if err == nil || errors.Is(err, conversation.ErrSessionNotFound) {
		return mutation.Confirmed, nil
	}
	if errors.Is(err, mutation.ErrReplayGuaranteeUnavailable) {
		outcome, resolveErr := resolveExpired(ctx, runtime, request.SessionID, replayGuard, policy)
		if outcome != mutation.Unknown {
			return outcome, resolveErr
		}
		return mutation.Unknown, errors.Join(err, resolveErr)
	}
	if mutation.OutcomeUnknown(err) {
		return mutation.Unknown, err
	}
	return mutation.Rejected, err
}

func RecoverDeletions(
	ctx context.Context,
	runtime deletionRuntime,
	store *workbench.Store,
	policy mutation.ReplayPolicy,
	backoff retry.Backoff,
) error {
	for _, pending := range store.PendingSessionDeletions() {
		settled := settlePendingDeletion(ctx, runtime, store, pending, policy, backoff)
		if settled.outcome == mutation.Unknown || settled.cleanup != nil {
			return fmt.Errorf("recover session deletion %s: %w", pending.SessionID, errors.Join(settled.failure, settled.cleanup))
		}
	}
	return nil
}

func resolveExpired(
	ctx context.Context,
	runtime deletionRuntime,
	sessionID string,
	replayGuard authoring.ReplayGuard,
	policy mutation.ReplayPolicy,
) (mutation.Outcome, error) {
	if !policy.SameStore(replayGuard) {
		return mutation.Unknown, errors.New("session deletion belongs to another runtime")
	}
	_, err := runtime.GetSession(ctx, sessionID)
	if errors.Is(err, conversation.ErrSessionNotFound) {
		return mutation.Confirmed, nil
	}
	if err != nil {
		return mutation.Unknown, fmt.Errorf("read deletion outcome: %w", err)
	}
	return mutation.Rejected, errors.New("the original deletion can no longer be replayed and the session still exists")
}
