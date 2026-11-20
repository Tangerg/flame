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
	capability, err := plugincontract.ParseCapability([]byte(contracts.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	methods := pluginmcp.NewMethodSet(capability)
	c := Contracts{
		Capability: capability,
		Execute:    pluginmcp.Implement[wire.ExecuteInput, sandbox.Result](methods, contracts.ExecuteV1Method),
		Limits:     pluginmcp.Implement[struct{}, wire.Limits](methods, contracts.LimitsV1Method),
	}
	if err := methods.Err(); err != nil {
		return Contracts{}, err
	}
	return c, nil
}
