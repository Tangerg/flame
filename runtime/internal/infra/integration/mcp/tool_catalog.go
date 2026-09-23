package mcp

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Admission covers the prospective live union because distinct MCP identities
// can collapse to the same provider-safe name. Reconnect excludes the session
// being replaced; the caller serializes access to servers.
func validateToolCatalog(servers []*server, replacing *server, candidateServer mcpserver.ServerName, candidate []toolcontract.Tool) error {
	var combined []toolcontract.Tool
	for _, current := range servers {
		if current != replacing && current.session != nil {
			combined = append(combined, current.tools...)
		}
	}
	combined = append(combined, candidate...)
	if _, err := toolcontract.NewRegistry(combined...); err != nil {
		return fmt.Errorf("mcp: admit catalog with server %q: %w", candidateServer, err)
	}
	return nil
}
