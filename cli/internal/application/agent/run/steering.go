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
	"github.com/Tangerg/flame/runtime/protocol"
)

type steerRuntime interface {
	SteerRun(context.Context, agent.SteerRun) (protocol.SteerRunResponse, error)
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
	policy mutation.ReplayPolicy,
	input *workbench.PreparedInput,
) (workbench.PendingSteer, error) {
	if authoring == nil {
		return workbench.PendingSteer{}, workbench.ErrUnavailable
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
	if err := authoring.StagePendingSteer(pending, sourceDraft, input); err != nil {
		return workbench.PendingSteer{}, fmt.Errorf("stage steer command: %w", err)
	}
	pending, _ = authoring.PendingSteer(sessionID)
	return pending, nil
}

// SteerResult binds settlement to the exact durable command.
type SteerResult struct {
	Pending workbench.PendingSteer
	Outcome mutation.Outcome
	Receipt protocol.SteerRunResponse
}

// DeliverSteer settles a freshly staged command. An unadvertised Runtime permits
// exactly one I/O attempt; only an advertised guard permits acknowledgement
// retries.
func DeliverSteer(
	ctx context.Context,
	runtime steerRuntime,
	pending workbench.PendingSteer,
	policy mutation.ReplayPolicy,
	backoff retry.Backoff,
) (SteerResult, error) {
	result := SteerResult{Pending: pending, Outcome: mutation.Unknown}
	if runtime == nil {
		return result, errors.New("steer runtime is unavailable")
	}
	if err := pending.Validate(); err != nil {
		return result, err
	}
	command, err := pending.ReplayCommand()
	if err != nil {
		return result, err
	}
	receipt, err := mutation.ConfirmAdmitted(ctx, backoff,
		mutation.FreshReplayAdmission(policy, pending.Replay()), func(ctx context.Context) (protocol.SteerRunResponse, error) {
			return runtime.SteerRun(ctx, command)
		})
	if err == nil {
		result.Outcome = mutation.Confirmed
		result.Receipt = receipt
		return result, nil
	}
	if mutation.OutcomeUnknown(err) || errors.Is(err, agent.ErrSteerReceiptUnavailable) {
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
// Accepted receipts retain their session and command identities for presentation
// against durable User Items after recovery opens the relevant Session.
func RecoverSteers(
	ctx context.Context,
	runtime steerRuntime,
	authoring *workbench.Store,
	policy mutation.ReplayPolicy,
	backoff retry.Backoff,
) ([]SteerResult, error) {
	if authoring == nil {
		return nil, workbench.ErrUnavailable
	}
	var accepted []SteerResult
	var deferredSessions []string
	var deferredFailures []error
	for _, pending := range authoring.PendingSteers() {
		if _, err := pending.ReplayCommand(); err != nil {
			deferredSessions = append(deferredSessions, pending.SessionID())
			deferredFailures = append(deferredFailures, err)
			continue
		}
		if !policy.Replayable(pending.Replay()) {
			deferredSessions = append(deferredSessions, pending.SessionID())
			continue
		}
		result, err := DeliverSteer(ctx, runtime, pending, policy, backoff)
		switch result.Outcome {
		case mutation.Confirmed:
			accepted = append(accepted, result)
			if acknowledgeErr := authoring.AcknowledgePendingSteer(
				pending.SessionID(), pending.CommandID(),
			); acknowledgeErr != nil {
				return accepted, errors.Join(err, acknowledgeErr)
			}
		case mutation.Rejected:
			draft, _ := authoring.Draft(pending.SessionID())
			if _, rejectErr := authoring.RejectPendingSteer(
				pending.SessionID(), pending.CommandID(), draft,
			); rejectErr != nil {
				return accepted, errors.Join(err, rejectErr)
			}
		case mutation.Unknown:
			return accepted, err
		default:
			return accepted, errors.New("steer settlement returned an invalid outcome")
		}
	}
	if len(deferredSessions) == 0 {
		return accepted, nil
	}
	return accepted, errors.Join(fmt.Errorf(
		"%w for sessions %s: input or runtime replay guarantee is unavailable",
		ErrSteerReplayUnavailable,
		strings.Join(deferredSessions, ", "),
	), errors.Join(deferredFailures...))
}
