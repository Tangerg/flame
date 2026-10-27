package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"log"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	guardrail "github.com/Tangerg/flame/plugins/guardrails/internal/domain"
	"github.com/Tangerg/flame/plugins/guardrails/internal/wire"
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
	get, err := qimumcp.NewMethod[struct{}, guardrail.Summary](qimumcp.MethodConfig{Name: "flame.guardrails/get-policy.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Inspect the private policy revision and protected phrase count without returning protected values."})
	if err != nil {
		return nil, err
	}
	replace, err := qimumcp.NewMethod[wire.ReplaceInput, guardrail.Summary](qimumcp.MethodConfig{Name: "flame.guardrails/replace-policy.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Replace exact, case-sensitive protected phrases at the inspected revision. Matching model-visible string fields refuse provider dispatch."})
	if err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: "flame.guardrails.v1", Composition: portable.Collection, Scope: extension.ScopeInstallation}, get.Descriptor(), replace.Descriptor())
}
