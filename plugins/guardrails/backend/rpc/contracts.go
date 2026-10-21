package rpc

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/guardrails/backend/cmd/contracts | go run github.com/Tangerg/flame/runtime/cmd/contractgen --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/runtime/cmd/modelcontracts input > ../../package/io.github.tangerg.flame/contracts/input.generated.json"

import (
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	guardrail "github.com/Tangerg/flame/plugins/guardrails/backend/domain"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/idempotency"
)

type ReplaceInput struct {
	ExpectedRevision int64    `json:"expectedRevision" jsonschema:"minimum=1"`
	Phrases          []string `json:"phrases" jsonschema:"maxItems=32"`
}

type Contracts struct {
	Capability *capability.Contract
	Get        *pluginsdk.Method[struct{}, guardrail.Summary]
	Replace    *pluginsdk.Method[ReplaceInput, guardrail.Summary]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Get, err = pluginsdk.NewMethod[struct{}, guardrail.Summary](pluginsdk.MethodConfig{Name: "flame.guardrails/get-policy.v1", Kind: capability.Query, Target: "installation", Description: "Inspect the private policy revision and protected phrase count without returning protected values."})
	if err != nil {
		return Contracts{}, err
	}
	c.Replace, err = pluginsdk.NewMethod[ReplaceInput, guardrail.Summary](pluginsdk.MethodConfig{Name: "flame.guardrails/replace-policy.v1", Kind: capability.Mutation, Target: "installation", Description: "Replace exact, case-sensitive protected phrases at the inspected revision. Matching model-visible string fields refuse provider dispatch."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = capability.New(capability.Config{Reference: "flame.guardrails.v1", Composition: capability.Collection, Scope: "installation"}, c.Get.Descriptor(), c.Replace.Descriptor())
	return c, err
}

func Classify(err error) string {
	switch {
	case errors.Is(err, guardrail.ErrInvalidPolicy):
		return "invalid_policy"
	case errors.Is(err, guardrail.ErrRevisionConflict):
		return "revision_conflict"
	case errors.Is(err, guardrail.ErrRevisionLimit):
		return "revision_limit"
	case errors.Is(err, guardrail.ErrProtectedContent):
		return "protected_content"
	case errors.Is(err, idempotency.ErrKeyConflict):
		return "command_conflict"
	default:
		return ""
	}
}
