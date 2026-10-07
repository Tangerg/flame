package run

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/application/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

type steerRuntime interface {
	SteerRun(context.Context, prompt.SteerRun) (protocol.SteerRunResponse, error)
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
	request prompt.SteerRun,
	sourceDraft prompt.Message,
	policy mutation.ReplayPolicy,
	input *workbench.PreparedInput,
) (workbench.PendingSteer, error) {
	if authoring == nil {
		return workbench.PendingSteer{}, workbench.ErrUnavailable
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
	if mutation.OutcomeUnknown(err) || errors.Is(err, conversation.ErrSteerReceiptUnavailable) {
		result.Outcome = mutation.Unknown
		return result, fmt.Errorf("steer command outcome is unknown: %w", err)
	}
	result.Outcome = mutation.Rejected
	return result, err
}

// SteerRefusal is a recovered steer that Runtime definitively refused. Its
// attachments are already back in the Session's durable draft; the cause is
// kept because nothing else tells the user the instruction never arrived.
type SteerRefusal struct {
	SessionID string
	Cause     error
}

// SteerRecovery reports every steer recovery settled.
type SteerRecovery struct {
	Accepted []SteerResult
	Refused  []SteerRefusal
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
) (SteerRecovery, error) {
	if authoring == nil {
		return SteerRecovery{}, workbench.ErrUnavailable
	}
	var recovered SteerRecovery
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
			recovered.Accepted = append(recovered.Accepted, result)
			if acknowledgeErr := authoring.AcknowledgePendingSteer(
				pending.SessionID(), pending.CommandID(),
			); acknowledgeErr != nil {
				return recovered, errors.Join(err, acknowledgeErr)
			}
		case mutation.Rejected:
			draft, _ := authoring.Draft(pending.SessionID())
			if _, rejectErr := authoring.RejectPendingSteer(
				pending.SessionID(), pending.CommandID(), draft,
			); rejectErr != nil {
				return recovered, errors.Join(err, rejectErr)
			}
			recovered.Refused = append(recovered.Refused, SteerRefusal{SessionID: pending.SessionID(), Cause: err})
		case mutation.Unknown:
			return recovered, err
		default:
			return recovered, errors.New("steer settlement returned an invalid outcome")
		}
	}
	if len(deferredSessions) == 0 {
		return recovered, nil
	}
	return recovered, errors.Join(fmt.Errorf(
		"%w for sessions %s: input or runtime replay guarantee is unavailable",
		ErrSteerReplayUnavailable,
		strings.Join(deferredSessions, ", "),
	), errors.Join(deferredFailures...))
}
