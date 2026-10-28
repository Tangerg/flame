package persistence

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
)

const (
	idempotencyPendingResponse = "pending_response"
	idempotencyResponse        = "response"
	idempotencyRunAcceptance   = "run_acceptance"
)

// IdempotencyStore persists replay records across runtime restarts.
type idempotencyStore struct{ db *sql.DB }

// NewIdempotencyStore returns a replay store backed by db.
func newIdempotencyStore(db *sql.DB) *idempotencyStore { return &idempotencyStore{db: db} }

// IdempotencyNamespace returns the opaque identity of db's durable replay
// store. It is published as a transport capability so a client never replays a
// persisted key into a different store that happens to occupy the same URL.
func idempotencyNamespace(ctx context.Context, db *sql.DB) (operation.ReplayNamespace, error) {
	var namespace string
	if err := db.QueryRowContext(ctx,
		`SELECT idempotency_namespace FROM runtime_identity WHERE id = 1`,
	).Scan(&namespace); err != nil {
		return operation.ReplayNamespace{}, fmt.Errorf("sqlite: read idempotency namespace: %w", err)
	}
	parsed, err := operation.ParseReplayNamespace(namespace)
	if err != nil {
		return operation.ReplayNamespace{}, fmt.Errorf("sqlite: invalid idempotency namespace: %w", err)
	}
	return parsed, nil
}

func (i *idempotencyStore) Claim(ctx context.Context, key, fingerprint string) (record operation.ReplayRecord, claimed bool, err error) {
	now := time.Now().Unix()
	// Route the claim's prune+insert+lookup through the shared tx seam rather than a
	// bare i.db.BeginTx: the pool runs at MaxOpenConns(1), so opening an independent
	// transaction while a caller's cross-store transaction is live would deadlock.
	// Standalone it still runs atomically (RunInTx begins its own).
	err = sqlite.RunInTx(ctx, i.db, func(ctx context.Context) error {
		db := sqlite.Connection(ctx, i.db)
		// Only completed responses expire. A pending_response reservation is an
		// unresolved claim: releasing it on elapsed wall time would let the same key
		// execute its business mutation again after a process crash, even though
		// the first mutation may already have committed. A run_acceptance acceptance has
		// no wall-clock expiry at all and is never pruned here.
		if _, execContextErr := db.ExecContext(ctx,
			`DELETE FROM idempotency_records WHERE kind = ? AND expires_at <= ?`, idempotencyResponse, now,
		); execContextErr != nil {
			return fmt.Errorf("sqlite: prune idempotency records: %w", execContextErr)
		}
		res, execContextErr := db.ExecContext(ctx, `INSERT INTO idempotency_records(
				key, kind, fingerprint, payload, expires_at
			) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(key) DO NOTHING`,
			key, idempotencyPendingResponse, fingerprint, []byte{}, now+int64(operation.ReplayRetention/time.Second))
		if execContextErr != nil {
			return fmt.Errorf("sqlite: insert idempotency claim: %w", execContextErr)
		}
		changed, execContextErr := res.RowsAffected()
		if execContextErr != nil {
			return fmt.Errorf("sqlite: inspect idempotency claim: %w", execContextErr)
		}
		if changed != 0 {
			record, claimed = operation.ReplayRecord{Key: key, Fingerprint: fingerprint}, true
			return nil
		}
		stored := operation.ReplayRecord{Key: key}
		if scanErr := db.QueryRowContext(ctx,
			`SELECT fingerprint, payload FROM idempotency_records WHERE key = ?`, key,
		).Scan(&stored.Fingerprint, &stored.Payload); scanErr != nil {
			if errors.Is(scanErr, sql.ErrNoRows) {
				return operation.ErrClaimLost
			}
			return fmt.Errorf("sqlite: read idempotency claim: %w", scanErr)
		}
		if stored.Fingerprint != fingerprint {
			return portable.ErrInvocationConflict
		}
		record, claimed = stored, false
		return nil
	})
	if err != nil {
		return operation.ReplayRecord{}, false, err
	}
	return record, claimed, nil
}

func (i *idempotencyStore) Complete(ctx context.Context, record operation.ReplayRecord) error {
	now := time.Now().Unix()
	res, err := sqlite.Connection(ctx, i.db).ExecContext(ctx,
		`UPDATE idempotency_records SET kind = ?, payload = ?, expires_at = ?
		 WHERE key = ? AND fingerprint = ? AND kind = ?`,
		idempotencyResponse, record.Payload, now+int64(operation.ReplayRetention/time.Second), record.Key, record.Fingerprint, idempotencyPendingResponse)
	if err != nil {
		return fmt.Errorf("sqlite: complete idempotency claim: %w", err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect idempotency completion: %w", err)
	}
	if changed != 0 {
		return nil
	}
	// The pending reservation is gone. An exact already-committed response is an
	// idempotent success; a different owner or outcome is a conflict; anything
	// else means the claim was lost. A run_acceptance never answers here: it is not a
	// generic response and cannot share this owner's fingerprint.
	var fingerprint string
	var payload []byte
	err = sqlite.Connection(ctx, i.db).QueryRowContext(ctx,
		`SELECT fingerprint, payload FROM idempotency_records WHERE key = ? AND kind = ? AND expires_at > ?`,
		record.Key, idempotencyResponse, now).Scan(&fingerprint, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return operation.ErrClaimLost
	}
	if err != nil {
		return fmt.Errorf("sqlite: inspect completed idempotency claim: %w", err)
	}
	if fingerprint != record.Fingerprint {
		return portable.ErrInvocationConflict
	}
	if !bytes.Equal(payload, record.Payload) {
		return operation.ErrOutcomeConflict
	}
	return nil
}
