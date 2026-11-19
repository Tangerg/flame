package delivery

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type cancellationSettlement struct {
	ExecutorID string         `json:"executorId"`
	Outcome    jsontext.Value `json:"outcome"`
}

func (e *Endpoint) replayCancellation(ctx context.Context, s *Handler, method *Method, in protocol.CancelRunRequest, key, fingerprint string) ([]byte, *protocol.Failure, error) {
	var observedFailure *protocol.Failure
	var encodedFailure []byte
	encodeFailure := func(err error) ([]byte, error) {
		observedFailure = projectError(err)
		data, err := encodeStoredOutcome(failed(observedFailure))
		encodedFailure = data
		return data, err
	}
	payload, err := e.idempotency.InvokeDeferred(ctx, key, fingerprint, func(complete *operation.DeferredCompletion) ([]byte, error) {
		var storeErr error
		var stagedSynchronously bool
		encodeSettlement := func(ref runs.ExecutorRef, result runs.CancelResult) ([]byte, error) {
			outcome, err := encodeAtomicResult(method, presentCancelResult(result))
			if err != nil {
				return nil, err
			}
			return json.Marshal(cancellationSettlement{ExecutorID: ref.ExecutorID, Outcome: outcome})
		}
		stage := func(tx context.Context, outcome func(context.Context) ([]byte, error)) error {
			storeErr = complete.Stage(tx, outcome)
			stagedSynchronously = storeErr == nil
			return storeErr
		}
		result, err := s.cancelRun(ctx, in, runs.CancellationCompletion{
			Reserve: func(ctx context.Context) error {
				storeErr = complete.Reserve(ctx)
				return storeErr
			},
			CommitParked: func(ctx context.Context, ref runs.ExecutorRef, write func(context.Context) (run.Run, error)) error {
				return stage(ctx, func(tx context.Context) ([]byte, error) {
					terminal, err := write(tx)
					if err != nil {
						return nil, err
					}
					return encodeSettlement(ref, runs.CancelResult{Run: terminal})
				})
			},
			RecordWaitingChild: func(tx context.Context, ref runs.ExecutorRef, result runs.WaitingSubtreeCancellationResult) error {
				return stage(tx, func(context.Context) ([]byte, error) {
					return encodeSettlement(ref, runs.CancelResult{Run: result.TargetRun, RootRun: &result.RootRun})
				})
			},
			RecordActiveRoot: func(tx context.Context, ref runs.ExecutorRef, terminal run.Run) error {
				return complete.Stage(tx, func(context.Context) ([]byte, error) {
					return encodeSettlement(ref, runs.CancelResult{Run: terminal})
				})
			},
		})
		_, settlementFailed := errors.AsType[*runs.CancellationSettlementError](err)
		switch {
		case storeErr != nil:
			return nil, storeErr
		case err != nil && (stagedSynchronously || settlementFailed || errors.Is(err, protocol.ErrCapabilityNotNeg)):
			return nil, err
		case err != nil:
			return encodeFailure(err)
		default:
			return encodeAtomicResult(method, presentCancelResult(result))
		}
	}, func(data []byte, recoverPending bool) ([]byte, error) {
		pending, response, err := decodeCancellationSettlement(method, in.RunID, data)
		if err != nil {
			return nil, err
		}
		if response.RootRun != nil && !s.requestCanUseFeature(ctx, protocol.FeatureChildRuns) {
			return nil, wireCancellationError(runs.ErrChildRunNotAllowed)
		}
		if !recoverPending {
			return pending.Outcome, nil
		}
		ref := runs.ExecutorRef{SessionID: response.Run.SessionID, ExecutorID: pending.ExecutorID}
		if response.RootRun == nil {
			err = s.runCancellations.SettleRootCancellation(ctx, ref, response.Run.ID)
		} else {
			err = s.runCancellations.SettleAcceptedSubtreeCancellation(ctx, ref, response.RootRun.ID, response.Run.ID)
		}
		if err != nil {
			return nil, wireCancellationError(err)
		}
		return pending.Outcome, nil
	})
	if err != nil || !bytes.Equal(payload, encodedFailure) {
		observedFailure = nil
	}
	return payload, observedFailure, err
}

func decodeCancellationSettlement(method *Method, runID string, data []byte) (cancellationSettlement, *protocol.CancelRunResponse, error) {
	var pending cancellationSettlement
	if err := decodeStoredJSON(data, &pending); err != nil {
		return pending, nil, fmt.Errorf("idempotency: decode cancellation settlement: %w", err)
	}
	reply := replayOutcome(method, pending.Outcome)
	if reply.Failure != nil {
		return pending, nil, fmt.Errorf("idempotency: invalid cancellation outcome: %w", reply.Failure)
	}
	result := reply.Value.(*protocol.CancelRunResponse)
	if pending.ExecutorID == "" || result.Run.ID != runID ||
		result.Run.Outcome == nil || result.Run.Outcome.Type != protocol.OutcomeCanceled {
		return pending, nil, errors.New("idempotency: invalid cancellation settlement")
	}
	return pending, result, nil
}
