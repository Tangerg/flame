package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/memory/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/plugins/memory/backend/cmd/contracts --caller-json ../../package/io.github.tangerg.flame/contracts/caller-policy.generated.json > /dev/null"

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
)

type Contracts struct {
	Capability *plugincontract.Capability
	List       *pluginmcp.Method[ListInput, Page]
	Add        *pluginmcp.Method[AddInput, AddResult]
	Update     *pluginmcp.Method[UpdateInput, Item]
	Delete     *pluginmcp.Method[ItemInput, Ack]
	Review     *pluginmcp.Method[ReviewInput, Ack]
	Items      *pluginmcp.Method[ItemsInput, []Item]
	Search     *pluginmcp.Method[SearchInput, []Item]
	Maintain   *pluginmcp.Method[MaintenanceInput, MaintenanceResult]
}

func NewContracts() (Contracts, error) {
	capability, err := plugincontract.ParseCapability([]byte(memorycontract.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	methods := pluginmcp.NewMethodSet(capability)
	c := Contracts{
		Capability: capability,
		List:       pluginmcp.Implement[ListInput, Page](methods, List),
		Add:        pluginmcp.Implement[AddInput, AddResult](methods, Add),
		Update:     pluginmcp.Implement[UpdateInput, Item](methods, Update),
		Delete:     pluginmcp.Implement[ItemInput, Ack](methods, Delete),
		Review:     pluginmcp.Implement[ReviewInput, Ack](methods, Review),
		Items:      pluginmcp.Implement[ItemsInput, []Item](methods, Items),
		Search:     pluginmcp.Implement[SearchInput, []Item](methods, Search),
		Maintain:   pluginmcp.Implement[MaintenanceInput, MaintenanceResult](methods, Maintain),
	}
	if err := methods.Err(); err != nil {
		return Contracts{}, err
	}
	return c, nil
}
