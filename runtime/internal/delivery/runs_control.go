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

func (s *Handler) CancelRun(ctx context.Context, in protocol.CancelRunRequest) (*protocol.CancelRunResponse, error) {
	result, err := s.cancelRun(ctx, in, nil)
	if err != nil {
		return nil, err
	}
	return presentCancelResult(result), nil
}

func (s *Handler) cancelRun(ctx context.Context, in protocol.CancelRunRequest, receipt runs.CancellationReceipt) (runs.CancelResult, error) {
	runID, err := runParameter(in.RunID)
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

func (s *Handler) SteerRun(ctx context.Context, in protocol.SteerRunRequest) (*protocol.SteerRunResponse, error) {
	input, err := decodeRunInput(in.Input)
	if err != nil {
		return nil, err
	}
	expected, err := wireIdentity("expectedSegmentId", in.ExpectedSegmentID, identity.ParseSegment)
	if err != nil {
		return nil, err
	}
	runID, err := runParameter(in.RunID)
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
		return invalidWireContentBlock(input.Index, wireContentField(input.Field), input.Detail)
	}
	if errors.Is(err, runs.ErrInputRequired) || errors.Is(err, runs.ErrUnsupportedMedia) {
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	}
	return wireLiveSegmentError(err)
}

const (
	sessionIDField = "sessionId"
	runIDField     = "runId"
	segmentIDField = "segmentId"
)

func sessionParameter(text string) (identity.SessionID, error) {
	return wireIdentity(sessionIDField, text, identity.ParseSession)
}

func optionalSessionParameter(text string) (identity.SessionID, error) {
	return wireIdentity(sessionIDField, text, identity.ParseOptionalSession)
}

func runParameter(text string) (identity.RunID, error) {
	return wireIdentity(runIDField, text, identity.ParseRun)
}

func optionalRunParameter(text string) (identity.RunID, error) {
	return wireIdentity(runIDField, text, identity.ParseOptionalRun)
}

func wireIdentity[ID any](field, text string, parse func(string) (ID, error)) (ID, error) {
	id, err := parse(text)
	if err != nil {
		var zero ID
		return zero, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("%s: %v", field, err))
	}
	return id, nil
}
