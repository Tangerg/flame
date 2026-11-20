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
	capability, err := plugincontract.ParseCapability([]byte(contracts.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	methods := pluginmcp.NewMethodSet(capability)
	c := Contracts{
		Capability: capability,
		Get:        pluginmcp.Implement[struct{}, routing.Snapshot](methods, contracts.GetPolicyV1Method),
		Replace:    pluginmcp.Implement[wire.ReplaceInput, routing.Snapshot](methods, contracts.ReplacePolicyV1Method),
	}
	if err := methods.Err(); err != nil {
		return Contracts{}, err
	}
	return c, nil
}
