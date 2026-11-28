package persistence

import (
	"context"
	"crypto/rand"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

func (m *MCPServerStore) PrepareDirectoryChange(ctx context.Context, pending mcpapp.PendingDirectoryChange, replacement *mcpserver.Server) (mcpapp.PendingDirectoryChange, error) {
	intent := pending.Intent
	if (intent.Change.Replacement == nil) != (replacement == nil) {
		return mcpapp.PendingDirectoryChange{}, errors.New("mcp: directory replacement and launch material must be staged together")
	}
	var material []byte
	if replacement != nil {
		if intent.Change.Replacement.LaunchGrant != "" || intent.Change.Name != replacement.Name.String() {
			return mcpapp.PendingDirectoryChange{}, errors.New("mcp: launch material differs from its directory change")
		}
		var err error
		material, err = encodeLaunchMaterial(*replacement)
		if err != nil {
			return mcpapp.PendingDirectoryChange{}, err
		}
		intent.Change.Replacement = new(*intent.Change.Replacement)
	}

	err := m.db.independent(ctx, func(tx context.Context) error {
		db := m.db.connection(tx)
		var previous bool
		if err := db.QueryRowContext(tx, `SELECT EXISTS (
			SELECT 1 FROM pending_mcp_directory_changes WHERE connector=? AND name=?
		)`, intent.Source.InstallationID, intent.Change.Name).Scan(&previous); err != nil {
			return err
		}
		if previous {
			return fmt.Errorf("%w: a directory change for %q is awaiting settlement", operation.ErrInProgress, intent.Change.Name)
		}
		_, claimed, err := m.receipts.Claim(tx, pending.Receipt)
		if err != nil {
			return err
		}
		if !claimed {
			return operation.ErrInProgress
		}
		var newGrant string
		if material != nil {
			grant := &intent.Change.Replacement.LaunchGrant
			err := db.QueryRowContext(tx, `SELECT grant_id FROM mcp_launch_grants
				WHERE connector=? AND source_id=`+mcpSourceIDQuery+` AND material=? AND proposal IS NULL`,
				intent.Source.InstallationID, string(mcpserver.OriginUser), nil, intent.Change.Name, material).Scan(grant)
			if errors.Is(err, sql.ErrNoRows) {
				newGrant = rand.Text()
				*grant = newGrant
			} else if err != nil {
				return err
			}
		}
		if err := intent.Validate(); err != nil {
			return err
		}
		data, err := json.Marshal(intent, json.Deterministic(true))
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(tx, `INSERT INTO pending_mcp_directory_changes(intent,receipt_key) VALUES(?,?)`, string(data), pending.Receipt.Key); err != nil {
			return err
		}
		if newGrant == "" {
			return nil
		}
		if _, err := db.ExecContext(tx, `INSERT INTO mcp_sources(origin,name) VALUES(?,?) ON CONFLICT DO NOTHING`, mcpserver.OriginUser, intent.Change.Name); err != nil {
			return err
		}
		_, err = db.ExecContext(tx, `INSERT INTO mcp_launch_grants(grant_id,connector,source_id,material,proposal)
			VALUES(?,?,`+mcpSourceIDQuery+`,?,?)`, newGrant, intent.Source.InstallationID, string(mcpserver.OriginUser), nil, intent.Change.Name, material, intent.Invocation.ID)
		return err
	})
	if err != nil {
		return mcpapp.PendingDirectoryChange{}, err
	}
	pending.Intent = intent
	return pending, nil
}

func (m *MCPServerStore) PendingDirectoryChange(ctx context.Context, receipt operation.ReplayClaim) (mcpapp.PendingDirectoryChange, bool, error) {
	return scopedPair(ctx, m.db.read, func(ctx context.Context) (mcpapp.PendingDirectoryChange, bool, error) {
		var body string
		err := m.db.connection(ctx).QueryRowContext(ctx, `SELECT pending.intent
			FROM pending_mcp_directory_changes pending
			JOIN idempotency_records receipt ON receipt.key=pending.receipt_key
			WHERE receipt.key=? AND receipt.fingerprint=? AND receipt.kind=?`,
			receipt.Key, receipt.Fingerprint, idempotencyPendingResponse).Scan(&body)
		if errors.Is(err, sql.ErrNoRows) {
			return mcpapp.PendingDirectoryChange{}, false, nil
		}
		if err != nil {
			return mcpapp.PendingDirectoryChange{}, false, err
		}
		intent, err := decodeDirectoryIntent(body)
		return mcpapp.PendingDirectoryChange{Intent: intent, Receipt: receipt}, err == nil, err
	})
}

func (m *MCPServerStore) PendingDirectoryChanges(ctx context.Context) ([]mcpapp.PendingDirectoryChange, error) {
	return scopedValue(ctx, m.db.read, func(ctx context.Context) ([]mcpapp.PendingDirectoryChange, error) {
		rows, err := m.db.connection(ctx).QueryContext(ctx, `SELECT pending.intent,
			pending.receipt_key, receipt.fingerprint
			FROM pending_mcp_directory_changes pending
			LEFT JOIN idempotency_records receipt ON receipt.key=pending.receipt_key AND receipt.kind=?
			ORDER BY pending.rowid`, idempotencyPendingResponse)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var changes []mcpapp.PendingDirectoryChange
		for rows.Next() {
			var body string
			var change mcpapp.PendingDirectoryChange
			if err := rows.Scan(&body, &change.Receipt.Key, &change.Receipt.Fingerprint); err != nil {
				return nil, err
			}
			change.Intent, err = decodeDirectoryIntent(body)
			if err != nil {
				return nil, err
			}
			changes = append(changes, change)
		}
		return changes, rows.Err()
	})
}

func decodeDirectoryIntent(body string) (mcpapp.DirectoryIntent, error) {
	var intent mcpapp.DirectoryIntent
	if err := json.Unmarshal([]byte(body), &intent, json.RejectUnknownMembers(true)); err != nil {
		return mcpapp.DirectoryIntent{}, err
	}
	return intent, intent.Validate()
}

func (m *MCPServerStore) CompleteDirectoryChange(ctx context.Context, pending mcpapp.PendingDirectoryChange, accepted bool, payload []byte) error {
	return m.db.independent(ctx, func(tx context.Context) error {
		if err := m.settleLaunchGrants(tx, pending.Intent, accepted); err != nil {
			return err
		}
		result, err := m.db.connection(tx).ExecContext(tx, `DELETE FROM pending_mcp_directory_changes WHERE invocation=? AND receipt_key=?`, pending.Intent.Invocation.ID, pending.Receipt.Key)
		if err != nil {
			return err
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if deleted != 1 {
			return operation.ErrClaimLost
		}
		_, err = m.receipts.Complete(tx, operation.ReplayRecord{
			Key: pending.Receipt.Key, Fingerprint: pending.Receipt.Fingerprint, Payload: payload,
		})
		return err
	})
}
