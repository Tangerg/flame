package operation

import (
	"context"
	"errors"
	"fmt"
)

// DeferredCompletion binds a committed write and its unfinished settlement to
// one request. Only Reserve authorizes effects outside that write transaction.
type DeferredCompletion struct {
	store    ReplayStore
	record   ReplayRecord
	reserved bool
}

func (c *DeferredCompletion) Reserve(ctx context.Context) error {
	if c.reserved {
		return errors.New("idempotency: settlement already reserved")
	}
	if err := c.claim(ctx); err != nil {
		return err
	}
	c.reserved = true
	return nil
}

func (c *DeferredCompletion) Stage(ctx context.Context, write func(context.Context) ([]byte, error)) error {
	if len(c.record.Settlement) != 0 {
		return errors.New("idempotency: settlement already staged")
	}
	staged := c.record
	err := c.store.Transact(ctx, func(tx context.Context) error {
		if !c.reserved {
			if err := c.claim(tx); err != nil {
				return err
			}
		}
		var err error
		staged.Settlement, err = write(tx)
		if err != nil {
			return err
		}
		return c.store.StageSettlement(tx, staged)
	})
	if err != nil {
		return err
	}
	c.record = staged
	c.reserved = true
	return nil
}

func (c *DeferredCompletion) claim(ctx context.Context) error {
	record, claimed, err := c.store.Claim(ctx, c.record.Key, c.record.Fingerprint)
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
	settle func([]byte) ([]byte, error),
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
		previous.Payload, err = settle(previous.Settlement)
		if err != nil {
			return nil, err
		}
		return r.finishRecord(ctx, previous)
	}

	completion := &DeferredCompletion{
		store: r.store, record: ReplayRecord{Key: key, Fingerprint: fingerprint},
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
	if !completion.reserved {
		outcome, _, err := r.commitOutcome(ctx, key, fingerprint, func(context.Context) ([]byte, error) { return payload, nil })
		return outcome, err
	}
	completion.record.Payload = payload
	return r.finishRecord(ctx, completion.record)
}
