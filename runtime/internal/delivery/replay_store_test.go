package delivery

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
)

type memoryIdempotencyStore struct {
	mu      sync.Mutex
	records map[string]memoryIdempotencyRecord
}

type memoryIdempotencyRecord struct {
	operation.ReplayRecord

	expiresAt time.Time
}

func newMemoryIdempotencyStore() *memoryIdempotencyStore {
	return &memoryIdempotencyStore{records: make(map[string]memoryIdempotencyRecord)}
}

func (m *memoryIdempotencyStore) Claim(_ context.Context, key, fingerprint string) (operation.ReplayRecord, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for storedKey, stored := range m.records {
		if len(stored.Payload) != 0 && !now.Before(stored.expiresAt) {
			delete(m.records, storedKey)
		}
	}
	stored, ok := m.records[key]
	if ok {
		if stored.Fingerprint != fingerprint {
			return operation.ReplayRecord{}, false, extension.ErrInvocationConflict
		}
		stored.Payload = bytes.Clone(stored.Payload)
		return stored.ReplayRecord, false, nil
	}
	record := operation.ReplayRecord{Key: key, Fingerprint: fingerprint}
	m.records[key] = memoryIdempotencyRecord{ReplayRecord: record}
	return record, true, nil
}

func (m *memoryIdempotencyStore) Complete(_ context.Context, record operation.ReplayRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.records[record.Key]
	if !ok {
		return operation.ErrClaimLost
	}
	if stored.Fingerprint != record.Fingerprint {
		return extension.ErrInvocationConflict
	}
	if len(stored.Payload) != 0 {
		if !time.Now().Before(stored.expiresAt) {
			delete(m.records, record.Key)
			return operation.ErrClaimLost
		}
		if !bytes.Equal(stored.Payload, record.Payload) {
			return operation.ErrOutcomeConflict
		}
		return nil
	}
	stored.Payload = bytes.Clone(record.Payload)
	stored.expiresAt = time.Now().Add(operation.ReplayRetention)
	m.records[record.Key] = stored
	return nil
}
