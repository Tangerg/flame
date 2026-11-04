package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/sandbox/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/plugins/sandbox/contracts"
	sandbox "github.com/Tangerg/flame/plugins/sandbox/internal/domain"
	"github.com/Tangerg/flame/plugins/sandbox/internal/wire"
)

type Contracts struct {
	Capability *plugincontract.Capability
	Execute    *pluginmcp.Method[wire.ExecuteInput, sandbox.Result]
	Limits     *pluginmcp.Method[struct{}, wire.Limits]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = plugincontract.ParseCapability([]byte(contracts.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.Execute, err = pluginmcp.ImplementMethod[wire.ExecuteInput, sandbox.Result](c.Capability, contracts.ExecuteV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Limits, err = pluginmcp.ImplementMethod[struct{}, wire.Limits](c.Capability, contracts.LimitsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
