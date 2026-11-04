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
	var c Contracts
	var err error
	c.Capability, err = plugincontract.ParseCapability([]byte(contracts.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.Agents, err = pluginmcp.ImplementMethod[struct{}, wire.Agents](c.Capability, contracts.AgentsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Send, err = pluginmcp.ImplementMethod[wire.SendRequest, wire.Result](c.Capability, contracts.SendV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Status, err = pluginmcp.ImplementMethod[wire.TaskRequest, wire.Result](c.Capability, contracts.StatusV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Cancel, err = pluginmcp.ImplementMethod[wire.TaskRequest, wire.Result](c.Capability, contracts.CancelV1Method)
	return c, err
}
