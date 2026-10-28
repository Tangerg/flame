package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/guardrails/backend/cmd/contracts | go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --model-contract input > ../../package/io.github.tangerg.flame/contracts/input.generated.json"

import (
	"github.com/Tangerg/flame/plugins/guardrails/contracts"
	guardrail "github.com/Tangerg/flame/plugins/guardrails/internal/domain"
	"github.com/Tangerg/flame/plugins/guardrails/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

type Contracts struct {
	Capability *portable.Capability
	Get        *qimumcp.Method[struct{}, guardrail.Summary]
	Replace    *qimumcp.Method[wire.ReplaceInput, guardrail.Summary]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = portable.ParseCapability([]byte(contracts.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.Get, err = qimumcp.ImplementMethod[struct{}, guardrail.Summary](c.Capability, contracts.GetPolicyV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Replace, err = qimumcp.ImplementMethod[wire.ReplaceInput, guardrail.Summary](c.Capability, contracts.ReplacePolicyV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
