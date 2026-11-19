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
)

type cancellationSettlement struct {
	ExecutorID string         `json:"executorId"`
	Outcome    jsontext.Value `json:"outcome"`
}

// cancellationReceipt stores a keyed CancelRun outcome through its deferred
// replay record. A failed reservation is the replay's own outcome, never a
// cancellation refusal to persist.
type cancellationReceipt struct {
	key        string
	method     *Method
	completion *operation.DeferredCompletion
	reserveErr error
}

func (r *cancellationReceipt) Key() string { return r.key }

func (r *cancellationReceipt) ReserveForRun(ctx context.Context, runID string) error {
	r.reserveErr = r.completion.ReserveForRun(ctx, runID)
	return r.reserveErr
}

func (r *cancellationReceipt) Refuse(ctx context.Context, reason error) error {
	payload, err := encodeStoredOutcome(failed(projectError(wireCancellationError(reason))))
	if err != nil {
		return err
	}
	return r.completion.Complete(ctx, payload)
}

func (r *cancellationReceipt) Stage(ctx context.Context, ref runs.ExecutorRef, commit func(context.Context) (runs.CancelResult, error)) error {
	err := r.completion.Stage(ctx, func(tx context.Context) ([]byte, error) {
		result, err := commit(tx)
		if err != nil {
			return nil, err
		}
		outcome, err := encodeAtomicResult(r.method, presentCancelResult(result))
		if err != nil {
			return nil, err
		}
		return json.Marshal(cancellationSettlement{ExecutorID: ref.ExecutorID, Outcome: outcome})
	})
	if err != nil {
		return &runs.CancellationSettlementError{Cause: err}
	}
	return nil
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
		receipt := &cancellationReceipt{key: key, method: method, completion: complete}
		result, err := s.cancelRun(ctx, in, receipt)
		_, settlementFailed := errors.AsType[*runs.CancellationSettlementError](err)
		switch {
		case receipt.reserveErr != nil:
			return nil, receipt.reserveErr
		case err != nil && (settlementFailed || errors.Is(err, protocol.ErrCapabilityNotNeg)):
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
	return payload, observedFailure, wireCancellationError(err)
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
