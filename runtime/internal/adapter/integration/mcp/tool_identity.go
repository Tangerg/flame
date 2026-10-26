package mcp

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type remoteToolIdentity interface {
	MCPToolIdentity() (sourceName, remoteName string)
}

type realizedSource interface{ SourceConfig() ServerConfig }

// IdentifyTool combines the server identity of the connection that admitted a
// tool with the remote name Scope reports through its capability chain. Public
// Tool names are presentation labels and cannot identify policy. Absence is
// valid for non-MCP Tools; malformed declarations remain errors.
func IdentifyTool(executable toolcontract.Tool) (tool.Ref, bool, error) {
	identity, found, err := toolcontract.Capability[remoteToolIdentity](executable)
	if err != nil || !found {
		return tool.Ref{}, false, err
	}
	source, found, err := toolcontract.Capability[realizedSource](executable)
	if err != nil {
		return tool.Ref{}, true, err
	}
	if !found {
		return tool.Ref{}, true, errors.New("mcp: tool has no realized connection")
	}
	_, remote := identity.MCPToolIdentity()
	remoteName, err := mcpserver.ParseRemoteToolName(remote)
	if err != nil {
		return tool.Ref{}, true, fmt.Errorf("mcp: remote tool identity: %w", err)
	}
	ref, err := tool.MCP(source.SourceConfig().ID(), remoteName)
	return ref, true, err
}
