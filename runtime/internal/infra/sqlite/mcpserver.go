package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// MCPServerStore persists MCP server entries in SQLite. One row per server
// name; Save atomically replaces one complete entry and its normalized tool
// policy relation. Args and the map columns (env / headers) are JSON-encoded;
// a bounded handshake timeout is stored as positive nanoseconds and NULL means
// unbounded. The DB must have been opened
// via [Open] so the mcp_servers table exists.
type MCPServerStore struct {
	db *sql.DB
}

// NewMCPServerStore wires the given *sql.DB to MCP server persistence.
func NewMCPServerStore(db *sql.DB) *MCPServerStore {
	return &MCPServerStore{db: db}
}

// mcpColumns is the column list shared by List and Get so the two reads and
// scanMCPServer stay in lockstep.
const mcpColumns = `name, transport, enabled, description, url, authorization, headers,
	        command, args, env, dir, timeout`

func (m *MCPServerStore) List(ctx context.Context) ([]mcpserver.Server, error) {
	rows, err := conn(ctx, m.db).QueryContext(ctx,
		`SELECT `+mcpColumns+` FROM mcp_servers ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list mcp servers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []mcpserver.Server
	for rows.Next() {
		srv, scanErr := scanMCPServer(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, srv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list mcp servers: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("sqlite: close mcp server rows: %w", err)
	}
	if err := m.loadToolPolicies(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (m *MCPServerStore) Get(ctx context.Context, name mcpserver.ServerName) (mcpserver.Server, bool, error) {
	row := conn(ctx, m.db).QueryRowContext(ctx,
		`SELECT `+mcpColumns+` FROM mcp_servers WHERE name = ?`, name.String())
	srv, err := scanMCPServer(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return mcpserver.Server{}, false, nil
	}
	if err != nil {
		return mcpserver.Server{}, false, err
	}
	policy, err := m.loadToolPolicy(ctx, name)
	if err != nil {
		return mcpserver.Server{}, false, err
	}
	srv.ToolPolicy = policy
	return srv, true, nil
}

func (m *MCPServerStore) Save(ctx context.Context, srv mcpserver.Server) error {
	if err := srv.Validate(); err != nil {
		return fmt.Errorf("sqlite: validate mcp server: %w", err)
	}
	var timeoutNS any
	if timeout, bounded := srv.HandshakeTimeout.Duration(); bounded {
		timeoutNS = int64(timeout)
	}
	return RunInTx(ctx, m.db, func(txCtx context.Context) error {
		if _, err := conn(txCtx, m.db).ExecContext(txCtx,
			`INSERT INTO mcp_servers
			   (name, transport, enabled, description, url, authorization, headers,
			    command, args, env, dir, timeout)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(name) DO UPDATE SET
			    transport = excluded.transport, enabled = excluded.enabled,
			    description = excluded.description, url = excluded.url,
			    authorization = excluded.authorization, headers = excluded.headers,
			    command = excluded.command, args = excluded.args, env = excluded.env,
			    dir = excluded.dir, timeout = excluded.timeout`,
			srv.Name.String(), string(srv.Transport), srv.Enabled, srv.Description, srv.URL, srv.Authorization,
			encodeStringMap(srv.Headers), srv.Command, encodeStrings(srv.Args),
			encodeStringMap(srv.Env), srv.Dir, timeoutNS,
		); err != nil {
			return fmt.Errorf("sqlite: save mcp server: %w", err)
		}
		if _, err := conn(txCtx, m.db).ExecContext(
			txCtx,
			`DELETE FROM mcp_server_tool_policies WHERE server_name = ?`,
			srv.Name.String(),
		); err != nil {
			return fmt.Errorf("sqlite: clear mcp server %q tool policy: %w", srv.Name, err)
		}
		for _, rule := range srv.ToolPolicy.Rules() {
			if _, err := conn(txCtx, m.db).ExecContext(
				txCtx,
				`INSERT INTO mcp_server_tool_policies (server_name, tool_name, decision) VALUES (?, ?, ?)`,
				srv.Name.String(),
				rule.Tool.String(),
				string(rule.Decision),
			); err != nil {
				return fmt.Errorf("sqlite: save mcp server %q tool %q policy: %w", srv.Name, rule.Tool, err)
			}
		}
		return nil
	})
}

func (m *MCPServerStore) Remove(ctx context.Context, name mcpserver.ServerName) error {
	if _, err := conn(ctx, m.db).ExecContext(ctx, `DELETE FROM mcp_servers WHERE name = ?`, name.String()); err != nil {
		return fmt.Errorf("sqlite: remove mcp server: %w", err)
	}
	return nil
}

// LoadOAuthSession returns the opaque MCP-owned credential payload only when
// both the server name and normalized origin match. A stale credential can
// never be restored for a different origin.
func (m *MCPServerStore) LoadOAuthSession(ctx context.Context, server mcpserver.ServerName, origin string) ([]byte, bool, error) {
	var payload []byte
	err := conn(ctx, m.db).QueryRowContext(ctx,
		`SELECT payload FROM mcp_oauth_sessions WHERE server_name = ? AND origin = ?`,
		server.String(), origin).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("sqlite: load mcp oauth session: %w", err)
	}
	return payload, true, nil
}

// SaveOAuthSession atomically replaces one server's origin-bound OAuth
// session. The foreign key rejects credentials for an unconfigured server.
func (m *MCPServerStore) SaveOAuthSession(ctx context.Context, server mcpserver.ServerName, origin string, payload []byte) error {
	if err := server.Validate(); err != nil {
		return fmt.Errorf("sqlite: mcp oauth session server: %w", err)
	}
	if origin == "" || len(payload) == 0 {
		return errors.New("sqlite: mcp oauth session requires origin and payload")
	}
	_, err := conn(ctx, m.db).ExecContext(ctx,
		`INSERT INTO mcp_oauth_sessions (server_name, origin, payload)
		 VALUES (?, ?, ?)
		 ON CONFLICT(server_name) DO UPDATE SET
		    origin = excluded.origin, payload = excluded.payload`,
		server.String(), origin, payload)
	if err != nil {
		return fmt.Errorf("sqlite: save mcp oauth session: %w", err)
	}
	return nil
}

// RemoveOAuthSession invalidates a server's persisted OAuth credentials. It is
// idempotent so a rejected token and a concurrent server removal converge.
func (m *MCPServerStore) RemoveOAuthSession(ctx context.Context, server mcpserver.ServerName) error {
	if _, err := conn(ctx, m.db).ExecContext(ctx,
		`DELETE FROM mcp_oauth_sessions WHERE server_name = ?`, server.String()); err != nil {
		return fmt.Errorf("sqlite: remove mcp oauth session: %w", err)
	}
	return nil
}

// scanMCPServer reads one row via the given Scan func (works for both
// *sql.Row and *sql.Rows), decoding the JSON list/map columns and the
// nanosecond timeout. Column order must match [mcpColumns].
func scanMCPServer(scan func(...any) error) (mcpserver.Server, error) {
	var (
		srv                                 mcpserver.Server
		name, transport, headers, args, env string
		timeoutNS                           sql.NullInt64
	)
	if err := scan(&name, &transport, &srv.Enabled, &srv.Description, &srv.URL,
		&srv.Authorization, &headers, &srv.Command, &args, &env, &srv.Dir, &timeoutNS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcpserver.Server{}, err
		}
		return mcpserver.Server{}, fmt.Errorf("sqlite: scan mcp server: %w", err)
	}
	parsedName, err := mcpserver.ParseServerName(name)
	if err != nil {
		return mcpserver.Server{}, fmt.Errorf("sqlite: decode MCP server identity: %w", err)
	}
	srv.Name = parsedName
	srv.Transport = mcpserver.Transport(transport)
	if srv.Headers, err = decodeStringMap(headers); err != nil {
		return mcpserver.Server{}, mcpJSONFieldError(srv.Name, "headers", err)
	}
	if srv.Args, err = decodeStrings(args); err != nil {
		return mcpserver.Server{}, mcpJSONFieldError(srv.Name, "args", err)
	}
	if srv.Env, err = decodeStringMap(env); err != nil {
		return mcpserver.Server{}, mcpJSONFieldError(srv.Name, "env", err)
	}
	if timeoutNS.Valid {
		srv.HandshakeTimeout, err = mcpserver.NewHandshakeTimeout(time.Duration(timeoutNS.Int64))
		if err != nil {
			return mcpserver.Server{}, fmt.Errorf("sqlite: decode MCP server %q handshake timeout: %w", srv.Name, err)
		}
	}
	if err := srv.Validate(); err != nil {
		return mcpserver.Server{}, fmt.Errorf("sqlite: validate mcp server %q: %w", srv.Name, err)
	}
	return srv, nil
}

func (m *MCPServerStore) loadToolPolicy(ctx context.Context, server mcpserver.ServerName) (mcpserver.ServerToolPolicy, error) {
	rows, err := conn(ctx, m.db).QueryContext(
		ctx,
		`SELECT tool_name, decision FROM mcp_server_tool_policies WHERE server_name = ? ORDER BY tool_name`,
		server.String(),
	)
	if err != nil {
		return mcpserver.ServerToolPolicy{}, fmt.Errorf("sqlite: list mcp server %q tool policy: %w", server, err)
	}
	defer func() { _ = rows.Close() }()
	var rules []mcpserver.ToolPolicyRule
	for rows.Next() {
		rule, scanErr := scanMCPToolPolicyRule(rows, server)
		if scanErr != nil {
			return mcpserver.ServerToolPolicy{}, scanErr
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return mcpserver.ServerToolPolicy{}, fmt.Errorf("sqlite: list mcp server %q tool policy: %w", server, err)
	}
	policy, err := mcpserver.RestoreServerToolPolicy(rules)
	if err != nil {
		return mcpserver.ServerToolPolicy{}, fmt.Errorf("sqlite: restore mcp server %q tool policy: %w", server, err)
	}
	return policy, nil
}

func (m *MCPServerStore) loadToolPolicies(ctx context.Context, servers []mcpserver.Server) error {
	if len(servers) == 0 {
		return nil
	}
	rows, err := conn(ctx, m.db).QueryContext(
		ctx,
		`SELECT server_name, tool_name, decision FROM mcp_server_tool_policies ORDER BY server_name, tool_name`,
	)
	if err != nil {
		return fmt.Errorf("sqlite: list mcp server tool policies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	indexes := make(map[mcpserver.ServerName]int, len(servers))
	rules := make(map[mcpserver.ServerName][]mcpserver.ToolPolicyRule, len(servers))
	for i := range servers {
		indexes[servers[i].Name] = i
	}
	for rows.Next() {
		var rawServer, rawTool string
		var decision mcpserver.ToolPolicyDecision
		if err := rows.Scan(&rawServer, &rawTool, &decision); err != nil {
			return fmt.Errorf("sqlite: scan mcp server tool policy: %w", err)
		}
		server, err := mcpserver.ParseServerName(rawServer)
		if err != nil {
			return fmt.Errorf("sqlite: decode MCP server tool-policy owner: %w", err)
		}
		tool, err := mcpserver.ParseRemoteToolName(rawTool)
		if err != nil {
			return fmt.Errorf("sqlite: decode MCP server %q remote tool identity: %w", server, err)
		}
		if _, ok := indexes[server]; !ok {
			return fmt.Errorf("sqlite: MCP tool policy references unloaded server %q", server)
		}
		rules[server] = append(rules[server], mcpserver.ToolPolicyRule{Tool: tool, Decision: decision})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: list mcp server tool policies: %w", err)
	}
	for server, index := range indexes {
		policy, err := mcpserver.RestoreServerToolPolicy(rules[server])
		if err != nil {
			return fmt.Errorf("sqlite: restore mcp server %q tool policy: %w", server, err)
		}
		servers[index].ToolPolicy = policy
	}
	return nil
}

func scanMCPToolPolicyRule(row scanRow, server mcpserver.ServerName) (mcpserver.ToolPolicyRule, error) {
	var rawTool string
	var decision mcpserver.ToolPolicyDecision
	if err := row.Scan(&rawTool, &decision); err != nil {
		return mcpserver.ToolPolicyRule{}, fmt.Errorf("sqlite: scan mcp server %q tool policy: %w", server, err)
	}
	tool, err := mcpserver.ParseRemoteToolName(rawTool)
	if err != nil {
		return mcpserver.ToolPolicyRule{}, fmt.Errorf("sqlite: decode MCP server %q remote tool identity: %w", server, err)
	}
	return mcpserver.ToolPolicyRule{Tool: tool, Decision: decision}, nil
}

func mcpJSONFieldError(server mcpserver.ServerName, field string, err error) error {
	return fmt.Errorf("sqlite: scan mcp server %q: decode %s: %w", server, field, err)
}

// encodeStrings JSON-encodes a string slice for a TEXT column; a nil/empty
// slice stores "" (decoded back to nil) so empty and absent read identically.
func encodeStrings(v []string) string {
	if len(v) == 0 {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// decodeStrings reverses encodeStrings. A blank column is the canonical empty
// value; malformed persisted JSON is a storage error, never an empty list.
func decodeStrings(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	var v []string
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, err
	}
	return v, nil
}

// encodeStringMap JSON-encodes a string map for a TEXT column; a nil/empty map
// stores "" (decoded back to nil) so empty and absent read identically.
func encodeStringMap(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// decodeStringMap reverses encodeStringMap. A blank column is the canonical
// empty value; malformed persisted JSON is a storage error, never an empty map.
func decodeStringMap(s string) (map[string]string, error) {
	if s == "" {
		return nil, nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	return m, nil
}
