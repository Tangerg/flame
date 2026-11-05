package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"log"
	"os"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	rpc "github.com/Tangerg/flame/plugins/memory/internal/contract"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
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

func define() (*plugincontract.Capability, error) {
	var contracts rpc.Contracts
	var err error
	contracts.List, err = pluginmcp.NewMethod[rpc.ListInput, rpc.Page](portable.MethodConfig{Name: rpc.List, Kind: portable.Query, Target: extension.ScopeInstallation, Description: "List memory items visible in the requested scope."})
	if err != nil {
		return nil, err
	}
	contracts.Add, err = pluginmcp.NewMethod[rpc.AddInput, rpc.AddResult](portable.MethodConfig{Name: rpc.Add, Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Add a human-authored memory item in the requested scope."})
	if err != nil {
		return nil, err
	}
	contracts.Update, err = pluginmcp.NewMethod[rpc.UpdateInput, memory.ItemSnapshot](portable.MethodConfig{Name: rpc.Update, Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Edit the current content or pin state of a memory item."})
	if err != nil {
		return nil, err
	}
	contracts.Delete, err = pluginmcp.NewMethod[rpc.ItemInput, rpc.Ack](portable.MethodConfig{Name: rpc.Delete, Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Delete one memory item through its private owner."})
	if err != nil {
		return nil, err
	}
	contracts.Review, err = pluginmcp.NewMethod[rpc.ReviewInput, rpc.Ack](portable.MethodConfig{Name: rpc.Review, Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Apply a human review decision to a pending memory item."})
	if err != nil {
		return nil, err
	}
	contracts.Items, err = pluginmcp.NewMethod[rpc.ItemsInput, []memory.ItemSnapshot](portable.MethodConfig{Name: rpc.Items, Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read active memory items that can inform a Run."})
	if err != nil {
		return nil, err
	}
	contracts.Search, err = pluginmcp.NewMethod[rpc.SearchInput, []memory.ItemSnapshot](portable.MethodConfig{Name: rpc.Search, Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Rank active memories for the supplied project and query."})
	if err != nil {
		return nil, err
	}
	contracts.Maintain, err = pluginmcp.NewMethod[rpc.MaintenanceInput, rpc.MaintenanceResult](portable.MethodConfig{Name: rpc.Maintain, Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Extract and curate captured conversation under the private ledger and watermark."})
	if err != nil {
		return nil, err
	}
	contracts.Capability, err = plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "memory", Composition: plugincontract.Selection, Scope: extension.ScopeInstallation}, contracts.List.Descriptor(), contracts.Add.Descriptor(), contracts.Update.Descriptor(), contracts.Delete.Descriptor(), contracts.Review.Descriptor(), contracts.Items.Descriptor(), contracts.Search.Descriptor(), contracts.Maintain.Descriptor())
	if err != nil {
		return nil, err
	}
	return contracts.Capability, nil
}
