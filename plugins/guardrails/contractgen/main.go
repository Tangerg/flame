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
	guardrail "github.com/Tangerg/flame/plugins/guardrails/internal/domain"
	"github.com/Tangerg/flame/plugins/guardrails/internal/wire"
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
	get, err := pluginmcp.NewMethod[struct{}, guardrail.Summary](portable.MethodConfig{Name: "flame.guardrails/get-policy.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Inspect the private policy revision and protected phrase count without returning protected values."})
	if err != nil {
		return nil, err
	}
	replace, err := pluginmcp.NewMethod[wire.ReplaceInput, guardrail.Summary](portable.MethodConfig{Name: "flame.guardrails/replace-policy.v1", Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Replace exact, case-sensitive protected phrases at the inspected revision. Matching model-visible string fields refuse provider dispatch."})
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.guardrails.v1", Composition: plugincontract.Collection, Scope: extension.ScopeInstallation}, get.Descriptor(), replace.Descriptor())
}
