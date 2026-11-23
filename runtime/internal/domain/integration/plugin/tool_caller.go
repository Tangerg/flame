package plugin

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
)

type ToolCaller struct{ value ToolCallerSnapshot }

type ToolCallerSnapshot struct {
	Invocation framecall.Invocation
	Tool       tool.Ref
	Release    framevalue.Digest
	Authority  framevalue.Digest
}

func NewToolCaller(value ToolCallerSnapshot) (ToolCaller, error) {
	if err := extension.ValidateInvocation(value.Invocation); err != nil {
		return ToolCaller{}, err
	}
	if err := value.Tool.Validate(); err != nil {
		return ToolCaller{}, err
	}
	if err := value.Release.Validate(); err != nil {
		return ToolCaller{}, err
	}
	if err := value.Authority.Validate(); err != nil {
		return ToolCaller{}, err
	}
	server, _, mcp := value.Tool.MCP()
	installation, installed := server.Origin().Installation()
	if !mcp || !installed || value.Invocation.Origin != extension.Model || value.Invocation.Parent == "" || value.Invocation.Target != (framecall.Target{Scope: extension.ScopeInstallation, Reference: installation.String()}) {
		return ToolCaller{}, errors.New("plugin: model Tool caller must name its admitted installation and execution member")
	}
	return ToolCaller{value: value}, nil
}

func (t ToolCaller) Snapshot() ToolCallerSnapshot { return t.value }
func (t ToolCaller) Validate() error              { _, err := NewToolCaller(t.value); return err }

func (t ToolCaller) Matches(invocation framecall.Invocation, installation identity.InstallationID, release, authority framevalue.Digest, server mcpserver.ServerName) bool {
	source, _, _ := t.value.Tool.MCP()
	owner, _ := source.Origin().Installation()
	return t.value.Invocation == invocation && owner == installation && t.value.Release == release && t.value.Authority == authority && source.Name() == server
}
