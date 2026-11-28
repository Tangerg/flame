package operation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/keylock"
	framecall "github.com/Tangerg/frame/call"

	"github.com/samber/lo"
)

var (
	ErrClaimLost       = errors.New("idempotency: claim is no longer available")
	ErrOutcomeConflict = errors.New("idempotency: completed outcome differs from the execution result")
	ErrInProgress      = errors.New("command outcome is not yet known; retain the original request identity")
	ErrOutcomePending  = errors.New("operation outcome persistence is still pending")
)

// An unresolved reservation cannot expire: elapsed time does not prove whether its
// business mutation committed.
const ReplayRetention = 24 * time.Hour

const replayWriteTimeout = 5 * time.Second

type ReplayRecord struct {
	Key         string
	Fingerprint string
	Payload     []byte
	Settlement  []byte
	Refusal     error
}

func (r ReplayRecord) Settled() bool { return len(r.Payload) != 0 || r.Refusal != nil }

type ReplayClaim struct {
	Key         string
	Fingerprint string
	RunID       identity.RunID
}

type ReplayAdmission func(context.Context) error

func (a ReplayAdmission) check(ctx context.Context) error {
	if a != nil {
		return a(ctx)
	}
	return nil
}

type ReplayStore interface {
	Lookup(context.Context, string, string) (ReplayRecord, bool, error)
	Transact(context.Context, func(context.Context) error) error
	// An existing claim never authorizes the business write to run again.
	Claim(context.Context, ReplayClaim) (ReplayRecord, bool, error)
	StageSettlement(context.Context, ReplayRecord) error
	Complete(context.Context, ReplayRecord) (ReplayRecord, error)
}

type Replay struct {
	store    ReplayStore
	inFlight *keylock.Set
}

func NewReplay(store ReplayStore) (*Replay, error) {
	if lo.IsNil(store) {
		return nil, errors.New("idempotency: durable store is required")
	}
	return &Replay{store: store, inFlight: keylock.NewSet()}, nil
}

func (r *Replay) Serialize(ctx context.Context, key string) (func(), error) {
	return r.inFlight.Acquire(ctx, key)
}

func (r *Replay) InvokeRecoverable(ctx context.Context, key, fingerprint string, admit ReplayAdmission, execute func(retained bool) error, refuse func(error) ([]byte, error)) ([]byte, error) {
	release, err := r.inFlight.Acquire(ctx, key)
	if err != nil {
		return nil, err
	}
	defer release()
	record, found, err := r.store.Lookup(ctx, key, fingerprint)
	if err != nil {
		return nil, err
	}
	if found && record.Settled() {
		return settledPayload(record)
	}
	if !found {
		if err := admit.check(ctx); err != nil {
			return nil, err
		}
	}
	workErr := execute(found)
	// The workflow can commit after the caller stops observing it. Only its
	// durable receipt may settle the original invocation.
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), replayWriteTimeout)
	defer cancel()
	record, found, err = r.store.Lookup(readCtx, key, fingerprint)
	if err != nil {
		return nil, settlementError(err)
	}
	if found && record.Settled() {
		return settledPayload(record)
	}
	if errors.Is(workErr, ErrOutcomePending) || errors.Is(workErr, ErrInProgress) || errors.Is(workErr, framecall.ErrConflict) {
		return nil, workErr
	}
	if found {
		return nil, errors.Join(ErrOutcomePending, workErr)
	}
	if workErr == nil {
		return nil, ErrClaimLost
	}
	payload, _, err := r.commitOutcome(ctx, key, fingerprint, func(context.Context) ([]byte, error) {
		return refuse(workErr)
	})
	return payload, err
}

func (r *Replay) finishRecord(ctx context.Context, record ReplayRecord) ([]byte, error) {
	settled, err := r.persistOutcome(ctx, record)
	if err != nil {
		return nil, err
	}
	return settledPayload(settled)
}

func (r *Replay) persistOutcome(ctx context.Context, record ReplayRecord) (ReplayRecord, error) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), replayWriteTimeout)
	defer cancel()
	settled, err := r.store.Complete(writeCtx, record)
	if err != nil {
		return ReplayRecord{}, settlementError(err)
	}
	return settled, nil
}

type Reply[T any] func(ctx context.Context, write func(context.Context) (T, error)) error

func DirectReply[T any](reply *T) Reply[T] {
	return func(ctx context.Context, write func(context.Context) (T, error)) error {
		value, err := write(ctx)
		if err != nil {
			return err
		}
		*reply = value
		return nil
	}
}

type AtomicCompletion struct {
	commit    func(context.Context, func(context.Context) ([]byte, error)) error
	attempted bool
}

func (c *AtomicCompletion) Commit(ctx context.Context, write func(context.Context) ([]byte, error)) error {
	if c.attempted {
		return errors.New("idempotency: atomic completion was already attempted")
	}
	c.attempted = true
	return c.commit(ctx, write)
}

func (c *AtomicCompletion) Attempted() bool { return c.attempted }

var errAtomicResultExists = errors.New("idempotency: another writer committed the outcome")

func (r *Replay) InvokeAtomic(ctx context.Context, key, fingerprint string, admit ReplayAdmission, execute func(*AtomicCompletion) error) ([]byte, error) {
	release, err := r.inFlight.Acquire(ctx, key)
	if err != nil {
		return nil, err
	}
	defer release()
	previous, found, err := r.store.Lookup(ctx, key, fingerprint)
	if err != nil {
		return nil, err
	}
	if found {
		return settledPayload(previous)
	}
	if err := admit.check(ctx); err != nil {
		return nil, err
	}
	var payload []byte
	completion := &AtomicCompletion{commit: func(ctx context.Context, write func(context.Context) ([]byte, error)) error {
		outcome, replayed, err := r.commitOutcome(ctx, key, fingerprint, write)
		if err != nil {
			return err
		}
		payload = outcome
		if replayed {
			return errAtomicResultExists
		}
		return nil
	}}
	err = execute(completion)
	if err != nil && !errors.Is(err, errAtomicResultExists) {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, errors.New("idempotency: atomic command omitted completion")
	}
	return payload, nil
}

func (r *Replay) commitOutcome(ctx context.Context, key, fingerprint string, write func(context.Context) ([]byte, error)) (payload []byte, replayed bool, err error) {
	err = r.store.Transact(ctx, func(tx context.Context) error {
		record, claimed, err := r.store.Claim(tx, ReplayClaim{Key: key, Fingerprint: fingerprint})
		if err != nil {
			return err
		}
		if !claimed {
			replayed = true
			payload, err = settledPayload(record)
			return err
		}
		record.Payload, err = write(tx)
		if err != nil {
			return err
		}
		settled, err := r.store.Complete(tx, record)
		if err != nil {
			return err
		}
		payload, err = settledPayload(settled)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return payload, replayed, nil
}

func settledPayload(record ReplayRecord) ([]byte, error) {
	if record.Refusal != nil {
		return nil, record.Refusal
	}
	if len(record.Payload) == 0 {
		return nil, ErrInProgress
	}
	return bytes.Clone(record.Payload), nil
}

func settlementError(err error) error {
	if errors.Is(err, framecall.ErrConflict) || errors.Is(err, ErrClaimLost) || errors.Is(err, ErrOutcomeConflict) {
		return fmt.Errorf("idempotency: persist execution outcome: %w", err)
	}
	return errors.Join(ErrOutcomePending, err)
}
