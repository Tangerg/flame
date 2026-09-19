package testsupport

import (
	"bytes"
	"context"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/idempotency"
)

// IdempotencyStore retains claims for delivery tests. Retention and restart
// contracts are exercised against the real SQLite store.
type IdempotencyStore struct {
	mu      sync.Mutex
	records map[string]idempotency.Record
}

func NewIdempotencyStore() *IdempotencyStore {
	return &IdempotencyStore{records: make(map[string]idempotency.Record)}
}

func (s *IdempotencyStore) Claim(_ context.Context, key, fingerprint string) (idempotency.Record, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record, ok := s.records[key]; ok {
		if record.Fingerprint != fingerprint {
			return idempotency.Record{}, false, idempotency.ErrKeyConflict
		}
		record.Payload = bytes.Clone(record.Payload)
		return record, false, nil
	}
	record := idempotency.Record{Key: key, Fingerprint: fingerprint}
	s.records[key] = record
	return record, true, nil
}

func (s *IdempotencyStore) Complete(_ context.Context, record idempotency.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.records[record.Key]
	if !ok {
		return idempotency.ErrClaimLost
	}
	if stored.Fingerprint != record.Fingerprint {
		return idempotency.ErrKeyConflict
	}
	if len(stored.Payload) == 0 {
		record.Payload = bytes.Clone(record.Payload)
		s.records[record.Key] = record
	}
	return nil
}
