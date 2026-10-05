package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

// mcpSourceIDQuery resolves one server identity to its mcp_sources row. It
// takes the three values from [mcpSourceArgs] and yields NULL when the server
// does not exist, which every referencing column refuses.
const mcpSourceIDQuery = `(SELECT id FROM mcp_sources WHERE origin = ? AND installation_id IS ? AND name = ?)`

func mcpSourceArgs(server mcpserver.ID) []any {
	var installation any
	if id, found := server.Origin().Installation(); found {
		installation = id.String()
	}
	return []any{string(server.Origin().Kind()), installation, server.Name().String()}
}

func noMCPSource() []any { return []any{nil, nil, nil} }

// storedMCPSource scans the identity columns of a possibly absent source row.
type storedMCPSource struct {
	origin, installation, name sql.NullString
}

func (s storedMCPSource) id() (mcpserver.ID, error) {
	if !s.origin.Valid || !s.name.Valid {
		return mcpserver.ID{}, fmt.Errorf("sqlite: MCP source is missing")
	}
	var origin mcpserver.Origin
	switch mcpserver.OriginKind(s.origin.String) {
	case mcpserver.OriginUser:
		if s.installation.Valid {
			return mcpserver.ID{}, fmt.Errorf("sqlite: user MCP source names an installation")
		}
		origin = mcpserver.UserOrigin()
	case mcpserver.OriginInstallation:
		installation, err := resourceid.ParseInstallation(s.installation.String)
		if err != nil {
			return mcpserver.ID{}, fmt.Errorf("sqlite: decode MCP source installation: %w", err)
		}
		if origin, err = mcpserver.InstallationOrigin(installation); err != nil {
			return mcpserver.ID{}, err
		}
	default:
		return mcpserver.ID{}, fmt.Errorf("sqlite: unknown MCP source origin %q", s.origin.String)
	}
	name, err := mcpserver.ParseServerName(s.name.String)
	if err != nil {
		return mcpserver.ID{}, fmt.Errorf("sqlite: decode MCP source name: %w", err)
	}
	return mcpserver.NewID(origin, name)
}
