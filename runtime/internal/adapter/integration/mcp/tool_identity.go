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

type CapturedSource struct {
	Config     ServerConfig
	RemoteName string
}

func (s CapturedSource) SourceConfig() ServerConfig { return s.Config.Clone() }

func Source(executable toolcontract.Tool) (CapturedSource, bool) {
	source, ok := toolcontract.As[interface{ MCPSource() (CapturedSource, bool) }](executable)
	if !ok {
		return CapturedSource{}, false
	}
	return source.MCPSource()
}

// Public Tool names are presentation labels and cannot identify policy.
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
