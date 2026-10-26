package rpc

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/memory/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/contractgen --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/plugins/memory/backend/cmd/contracts --caller-json ../../package/io.github.tangerg.flame/contracts/caller-policy.generated.json > /dev/null"

import (
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	memory "github.com/Tangerg/flame/plugins/memory/backend/domain"
	"github.com/Tangerg/flame/packages/go-sdk/capability"
)

type Contracts struct {
	Capability *capability.Contract
	List       *pluginsdk.Method[ListInput, Page]
	Add        *pluginsdk.Method[AddInput, AddResult]
	Update     *pluginsdk.Method[UpdateInput, memory.ItemSnapshot]
	Delete     *pluginsdk.Method[ItemInput, Ack]
	Review     *pluginsdk.Method[ReviewInput, Ack]
	Items      *pluginsdk.Method[ItemsInput, []memory.ItemSnapshot]
	Search     *pluginsdk.Method[SearchInput, []memory.ItemSnapshot]
	Maintain   *pluginsdk.Method[MaintenanceInput, MaintenanceResult]
}

func NewContracts() (Contracts, error) {
	var contracts Contracts
	var err error
	contracts.Capability, err = capability.Parse([]byte(memorycontract.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	contracts.List, err = pluginsdk.ImplementMethod[ListInput, Page](contracts.Capability, List)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Add, err = pluginsdk.ImplementMethod[AddInput, AddResult](contracts.Capability, Add)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Update, err = pluginsdk.ImplementMethod[UpdateInput, memory.ItemSnapshot](contracts.Capability, Update)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Delete, err = pluginsdk.ImplementMethod[ItemInput, Ack](contracts.Capability, Delete)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Review, err = pluginsdk.ImplementMethod[ReviewInput, Ack](contracts.Capability, Review)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Items, err = pluginsdk.ImplementMethod[ItemsInput, []memory.ItemSnapshot](contracts.Capability, Items)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Search, err = pluginsdk.ImplementMethod[SearchInput, []memory.ItemSnapshot](contracts.Capability, Search)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Maintain, err = pluginsdk.ImplementMethod[MaintenanceInput, MaintenanceResult](contracts.Capability, Maintain)
	if err != nil {
		return Contracts{}, err
	}
	return contracts, nil
}
