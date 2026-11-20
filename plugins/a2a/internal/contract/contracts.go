package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/a2a/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/plugins/a2a/contracts"
	"github.com/Tangerg/flame/plugins/a2a/internal/wire"
)

type Contracts struct {
	Capability *plugincontract.Capability
	Agents     *pluginmcp.Method[struct{}, wire.Agents]
	Send       *pluginmcp.Method[wire.SendRequest, wire.Result]
	Status     *pluginmcp.Method[wire.TaskRequest, wire.Result]
	Cancel     *pluginmcp.Method[wire.TaskRequest, wire.Result]
}

func NewContracts() (Contracts, error) {
	capability, err := plugincontract.ParseCapability([]byte(contracts.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	methods := pluginmcp.NewMethodSet(capability)
	c := Contracts{
		Capability: capability,
		Agents:     pluginmcp.Implement[struct{}, wire.Agents](methods, contracts.AgentsV1Method),
		Send:       pluginmcp.Implement[wire.SendRequest, wire.Result](methods, contracts.SendV1Method),
		Status:     pluginmcp.Implement[wire.TaskRequest, wire.Result](methods, contracts.StatusV1Method),
		Cancel:     pluginmcp.Implement[wire.TaskRequest, wire.Result](methods, contracts.CancelV1Method),
	}
	if err := methods.Err(); err != nil {
		return Contracts{}, err
	}
	return c, nil
}
