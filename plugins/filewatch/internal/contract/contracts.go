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
	capability, err := plugincontract.ParseCapability([]byte(published.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	methods := pluginmcp.NewMethodSet(capability)
	c := Contracts{
		Capability: capability,
		Get:        pluginmcp.Implement[struct{}, filewatch.Snapshot](methods, published.GetV1Method),
		Configure:  pluginmcp.Implement[wire.ConfigureInput, filewatch.Snapshot](methods, published.ConfigureV1Method),
		Status:     pluginmcp.Implement[struct{}, filewatch.Status](methods, published.StatusV1Method),
		Events:     pluginmcp.Implement[struct{}, []filewatch.Header](methods, published.EventsV1Method),
		Event:      pluginmcp.Implement[wire.EventInput, filewatch.Event](methods, published.EventV1Method),
	}
	if err := methods.Err(); err != nil {
		return Contracts{}, err
	}
	return c, nil
}
