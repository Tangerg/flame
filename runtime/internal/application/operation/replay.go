package operation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

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

// Completed results expire; an unresolved reservation cannot expire because
// elapsed time does not prove whether its business mutation committed.
const ReplayRetention = 24 * time.Hour

const replayWriteTimeout = 5 * time.Second

type ReplayRecord struct {
	Key         string
	Fingerprint string
	Payload     []byte
	Settlement  []byte
}

type ReplayStore interface {
	Lookup(context.Context, string, string) (ReplayRecord, bool, error)
	Transact(context.Context, func(context.Context) error) error
	// An existing claim never authorizes the business write to run again.
	Claim(context.Context, string, string) (ReplayRecord, bool, error)
	// StageSettlement retains the work that can be settled after the original
	// business transaction commits.
	StageSettlement(context.Context, ReplayRecord) error
	// Exact completion is idempotent. A lost claim cannot be recreated here.
	Complete(context.Context, ReplayRecord) error
}

// Replay owns admission and receipt settlement for opaque operation results.
// Delivery alone chooses their encoding. A known execution result stays with
// this owner until durable completion is acknowledged, including during close.
type Replay struct {
	store     ReplayStore
	inFlight  *keylock.Set
	pendingMu sync.Mutex
	pending   map[string]ReplayRecord
}

func NewReplay(store ReplayStore) (*Replay, error) {
	if lo.IsNil(store) {
		return nil, errors.New("idempotency: durable store is required")
	}
	return &Replay{store: store, inFlight: keylock.NewSet(), pending: make(map[string]ReplayRecord)}, nil
}

// Serialize holds a Run command through its opening acknowledgement. Its
// acceptance transaction remains that command's durable result owner.
func (r *Replay) Serialize(ctx context.Context, key string) (func(), error) {
	return r.inFlight.Acquire(ctx, key)
}

// A recoverable command owns admission and settlement of its durable intent.
// Only that intent can authorize adoption of a pending key. An admission
// refusal can settle directly if no intent was ever accepted.
func (r *Replay) InvokeRecoverable(ctx context.Context, key, fingerprint string, execute func() ([]byte, error), refuse func(error) ([]byte, error)) ([]byte, error) {
	release, err := r.inFlight.Acquire(ctx, key)
	if err != nil {
		return nil, err
	}
	defer release()
	record, found, err := r.store.Lookup(ctx, key, fingerprint)
	if err != nil {
		return nil, err
	}
	if found && len(record.Payload) != 0 {
		return settledPayload(record)
	}
	payload, err := execute()
	if err == nil || errors.Is(err, ErrOutcomePending) || errors.Is(err, ErrInProgress) || errors.Is(err, framecall.ErrConflict) {
		return payload, err
	}
	payload, _, commitErr := r.commitOutcome(ctx, key, fingerprint, func(context.Context) ([]byte, error) {
		return refuse(err)
	})
	return payload, commitErr
}

func (r *Replay) Invoke(ctx context.Context, key, fingerprint string, execute func() ([]byte, error)) ([]byte, error) {
	release, err := r.inFlight.Acquire(ctx, key)
	if err != nil {
		return nil, err
	}
	defer release()
	if payload, pending, err := r.finishPending(ctx, key, fingerprint); pending {
		return payload, err
	}
	record, claimed, err := r.store.Claim(ctx, key, fingerprint)
	if err != nil {
		return nil, fmt.Errorf("idempotency: claim replay key: %w", err)
	}
	if !claimed {
		return settledPayload(record)
	}
	payload, err := execute()
	if err != nil {
		return nil, err
	}
	record.Payload = payload
	return r.finishRecord(ctx, record)
}

func (r *Replay) finishPending(ctx context.Context, key, fingerprint string) ([]byte, bool, error) {
	record, pending := r.pendingCompletion(key)
	if !pending {
		return nil, false, nil
	}
	if record.Fingerprint != fingerprint {
		return nil, true, framecall.ErrConflict
	}
	if err := r.completeWithin(context.WithoutCancel(ctx), record); err != nil {
		return nil, true, settlementError(err)
	}
	r.forgetPendingCompletion(key, fingerprint)
	return record.Payload, true, nil
}

func (r *Replay) finishRecord(ctx context.Context, record ReplayRecord) ([]byte, error) {
	if err := r.completeWithin(context.WithoutCancel(ctx), record); err != nil {
		if errors.Is(err, framecall.ErrConflict) {
			return nil, err
		}
		r.rememberPendingCompletion(record)
		return nil, settlementError(err)
	}
	return record.Payload, nil
}

// AtomicCompletion is borrowed by one synchronous command. A failed write
// remains retryable by its caller after the database rolls back both effects.
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

// The command keeps its own admission through completion. Only its database
// write and result encoding run in the receipt transaction.
func (r *Replay) InvokeAtomic(ctx context.Context, key, fingerprint string, execute func(*AtomicCompletion) error) ([]byte, error) {
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
		record, claimed, err := r.store.Claim(tx, key, fingerprint)
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
		if err := r.store.Complete(tx, record); err != nil {
			return err
		}
		payload = record.Payload
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return payload, replayed, nil
}

func settledPayload(record ReplayRecord) ([]byte, error) {
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

func (r *Replay) completeWithin(ctx context.Context, record ReplayRecord) error {
	writeContext, cancel := context.WithTimeout(ctx, replayWriteTimeout)
	defer cancel()
	return r.store.Complete(writeContext, record)
}

// FlushPending is called after operation admission closes and every accepted
// operation has returned. Failed settlements remain available to a later close.
func (r *Replay) FlushPending(ctx context.Context) error {
	if ctx == nil {
		return errors.New("idempotency: flush context is required")
	}
	var errs []error
	for _, key := range r.pendingKeys() {
		release, err := r.inFlight.Acquire(ctx, key)
		if err != nil {
			errs = append(errs, fmt.Errorf("idempotency: flush pending outcome: %w", err))
			continue
		}
		if pending, ok := r.pendingCompletion(key); ok {
			if err := r.completeWithin(ctx, pending); err != nil {
				errs = append(errs, fmt.Errorf("idempotency: flush pending outcome: %w", err))
			} else {
				r.forgetPendingCompletion(key, pending.Fingerprint)
			}
		}
		release()
	}
	return errors.Join(errs...)
}

func (r *Replay) pendingCompletion(key string) (ReplayRecord, bool) {
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()
	record, ok := r.pending[key]
	record.Payload = bytes.Clone(record.Payload)
	record.Settlement = bytes.Clone(record.Settlement)
	return record, ok
}

func (r *Replay) rememberPendingCompletion(record ReplayRecord) {
	record.Payload = bytes.Clone(record.Payload)
	record.Settlement = bytes.Clone(record.Settlement)
	r.pendingMu.Lock()
	r.pending[record.Key] = record
	r.pendingMu.Unlock()
}

func (r *Replay) forgetPendingCompletion(key, fingerprint string) {
	r.pendingMu.Lock()
	if r.pending[key].Fingerprint == fingerprint {
		delete(r.pending, key)
	}
	r.pendingMu.Unlock()
}

func (r *Replay) pendingKeys() []string {
	r.pendingMu.Lock()
	keys := make([]string, 0, len(r.pending))
	for key := range r.pending {
		keys = append(keys, key)
	}
	r.pendingMu.Unlock()
	slices.Sort(keys)
	return keys
}
