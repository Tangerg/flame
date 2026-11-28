package contract

//go:generate sh -c "go run ../../contractgen | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/plugins/methods/internal/wire"
	"github.com/Tangerg/frame"
)

const Export = "project-methods"

type Methods struct {
	Board   *pluginmcp.Method[wire.ProjectInput, wire.Board]
	Read    *pluginmcp.Method[wire.ExecutionInput, wire.Execution]
	Save    *pluginmcp.Method[wire.SaveInput, wire.Version]
	Start   *pluginmcp.Method[wire.StartInput, wire.Execution]
	Approve *pluginmcp.Method[wire.ApproveInput, wire.Execution]
}

func New() (Methods, error) {
	board, e1 := method[wire.ProjectInput, wire.Board](wire.BoardMethod, frame.Query, "Read the current project method and recent executions.")
	read, e2 := method[wire.ExecutionInput, wire.Execution](wire.ReadMethod, frame.Query, "Read the original method execution and captured evidence.")
	save, e3 := method[wire.SaveInput, wire.Version](wire.SaveMethod, frame.Command, "Save an immutable project method version.")
	start, e4 := method[wire.StartInput, wire.Execution](wire.StartMethod, frame.Command, "Admit one execution of a captured method on a managed worktree.")
	approve, e5 := method[wire.ApproveInput, wire.Execution](wire.ApproveMethod, frame.Command, "Approve the original method report and retain the human decision.")
	return Methods{board, read, save, start, approve}, errors.Join(e1, e2, e3, e4, e5)
}

func method[P, R any](name string, kind frame.MethodKind, description string) (*pluginmcp.Method[P, R], error) {
	return pluginmcp.NewMethod[P, R](frame.MethodConfig{Name: name, Kind: kind, Target: extension.ScopeInstallation, Description: description})
}

func (m Methods) Capability() (*plugincontract.Capability, error) {
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.methods.v1", Composition: plugincontract.Collection, Scope: extension.ScopeInstallation},
		m.Board.Descriptor(), m.Read.Descriptor(), m.Save.Descriptor(), m.Start.Descriptor(), m.Approve.Descriptor())
}
