package sqlite

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func (m *MCPServerStore) ListExposure(ctx context.Context) ([]tool.Ref, error) {
	rows, err := conn(ctx, m.db).QueryContext(ctx, `SELECT server_name, tool_name FROM mcp_tool_exposure ORDER BY server_name, tool_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []tool.Ref
	for rows.Next() {
		var source, name string
		if err := rows.Scan(&source, &name); err != nil {
			return nil, err
		}
		server, err := mcpserver.ParseServerName(source)
		if err != nil {
			return nil, err
		}
		remote, err := mcpserver.ParseRemoteToolName(name)
		if err != nil {
			return nil, err
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
	if ref.Kind() != tool.MCPKind {
		return fmt.Errorf("sqlite: exposure requires an MCP reference")
	}
	statement := `DELETE FROM mcp_tool_exposure WHERE server_name = ? AND tool_name = ?`
	if disabled {
		statement = `INSERT INTO mcp_tool_exposure (server_name, tool_name) VALUES (?, ?) ON CONFLICT DO NOTHING`
	}
	_, err := conn(ctx, m.db).ExecContext(ctx, statement, ref.Server().String(), ref.Remote().String())
	return err
}
