package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"log"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	routing "github.com/Tangerg/flame/plugins/modelrouter/internal/domain"
	"github.com/Tangerg/flame/plugins/modelrouter/internal/wire"
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
	get, err := qimumcp.NewMethod[struct{}, routing.Snapshot](qimumcp.MethodConfig{Name: "flame.model-router/get-policy.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Inspect the private routing policy and revision before editing."})
	if err != nil {
		return nil, err
	}
	replace, err := qimumcp.NewMethod[wire.ReplaceInput, routing.Snapshot](qimumcp.MethodConfig{Name: "flame.model-router/replace-policy.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Replace the routing policy at the inspected revision. Each use has at most one exact target and an optional caller-envelope ceiling."})
	if err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: "flame.model-router.v1", Composition: portable.Collection, Scope: extension.ScopeInstallation}, get.Descriptor(), replace.Descriptor())
}
