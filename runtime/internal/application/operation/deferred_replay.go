package operation

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
)

// DeferredCompletion binds a committed write and its unfinished settlement to
// one request. Only ReserveForRun authorizes effects outside that write transaction.
type DeferredCompletion struct {
	replay   *Replay
	record   ReplayRecord
	reserved atomic.Bool
}

func (c *DeferredCompletion) ReserveForRun(ctx context.Context, runID string) error {
	if runID == "" {
		return errors.New("idempotency: deferred live work requires its original Run")
	}
	if err := c.claim(ctx, runID); err != nil {
		return err
	}
	c.reserved.Store(true)
	return nil
}

func (c *DeferredCompletion) Stage(ctx context.Context, write func(context.Context) ([]byte, error)) error {
	return c.replay.store.Transact(ctx, func(tx context.Context) error {
		if !c.reserved.Load() {
			if err := c.claim(tx, ""); err != nil {
				return err
			}
		}
		staged := c.record
		var err error
		staged.Settlement, err = write(tx)
		if err != nil {
			return err
		}
		return c.replay.store.StageSettlement(tx, staged)
	})
}

// Complete records a reserved outcome that requires no further settlement.
// This owner can outlive the invocation that reserved the key.
func (c *DeferredCompletion) Complete(ctx context.Context, payload []byte) error {
	if !c.reserved.Load() {
		return ErrClaimLost
	}
	if len(payload) == 0 {
		return errors.New("idempotency: reserved completion requires an outcome")
	}
	record := c.record
	record.Payload = payload
	_, err := c.replay.finishRecord(ctx, record)
	return err
}

func (c *DeferredCompletion) claim(ctx context.Context, runID string) error {
	record, claimed, err := c.replay.store.Claim(ctx, ReplayClaim{
		Key: c.record.Key, Fingerprint: c.record.Fingerprint, RunID: runID,
	})
	if err != nil {
		return fmt.Errorf("idempotency: reserve settlement: %w", err)
	}
	if claimed {
		return nil
	}
	if len(record.Payload) != 0 {
		return errAtomicResultExists
	}
	return ErrInProgress
}

func (r *Replay) InvokeDeferred(
	ctx context.Context,
	key, fingerprint string,
	execute func(*DeferredCompletion) ([]byte, error),
	settle func(data []byte, recoverPending bool) ([]byte, error),
) ([]byte, error) {
	release, err := r.inFlight.Acquire(ctx, key)
	if err != nil {
		return nil, err
	}
	defer release()
	if payload, pending, err := r.finishPending(ctx, key, fingerprint); pending {
		return payload, err
	}
	previous, found, err := r.store.Lookup(ctx, key, fingerprint)
	if err != nil {
		return nil, err
	}
	if found {
		if len(previous.Settlement) == 0 {
			return settledPayload(previous)
		}
		previous.Payload, err = settle(previous.Settlement, true)
		if err != nil {
			return nil, err
		}
		return r.finishRecord(ctx, previous)
	}

	completion := &DeferredCompletion{
		replay: r, record: ReplayRecord{Key: key, Fingerprint: fingerprint},
	}
	payload, err := execute(completion)
	if errors.Is(err, errAtomicResultExists) {
		previous, found, err := r.store.Lookup(ctx, key, fingerprint)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, ErrClaimLost
		}
		return settledPayload(previous)
	}
	if err != nil {
		return nil, err
	}
	// The business transaction can run on the Run owner and can roll back
	// after staging. Only the committed record may advance acknowledgement.
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), replayWriteTimeout)
	defer cancel()
	committed, found, err := r.store.Lookup(readCtx, key, fingerprint)
	if err != nil {
		return nil, settlementError(err)
	}
	if !found {
		if completion.reserved.Load() {
			return nil, ErrClaimLost
		}
		outcome, _, err := r.commitOutcome(ctx, key, fingerprint, func(context.Context) ([]byte, error) { return payload, nil })
		return outcome, err
	}
	if len(committed.Payload) != 0 {
		return settledPayload(committed)
	}
	if len(committed.Settlement) == 0 {
		return nil, ErrInProgress
	}
	payload, err = settle(committed.Settlement, false)
	if err != nil {
		return nil, err
	}
	committed.Payload = payload
	return r.finishRecord(ctx, committed)
}
