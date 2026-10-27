package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/guardrails/backend/cmd/contracts | go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --model-contract input > ../../package/io.github.tangerg.flame/contracts/input.generated.json"

import (
	"github.com/invopop/jsonschema"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	guardrail "github.com/Tangerg/flame/plugins/guardrails/internal/domain"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

type ReplaceInput struct {
	ExpectedRevision int64    `json:"expectedRevision" jsonschema:"minimum=1"`
	Phrases          []string `json:"phrases"`
}

type Contracts struct {
	Capability *portable.Capability
	Get        *qimumcp.Method[struct{}, guardrail.Summary]
	Replace    *qimumcp.Method[ReplaceInput, guardrail.Summary]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Get, err = qimumcp.NewMethod[struct{}, guardrail.Summary](qimumcp.MethodConfig{Name: "flame.guardrails/get-policy.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Inspect the private policy revision and protected phrase count without returning protected values."})
	if err != nil {
		return Contracts{}, err
	}
	c.Replace, err = qimumcp.NewMethod[ReplaceInput, guardrail.Summary](qimumcp.MethodConfig{Name: "flame.guardrails/replace-policy.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Replace exact, case-sensitive protected phrases at the inspected revision. Matching model-visible string fields refuse provider dispatch."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = portable.NewCapability(portable.CapabilityConfig{Reference: "flame.guardrails.v1", Composition: portable.Collection, Scope: extension.ScopeInstallation}, c.Get.Descriptor(), c.Replace.Descriptor())
	return c, err
}

func (ReplaceInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	phrases, _ := schema.Properties.Get("phrases")
	phrases.MaxItems = new(uint64(guardrail.MaxPhrases))
}
