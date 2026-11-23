package run

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/retry"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	flameauthoring "github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type steerRuntime interface {
	SteerRun(context.Context, flameauthoring.SteerRun) (protocol.SteerRunResponse, error)
}

var ErrSteerReplayUnavailable = errors.New("steer replay guarantee is unavailable")

func StageSteer(
	authoring *workbench.Store,
	sessionID string,
	request flameauthoring.SteerRun,
	sourceDraft flameauthoring.Message,
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
	staged, err := authoring.StagePendingSteer(pending, sourceDraft, input)
	if err != nil {
		return workbench.PendingSteer{}, fmt.Errorf("stage steer command: %w", err)
	}
	return staged, nil
}

type SteerResult struct {
	Pending workbench.PendingSteer
	Outcome mutation.Outcome
	Receipt protocol.SteerRunResponse
}

func DeliverSteer(
	ctx context.Context,
	runtime steerRuntime,
	store *workbench.Store,
	pending workbench.PendingSteer,
	policy mutation.ReplayPolicy,
	backoff retry.Backoff,
) (SteerResult, error) {
	result := SteerResult{Pending: pending, Outcome: mutation.Unknown}
	if store == nil {
		return result, workbench.ErrUnavailable
	}
	if runtime == nil {
		return result, errors.New("steer runtime is unavailable")
	}
	command, err := pending.ReplayCommand()
	if err != nil {
		return result, err
	}
	receipt, err := mutation.ConfirmAdmitted(ctx, backoff,
		mutation.ReplayAdmission(policy, pending.Replay()), func(ctx context.Context) (protocol.SteerRunResponse, error) {
			if err := store.DispatchReady(); err != nil {
				return protocol.SteerRunResponse{}, err
			}
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

type SteerRefusal struct {
	SessionID string
	Cause     error
}

type SteerRecovery struct {
	Accepted []SteerResult
	Refused  []SteerRefusal
}

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
		result, err := DeliverSteer(ctx, runtime, authoring, pending, policy, backoff)
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
