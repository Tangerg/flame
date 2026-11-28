package persistence

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
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
		_, claimed, err := m.receipts.Claim(tx, pending.Receipt)
		if err != nil {
			return err
		}
		if !claimed {
			return operation.ErrInProgress
		}
		_, err = db.ExecContext(tx, `INSERT INTO pending_mcp_authorization_starts(intent,receipt_key) VALUES(?,?)`, string(intent), pending.Receipt.Key)
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
		rows, err := m.db.connection(tx).QueryContext(tx, `SELECT start.intent, start.receipt_key, receipt.fingerprint
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
	var intent mcpapp.AuthorizationIntent
	if err := json.Unmarshal([]byte(data), &intent, json.RejectUnknownMembers(true)); err != nil {
		return mcpapp.PendingAuthorizationStart{}, err
	}
	return mcpapp.PendingAuthorizationStart{Intent: intent, Receipt: receipt}, intent.Validate()
}

func (m *MCPServerStore) CompleteAuthorizationStart(ctx context.Context, pending mcpapp.PendingAuthorizationStart, accepted bool, payload []byte) error {
	return m.db.independent(ctx, func(tx context.Context) error {
		result, err := m.db.connection(tx).ExecContext(tx, `DELETE FROM pending_mcp_authorization_starts WHERE invocation=? AND receipt_key=?`, pending.Intent.Invocation.ID, pending.Receipt.Key)
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
		if accepted {
			source, err := json.Marshal(pending.Intent.Source)
			if err != nil {
				return err
			}
			start := pending.Intent.Start
			_, err = m.db.connection(tx).ExecContext(tx, `INSERT INTO mcp_authorization_routes(attempt,source,name,epoch) VALUES (?,?,?,?)`,
				start.AttemptID, string(source), start.Name, start.Epoch)
			if err != nil {
				return err
			}
		}
		_, err = m.receipts.Complete(tx, operation.ReplayRecord{Key: pending.Receipt.Key, Fingerprint: pending.Receipt.Fingerprint, Payload: payload})
		return err
	})
}

func (m *MCPServerStore) AuthorizationRoute(ctx context.Context, id mcpapp.AuthorizationAttemptID) (mcpapp.AuthorizationRoute, bool, error) {
	return scopedPair(ctx, m.db.read, func(tx context.Context) (mcpapp.AuthorizationRoute, bool, error) {
		var source, name, epoch string
		err := m.db.connection(tx).QueryRowContext(tx, `SELECT source,name,epoch FROM mcp_authorization_routes WHERE attempt=?`, id.String()).Scan(&source, &name, &epoch)
		if errors.Is(err, sql.ErrNoRows) {
			return mcpapp.AuthorizationRoute{}, false, nil
		}
		if err != nil {
			return mcpapp.AuthorizationRoute{}, false, err
		}
		route, err := decodeAuthorizationRoute(source, name, epoch)
		return route, err == nil, err
	})
}

func decodeAuthorizationRoute(source, name, epoch string) (mcpapp.AuthorizationRoute, error) {
	var binding extension.ContributionSource
	if err := json.Unmarshal([]byte(source), &binding, json.RejectUnknownMembers(true)); err != nil {
		return mcpapp.AuthorizationRoute{}, err
	}
	server, err := mcpserver.ParseServerName(name)
	if err != nil {
		return mcpapp.AuthorizationRoute{}, err
	}
	return mcpapp.AuthorizationRoute{Source: binding, Name: server, Epoch: epoch}, nil
}

func (m *MCPServerStore) AuthorizationConnectors(ctx context.Context) ([]extension.ContributionSource, error) {
	return scopedValue(ctx, m.db.read, func(tx context.Context) ([]extension.ContributionSource, error) {
		rows, err := m.db.connection(tx).QueryContext(tx, `SELECT DISTINCT source FROM mcp_authorization_routes ORDER BY source`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var connectors []extension.ContributionSource
		for rows.Next() {
			var source string
			if err := rows.Scan(&source); err != nil {
				return nil, err
			}
			var binding extension.ContributionSource
			if err := json.Unmarshal([]byte(source), &binding, json.RejectUnknownMembers(true)); err != nil {
				return nil, err
			}
			connectors = append(connectors, binding)
		}
		return connectors, rows.Err()
	})
}
