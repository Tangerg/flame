package mcp

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
	toolcontract "github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

const rejectedRemoteToolPlaceholder = "invalid_remote_tool"

// sourceTools lists one MCP source's model-facing tools. Isolated per source so
// a single server's tools/list failure stays its own. owner is the connection
// registry that will hold session; nil for a throwaway probe, whose
// executables are never current.
func sourceTools(ctx context.Context, owner *Connections, descriptor ServerConfig, session *sdkmcp.ClientSession) ([]Executable, error) {
	server := descriptor.ID()
	source := scopemcp.ToolSource{Name: descriptor.Name.String(), Session: session}
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
	config := descriptor.Clone()
	config.OAuthHandler = nil
	result := make([]Executable, len(tools))
	for index, executable := range tools {
		result[index] = Executable{Tool: executable, config: config, owner: owner, session: session}
	}
	if err := validateSourceToolMaterial(server, result); err != nil {
		return nil, err
	}
	return result, nil
}

func validateSourceToolMaterial(server mcpserver.ID, tools []Executable) error {
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
		_, remote, _ := ref.MCP()
		binding, err := toolcontract.Bind(tool)
		if err != nil {
			return fmt.Errorf("mcp: admit tool %q from server %q: %w", remote, server, err)
		}
		definition := binding.Contract().Definition()
		if err := mcpserver.ValidateRemoteToolDescription(definition.Description); err != nil {
			return fmt.Errorf("mcp: validate tool %q from server %q: %w", definition.Name, server, err)
		}
	}
	return nil
}

// Executable retains the configuration and the session that admitted it. It
// cannot advance registry configuration or the connection's live OAuth
// credentials.
type Executable struct {
	toolcontract.Tool
	config  ServerConfig
	owner   *Connections
	session *sdkmcp.ClientSession
}

// Current reports whether the session that admitted this executable is still
// its source's live connection. A frozen Run manifest outlives reconnects and
// refusals; once its session is replaced or withdrawn, dispatching through it
// would reach a closed transport instead of the current source.
func (t Executable) Current() bool {
	return t.owner != nil && t.owner.current(t.config.ID(), t.session)
}

func (t Executable) Unwrap() toolcontract.Tool             { return t.Tool }
func (t Executable) SourceFingerprint() fingerprint.Digest { return t.config.SourceFingerprint }
func (t Executable) SourceConfig() ServerConfig            { return t.config.Clone() }
