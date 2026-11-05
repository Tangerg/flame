package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/guardrails/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --model-contract input > ../../package/io.github.tangerg.flame/contracts/input.generated.json"

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/plugins/guardrails/contracts"
	guardrail "github.com/Tangerg/flame/plugins/guardrails/internal/domain"
	"github.com/Tangerg/flame/plugins/guardrails/internal/wire"
)

type Contracts struct {
	Capability *plugincontract.Capability
	Get        *pluginmcp.Method[struct{}, guardrail.Summary]
	Replace    *pluginmcp.Method[wire.ReplaceInput, guardrail.Summary]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = plugincontract.ParseCapability([]byte(contracts.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.Get, err = pluginmcp.ImplementMethod[struct{}, guardrail.Summary](c.Capability, contracts.GetPolicyV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Replace, err = pluginmcp.ImplementMethod[wire.ReplaceInput, guardrail.Summary](c.Capability, contracts.ReplacePolicyV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
