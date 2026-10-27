package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/modelrouter/backend/cmd/contracts | go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --model-contract selection > ../../package/io.github.tangerg.flame/contracts/selection.generated.json"

import (
	"github.com/Tangerg/flame/plugins/modelrouter/contracts"
	routing "github.com/Tangerg/flame/plugins/modelrouter/internal/domain"
	"github.com/Tangerg/flame/plugins/modelrouter/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

type Contracts struct {
	Capability *portable.Capability
	Get        *qimumcp.Method[struct{}, routing.Snapshot]
	Replace    *qimumcp.Method[wire.ReplaceInput, routing.Snapshot]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = portable.ParseCapability([]byte(contracts.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.Get, err = qimumcp.ImplementMethod[struct{}, routing.Snapshot](c.Capability, contracts.GetPolicyV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Replace, err = qimumcp.ImplementMethod[wire.ReplaceInput, routing.Snapshot](c.Capability, contracts.ReplacePolicyV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
