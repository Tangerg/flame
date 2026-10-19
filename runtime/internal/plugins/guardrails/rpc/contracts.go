package rpc

//go:generate sh -c "go run github.com/Tangerg/flame/runtime/cmd/guardrailcontracts | go run github.com/Tangerg/flame/runtime/cmd/contractgen --feature - --out ../../../../../plugins/guardrails/io.github.tangerg.flame/contracts --ts ../../../../../plugins/guardrails/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/runtime/cmd/modelinputcontracts > ../../../../../plugins/guardrails/io.github.tangerg.flame/contracts/input.generated.json"

import (
	"errors"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/idempotency"
	"github.com/Tangerg/flame/runtime/internal/plugins/guardrails/domain"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

type ReplaceInput struct {
	ExpectedRevision int64    `json:"expectedRevision" jsonschema:"minimum=1"`
	Phrases          []string `json:"phrases" jsonschema:"maxItems=32"`
}

type Contracts struct {
	Capability *capability.Contract
	Get        *pluginsdk.Method[struct{}, domain.Summary]
	Replace    *pluginsdk.Method[ReplaceInput, domain.Summary]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Get, err = pluginsdk.NewMethod[struct{}, domain.Summary](pluginsdk.MethodConfig{Name: "flame.guardrails/get-policy.v1", Kind: capability.Query, Target: "installation", Description: "Inspect the private policy revision and protected phrase count without returning protected values."})
	if err != nil {
		return Contracts{}, err
	}
	c.Replace, err = pluginsdk.NewMethod[ReplaceInput, domain.Summary](pluginsdk.MethodConfig{Name: "flame.guardrails/replace-policy.v1", Kind: capability.Mutation, Target: "installation", Description: "Replace exact, case-sensitive protected phrases at the inspected revision. Matching model-visible string fields refuse provider dispatch."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = capability.New(capability.Config{Reference: "flame.guardrails.v1", Composition: capability.Collection, Scope: "installation"}, c.Get.Descriptor(), c.Replace.Descriptor())
	return c, err
}

func Classify(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalidPolicy):
		return "invalid_policy"
	case errors.Is(err, domain.ErrRevisionConflict):
		return "revision_conflict"
	case errors.Is(err, domain.ErrRevisionLimit):
		return "revision_limit"
	case errors.Is(err, domain.ErrProtectedContent):
		return "protected_content"
	case errors.Is(err, idempotency.ErrKeyConflict):
		return "command_conflict"
	default:
		return ""
	}
}
