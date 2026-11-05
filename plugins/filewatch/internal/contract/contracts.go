package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/filewatch/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	published "github.com/Tangerg/flame/plugins/filewatch/contracts"
	filewatch "github.com/Tangerg/flame/plugins/filewatch/internal/domain"
	"github.com/Tangerg/flame/plugins/filewatch/internal/wire"
)

type Contracts struct {
	Capability *plugincontract.Capability
	Get        *pluginmcp.Method[struct{}, filewatch.Snapshot]
	Configure  *pluginmcp.Method[wire.ConfigureInput, filewatch.Snapshot]
	Status     *pluginmcp.Method[struct{}, filewatch.Status]
	Events     *pluginmcp.Method[struct{}, []filewatch.Header]
	Event      *pluginmcp.Method[wire.EventInput, filewatch.Event]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = plugincontract.ParseCapability([]byte(published.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.Get, err = pluginmcp.ImplementMethod[struct{}, filewatch.Snapshot](c.Capability, published.GetV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Configure, err = pluginmcp.ImplementMethod[wire.ConfigureInput, filewatch.Snapshot](c.Capability, published.ConfigureV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Status, err = pluginmcp.ImplementMethod[struct{}, filewatch.Status](c.Capability, published.StatusV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Events, err = pluginmcp.ImplementMethod[struct{}, []filewatch.Header](c.Capability, published.EventsV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Event, err = pluginmcp.ImplementMethod[wire.EventInput, filewatch.Event](c.Capability, published.EventV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
