// Package mcp binds the external connector contract to Frame's MCP transport.
package mcp

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/frame/portable"
)

type Methods struct {
	Recover  *pluginmcp.Method[mcpconnector.Recovery, mcpconnector.Decision]
	List     *pluginmcp.Method[mcpconnector.Selector, mcpconnector.Directory]
	Change   *pluginmcp.Method[mcpconnector.Change, mcpconnector.Decision]
	Start    *pluginmcp.Method[mcpconnector.Start, mcpconnector.AttemptResult]
	Finish   *pluginmcp.Method[mcpconnector.Completion, mcpconnector.Decision]
	Get      *pluginmcp.Method[mcpconnector.Request, mcpconnector.AttemptResult]
	Contract *plugincontract.Capability
}

func method[P, R any](name, description string, kind portable.MethodKind) (*pluginmcp.Method[P, R], error) {
	return pluginmcp.NewMethod[P, R](portable.MethodConfig{Name: "flame.external-mcp/" + name + ".v1", Description: description, Kind: kind, Target: extension.ScopeInstallation})
}
func NewMethods() (Methods, error) {
	var m Methods
	var err error
	if m.Recover, err = method[mcpconnector.Recovery, mcpconnector.Decision]("recover", "Cancel uncompleted attempts admitted by a previous Runtime incarnation.", portable.Command); err != nil {
		return m, err
	}
	if m.List, err = method[mcpconnector.Selector, mcpconnector.Directory]("list", "List external servers and their opaque host launch grants.", portable.Query); err != nil {
		return m, err
	}
	if m.Change, err = method[mcpconnector.Change, mcpconnector.Decision]("change", "Compare and change one external directory entry.", portable.Command); err != nil {
		return m, err
	}
	if m.Start, err = method[mcpconnector.Start, mcpconnector.AttemptResult]("authorize", "Begin an authorization attempt against the current enabled entry.", portable.Command); err != nil {
		return m, err
	}
	if m.Finish, err = method[mcpconnector.Completion, mcpconnector.Decision]("settle", "Record the original authorization attempt's terminal result.", portable.Command); err != nil {
		return m, err
	}
	if m.Get, err = method[mcpconnector.Request, mcpconnector.AttemptResult]("attempt", "Read a retained external authorization attempt.", portable.Query); err != nil {
		return m, err
	}
	m.Contract, err = plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: mcpconnector.Reference, Composition: plugincontract.Selection, Scope: extension.ScopeInstallation}, m.Recover.Descriptor(), m.List.Descriptor(), m.Change.Descriptor(), m.Start.Descriptor(), m.Finish.Descriptor(), m.Get.Descriptor())
	return m, err
}
