package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/httporigin"
)

// An authorization attempt owns one grant even before it has tokens. Replacing
// that grant revokes every refresh and rejection callback from the old handler.
func (m *MCPServerStore) BeginOAuthSession(ctx context.Context, target mcpserver.OAuthTarget) (string, error) {
	var binding string
	err := RunInTx(ctx, m.db, func(ctx context.Context) error {
		origin, err := m.oauthTargetOrigin(ctx, target)
		if err != nil {
			return err
		}
		return conn(ctx, m.db).QueryRowContext(ctx,
			`INSERT INTO mcp_oauth_sessions(server_name, origin, binding, payload)
			 VALUES (?, ?, lower(hex(randomblob(16))), x'')
			 ON CONFLICT(server_name) DO UPDATE SET
			   origin = excluded.origin, binding = excluded.binding, payload = excluded.payload
			 RETURNING binding`, target.Server.String(), origin).Scan(&binding)
	})
	if err != nil {
		return "", fmt.Errorf("sqlite: begin mcp oauth session: %w", err)
	}
	return binding, nil
}

func (m *MCPServerStore) LoadOAuthSession(ctx context.Context, target mcpserver.OAuthTarget) ([]byte, string, bool, error) {
	var payload []byte
	var binding string
	found := false
	err := RunInTx(ctx, m.db, func(ctx context.Context) error {
		origin, err := m.oauthTargetOrigin(ctx, target)
		if err != nil {
			return err
		}
		err = conn(ctx, m.db).QueryRowContext(ctx,
			`SELECT payload, binding FROM mcp_oauth_sessions
			 WHERE server_name = ? AND origin = ? AND length(payload) > 0`,
			target.Server.String(), origin).Scan(&payload, &binding)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if binding == "" {
			return errors.New("sqlite: mcp oauth credentials have no binding")
		}
		found = true
		return nil
	})
	if err != nil {
		return nil, "", false, fmt.Errorf("sqlite: load mcp oauth session: %w", err)
	}
	return payload, binding, found, nil
}

func (m *MCPServerStore) oauthTargetOrigin(ctx context.Context, target mcpserver.OAuthTarget) (string, error) {
	current, found, err := m.Get(ctx, target.Server)
	if err != nil {
		return "", err
	}
	if !found || !target.Matches(current) {
		return "", mcpserver.ErrOAuthSessionSuperseded
	}
	origin, err := httporigin.Parse(target.URL)
	if err != nil {
		return "", fmt.Errorf("sqlite: mcp oauth origin: %w", err)
	}
	return origin.String(), nil
}

func (m *MCPServerStore) SaveOAuthSession(ctx context.Context, server mcpserver.ServerName, origin, binding string, payload []byte) error {
	if binding == "" {
		return mcpserver.ErrOAuthSessionSuperseded
	}
	if origin == "" || len(payload) == 0 {
		return errors.New("sqlite: mcp oauth session requires origin and payload")
	}
	result, err := conn(ctx, m.db).ExecContext(ctx,
		`UPDATE mcp_oauth_sessions SET payload = ?
		 WHERE server_name = ? AND origin = ? AND binding = ?`,
		payload, server.String(), origin, binding)
	if err != nil {
		return fmt.Errorf("sqlite: save mcp oauth session: %w", err)
	}
	return oauthGrantApplied(result)
}

func (m *MCPServerStore) RemoveOAuthSession(ctx context.Context, server mcpserver.ServerName, binding string) error {
	if binding == "" {
		return mcpserver.ErrOAuthSessionSuperseded
	}
	result, err := conn(ctx, m.db).ExecContext(ctx,
		`DELETE FROM mcp_oauth_sessions WHERE server_name = ? AND binding = ?`, server.String(), binding)
	if err != nil {
		return fmt.Errorf("sqlite: remove mcp oauth session: %w", err)
	}
	return oauthGrantApplied(result)
}

func oauthGrantApplied(result sql.Result) error {
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect mcp oauth grant: %w", err)
	}
	if changed != 1 {
		return mcpserver.ErrOAuthSessionSuperseded
	}
	return nil
}
