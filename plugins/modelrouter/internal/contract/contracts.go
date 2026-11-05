package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/modelrouter/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --model-contract selection > ../../package/io.github.tangerg.flame/contracts/selection.generated.json"

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/plugins/modelrouter/contracts"
	routing "github.com/Tangerg/flame/plugins/modelrouter/internal/domain"
	"github.com/Tangerg/flame/plugins/modelrouter/internal/wire"
)

type Contracts struct {
	Capability *plugincontract.Capability
	Get        *pluginmcp.Method[struct{}, routing.Snapshot]
	Replace    *pluginmcp.Method[wire.ReplaceInput, routing.Snapshot]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = plugincontract.ParseCapability([]byte(contracts.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.Get, err = pluginmcp.ImplementMethod[struct{}, routing.Snapshot](c.Capability, contracts.GetPolicyV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Replace, err = pluginmcp.ImplementMethod[wire.ReplaceInput, routing.Snapshot](c.Capability, contracts.ReplacePolicyV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
