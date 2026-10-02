package mcp

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
	toolcontract "github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

const rejectedRemoteToolPlaceholder = "invalid_remote_tool"

// sourceTools lists one MCP source's model-facing tools. Isolated per source so
// a single server's tools/list failure stays its own.
func sourceTools(ctx context.Context, descriptor ServerConfig, session *sdkmcp.ClientSession) ([]toolcontract.Tool, error) {
	server := descriptor.Name
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
			ref, _ := domaintool.MCP(server, remoteName)
			return ref.ModelName()
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
	for index, executable := range tools {
		tools[index] = sourceTool{Tool: executable, fingerprint: descriptor.SourceFingerprint}
	}
	return tools, nil
}

func validateSourceToolMaterial(server mcpserver.ServerName, tools []toolcontract.Tool) error {
	if err := mcpserver.ValidateRemoteToolCount(len(tools)); err != nil {
		return fmt.Errorf("mcp: validate tools from server %q: %w", server, err)
	}
	for _, tool := range tools {
		ref, found, err := IdentifyTool(tool)
		if err != nil {
			return fmt.Errorf("mcp: validate tool from server %q: %w", server, err)
		}
		if !found {
			return fmt.Errorf("mcp: tool from server %q has no MCP identity", server)
		}
		if ref.Server() != server {
			return fmt.Errorf("mcp: tool source %q does not match server %q", ref.Server(), server)
		}
		binding, err := toolcontract.Bind(tool)
		if err != nil {
			return fmt.Errorf("mcp: admit tool %q from server %q: %w", ref.Remote(), server, err)
		}
		definition := binding.Contract().Definition()
		if err := mcpserver.ValidateRemoteToolDescription(definition.Description); err != nil {
			return fmt.Errorf("mcp: validate tool %q from server %q: %w", definition.Name, server, err)
		}
	}
	return nil
}

// sourceTool retains the authority of the connection that owns this executable.
type sourceTool struct {
	toolcontract.Tool
	fingerprint string
}

func (t sourceTool) Unwrap() toolcontract.Tool { return t.Tool }
func (t sourceTool) SourceFingerprint() string { return t.fingerprint }
