package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"log"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	rpc "github.com/Tangerg/flame/plugins/memory/internal/contract"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
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

func define() (*portable.Capability, error) {
	var contracts rpc.Contracts
	var err error
	contracts.List, err = qimumcp.NewMethod[rpc.ListInput, rpc.Page](qimumcp.MethodConfig{Name: rpc.List, Kind: portable.Query, Target: extension.ScopeInstallation, Description: "List memory items visible in the requested scope."})
	if err != nil {
		return nil, err
	}
	contracts.Add, err = qimumcp.NewMethod[rpc.AddInput, rpc.AddResult](qimumcp.MethodConfig{Name: rpc.Add, Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Add a human-authored memory item in the requested scope."})
	if err != nil {
		return nil, err
	}
	contracts.Update, err = qimumcp.NewMethod[rpc.UpdateInput, memory.ItemSnapshot](qimumcp.MethodConfig{Name: rpc.Update, Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Edit the current content or pin state of a memory item."})
	if err != nil {
		return nil, err
	}
	contracts.Delete, err = qimumcp.NewMethod[rpc.ItemInput, rpc.Ack](qimumcp.MethodConfig{Name: rpc.Delete, Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Delete one memory item through its private owner."})
	if err != nil {
		return nil, err
	}
	contracts.Review, err = qimumcp.NewMethod[rpc.ReviewInput, rpc.Ack](qimumcp.MethodConfig{Name: rpc.Review, Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Apply a human review decision to a pending memory item."})
	if err != nil {
		return nil, err
	}
	contracts.Items, err = qimumcp.NewMethod[rpc.ItemsInput, []memory.ItemSnapshot](qimumcp.MethodConfig{Name: rpc.Items, Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read active memory items that can inform a Run."})
	if err != nil {
		return nil, err
	}
	contracts.Search, err = qimumcp.NewMethod[rpc.SearchInput, []memory.ItemSnapshot](qimumcp.MethodConfig{Name: rpc.Search, Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Rank active memories for the supplied project and query."})
	if err != nil {
		return nil, err
	}
	contracts.Maintain, err = qimumcp.NewMethod[rpc.MaintenanceInput, rpc.MaintenanceResult](qimumcp.MethodConfig{Name: rpc.Maintain, Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Extract and curate captured conversation under the private ledger and watermark."})
	if err != nil {
		return nil, err
	}
	contracts.Capability, err = portable.NewCapability(portable.CapabilityConfig{Reference: "memory", Composition: portable.Selection, Scope: extension.ScopeInstallation}, contracts.List.Descriptor(), contracts.Add.Descriptor(), contracts.Update.Descriptor(), contracts.Delete.Descriptor(), contracts.Review.Descriptor(), contracts.Items.Descriptor(), contracts.Search.Descriptor(), contracts.Maintain.Descriptor())
	if err != nil {
		return nil, err
	}
	return contracts.Capability, nil
}
