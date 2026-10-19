package rpc

//go:generate sh -c "go run github.com/Tangerg/flame/runtime/cmd/routercontracts | go run github.com/Tangerg/flame/runtime/cmd/contractgen --feature - --out ../../../../../plugins/modelrouter/io.github.tangerg.flame/contracts --ts ../../../../../plugins/modelrouter/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/runtime/cmd/modelselectioncontracts > ../../../../../plugins/modelrouter/io.github.tangerg.flame/contracts/selection.generated.json"

import (
	"errors"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/idempotency"
	"github.com/Tangerg/flame/runtime/internal/plugins/modelrouter/domain"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

type ReplaceInput struct {
	ExpectedRevision int64         `json:"expectedRevision" jsonschema:"minimum=1"`
	Rules            []domain.Rule `json:"rules" jsonschema:"maxItems=3"`
}

type Contracts struct {
	Capability *capability.Contract
	Get        *pluginsdk.Method[struct{}, domain.Snapshot]
	Replace    *pluginsdk.Method[ReplaceInput, domain.Snapshot]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Get, err = pluginsdk.NewMethod[struct{}, domain.Snapshot](pluginsdk.MethodConfig{Name: "flame.model-router/get-policy.v1", Kind: capability.Query, Target: "installation", Description: "Inspect the private routing policy and revision before editing."})
	if err != nil {
		return Contracts{}, err
	}
	c.Replace, err = pluginsdk.NewMethod[ReplaceInput, domain.Snapshot](pluginsdk.MethodConfig{Name: "flame.model-router/replace-policy.v1", Kind: capability.Mutation, Target: "installation", Description: "Replace the routing policy at the inspected revision. Each use has at most one exact target and an optional caller-envelope ceiling."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = capability.New(capability.Config{Reference: "flame.model-router.v1", Composition: capability.Collection, Scope: "installation"}, c.Get.Descriptor(), c.Replace.Descriptor())
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
	case errors.Is(err, idempotency.ErrKeyConflict):
		return "command_conflict"
	default:
		return ""
	}
}
