package run

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/cli/internal/application/agent/mutation"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

type steerRuntime interface {
	SteerRun(context.Context, agent.SteerRun) error
}

// ErrSteerReplayUnavailable reports a durable steer whose outcome can no
// longer be queried safely from the Runtime replay store. Recovery preserves
// the journal and its attachments for explicit user reconciliation.
var ErrSteerReplayUnavailable = errors.New("steer replay guarantee is unavailable")

// StageSteer atomically transfers the source draft's attachments into a durable
// command journal before delivery can begin.
func StageSteer(
	authoring *workbench.Store,
	sessionID string,
	request agent.SteerRun,
	sourceDraft agent.Message,
	policy commandreplay.Policy,
) (workbench.PendingSteer, error) {
	if authoring == nil {
		return workbench.PendingSteer{}, errors.New("CLI workbench is unavailable")
	}
	if err := request.Validate(); err != nil {
		return workbench.PendingSteer{}, err
	}
	if request.CommandID == "" {
		return workbench.PendingSteer{}, errors.New("steer command id is empty")
	}
	if err := policy.Validate(); err != nil {
		return workbench.PendingSteer{}, err
	}
	stagedAt := policy.Now()
	guard, err := policy.NewGuardAt(stagedAt)
	if err != nil {
		return workbench.PendingSteer{}, err
	}
	pending, err := workbench.NewPendingSteer(sessionID, request, stagedAt, guard)
	if err != nil {
		return workbench.PendingSteer{}, err
	}
	if err := authoring.StagePendingSteer(pending, sourceDraft); err != nil {
		return workbench.PendingSteer{}, fmt.Errorf("stage steer command: %w", err)
	}
	return pending, nil
}

// SteerResult binds settlement to the exact durable command.
type SteerResult struct {
	Pending workbench.PendingSteer
	Outcome mutation.Outcome
}

// DeliverSteer settles a freshly staged command. An unadvertised Runtime permits
// exactly one I/O attempt; only an advertised guard permits acknowledgement
// retries.
func DeliverSteer(
	ctx context.Context,
	runtime steerRuntime,
	pending workbench.PendingSteer,
	policy commandreplay.Policy,
	backoff retry.Backoff,
) (SteerResult, error) {
	result := SteerResult{Pending: pending}
	if runtime == nil {
		return result, errors.New("steer runtime is unavailable")
	}
	if err := pending.Validate(); err != nil {
		return result, err
	}
	_, err := mutation.ConfirmAdmitted(ctx, backoff,
		mutation.FreshReplayAdmission(policy, pending.Replay()), func(ctx context.Context) (struct{}, error) {
			return struct{}{}, runtime.SteerRun(ctx, pending.Command())
		})
	if err == nil {
		result.Outcome = mutation.Confirmed
		return result, nil
	}
	if mutation.OutcomeUnknown(err) {
		result.Outcome = mutation.Unknown
		return result, fmt.Errorf("steer command outcome is unknown: %w", err)
	}
	result.Outcome = mutation.Rejected
	return result, err
}

// RecoverSteers replays every unsettled command only while the same runtime
// idempotency namespace still guarantees its original response. Definitive
// refusals atomically return attachments to the durable session draft. Commands
// outside that guarantee remain journaled while recovery continues for other
// sessions, then return [ErrSteerReplayUnavailable] for user-visible health.
func RecoverSteers(
	ctx context.Context,
	runtime steerRuntime,
	authoring *workbench.Store,
	policy commandreplay.Policy,
	backoff retry.Backoff,
) error {
	if authoring == nil {
		return errors.New("CLI workbench is unavailable")
	}
	var deferredSessions []string
	for _, pending := range authoring.PendingSteers() {
		if !policy.Replayable(pending.Replay()) {
			deferredSessions = append(deferredSessions, pending.SessionID())
			continue
		}
		result, err := DeliverSteer(ctx, runtime, pending, policy, backoff)
		switch result.Outcome {
		case mutation.Confirmed:
			if acknowledgeErr := authoring.AcknowledgePendingSteer(
				pending.SessionID(), pending.CommandID(),
			); acknowledgeErr != nil {
				return errors.Join(err, acknowledgeErr)
			}
		case mutation.Rejected:
			draft, _, draftErr := authoring.Draft(pending.SessionID())
			if draftErr != nil {
				return errors.Join(err, draftErr)
			}
			if _, rejectErr := authoring.RejectPendingSteer(
				pending.SessionID(), pending.CommandID(), draft,
			); rejectErr != nil {
				return errors.Join(err, rejectErr)
			}
		case mutation.Unknown:
			return err
		default:
			return errors.New("steer settlement returned an invalid outcome")
		}
	}
	if len(deferredSessions) == 0 {
		return nil
	}
	return fmt.Errorf(
		"%w for sessions %s: guarantee expired or belongs to another runtime",
		ErrSteerReplayUnavailable,
		strings.Join(deferredSessions, ", "),
	)
}
