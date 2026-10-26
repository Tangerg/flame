package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/modelrouter/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --model-contract selection > ../../package/io.github.tangerg.flame/contracts/selection.generated.json"

import (
	"github.com/invopop/jsonschema"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	routing "github.com/Tangerg/flame/plugins/modelrouter/internal/domain"
)

type ReplaceInput struct {
	ExpectedRevision int64          `json:"expectedRevision" jsonschema:"minimum=1"`
	Rules            []routing.Rule `json:"rules"`
}

type Contracts struct {
	Capability *extension.Capability
	Get        *pluginsdk.Method[struct{}, routing.Snapshot]
	Replace    *pluginsdk.Method[ReplaceInput, routing.Snapshot]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Get, err = pluginsdk.NewMethod[struct{}, routing.Snapshot](pluginsdk.MethodConfig{Name: "flame.model-router/get-policy.v1", Kind: extension.Query, Target: extension.ScopeInstallation, Description: "Inspect the private routing policy and revision before editing."})
	if err != nil {
		return Contracts{}, err
	}
	c.Replace, err = pluginsdk.NewMethod[ReplaceInput, routing.Snapshot](pluginsdk.MethodConfig{Name: "flame.model-router/replace-policy.v1", Kind: extension.Mutation, Target: extension.ScopeInstallation, Description: "Replace the routing policy at the inspected revision. Each use has at most one exact target and an optional caller-envelope ceiling."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = extension.NewCapability(extension.CapabilityConfig{Reference: "flame.model-router.v1", Composition: extension.Collection, Scope: extension.ScopeInstallation}, c.Get.Descriptor(), c.Replace.Descriptor())
	return c, err
}

func (ReplaceInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	rules, _ := schema.Properties.Get("rules")
	rules.MaxItems = new(uint64(routing.MaxRules))
}
