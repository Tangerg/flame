package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/sandbox/backend/cmd/contracts | go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	"github.com/Tangerg/flame/plugins/sandbox/contracts"
	sandbox "github.com/Tangerg/flame/plugins/sandbox/internal/domain"
	"github.com/Tangerg/flame/plugins/sandbox/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

type Contracts struct {
	Capability *portable.Capability
	Execute    *qimumcp.Method[wire.ExecuteInput, sandbox.Result]
	Limits     *qimumcp.Method[struct{}, wire.Limits]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = portable.ParseCapability([]byte(contracts.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.Execute, err = qimumcp.ImplementMethod[wire.ExecuteInput, sandbox.Result](c.Capability, contracts.ExecuteV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Limits, err = qimumcp.ImplementMethod[struct{}, wire.Limits](c.Capability, contracts.LimitsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
