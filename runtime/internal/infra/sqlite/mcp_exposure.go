package sqlite

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func (m *MCPServerStore) ListExposure(ctx context.Context) ([]tool.Ref, error) {
	rows, err := conn(ctx, m.db).QueryContext(ctx,
		`SELECT s.origin, s.installation_id, s.name, e.tool_name
		   FROM mcp_tool_exposure e JOIN mcp_sources s ON s.id = e.source_id
		  ORDER BY s.origin <> 'user', s.installation_id, s.name, e.tool_name`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list mcp tool exposure: %w", err)
	}
	defer rows.Close()
	var out []tool.Ref
	for rows.Next() {
		var source storedMCPSource
		var name string
		if err := rows.Scan(&source.origin, &source.installation, &source.name, &name); err != nil {
			return nil, fmt.Errorf("sqlite: scan mcp tool exposure: %w", err)
		}
		server, err := source.id()
		if err != nil {
			return nil, err
		}
		remote, err := mcpserver.ParseRemoteToolName(name)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decode mcp tool exposure: %w", err)
		}
		ref, err := tool.MCP(server, remote)
		if err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

func (m *MCPServerStore) SetToolExposure(ctx context.Context, ref tool.Ref, disabled bool) error {
	server, remote, ok := ref.MCP()
	if !ok {
		return fmt.Errorf("sqlite: exposure requires an MCP reference")
	}
	statement := `DELETE FROM mcp_tool_exposure WHERE source_id = ` + mcpSourceIDQuery + ` AND tool_name = ?`
	if disabled {
		statement = `INSERT INTO mcp_tool_exposure (source_id, tool_name) VALUES (` + mcpSourceIDQuery + `, ?) ON CONFLICT DO NOTHING`
	}
	args := append(mcpSourceArgs(server), remote.String())
	if _, err := conn(ctx, m.db).ExecContext(ctx, statement, args...); err != nil {
		return fmt.Errorf("sqlite: set mcp tool exposure: %w", err)
	}
	return nil
}
