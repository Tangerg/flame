package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/a2a/backend/cmd/contracts | go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	"github.com/Tangerg/flame/plugins/a2a/contracts"
	"github.com/Tangerg/flame/plugins/a2a/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

type Contracts struct {
	Capability *portable.Capability
	Agents     *qimumcp.Method[struct{}, wire.Agents]
	Send       *qimumcp.Method[wire.SendRequest, wire.Result]
	Status     *qimumcp.Method[wire.TaskRequest, wire.Result]
	Cancel     *qimumcp.Method[wire.TaskRequest, wire.Result]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = portable.ParseCapability([]byte(contracts.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.Agents, err = qimumcp.ImplementMethod[struct{}, wire.Agents](c.Capability, contracts.AgentsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Send, err = qimumcp.ImplementMethod[wire.SendRequest, wire.Result](c.Capability, contracts.SendV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Status, err = qimumcp.ImplementMethod[wire.TaskRequest, wire.Result](c.Capability, contracts.StatusV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Cancel, err = qimumcp.ImplementMethod[wire.TaskRequest, wire.Result](c.Capability, contracts.CancelV1Method)
	return c, err
}
