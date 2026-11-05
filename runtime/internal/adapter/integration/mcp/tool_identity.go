package mcp

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type remoteToolIdentity interface {
	MCPToolIdentity() (sourceName, remoteName string)
}

// CapturedSource is a detached identity projection of the connection that
// admitted a tool. It grants no connection or dispatch authority.
type CapturedSource struct {
	Config     ServerConfig
	RemoteName string
}

func (s CapturedSource) SourceConfig() ServerConfig { return s.Config.Clone() }

// Source reads the explicit declaration preserved by the product decorators.
func Source(executable toolcontract.Tool) (CapturedSource, bool) {
	source, ok := executable.(interface{ MCPSource() (CapturedSource, bool) })
	if !ok {
		return CapturedSource{}, false
	}
	return source.MCPSource()
}

// IdentifyTool combines the server identity of the connection that admitted a
// tool with the remote name Scope reports on its admitted executable. Public
// Tool names are presentation labels and cannot identify policy. Absence is
// valid for non-MCP Tools; malformed declarations remain errors.
func IdentifyTool(executable toolcontract.Tool) (tool.Ref, bool, error) {
	source, found := Source(executable)
	if !found {
		return tool.Ref{}, false, nil
	}

	remoteName, err := mcpserver.ParseRemoteToolName(source.RemoteName)
	if err != nil {
		return tool.Ref{}, true, fmt.Errorf("mcp: remote tool identity: %w", err)
	}
	ref, err := tool.MCP(source.SourceConfig().ID(), remoteName)
	return ref, true, err
}
