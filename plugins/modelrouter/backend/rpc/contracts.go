package rpc

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/modelrouter/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/contractgen --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/packages/go-sdk/cmd/contractgen --model-contract selection > ../../package/io.github.tangerg.flame/contracts/selection.generated.json"

import (
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	routing "github.com/Tangerg/flame/plugins/modelrouter/backend/domain"
	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/idempotency"
)

type ReplaceInput struct {
	ExpectedRevision int64          `json:"expectedRevision" jsonschema:"minimum=1"`
	Rules            []routing.Rule `json:"rules" jsonschema:"maxItems=3"`
}

type Contracts struct {
	Capability *capability.Contract
	Get        *pluginsdk.Method[struct{}, routing.Snapshot]
	Replace    *pluginsdk.Method[ReplaceInput, routing.Snapshot]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Get, err = pluginsdk.NewMethod[struct{}, routing.Snapshot](pluginsdk.MethodConfig{Name: "flame.model-router/get-policy.v1", Kind: capability.Query, Target: capability.ScopeInstallation, Description: "Inspect the private routing policy and revision before editing."})
	if err != nil {
		return Contracts{}, err
	}
	c.Replace, err = pluginsdk.NewMethod[ReplaceInput, routing.Snapshot](pluginsdk.MethodConfig{Name: "flame.model-router/replace-policy.v1", Kind: capability.Mutation, Target: capability.ScopeInstallation, Description: "Replace the routing policy at the inspected revision. Each use has at most one exact target and an optional caller-envelope ceiling."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = capability.New(capability.Config{Reference: "flame.model-router.v1", Composition: capability.Collection, Scope: capability.ScopeInstallation}, c.Get.Descriptor(), c.Replace.Descriptor())
	return c, err
}

func Classify(err error) string {
	switch {
	case errors.Is(err, routing.ErrInvalidPolicy):
		return "invalid_policy"
	case errors.Is(err, routing.ErrRevisionConflict):
		return "revision_conflict"
	case errors.Is(err, routing.ErrRevisionLimit):
		return "revision_limit"
	case errors.Is(err, idempotency.ErrKeyConflict):
		return "command_conflict"
	default:
		return ""
	}
}
