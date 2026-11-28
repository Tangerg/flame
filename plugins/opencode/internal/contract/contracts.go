package contract

//go:generate sh -c "go run ../../contractgen | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/plugins/opencode/internal/wire"
	"github.com/Tangerg/frame"
)

const Export = "native-sessions"

type Methods struct {
	Agents     *pluginmcp.Method[wire.AgentsRequest, wire.AgentsResponse]
	List       *pluginmcp.Method[wire.ListRequest, wire.Sessions]
	Create     *pluginmcp.Method[wire.CreateRequest, wire.Session]
	Open       *pluginmcp.Method[wire.OpenRequest, wire.Session]
	Get        *pluginmcp.Method[wire.SessionRequest, wire.Session]
	Prompt     *pluginmcp.Method[wire.PromptRequest, wire.Session]
	Cancel     *pluginmcp.Method[wire.TurnRequest, wire.Session]
	Permission *pluginmcp.Method[wire.AnswerRequest, wire.Session]
	Fork       *pluginmcp.Method[wire.SessionRequest, wire.Session]
}

func New() (Methods, error) {
	list, e1 := method[wire.ListRequest, wire.Sessions](wire.List, frame.Query, "List native sessions in the canonical workspace.")
	open, e2 := method[wire.OpenRequest, wire.Session](wire.Open, frame.Command, "Restore the original native session through an advertised ACP operation.")
	get, e3 := method[wire.SessionRequest, wire.Session](wire.Get, frame.Query, "Read the captured session, progress, permissions and turn settlement.")
	prompt, e4 := method[wire.PromptRequest, wire.Session](wire.Prompt, frame.Command, "Accept one prompt for the bound native session.")
	cancel, e5 := method[wire.TurnRequest, wire.Session](wire.Cancel, frame.Command, "Request cancellation of the original prompt and await its stop reason.")
	permission, e6 := method[wire.AnswerRequest, wire.Session](wire.Permission, frame.Command, "Select an option from the original pending permission.")
	fork, e7 := method[wire.SessionRequest, wire.Session](wire.Fork, frame.Command, "Fork the native session when OpenCode advertises the capability.")
	agents, e8 := method[wire.AgentsRequest, wire.AgentsResponse](wire.Agents, frame.Query, "List the approved ACP agent profiles on the Runtime machine.")
	create, e9 := method[wire.CreateRequest, wire.Session](wire.Create, frame.Command, "Create one native ACP session and retain its original acknowledgement.")
	return Methods{agents, list, create, open, get, prompt, cancel, permission, fork}, errors.Join(e1, e2, e3, e4, e5, e6, e7, e8, e9)
}

func method[P, R any](name string, kind frame.MethodKind, description string) (*pluginmcp.Method[P, R], error) {
	return pluginmcp.NewMethod[P, R](frame.MethodConfig{Name: name, Kind: kind, Target: extension.ScopeInstallation, Description: description})
}

func (m Methods) Capability() (*plugincontract.Capability, error) {
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{
		Reference: "flame.opencode.v1", Composition: plugincontract.Collection, Scope: extension.ScopeInstallation,
	},
		m.Agents.Descriptor(), m.List.Descriptor(), m.Create.Descriptor(), m.Open.Descriptor(), m.Get.Descriptor(), m.Prompt.Descriptor(),
		m.Cancel.Descriptor(), m.Permission.Descriptor(), m.Fork.Descriptor(),
	)
}
