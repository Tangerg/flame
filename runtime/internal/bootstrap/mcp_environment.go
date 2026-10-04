package bootstrap

import (
	"context"
	"fmt"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// mcpServerList is the boot-time snapshot view of the MCP registry: building the
// exposure and dial descriptors come from the same initial registry view.
type mcpServerList interface {
	Catalog(ctx context.Context) ([]mcpapp.Source, error)
	ListExposure(ctx context.Context) ([]tool.Ref, error)
}

// mcpEnvironment is the boot-time MCP material: the application-owned live
// exposure state and the enabled durable server definitions to connect.
type mcpEnvironment struct {
	exposure *mcpapp.ExposureState
	servers  []mcpserver.Server
}

func buildMCPEnvironment(ctx context.Context, registry mcpServerList) (mcpEnvironment, error) {
	sources, err := registry.Catalog(ctx)
	if err != nil {
		return mcpEnvironment{}, fmt.Errorf("bootstrap: load mcp registry: %w", err)
	}
	servers := make([]mcpserver.Server, 0, len(sources))
	for _, source := range sources {
		servers = append(servers, source.Server)
	}
	disabled, err := registry.ListExposure(ctx)
	if err != nil {
		return mcpEnvironment{}, err
	}
	return mcpEnvironment{
		exposure: mcpapp.NewExposureState(servers, disabled),
		servers:  enabledMCPServers(servers),
	}, nil
}

func enabledMCPServers(servers []mcpserver.Server) []mcpserver.Server {
	var enabled []mcpserver.Server
	for _, server := range servers {
		if server.Enabled {
			enabled = append(enabled, server)
		}
	}
	return enabled
}
