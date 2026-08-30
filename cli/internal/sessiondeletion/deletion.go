// Package sessiondeletion owns crash-safe settlement of runtime session
// deletion and the corresponding CLI-local authoring state.
package sessiondeletion

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/agent"
	"github.com/Tangerg/flame/cli/internal/commandreplay"
	"github.com/Tangerg/flame/cli/internal/mutation"
	"github.com/Tangerg/flame/cli/internal/retry"
	"github.com/Tangerg/flame/cli/internal/sessionidentity"
	"github.com/Tangerg/flame/cli/internal/workbench"
)

type runtime interface {
	DeleteSession(context.Context, agent.DeleteSession) error
	GetSession(context.Context, string) (agent.SessionSnapshot, error)
}

// Result binds settlement to the exact durable runtime command.
type Result struct {
	Request agent.DeleteSession
	Outcome mutation.Outcome
}

// Execute stages or resumes one deletion intent, then converges its runtime
// outcome without modifying local authoring state. Callers apply Confirm or
// Reject only after receiving the result on their own presentation boundary.
func Execute(
	ctx context.Context,
	runtime runtime,
	authoring *workbench.Store,
	sessionID string,
	policy commandreplay.Policy,
	backoff retry.Backoff,
) (Result, error) {
	if authoring == nil {
		return Result{}, errors.New("CLI workbench is unavailable")
	}
	identity, err := sessionidentity.Parse(sessionID)
	if err != nil {
		return Result{}, err
	}
	sessionID = identity.String()
	pending, exists := authoring.PendingSessionDeletion(sessionID)
	fresh := !exists
	if exists && pending.Phase == workbench.SessionDeletionConfirmed {
		return Result{Request: pending.Request(), Outcome: mutation.Confirmed}, nil
	}
	request := pending.Request()
	if !exists {
		commandID, err := agent.NewCommandID()
		if err != nil {
			return Result{}, fmt.Errorf("create session deletion identity: %w", err)
		}
		request = agent.DeleteSession{CommandID: commandID, SessionID: sessionID}
		replay, err := policy.NewGuard()
		if err != nil {
			return Result{}, err
		}
		if err := authoring.StageSessionDeletion(request, replay); err != nil {
			return Result{}, fmt.Errorf("stage session deletion: %w", err)
		}
		pending, exists = authoring.PendingSessionDeletion(sessionID)
		if !exists {
			return Result{}, errors.New("staged session deletion is absent")
		}
	}
	if fresh {
		outcome, err := settle(ctx, runtime, request, pending.Replay, policy, backoff, true)
		return Result{Request: request, Outcome: outcome}, err
	}
	if !policy.Replayable(pending.Replay) {
		outcome, err := resolveExpired(ctx, runtime, pending.SessionID, pending.Replay, policy)
		return Result{Request: request, Outcome: outcome}, err
	}
	outcome, err := Settle(ctx, runtime, request, pending.Replay, policy, backoff)
	return Result{Request: request, Outcome: outcome}, err
}

// Settle observes the authoritative runtime outcome. A successful delete may
// still return a post-commit cleanup error, so an authoritative not-found read
// is the confirmation when no acknowledgement was delivered.
func Settle(
	ctx context.Context,
	runtime runtime,
	request agent.DeleteSession,
	replay commandreplay.Guard,
	policy commandreplay.Policy,
	backoff retry.Backoff,
) (mutation.Outcome, error) {
	return settle(ctx, runtime, request, replay, policy, backoff, false)
}

func settle(
	ctx context.Context,
	runtime runtime,
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

// Confirm upgrades a prepared command to a durable tombstone and retires all
// local state. It is idempotent for an already-confirmed cleanup record.
func Confirm(authoring *workbench.Store, result Result) error {
	pending, exists := authoring.PendingSessionDeletion(result.Request.SessionID)
	if exists && pending.Phase == workbench.SessionDeletionPrepared {
		return authoring.ConfirmSessionDeletion(result.Request.SessionID, result.Request.CommandID)
	}
	return authoring.RetireSessionState(result.Request.SessionID)
}

// Reject removes only the exact prepared intent after a definitive refusal.
func Reject(authoring *workbench.Store, result Result) error {
	return authoring.RejectSessionDeletion(result.Request.SessionID, result.Request.CommandID)
}

// Recover settles every journal before any session draft is made visible.
func Recover(
	ctx context.Context,
	runtime runtime,
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
		result := Result{Request: pending.Request()}
		if !policy.Replayable(pending.Replay) {
			outcome, err := resolveExpired(ctx, runtime, pending.SessionID, pending.Replay, policy)
			result.Outcome = outcome
			switch outcome {
			case mutation.Confirmed:
				if confirmErr := Confirm(authoring, result); confirmErr != nil {
					return confirmErr
				}
			case mutation.Rejected:
				if rejectErr := Reject(authoring, result); rejectErr != nil {
					return rejectErr
				}
			case mutation.Unknown:
				return fmt.Errorf("recover session deletion %s: %w", pending.SessionID, err)
			}
			continue
		}
		outcome, err := Settle(ctx, runtime, result.Request, pending.Replay, policy, backoff)
		result.Outcome = outcome
		switch outcome {
		case mutation.Confirmed:
			if confirmErr := Confirm(authoring, result); confirmErr != nil {
				return confirmErr
			}
		case mutation.Rejected:
			if rejectErr := Reject(authoring, result); rejectErr != nil {
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
	runtime runtime,
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
