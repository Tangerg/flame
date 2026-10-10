package bootstrap

import (
	"context"
	"fmt"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// mcpDefinitions supplies one desired configuration snapshot for startup.
// Live availability is resolved by each admitted connection attempt.
type mcpDefinitions interface {
	Definitions(ctx context.Context) ([]mcpserver.Server, error)
}

type mcpExposure interface {
	ListExposure(ctx context.Context) ([]tool.Ref, error)
}

// mcpEnvironment is the boot-time MCP material: the application-owned live
// exposure state and the enabled durable server definitions to connect.
type mcpEnvironment struct {
	exposure *mcpapp.ExposureState
	servers  []mcpserver.Server
}

func buildMCPEnvironment(ctx context.Context, registry mcpDefinitions, exposure mcpExposure) (mcpEnvironment, error) {
	servers, err := registry.Definitions(ctx)
	if err != nil {
		return mcpEnvironment{}, fmt.Errorf("bootstrap: load mcp registry: %w", err)
	}
	disabled, err := exposure.ListExposure(ctx)
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
