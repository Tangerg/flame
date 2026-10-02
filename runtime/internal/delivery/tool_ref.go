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
		return tool.BuiltIn(ref.Name)
	case protocol.ToolRefA2A:
		return tool.A2A(ref.Endpoint)
	case protocol.ToolRefMCP:
		server, err := mcpserver.ParseServerName(ref.Server)
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
func presentToolRef(ref tool.Ref) protocol.ToolRef {
	switch ref.Kind() {
	case tool.BuiltInKind:
		return protocol.ToolRef{Type: protocol.ToolRefBuiltIn, Name: ref.Name()}
	case tool.A2AKind:
		return protocol.ToolRef{Type: protocol.ToolRefA2A, Endpoint: ref.Name()}
	case tool.MCPKind:
		return protocol.ToolRef{Type: protocol.ToolRefMCP, Name: ref.Remote().String(), Server: ref.Server().String()}
	default:
		panic("delivery: invalid admitted tool reference")
	}
}
