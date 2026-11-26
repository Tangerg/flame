package persistence

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
)

func (m *MCPServerStore) PrepareAuthorizationStart(ctx context.Context, pending mcpapp.PendingAuthorizationStart) error {
	if err := pending.Intent.Validate(); err != nil {
		return err
	}
	intent, err := json.Marshal(pending.Intent, json.Deterministic(true))
	if err != nil {
		return err
	}
	return m.db.independent(ctx, func(tx context.Context) error {
		db := m.db.connection(tx)
		var existing bool
		if err := db.QueryRowContext(tx, `SELECT EXISTS(SELECT 1 FROM pending_mcp_authorization_starts WHERE connector=? AND epoch=? AND name=?)`,
			pending.Intent.Source.InstallationID, pending.Intent.Start.Epoch, pending.Intent.Start.Name).Scan(&existing); err != nil {
			return err
		}
		if existing {
			return operation.ErrInProgress
		}
		if pending.Receipt.Key != "" {
			_, claimed, err := m.receipts.Claim(tx, pending.Receipt)
			if err != nil {
				return err
			}
			if !claimed {
				return operation.ErrInProgress
			}
		}
		_, err := db.ExecContext(tx, `INSERT INTO pending_mcp_authorization_starts(intent,receipt_key) VALUES(?,NULLIF(?,''))`, string(intent), pending.Receipt.Key)
		return err
	})
}

func (m *MCPServerStore) PendingAuthorizationStart(ctx context.Context, receipt operation.ReplayClaim) (mcpapp.PendingAuthorizationStart, bool, error) {
	return scopedPair(ctx, m.db.read, func(tx context.Context) (mcpapp.PendingAuthorizationStart, bool, error) {
		var data string
		err := m.db.connection(tx).QueryRowContext(tx, `SELECT start.intent
			FROM pending_mcp_authorization_starts start JOIN idempotency_records receipt ON receipt.key=start.receipt_key
			WHERE receipt.key=? AND receipt.fingerprint=? AND receipt.kind=?`, receipt.Key, receipt.Fingerprint, idempotencyPendingResponse).Scan(&data)
		if errors.Is(err, sql.ErrNoRows) {
			return mcpapp.PendingAuthorizationStart{}, false, nil
		}
		if err != nil {
			return mcpapp.PendingAuthorizationStart{}, false, err
		}
		pending, err := decodeAuthorizationStart(data, receipt)
		return pending, err == nil, err
	})
}

func (m *MCPServerStore) PendingAuthorizationStarts(ctx context.Context) ([]mcpapp.PendingAuthorizationStart, error) {
	return scopedValue(ctx, m.db.read, func(tx context.Context) ([]mcpapp.PendingAuthorizationStart, error) {
		rows, err := m.db.connection(tx).QueryContext(tx, `SELECT start.intent, COALESCE(start.receipt_key,''), COALESCE(receipt.fingerprint,'')
			FROM pending_mcp_authorization_starts start LEFT JOIN idempotency_records receipt ON receipt.key=start.receipt_key AND receipt.kind=?
			ORDER BY start.rowid`, idempotencyPendingResponse)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var starts []mcpapp.PendingAuthorizationStart
		for rows.Next() {
			var data string
			var receipt operation.ReplayClaim
			if err := rows.Scan(&data, &receipt.Key, &receipt.Fingerprint); err != nil {
				return nil, err
			}
			start, err := decodeAuthorizationStart(data, receipt)
			if err != nil {
				return nil, err
			}
			starts = append(starts, start)
		}
		return starts, rows.Err()
	})
}

func decodeAuthorizationStart(data string, receipt operation.ReplayClaim) (mcpapp.PendingAuthorizationStart, error) {
	if receipt.Key != "" && receipt.Fingerprint == "" {
		return mcpapp.PendingAuthorizationStart{}, operation.ErrClaimLost
	}
	var intent mcpapp.AuthorizationIntent
	if err := json.Unmarshal([]byte(data), &intent, json.RejectUnknownMembers(true)); err != nil {
		return mcpapp.PendingAuthorizationStart{}, err
	}
	return mcpapp.PendingAuthorizationStart{Intent: intent, Receipt: receipt}, intent.Validate()
}

func (m *MCPServerStore) CompleteAuthorizationStart(ctx context.Context, pending mcpapp.PendingAuthorizationStart, payload []byte) error {
	return m.db.independent(ctx, func(tx context.Context) error {
		result, err := m.db.connection(tx).ExecContext(tx, `DELETE FROM pending_mcp_authorization_starts WHERE invocation=?`, pending.Intent.Invocation.ID)
		if err != nil {
			return err
		}
		removed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if removed != 1 {
			return operation.ErrClaimLost
		}
		if pending.Receipt.Key == "" {
			return nil
		}
		_, err = m.receipts.Complete(tx, operation.ReplayRecord{Key: pending.Receipt.Key, Fingerprint: pending.Receipt.Fingerprint, Payload: payload})
		return err
	})
}
