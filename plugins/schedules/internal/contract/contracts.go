package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/schedules/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	published "github.com/Tangerg/flame/plugins/schedules/contracts"
	app "github.com/Tangerg/flame/plugins/schedules/internal/application"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/Tangerg/flame/plugins/schedules/internal/wire"
)

type Contracts struct {
	Capability *plugincontract.Capability
	List       *pluginmcp.Method[wire.ListInput, wire.Page]
	Create     *pluginmcp.Method[schedule.Draft, schedule.Snapshot]
	Update     *pluginmcp.Method[wire.UpdateInput, schedule.Snapshot]
	Delete     *pluginmcp.Method[wire.ItemInput, wire.Ack]
	RunNow     *pluginmcp.Method[wire.ItemInput, app.RunReference]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = plugincontract.ParseCapability([]byte(published.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.List, err = pluginmcp.ImplementMethod[wire.ListInput, wire.Page](c.Capability, published.ListV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Create, err = pluginmcp.ImplementMethod[schedule.Draft, schedule.Snapshot](c.Capability, published.CreateV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Update, err = pluginmcp.ImplementMethod[wire.UpdateInput, schedule.Snapshot](c.Capability, published.UpdateV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Delete, err = pluginmcp.ImplementMethod[wire.ItemInput, wire.Ack](c.Capability, published.DeleteV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.RunNow, err = pluginmcp.ImplementMethod[wire.ItemInput, app.RunReference](c.Capability, published.RunNowV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
