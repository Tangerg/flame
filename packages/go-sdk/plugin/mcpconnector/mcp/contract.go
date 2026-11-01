// Package mcp binds the external connector contract to Qimu's MCP transport.
package mcp

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/Tangerg/qimu/portable"
)

type Methods struct {
	Recover  *qimumcp.Method[mcpconnector.Recovery, mcpconnector.Decision]
	List     *qimumcp.Method[mcpconnector.Selector, mcpconnector.Directory]
	Change   *qimumcp.Method[mcpconnector.Change, mcpconnector.Decision]
	Start    *qimumcp.Method[mcpconnector.Start, mcpconnector.AttemptResult]
	Finish   *qimumcp.Method[mcpconnector.Completion, mcpconnector.Decision]
	Get      *qimumcp.Method[mcpconnector.Request, mcpconnector.AttemptResult]
	Contract *portable.Capability
}

func method[P, R any](name, description string, kind portable.MethodKind) (*qimumcp.Method[P, R], error) {
	return qimumcp.NewMethod[P, R](qimumcp.MethodConfig{Name: "flame.external-mcp/" + name + ".v1", Description: description, Kind: kind, Target: extension.ScopeInstallation})
}
func NewMethods() (Methods, error) {
	var m Methods
	var err error
	if m.Recover, err = method[mcpconnector.Recovery, mcpconnector.Decision]("recover", "Cancel uncompleted attempts admitted by a previous Runtime incarnation.", portable.Mutation); err != nil {
		return m, err
	}
	if m.List, err = method[mcpconnector.Selector, mcpconnector.Directory]("list", "List external servers and their opaque host launch grants.", portable.Query); err != nil {
		return m, err
	}
	if m.Change, err = method[mcpconnector.Change, mcpconnector.Decision]("change", "Compare and change one external directory entry.", portable.Mutation); err != nil {
		return m, err
	}
	if m.Start, err = method[mcpconnector.Start, mcpconnector.AttemptResult]("authorize", "Begin an authorization attempt against the current enabled entry.", portable.Mutation); err != nil {
		return m, err
	}
	if m.Finish, err = method[mcpconnector.Completion, mcpconnector.Decision]("settle", "Record the original authorization attempt's terminal result.", portable.Mutation); err != nil {
		return m, err
	}
	if m.Get, err = method[mcpconnector.Request, mcpconnector.AttemptResult]("attempt", "Read a retained external authorization attempt.", portable.Query); err != nil {
		return m, err
	}
	m.Contract, err = portable.NewCapability(portable.CapabilityConfig{Reference: mcpconnector.Reference, Composition: portable.Selection, Scope: extension.ScopeInstallation}, m.Recover.Descriptor(), m.List.Descriptor(), m.Change.Descriptor(), m.Start.Descriptor(), m.Finish.Descriptor(), m.Get.Descriptor())
	return m, err
}
