package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/guardrails/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --model-contract input > ../../package/io.github.tangerg.flame/contracts/input.generated.json"

import (
	"github.com/invopop/jsonschema"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	guardrail "github.com/Tangerg/flame/plugins/guardrails/internal/domain"
)

type ReplaceInput struct {
	ExpectedRevision int64    `json:"expectedRevision" jsonschema:"minimum=1"`
	Phrases          []string `json:"phrases"`
}

type Contracts struct {
	Capability *extension.Capability
	Get        *pluginsdk.Method[struct{}, guardrail.Summary]
	Replace    *pluginsdk.Method[ReplaceInput, guardrail.Summary]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Get, err = pluginsdk.NewMethod[struct{}, guardrail.Summary](pluginsdk.MethodConfig{Name: "flame.guardrails/get-policy.v1", Kind: extension.Query, Target: extension.ScopeInstallation, Description: "Inspect the private policy revision and protected phrase count without returning protected values."})
	if err != nil {
		return Contracts{}, err
	}
	c.Replace, err = pluginsdk.NewMethod[ReplaceInput, guardrail.Summary](pluginsdk.MethodConfig{Name: "flame.guardrails/replace-policy.v1", Kind: extension.Mutation, Target: extension.ScopeInstallation, Description: "Replace exact, case-sensitive protected phrases at the inspected revision. Matching model-visible string fields refuse provider dispatch."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = extension.NewCapability(extension.CapabilityConfig{Reference: "flame.guardrails.v1", Composition: extension.Collection, Scope: extension.ScopeInstallation}, c.Get.Descriptor(), c.Replace.Descriptor())
	return c, err
}

func (ReplaceInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	phrases, _ := schema.Properties.Get("phrases")
	phrases.MaxItems = new(uint64(guardrail.MaxPhrases))
}
