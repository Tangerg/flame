package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/schedules/backend/cmd/contracts | go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	published "github.com/Tangerg/flame/plugins/schedules/contracts"
	app "github.com/Tangerg/flame/plugins/schedules/internal/application"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/Tangerg/flame/plugins/schedules/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

type Contracts struct {
	Capability *portable.Capability
	List       *qimumcp.Method[wire.ListInput, wire.Page]
	Create     *qimumcp.Method[schedule.Draft, schedule.Snapshot]
	Update     *qimumcp.Method[wire.UpdateInput, schedule.Snapshot]
	Delete     *qimumcp.Method[wire.ItemInput, wire.Ack]
	RunNow     *qimumcp.Method[wire.ItemInput, app.RunReference]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = portable.ParseCapability([]byte(published.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.List, err = qimumcp.ImplementMethod[wire.ListInput, wire.Page](c.Capability, published.ListV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Create, err = qimumcp.ImplementMethod[schedule.Draft, schedule.Snapshot](c.Capability, published.CreateV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Update, err = qimumcp.ImplementMethod[wire.UpdateInput, schedule.Snapshot](c.Capability, published.UpdateV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Delete, err = qimumcp.ImplementMethod[wire.ItemInput, wire.Ack](c.Capability, published.DeleteV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.RunNow, err = qimumcp.ImplementMethod[wire.ItemInput, app.RunReference](c.Capability, published.RunNowV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
