package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/memory/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/plugins/memory/backend/cmd/contracts --caller-json ../../package/io.github.tangerg.flame/contracts/caller-policy.generated.json > /dev/null"

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
)

type Contracts struct {
	Capability *plugincontract.Capability
	List       *pluginmcp.Method[ListInput, Page]
	Add        *pluginmcp.Method[AddInput, AddResult]
	Update     *pluginmcp.Method[UpdateInput, memory.ItemSnapshot]
	Delete     *pluginmcp.Method[ItemInput, Ack]
	Review     *pluginmcp.Method[ReviewInput, Ack]
	Items      *pluginmcp.Method[ItemsInput, []memory.ItemSnapshot]
	Search     *pluginmcp.Method[SearchInput, []memory.ItemSnapshot]
	Maintain   *pluginmcp.Method[MaintenanceInput, MaintenanceResult]
}

func NewContracts() (Contracts, error) {
	var contracts Contracts
	var err error
	contracts.Capability, err = plugincontract.ParseCapability([]byte(memorycontract.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	contracts.List, err = pluginmcp.ImplementMethod[ListInput, Page](contracts.Capability, List)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Add, err = pluginmcp.ImplementMethod[AddInput, AddResult](contracts.Capability, Add)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Update, err = pluginmcp.ImplementMethod[UpdateInput, memory.ItemSnapshot](contracts.Capability, Update)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Delete, err = pluginmcp.ImplementMethod[ItemInput, Ack](contracts.Capability, Delete)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Review, err = pluginmcp.ImplementMethod[ReviewInput, Ack](contracts.Capability, Review)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Items, err = pluginmcp.ImplementMethod[ItemsInput, []memory.ItemSnapshot](contracts.Capability, Items)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Search, err = pluginmcp.ImplementMethod[SearchInput, []memory.ItemSnapshot](contracts.Capability, Search)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Maintain, err = pluginmcp.ImplementMethod[MaintenanceInput, MaintenanceResult](contracts.Capability, Maintain)
	if err != nil {
		return Contracts{}, err
	}
	return contracts, nil
}
