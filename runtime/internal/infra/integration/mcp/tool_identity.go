package mcp

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type remoteToolIdentity interface {
	MCPToolIdentity() (sourceName, remoteName string)
}

// IdentifyTool translates Scope's original MCP identity through its capability
// chain. Public Tool names are presentation labels and cannot identify policy.
// Absence is valid for non-MCP Tools; malformed declarations remain errors.
func IdentifyTool(executable toolcontract.Tool) (mcpserver.ToolRef, bool, error) {
	identity, found, err := toolcontract.Capability[remoteToolIdentity](executable)
	if err != nil || !found {
		return mcpserver.ToolRef{}, false, err
	}
	server, remote := identity.MCPToolIdentity()
	serverName, err := mcpserver.ParseServerName(server)
	if err != nil {
		return mcpserver.ToolRef{}, true, fmt.Errorf("mcp: tool source identity: %w", err)
	}
	remoteName, err := mcpserver.ParseRemoteToolName(remote)
	if err != nil {
		return mcpserver.ToolRef{}, true, fmt.Errorf("mcp: remote tool identity: %w", err)
	}
	return mcpserver.ToolRef{Server: serverName, Tool: remoteName}, true, nil
}
