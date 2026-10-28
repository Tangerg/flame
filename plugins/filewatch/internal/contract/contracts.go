package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/filewatch/backend/cmd/contracts | go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	published "github.com/Tangerg/flame/plugins/filewatch/contracts"
	filewatch "github.com/Tangerg/flame/plugins/filewatch/internal/domain"
	"github.com/Tangerg/flame/plugins/filewatch/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

type Contracts struct {
	Capability *portable.Capability
	Get        *qimumcp.Method[struct{}, filewatch.Snapshot]
	Configure  *qimumcp.Method[wire.ConfigureInput, filewatch.Snapshot]
	Status     *qimumcp.Method[struct{}, filewatch.Status]
	Events     *qimumcp.Method[struct{}, []filewatch.Header]
	Event      *qimumcp.Method[wire.EventInput, filewatch.Event]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = portable.ParseCapability([]byte(published.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.Get, err = qimumcp.ImplementMethod[struct{}, filewatch.Snapshot](c.Capability, published.GetV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Configure, err = qimumcp.ImplementMethod[wire.ConfigureInput, filewatch.Snapshot](c.Capability, published.ConfigureV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Status, err = qimumcp.ImplementMethod[struct{}, filewatch.Status](c.Capability, published.StatusV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Events, err = qimumcp.ImplementMethod[struct{}, []filewatch.Header](c.Capability, published.EventsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Event, err = qimumcp.ImplementMethod[wire.EventInput, filewatch.Event](c.Capability, published.EventV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
