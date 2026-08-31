package mcp

import (
	"context"
	"fmt"

	toolcontract "github.com/Tangerg/scope/core/tool"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	scopemcp "github.com/Tangerg/scope/mcp"
)

const rejectedRemoteToolPlaceholder = "invalid_remote_tool"

// sourceTools lists one MCP source's model-facing tools. Isolated per source so
// a single server's tools/list failure stays its own.
func sourceTools(ctx context.Context, server mcpserver.ServerName, session *sdkmcp.ClientSession) ([]toolcontract.Tool, error) {
	source := scopemcp.ToolSource{Name: server.String(), Session: session}
	var remoteNameErr error
	tools, discoverErr := scopemcp.DiscoverTools(ctx, []scopemcp.ToolSource{source}, scopemcp.ToolDiscoveryConfig{
		PublicName: func(_ string, toolName string) string {
			remoteName, err := mcpserver.ParseRemoteToolName(toolName)
			if err != nil {
				if remoteNameErr == nil {
					remoteNameErr = err
				}
				// Scope requires a provider-safe label before it returns wrappers.
				// The captured identity error wins below, so this label can never
				// enter either live catalog.
				return rejectedRemoteToolPlaceholder
			}
			return mcpserver.ToolName(server, remoteName)
		},
		ConcurrencyPolicy: scopemcp.AnnotatedReadOnlyConcurrencyPolicy,
	})
	if remoteNameErr != nil {
		return nil, fmt.Errorf("mcp: validate tool from server %q: %w", server, remoteNameErr)
	}
	if discoverErr != nil {
		return nil, discoverErr
	}
	if err := validateSourceToolMaterial(server, tools); err != nil {
		return nil, err
	}
	return tools, nil
}

func validateSourceToolMaterial(server mcpserver.ServerName, tools []toolcontract.Tool) error {
	if err := mcpserver.ValidateRemoteToolCount(len(tools)); err != nil {
		return fmt.Errorf("mcp: validate tools from server %q: %w", server, err)
	}
	for _, tool := range tools {
		ref, err := remoteToolRef(tool)
		if err != nil {
			return fmt.Errorf("mcp: validate tool from server %q: %w", server, err)
		}
		if ref.Server != server {
			return fmt.Errorf("mcp: tool source %q does not match server %q", ref.Server, server)
		}
		definition := tool.Definition()
		if err := mcpserver.ValidateRemoteToolMaterial(definition.Description, definition.InputSchema); err != nil {
			return fmt.Errorf("mcp: validate tool %q from server %q: %w", definition.Name, server, err)
		}
	}
	return nil
}

type remoteToolIdentity interface {
	MCPToolIdentity() (sourceName, remoteName string)
}

func remoteToolRef(tool toolcontract.Tool) (mcpserver.ToolRef, error) {
	identity, ok := tool.(remoteToolIdentity)
	if !ok {
		return mcpserver.ToolRef{}, fmt.Errorf("tool %q has no MCP identity", tool.Definition().Name)
	}
	server, remote := identity.MCPToolIdentity()
	serverName, err := mcpserver.ParseServerName(server)
	if err != nil {
		return mcpserver.ToolRef{}, err
	}
	remoteName, err := mcpserver.ParseRemoteToolName(remote)
	if err != nil {
		return mcpserver.ToolRef{}, err
	}
	return mcpserver.ToolRef{Server: serverName, Tool: remoteName}, nil
}

// inputSchema converts the SDK's open schema representation at the MCP
// boundary. Missing or malformed advertised schemas fail the catalog read
// instead of being silently presented as schema-less tools.
func inputSchema(schema any) (mcpserver.InputSchema, error) {
	parsed, err := mcpserver.NewInputSchema(schema)
	if err != nil {
		return mcpserver.InputSchema{}, err
	}
	return parsed, nil
}
