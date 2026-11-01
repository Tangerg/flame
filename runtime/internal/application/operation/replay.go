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
	"github.com/Tangerg/qimu/portable"
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
}

type ReplayStore interface {
	Lookup(context.Context, string, string) (ReplayRecord, bool, error)
	Transact(context.Context, func(context.Context) error) error
	// An existing empty payload is unresolved; it never authorizes execution.
	Claim(context.Context, string, string) (ReplayRecord, bool, error)
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

func (r *Replay) Invoke(ctx context.Context, key, fingerprint string, execute func() ([]byte, error)) ([]byte, error) {
	release, err := r.inFlight.Acquire(ctx, key)
	if err != nil {
		return nil, err
	}
	defer release()
	if pending, ok := r.pendingCompletion(key); ok {
		if pending.Fingerprint != fingerprint {
			return nil, portable.ErrInvocationConflict
		}
		if err := r.completeWithin(context.WithoutCancel(ctx), pending); err != nil {
			return nil, settlementError(err)
		}
		r.forgetPendingCompletion(key, fingerprint)
		return pending.Payload, nil
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
	record = ReplayRecord{Key: key, Fingerprint: fingerprint, Payload: payload}
	if err := r.completeWithin(context.WithoutCancel(ctx), record); err != nil {
		if errors.Is(err, portable.ErrInvocationConflict) {
			return nil, err
		}
		r.rememberPendingCompletion(record)
		return nil, settlementError(err)
	}
	return payload, nil
}

type PreparedOutcome struct {
	Payload []byte
	Commit  func(context.Context) error
	Publish func()
}

// Local database commands can couple their outcome to their only effect.
// Preparation can call external providers, so it runs outside the transaction.
func (r *Replay) InvokeAtomic(ctx context.Context, key, fingerprint string, prepare func() (PreparedOutcome, error)) ([]byte, error) {
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
	prepared, err := prepare()
	if err != nil {
		return nil, err
	}
	var payload []byte
	var committed bool
	err = r.store.Transact(ctx, func(tx context.Context) error {
		record, claimed, err := r.store.Claim(tx, key, fingerprint)
		if err != nil {
			return err
		}
		if !claimed {
			payload, err = settledPayload(record)
			return err
		}
		if prepared.Commit != nil {
			if err := prepared.Commit(tx); err != nil {
				return err
			}
		}
		record.Payload = prepared.Payload
		if err := r.store.Complete(tx, record); err != nil {
			return err
		}
		payload = record.Payload
		committed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if committed && prepared.Publish != nil {
		prepared.Publish()
	}
	return payload, nil
}

func settledPayload(record ReplayRecord) ([]byte, error) {
	if len(record.Payload) == 0 {
		return nil, ErrInProgress
	}
	return bytes.Clone(record.Payload), nil
}

func settlementError(err error) error {
	if errors.Is(err, portable.ErrInvocationConflict) || errors.Is(err, ErrClaimLost) || errors.Is(err, ErrOutcomeConflict) {
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
	return record, ok
}

func (r *Replay) rememberPendingCompletion(record ReplayRecord) {
	record.Payload = bytes.Clone(record.Payload)
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
