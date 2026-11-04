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
	routing "github.com/Tangerg/flame/plugins/modelrouter/internal/domain"
	"github.com/Tangerg/flame/plugins/modelrouter/internal/wire"
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
	get, err := pluginmcp.NewMethod[struct{}, routing.Snapshot](portable.MethodConfig{Name: "flame.model-router/get-policy.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Inspect the private routing policy and revision before editing."})
	if err != nil {
		return nil, err
	}
	replace, err := pluginmcp.NewMethod[wire.ReplaceInput, routing.Snapshot](portable.MethodConfig{Name: "flame.model-router/replace-policy.v1", Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Replace the routing policy at the inspected revision. Each use has at most one exact target and an optional caller-envelope ceiling."})
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.model-router.v1", Composition: plugincontract.Collection, Scope: extension.ScopeInstallation}, get.Descriptor(), replace.Descriptor())
}
