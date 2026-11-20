package persistence

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/operation"

	framecall "github.com/Tangerg/frame/call"
	framesqlite "github.com/Tangerg/frame/sqlite"
)

const (
	idempotencyPendingResponse     = "pending_response"
	idempotencyResponse            = "response"
	idempotencyRunAcceptance       = "run_acceptance"
	idempotencyCancellationRefusal = "cancellation_refusal"
)

// idempotencyStore persists replay records across runtime restarts.
type idempotencyStore struct{ db *database }

// newIdempotencyStore returns a replay store backed by db.
func newIdempotencyStore(db *database) *idempotencyStore {
	return &idempotencyStore{db: db}
}

// idempotencyNamespace returns the opaque identity of db's durable replay
// store. It is published as a transport capability so a client never replays a
// persisted key into a different store that happens to occupy the same URL.
func idempotencyNamespace(ctx context.Context, db *database) (operation.ReplayNamespace, error) {
	return scopedValue(ctx, db.read, func(ctx context.Context) (operation.ReplayNamespace, error) {
		var namespace string
		if err := db.connection(ctx).QueryRowContext(ctx, `SELECT idempotency_namespace FROM runtime_identity WHERE id = 1`).Scan(&namespace); err != nil {
			return operation.ReplayNamespace{}, fmt.Errorf("sqlite: read idempotency namespace: %w", err)
		}
		parsed, err := operation.ParseReplayNamespace(namespace)
		if err != nil {
			return operation.ReplayNamespace{}, fmt.Errorf("sqlite: invalid idempotency namespace: %w", err)
		}
		return parsed, nil
	})
}

func (i *idempotencyStore) Transact(ctx context.Context, write func(context.Context) error) error {
	return i.db.write(ctx, write)
}

// Response records expire with their retention window; a pending response and
// a Run acceptance stay until their owner settles them.
var (
	expiringRecordKinds = []any{idempotencyResponse, idempotencyCancellationRefusal}
	durableRecordKinds  = []any{idempotencyPendingResponse, idempotencyRunAcceptance}
)

func (i *idempotencyStore) Lookup(ctx context.Context, key, fingerprint string) (operation.ReplayRecord, bool, error) {
	return scopedPair(ctx, i.db.read, func(ctx context.Context) (operation.ReplayRecord, bool, error) {
		row := i.db.connection(ctx).QueryRowContext(ctx, `SELECT kind,fingerprint,payload FROM idempotency_records
			WHERE key = ? AND (kind IN (`+placeholders(len(durableRecordKinds))+`) OR expires_at > ?)`,
			slices.Concat([]any{key}, durableRecordKinds, []any{time.Now().Unix()})...)
		return readReplayRecord(row, key, fingerprint)
	})
}

func readReplayRecord(row *framesqlite.Row, key, fingerprint string) (operation.ReplayRecord, bool, error) {
	record := operation.ReplayRecord{Key: key}
	var kind string
	var payload []byte
	err := row.Scan(&kind, &record.Fingerprint, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return operation.ReplayRecord{}, false, nil
	}
	if err != nil {
		return operation.ReplayRecord{}, false, err
	}
	if record.Fingerprint != fingerprint {
		return operation.ReplayRecord{}, false, framecall.ErrConflict
	}
	switch kind {
	case idempotencyResponse:
		record.Payload = payload
	case idempotencyPendingResponse:
		record.Settlement = payload
	case idempotencyCancellationRefusal:
		record.Refusal = cancellationRefusal(payload).cause()
	case idempotencyRunAcceptance:
		// The Run command store alone can read an opening or input receipt.
	default:
		return operation.ReplayRecord{}, false, fmt.Errorf("sqlite: unknown idempotency record kind %q", kind)
	}
	return record, true, nil
}

// Route the claim's prune+insert+lookup through the shared tx seam rather than a
// bare i.db.BeginTx: the pool runs at MaxOpenConns(1), so opening an independent
// transaction while a caller's cross-store transaction is live would deadlock.
// Standalone it still runs atomically (RunInTx begins its own).
// Only completed responses expire. A pending_response reservation is an
// unresolved claim: releasing it on elapsed wall time would let the same key
// execute its business mutation again after a process crash, even though
// the first mutation may already have committed. A run_acceptance acceptance has
// no wall-clock expiry at all and is never pruned here.
func (i *idempotencyStore) Claim(ctx context.Context, claim operation.ReplayClaim) (record operation.ReplayRecord, claimed bool, err error) {
	now := time.Now().Unix()
	err = i.db.write(ctx, func(ctx context.Context) error {
		db := i.db.connection(ctx)
		if _, execContextErr := db.ExecContext(ctx, `DELETE FROM idempotency_records WHERE kind IN (`+placeholders(len(expiringRecordKinds))+`) AND expires_at <= ?`,
			slices.Concat(expiringRecordKinds, []any{now})...); execContextErr != nil {
			return fmt.Errorf("sqlite: prune idempotency records: %w", execContextErr)
		}
		kind, payload, owner := idempotencyPendingResponse, []byte{}, claim.RunID
		refusal, err := i.refuseLateCancellation(ctx, claim.RunID)
		if err != nil {
			return err
		}
		if refusal != "" {
			kind, payload, owner = idempotencyCancellationRefusal, []byte(refusal), identity.RunID{}
		}
		res, execContextErr := db.ExecContext(ctx, `INSERT INTO idempotency_records(
				key, kind, fingerprint, payload, expires_at, material_run_id
			) VALUES (?, ?, ?, ?, ?, NULLIF(?, ''))
			ON CONFLICT(key) DO NOTHING`, claim.Key, kind, claim.Fingerprint, payload,
			now+int64(operation.ReplayRetention/time.Second), owner)
		if execContextErr != nil {
			return fmt.Errorf("sqlite: insert idempotency claim: %w", execContextErr)
		}
		changed, execContextErr := res.RowsAffected()
		if execContextErr != nil {
			return fmt.Errorf("sqlite: inspect idempotency claim: %w", execContextErr)
		}
		if changed != 0 && kind == idempotencyPendingResponse {
			record, claimed = operation.ReplayRecord{Key: claim.Key, Fingerprint: claim.Fingerprint}, true
			return nil
		}
		row := db.QueryRowContext(ctx, `SELECT kind,fingerprint,payload FROM idempotency_records WHERE key = ?`, claim.Key)
		stored, found, err := readReplayRecord(row, claim.Key, claim.Fingerprint)
		if err != nil {
			return fmt.Errorf("sqlite: read idempotency claim: %w", err)
		}
		if !found {
			return operation.ErrClaimLost
		}
		record, claimed = stored, false
		return nil
	})
	if err != nil {
		return operation.ReplayRecord{}, false, err
	}
	return record, claimed, nil
}

func (i *idempotencyStore) StageSettlement(ctx context.Context, record operation.ReplayRecord) error {
	if len(record.Settlement) == 0 || len(record.Payload) != 0 {
		return errors.New("sqlite: a staged settlement requires its unfinished work")
	}
	return i.db.write(ctx, func(ctx context.Context) error {
		result, err := i.db.connection(ctx).ExecContext(ctx, `UPDATE idempotency_records
		  SET payload = ? WHERE key = ? AND fingerprint = ? AND kind = ? AND length(payload) = 0`,
			record.Settlement, record.Key, record.Fingerprint, idempotencyPendingResponse)
		if err != nil {
			return fmt.Errorf("sqlite: stage idempotency settlement: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect idempotency settlement: %w", err)
		}
		if changed != 1 {
			return operation.ErrClaimLost
		}
		return nil
	})
}

// The pending reservation is gone. An exact already-committed response is an
// idempotent success; a different owner or outcome is a conflict; anything
// else means the claim was lost. A run_acceptance never answers here: it is not a
// generic response and cannot share this owner's fingerprint. A refusal filed
// by the Run's terminal transaction is returned as the authoritative receipt.
func (i *idempotencyStore) Complete(ctx context.Context, record operation.ReplayRecord) (operation.ReplayRecord, error) {
	if len(record.Payload) == 0 || record.Refusal != nil {
		return operation.ReplayRecord{}, errors.New("sqlite: an idempotency completion requires its encoded outcome")
	}
	var settled operation.ReplayRecord
	err := i.db.write(ctx, func(ctx context.Context) error {
		now := time.Now().Unix()
		expected := record.Settlement
		if expected == nil {
			expected = []byte{}
		}
		res, err := i.db.connection(ctx).ExecContext(ctx, `UPDATE idempotency_records
		 SET kind = ?, payload = ?, expires_at = ?, material_run_id = NULL
		 WHERE key = ? AND fingerprint = ? AND kind = ? AND payload = ?`,
			idempotencyResponse, record.Payload, now+int64(operation.ReplayRetention/time.Second),
			record.Key, record.Fingerprint, idempotencyPendingResponse, expected)
		if err != nil {
			return fmt.Errorf("sqlite: complete idempotency claim: %w", err)
		}
		changed, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect idempotency completion: %w", err)
		}
		if changed != 0 {
			settled = operation.ReplayRecord{Key: record.Key, Fingerprint: record.Fingerprint, Payload: record.Payload}
			return nil
		}
		row := i.db.connection(ctx).QueryRowContext(ctx, `SELECT kind, fingerprint, payload FROM idempotency_records
			WHERE key = ? AND kind IN (`+placeholders(len(expiringRecordKinds))+`) AND expires_at > ?`,
			slices.Concat([]any{record.Key}, expiringRecordKinds, []any{now})...)
		prior, found, err := readReplayRecord(row, record.Key, record.Fingerprint)
		if err != nil {
			return fmt.Errorf("sqlite: inspect completed idempotency claim: %w", err)
		}
		if !found {
			return operation.ErrClaimLost
		}
		if prior.Refusal != nil && len(record.Settlement) != 0 ||
			prior.Refusal == nil && !bytes.Equal(prior.Payload, record.Payload) {
			return operation.ErrOutcomeConflict
		}
		settled = prior
		return nil
	})
	if err != nil {
		return operation.ReplayRecord{}, err
	}
	return settled, nil
}
