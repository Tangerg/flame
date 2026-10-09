// Package idempotency defines durable request-result replay coordination.
//
// It intentionally deals in opaque keys, fingerprints, and bytes: callers
// decide how to derive a request fingerprint and encode a result. The mechanism
// does not prescribe business or transport vocabulary.
package idempotency

import (
	"context"
	"errors"
	"time"
)

// ErrKeyConflict reports that a key is already bound to another request.
var ErrKeyConflict = errors.New("idempotency: key reused with different request")

// ErrClaimLost reports that completion no longer owns the reserved key.
var ErrClaimLost = errors.New("idempotency: claim is no longer available")

// ErrOutcomeConflict reports a stored result that differs from its execution's result.
var ErrOutcomeConflict = errors.New("idempotency: completed outcome differs from the execution result")

// Retention is the default replay window for a completed idempotency result.
// An unresolved reservation does not expire: elapsed time cannot prove whether
// its business mutation committed before a process crash.
const Retention = 24 * time.Hour

// Record is the durable state of one idempotent request.
type Record struct {
	Key         string
	Fingerprint string
	Payload     []byte
}

// Store atomically claims logical operations and persists their execution result.
type Store interface {
	// Claim atomically reserves key for fingerprint. claimed=false returns the
	// existing claim: an empty Payload means its first execution is still in
	// progress; a non-empty Payload is the completed opaque result to replay.
	Claim(ctx context.Context, key, fingerprint string) (record Record, claimed bool, err error)
	// Complete stores the execution's result for a previously acquired claim.
	// Repeating that exact completion confirms it; a different payload is an
	// invariant violation, and a lost claim must never be recreated here.
	Complete(ctx context.Context, record Record) error
}
