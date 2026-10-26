package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"log"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	memory "github.com/Tangerg/flame/plugins/memory/backend/domain"
	"github.com/Tangerg/flame/plugins/memory/backend/rpc"
)

func main() {
	contract, err := define()
	if err != nil {
		log.Fatal(err)
	}
	data, err := json.Marshal(contract.Descriptor(), json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
		log.Fatal(err)
	}
}

func define() (*capability.Contract, error) {
	var contracts rpc.Contracts
	var err error
	contracts.List, err = pluginsdk.NewMethod[rpc.ListInput, rpc.Page](pluginsdk.MethodConfig{Name: rpc.List, Kind: capability.Query, Target: capability.ScopeInstallation, Description: "List memory items visible in the requested scope."})
	if err != nil {
		return nil, err
	}
	contracts.Add, err = pluginsdk.NewMethod[rpc.AddInput, rpc.AddResult](pluginsdk.MethodConfig{Name: rpc.Add, Kind: capability.Mutation, Target: capability.ScopeInstallation, Description: "Add a human-authored memory item in the requested scope."})
	if err != nil {
		return nil, err
	}
	contracts.Update, err = pluginsdk.NewMethod[rpc.UpdateInput, memory.ItemSnapshot](pluginsdk.MethodConfig{Name: rpc.Update, Kind: capability.Mutation, Target: capability.ScopeInstallation, Description: "Edit the current content or pin state of a memory item."})
	if err != nil {
		return nil, err
	}
	contracts.Delete, err = pluginsdk.NewMethod[rpc.ItemInput, rpc.Ack](pluginsdk.MethodConfig{Name: rpc.Delete, Kind: capability.Mutation, Target: capability.ScopeInstallation, Description: "Delete one memory item through its private owner."})
	if err != nil {
		return nil, err
	}
	contracts.Review, err = pluginsdk.NewMethod[rpc.ReviewInput, rpc.Ack](pluginsdk.MethodConfig{Name: rpc.Review, Kind: capability.Mutation, Target: capability.ScopeInstallation, Description: "Apply a human review decision to a pending memory item."})
	if err != nil {
		return nil, err
	}
	contracts.Items, err = pluginsdk.NewMethod[rpc.ItemsInput, []memory.ItemSnapshot](pluginsdk.MethodConfig{Name: rpc.Items, Kind: capability.Query, Target: capability.ScopeInstallation, Description: "Read active memory items that can inform a Run."})
	if err != nil {
		return nil, err
	}
	contracts.Search, err = pluginsdk.NewMethod[rpc.SearchInput, []memory.ItemSnapshot](pluginsdk.MethodConfig{Name: rpc.Search, Kind: capability.Query, Target: capability.ScopeInstallation, Description: "Rank active memories for the supplied project and query."})
	if err != nil {
		return nil, err
	}
	contracts.Maintain, err = pluginsdk.NewMethod[rpc.MaintenanceInput, rpc.MaintenanceResult](pluginsdk.MethodConfig{Name: rpc.Maintain, Kind: capability.Mutation, Target: capability.ScopeInstallation, Description: "Extract and curate captured conversation under the private ledger and watermark."})
	if err != nil {
		return nil, err
	}
	contracts.Capability, err = capability.New(capability.Config{Reference: "memory", Composition: capability.Selection, Scope: capability.ScopeInstallation}, contracts.List.Descriptor(), contracts.Add.Descriptor(), contracts.Update.Descriptor(), contracts.Delete.Descriptor(), contracts.Review.Descriptor(), contracts.Items.Descriptor(), contracts.Search.Descriptor(), contracts.Maintain.Descriptor())
	if err != nil {
		return nil, err
	}
	return contracts.Capability, nil
}
