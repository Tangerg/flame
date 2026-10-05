package delivery

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/protocol"
)

func toolRefFromWire(ref protocol.ToolRef) (tool.Ref, error) {
	switch ref.Type {
	case protocol.ToolRefBuiltIn:
		return tool.BuiltIn(tool.BuiltInName(ref.Name))
	case protocol.ToolRefA2A:
		return tool.A2A(ref.Endpoint)
	case protocol.ToolRefMCP:
		if ref.Server == nil {
			return tool.Ref{}, fmt.Errorf("delivery: MCP tool source requires a server")
		}
		server, err := mcpServerIDFromWire(*ref.Server)
		if err != nil {
			return tool.Ref{}, err
		}
		name, err := mcpserver.ParseRemoteToolName(ref.Name)
		if err != nil {
			return tool.Ref{}, err
		}
		return tool.MCP(server, name)
	default:
		return tool.Ref{}, fmt.Errorf("delivery: invalid tool source %q", ref.Type)
	}
}

func presentToolRef(ref tool.Ref) (protocol.ToolRef, error) {
	if err := ref.Validate(); err != nil {
		return protocol.ToolRef{}, fmt.Errorf("delivery: project tool reference: %w", err)
	}
	switch ref.Kind() {
	case tool.BuiltInKind:
		name, _ := ref.BuiltIn()
		return protocol.ToolRef{Type: protocol.ToolRefBuiltIn, Name: string(name)}, nil
	case tool.A2AKind:
		endpoint, _ := ref.A2A()
		return protocol.ToolRef{Type: protocol.ToolRefA2A, Endpoint: endpoint}, nil
	case tool.MCPKind:
		server, remote, _ := ref.MCP()
		id := presentMCPServerID(server)
		return protocol.ToolRef{Type: protocol.ToolRefMCP, Name: remote.String(), Server: &id}, nil
	default:
		return protocol.ToolRef{}, fmt.Errorf("delivery: unsupported tool source %q", ref.Kind())
	}
}
