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

type parkedCancellationSettlement struct {
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
		var parkedOutcome []byte
		var storeErr error
		result, err := s.cancelRun(ctx, in, runs.CancellationCompletion{
			Reserve: func(ctx context.Context) error {
				storeErr = complete.Reserve(ctx)
				return storeErr
			},
			CommitParked: func(ctx context.Context, ref runs.ExecutorRef, write func(context.Context) (run.Run, error)) error {
				storeErr = complete.Stage(ctx, func(tx context.Context) ([]byte, error) {
					terminal, err := write(tx)
					if err != nil {
						return nil, err
					}
					parkedOutcome, err = encodeAtomicResult(method, presentCancelResult(runs.CancelResult{Run: terminal}))
					if err != nil {
						return nil, err
					}
					return json.Marshal(parkedCancellationSettlement{ExecutorID: ref.ExecutorID, Outcome: parkedOutcome})
				})
				return storeErr
			},
		})
		switch {
		case storeErr != nil:
			return nil, storeErr
		case err != nil && len(parkedOutcome) != 0:
			return nil, err
		case err != nil:
			return encodeFailure(err)
		case len(parkedOutcome) != 0:
			return parkedOutcome, nil
		default:
			return encodeAtomicResult(method, presentCancelResult(result))
		}
	}, func(data []byte) ([]byte, error) {
		pending, ref, err := decodeParkedCancellation(method, in.RunID, data)
		if err != nil {
			return nil, err
		}
		if err := s.runCancellations.SettleParkedCancellation(ctx, ref); err != nil {
			return nil, wireCancellationError(err)
		}
		return pending.Outcome, nil
	})
	if err != nil || !bytes.Equal(payload, encodedFailure) {
		observedFailure = nil
	}
	return payload, observedFailure, err
}

func decodeParkedCancellation(method *Method, runID string, data []byte) (parkedCancellationSettlement, runs.ExecutorRef, error) {
	var pending parkedCancellationSettlement
	if err := decodeStoredJSON(data, &pending); err != nil {
		return pending, runs.ExecutorRef{}, fmt.Errorf("idempotency: decode parked cancellation: %w", err)
	}
	reply := replayOutcome(method, pending.Outcome)
	if reply.Failure != nil {
		return pending, runs.ExecutorRef{}, fmt.Errorf("idempotency: invalid parked cancellation outcome: %w", reply.Failure)
	}
	result := reply.Value.(*protocol.CancelRunResponse)
	if pending.ExecutorID == "" || result.Type != protocol.CancelRunRoot || result.Run.ID != runID ||
		result.Run.Outcome == nil || result.Run.Outcome.Type != protocol.OutcomeCanceled {
		return pending, runs.ExecutorRef{}, errors.New("idempotency: invalid parked cancellation settlement")
	}
	return pending, runs.ExecutorRef{SessionID: result.Run.SessionID, ExecutorID: pending.ExecutorID}, nil
}
