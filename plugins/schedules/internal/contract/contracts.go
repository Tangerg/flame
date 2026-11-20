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
	capability, err := plugincontract.ParseCapability([]byte(published.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	methods := pluginmcp.NewMethodSet(capability)
	c := Contracts{
		Capability: capability,
		List:       pluginmcp.Implement[wire.ListInput, wire.Page](methods, published.ListV1Method),
		Create:     pluginmcp.Implement[schedule.Draft, schedule.Snapshot](methods, published.CreateV1Method),
		Update:     pluginmcp.Implement[wire.UpdateInput, schedule.Snapshot](methods, published.UpdateV1Method),
		Delete:     pluginmcp.Implement[wire.ItemInput, wire.Ack](methods, published.DeleteV1Method),
		RunNow:     pluginmcp.Implement[wire.ItemInput, app.RunReference](methods, published.RunNowV1Method),
	}
	if err := methods.Err(); err != nil {
		return Contracts{}, err
	}
	return c, nil
}
