package delivery

import (
	"fmt"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func toolRefFromWire(ref protocol.ToolRef) (tool.Ref, error) {
	switch ref.Type {
	case protocol.ToolRefBuiltIn:
		if ref.Server != nil {
			return tool.Ref{}, fmt.Errorf("delivery: built-in Tool cannot have an MCP server")
		}
		return tool.BuiltIn(builtincontract.Name(ref.Name))
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
	return projectCapturedRef(ref), nil
}

func projectCapturedRef(ref tool.Ref) protocol.ToolRef {
	if name, ok := ref.BuiltIn(); ok {
		return protocol.ToolRef{Type: protocol.ToolRefBuiltIn, Name: string(name)}
	}
	server, remote, _ := ref.MCP()
	id := presentMCPServerID(server)
	return protocol.ToolRef{Type: protocol.ToolRefMCP, Name: remote.String(), Server: &id}
}

func recordedSource(ref tool.Ref) *protocol.RecordedToolSource {
	if ref == (tool.Ref{}) {
		return nil
	}
	return &protocol.RecordedToolSource{Ref: projectCapturedRef(ref), ModelName: ref.ModelName()}
}
