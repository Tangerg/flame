package persistence

import (
	"context"
	json "encoding/json/v2"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
)

func (m *MCPServerStore) RecordAuthorizationFinish(ctx context.Context, finish mcpapp.AuthorizationFinish) error {
	invocation, err := json.Marshal(finish.Invocation)
	if err != nil {
		return err
	}
	return m.db.independent(ctx, func(tx context.Context) error {
		_, err := m.db.connection(tx).ExecContext(tx, `INSERT INTO pending_mcp_authorization_finishes(attempt,outcome,invocation) VALUES(?,?,?)`,
			finish.ID.String(), finish.Outcome, string(invocation))
		return err
	})
}

func (m *MCPServerStore) PendingAuthorizationFinishes(ctx context.Context) ([]mcpapp.PendingAuthorizationFinish, error) {
	return scopedValue(ctx, m.db.read, func(tx context.Context) ([]mcpapp.PendingAuthorizationFinish, error) {
		rows, err := m.db.connection(tx).QueryContext(tx, `SELECT f.attempt,f.outcome,f.invocation,r.source,r.name,r.epoch
			FROM pending_mcp_authorization_finishes f JOIN mcp_authorization_routes r ON f.attempt=r.attempt
			ORDER BY f.rowid`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var finishes []mcpapp.PendingAuthorizationFinish
		for rows.Next() {
			var id, invocation, source, name, epoch string
			var pending mcpapp.PendingAuthorizationFinish
			if err := rows.Scan(&id, &pending.Outcome, &invocation, &source, &name, &epoch); err != nil {
				return nil, err
			}
			pending.ID, err = mcpapp.ParseAuthorizationAttemptID(id)
			if err != nil {
				return nil, err
			}
			pending.Route, err = decodeAuthorizationRoute(source, name, epoch)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal([]byte(invocation), &pending.Invocation, json.RejectUnknownMembers(true)); err != nil {
				return nil, err
			}
			finishes = append(finishes, pending)
		}
		return finishes, rows.Err()
	})
}

func (m *MCPServerStore) CompleteAuthorizationFinish(ctx context.Context, id mcpapp.AuthorizationAttemptID) error {
	return m.db.independent(ctx, func(tx context.Context) error {
		result, err := m.db.connection(tx).ExecContext(tx, `DELETE FROM pending_mcp_authorization_finishes WHERE attempt=?`, id.String())
		if err != nil {
			return err
		}
		removed, err := result.RowsAffected()
		if err == nil && removed != 1 {
			err = operation.ErrClaimLost
		}
		return err
	})
}
