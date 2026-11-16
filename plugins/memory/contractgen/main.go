package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"log"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	rpc "github.com/Tangerg/flame/plugins/memory/internal/contract"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"

	"github.com/Tangerg/frame"
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
	contracts.List, err = pluginmcp.NewMethod[rpc.ListInput, rpc.Page](frame.MethodConfig{Name: rpc.List, Kind: frame.Query, Target: extension.ScopeInstallation, Description: "List memory items visible in the requested scope."})
	if err != nil {
		return nil, err
	}
	contracts.Add, err = pluginmcp.NewMethod[rpc.AddInput, rpc.AddResult](frame.MethodConfig{Name: rpc.Add, Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Add a human-authored memory item in the requested scope."})
	if err != nil {
		return nil, err
	}
	contracts.Update, err = pluginmcp.NewMethod[rpc.UpdateInput, memory.ItemSnapshot](frame.MethodConfig{Name: rpc.Update, Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Edit the current content or pin state of a memory item."})
	if err != nil {
		return nil, err
	}
	contracts.Delete, err = pluginmcp.NewMethod[rpc.ItemInput, rpc.Ack](frame.MethodConfig{Name: rpc.Delete, Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Delete one memory item through its private owner."})
	if err != nil {
		return nil, err
	}
	contracts.Review, err = pluginmcp.NewMethod[rpc.ReviewInput, rpc.Ack](frame.MethodConfig{Name: rpc.Review, Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Apply a human review decision to a pending memory item."})
	if err != nil {
		return nil, err
	}
	contracts.Items, err = pluginmcp.NewMethod[rpc.ItemsInput, []memory.ItemSnapshot](frame.MethodConfig{Name: rpc.Items, Kind: frame.Query, Target: extension.ScopeInstallation, Description: "Read active memory items that can inform a Run."})
	if err != nil {
		return nil, err
	}
	contracts.Search, err = pluginmcp.NewMethod[rpc.SearchInput, []memory.ItemSnapshot](frame.MethodConfig{Name: rpc.Search, Kind: frame.Query, Target: extension.ScopeInstallation, Description: "Rank active memories for the supplied project and query."})
	if err != nil {
		return nil, err
	}
	contracts.Maintain, err = pluginmcp.NewMethod[rpc.MaintenanceInput, rpc.MaintenanceResult](frame.MethodConfig{Name: rpc.Maintain, Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Extract and curate captured conversation under the private ledger and watermark."})
	if err != nil {
		return nil, err
	}
	contracts.Capability, err = plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "memory", Composition: plugincontract.Selection, Scope: extension.ScopeInstallation}, contracts.List.Descriptor(), contracts.Add.Descriptor(), contracts.Update.Descriptor(), contracts.Delete.Descriptor(), contracts.Review.Descriptor(), contracts.Items.Descriptor(), contracts.Search.Descriptor(), contracts.Maintain.Descriptor())
	if err != nil {
		return nil, err
	}
	return contracts.Capability, nil
}
