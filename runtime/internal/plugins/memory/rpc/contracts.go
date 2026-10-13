package rpc

//go:generate sh -c "go run github.com/Tangerg/flame/runtime/cmd/memorycontracts | go run github.com/Tangerg/flame/runtime/cmd/contractgen --feature - --out ../../../../../plugins/memory/io.github.tangerg.flame/contracts --ts ../../../../../plugins/memory/io.github.tangerg.flame/contracts --go-out ../../../../../cli/internal/domain/memory/contract.generated.go --go-package memory"

import (
	"github.com/Tangerg/flame/runtime/capability"
	memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
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
	contracts.List, err = pluginsdk.NewMethod[ListInput, Page](pluginsdk.MethodConfig{Name: List, Kind: capability.Query, Target: "installation", Description: "List memory items visible in the requested scope."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Add, err = pluginsdk.NewMethod[AddInput, AddResult](pluginsdk.MethodConfig{Name: Add, Kind: capability.Mutation, Target: "installation", Description: "Add a human-authored memory item in the requested scope."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Update, err = pluginsdk.NewMethod[UpdateInput, memory.ItemSnapshot](pluginsdk.MethodConfig{Name: Update, Kind: capability.Mutation, Target: "installation", Description: "Edit the current content or pin state of a memory item."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Delete, err = pluginsdk.NewMethod[ItemInput, Ack](pluginsdk.MethodConfig{Name: Delete, Kind: capability.Mutation, Target: "installation", Description: "Delete one memory item through its private owner."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Review, err = pluginsdk.NewMethod[ReviewInput, Ack](pluginsdk.MethodConfig{Name: Review, Kind: capability.Mutation, Target: "installation", Description: "Apply a human review decision to a pending memory item."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Items, err = pluginsdk.NewMethod[ItemsInput, []memory.ItemSnapshot](pluginsdk.MethodConfig{Name: Items, Kind: capability.Query, Target: "installation", Description: "Read active memory items that can inform a Run."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Search, err = pluginsdk.NewMethod[SearchInput, []memory.ItemSnapshot](pluginsdk.MethodConfig{Name: Search, Kind: capability.Query, Target: "installation", Description: "Rank active memories for the supplied project and query."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Maintain, err = pluginsdk.NewMethod[MaintenanceInput, MaintenanceResult](pluginsdk.MethodConfig{Name: Maintain, Kind: capability.Mutation, Target: "installation", Description: "Extract and curate captured conversation under the private ledger and watermark."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Capability, err = capability.New(capability.Config{Reference: "memory", Composition: capability.Selection, Scope: "installation"}, contracts.List.Descriptor(), contracts.Add.Descriptor(), contracts.Update.Descriptor(), contracts.Delete.Descriptor(), contracts.Review.Descriptor(), contracts.Items.Descriptor(), contracts.Search.Descriptor(), contracts.Maintain.Descriptor())
	if err != nil {
		return Contracts{}, err
	}
	return contracts, nil
}
