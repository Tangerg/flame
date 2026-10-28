package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/memory/backend/cmd/contracts | go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/plugins/memory/backend/cmd/contracts --caller-json ../../package/io.github.tangerg.flame/contracts/caller-policy.generated.json > /dev/null"

import (
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

type Contracts struct {
	Capability *portable.Capability
	List       *qimumcp.Method[ListInput, Page]
	Add        *qimumcp.Method[AddInput, AddResult]
	Update     *qimumcp.Method[UpdateInput, memory.ItemSnapshot]
	Delete     *qimumcp.Method[ItemInput, Ack]
	Review     *qimumcp.Method[ReviewInput, Ack]
	Items      *qimumcp.Method[ItemsInput, []memory.ItemSnapshot]
	Search     *qimumcp.Method[SearchInput, []memory.ItemSnapshot]
	Maintain   *qimumcp.Method[MaintenanceInput, MaintenanceResult]
}

func NewContracts() (Contracts, error) {
	var contracts Contracts
	var err error
	contracts.Capability, err = portable.ParseCapability([]byte(memorycontract.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	contracts.List, err = qimumcp.ImplementMethod[ListInput, Page](contracts.Capability, List)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Add, err = qimumcp.ImplementMethod[AddInput, AddResult](contracts.Capability, Add)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Update, err = qimumcp.ImplementMethod[UpdateInput, memory.ItemSnapshot](contracts.Capability, Update)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Delete, err = qimumcp.ImplementMethod[ItemInput, Ack](contracts.Capability, Delete)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Review, err = qimumcp.ImplementMethod[ReviewInput, Ack](contracts.Capability, Review)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Items, err = qimumcp.ImplementMethod[ItemsInput, []memory.ItemSnapshot](contracts.Capability, Items)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Search, err = qimumcp.ImplementMethod[SearchInput, []memory.ItemSnapshot](contracts.Capability, Search)
	if err != nil {
		return Contracts{}, err
	}
	contracts.Maintain, err = qimumcp.ImplementMethod[MaintenanceInput, MaintenanceResult](contracts.Capability, Maintain)
	if err != nil {
		return Contracts{}, err
	}
	return contracts, nil
}
