package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// MCPAuthorizationStore keeps at most one credential per MCP source, bound to
// the fingerprint of the OAuth target that requested it. A configuration
// change yields a different target, so the earlier credential stops matching
// without any invalidation step; removing the source removes it by cascade.
type MCPAuthorizationStore struct{ db *sql.DB }

func NewMCPAuthorizationStore(db *sql.DB) *MCPAuthorizationStore {
	return &MCPAuthorizationStore{db: db}
}

// An authorization attempt owns one grant even before it has tokens. Replacing
// that grant revokes every refresh and rejection callback from the old handler.
func (m *MCPAuthorizationStore) BeginOAuthSession(ctx context.Context, target mcpserver.OAuthTarget) (string, error) {
	var binding string
	args := append([]any{target.Fingerprint().String()}, mcpSourceArgs(target.ID())...)
	err := conn(ctx, m.db).QueryRowContext(ctx,
		`INSERT INTO mcp_oauth_sessions(source_id, target_fingerprint, binding, payload)
		 SELECT id, ?, lower(hex(randomblob(16))), x'' FROM mcp_sources WHERE origin = ? AND installation_id IS ? AND name = ?
		 ON CONFLICT(source_id) DO UPDATE SET
		   target_fingerprint = excluded.target_fingerprint, binding = excluded.binding, payload = excluded.payload
		 RETURNING binding`, args...).Scan(&binding)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("sqlite: begin mcp oauth session: %w: server %s was removed", mcpserver.ErrOAuthSessionSuperseded, target.ID())
	}
	if err != nil {
		return "", fmt.Errorf("sqlite: begin mcp oauth session: %w", err)
	}
	return binding, nil
}

func (m *MCPAuthorizationStore) LoadOAuthSession(ctx context.Context, target mcpserver.OAuthTarget) ([]byte, string, bool, error) {
	var payload []byte
	var binding string
	args := append(mcpSourceArgs(target.ID()), target.Fingerprint().String())
	err := conn(ctx, m.db).QueryRowContext(ctx,
		`SELECT payload, binding FROM mcp_oauth_sessions
		 WHERE source_id = `+mcpSourceIDQuery+` AND target_fingerprint = ? AND length(payload) > 0`,
		args...).Scan(&payload, &binding)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("sqlite: load mcp oauth session: %w", err)
	}
	if binding == "" {
		return nil, "", false, errors.New("sqlite: mcp oauth credentials have no binding")
	}
	return payload, binding, true, nil
}

func (m *MCPAuthorizationStore) SaveOAuthSession(ctx context.Context, server mcpserver.ID, binding string, payload []byte) error {
	if binding == "" {
		return mcpserver.ErrOAuthSessionSuperseded
	}
	if len(payload) == 0 {
		return errors.New("sqlite: mcp oauth session requires a payload")
	}
	result, err := conn(ctx, m.db).ExecContext(ctx,
		`UPDATE mcp_oauth_sessions SET payload = ? WHERE source_id = `+mcpSourceIDQuery+` AND binding = ?`,
		append(append([]any{payload}, mcpSourceArgs(server)...), binding)...)
	if err != nil {
		return fmt.Errorf("sqlite: save mcp oauth session: %w", err)
	}
	return oauthGrantApplied(result)
}

func (m *MCPAuthorizationStore) RemoveOAuthSession(ctx context.Context, server mcpserver.ID, binding string) error {
	if binding == "" {
		return mcpserver.ErrOAuthSessionSuperseded
	}
	result, err := conn(ctx, m.db).ExecContext(ctx,
		`DELETE FROM mcp_oauth_sessions WHERE source_id = `+mcpSourceIDQuery+` AND binding = ?`, append(mcpSourceArgs(server), binding)...)
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
