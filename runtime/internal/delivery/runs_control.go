package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

// CancelRun hard-stops a running run with a canceled outcome.
// A parked Run is also abandoned — its live parked execution is torn down
// and its open interrupt dropped so it stops surfacing as resumable.
func (s *Handler) CancelRun(ctx context.Context, in protocol.CancelRunRequest) (*protocol.CancelRunResponse, error) {
	result, err := s.cancelRun(ctx, in, nil)
	if err != nil {
		return nil, err
	}
	return presentCancelResult(result), nil
}

func (s *Handler) cancelRun(ctx context.Context, in protocol.CancelRunRequest, receipt runs.CancellationReceipt) (runs.CancelResult, error) {
	// Root cancel is the emergency stop and is always allowed. Whether the target
	// is a child is durable state resolved by the application, so do not reject
	// unsupported client preferences before that identity is known.
	runID, err := wireIdentity("runId", in.RunID, identity.ParseRun)
	if err != nil {
		return runs.CancelResult{}, err
	}
	result, err := s.runCancellations.Cancel(ctx, runs.CancelCommand{
		RunID:         runID,
		Reason:        in.Reason,
		AllowChildRun: s.requestCanUseFeature(ctx, protocol.FeatureChildRuns),
		Receipt:       receipt,
	})
	return result, wireCancellationError(err)
}

func wireCancellationError(err error) error {
	_, settlementFailed := errors.AsType[*runs.CancellationSettlementError](err)
	switch {
	case settlementFailed:
		return protocol.NewFailure(protocol.ErrInternalError, "cancellation settlement could not be confirmed", err)
	case errors.Is(err, runs.ErrRunNotFound):
		return protocol.ErrRunNotFound
	case errors.Is(err, runs.ErrRunFinished):
		return protocol.ErrRunFinished
	case errors.Is(err, rundomain.ErrSessionBusy):
		return protocol.ErrSessionBusy
	case errors.Is(err, runs.ErrInvalidCancellationReason):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	case errors.Is(err, runs.ErrChildRunNotAllowed):
		return NewCapabilityGapError(protocol.CapabilityRequirement{
			Type: protocol.RequirementFeature,
			Name: protocol.FeatureChildRuns,
		})
	default:
		return err
	}
}

// SteerRun injects a user message into the segment the request names so the model
// reads it on its next tool round.
//
// Only the addressed segment is steerable, and every other position says so by
// name: a waiting run is answered via runs.resume, a finished one cannot be
// steered at all, and a run that has moved to a different segment refuses rather
// than delivering the instruction to work the user never saw. The refusals are
// the same set a subscribe gets, because both are addressing one live segment.
func (s *Handler) SteerRun(ctx context.Context, in protocol.SteerRunRequest) (*protocol.SteerRunResponse, error) {
	input, err := decodeRunInput(in.Input)
	if err != nil {
		return nil, err
	}
	expected, err := wireIdentity("expectedSegmentId", in.ExpectedSegmentID, identity.ParseOptionalSegment)
	if err != nil {
		return nil, err
	}
	runID, err := wireIdentity("runId", in.RunID, identity.ParseRun)
	if err != nil {
		return nil, err
	}
	itemID, err := s.runs.Steer(ctx, runs.SteerCommand{
		RunID: runID, ExpectedSegmentID: expected, Input: input, Identity: runCommandIdentity(ctx),
	})
	if err != nil {
		return nil, wireSteerError(err)
	}
	return &protocol.SteerRunResponse{UserItemID: itemID.String()}, nil
}

func wireSteerError(err error) error {
	if errors.Is(err, runs.ErrCommandKeyConflict) {
		return protocol.ErrIdempotencyConflict
	}
	if input, ok := errors.AsType[*runs.InputBlockError](err); ok {
		constraint := &protocol.ConstraintError{Shape: "RunInput", Fields: []protocol.FieldError{{
			Field:  fmt.Sprintf("input[%d].%s", input.Index, wireContentField(input.Field)),
			Detail: input.Detail,
		}}}
		return protocol.NewFailure(protocol.ErrInvalidParams, constraint.Error(), constraint)
	}
	if errors.Is(err, runs.ErrInputRequired) || errors.Is(err, runs.ErrUnsupportedMedia) {
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	}
	return wireLiveSegmentError(err)
}

// wireIdentity parses an identity a request addressed; a malformed one is an
// invalid parameter.
func wireIdentity[ID any](field, text string, parse func(string) (ID, error)) (ID, error) {
	id, err := parse(text)
	if err != nil {
		var zero ID
		return zero, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("%s: %v", field, err))
	}
	return id, nil
}
