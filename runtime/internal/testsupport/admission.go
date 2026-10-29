package testsupport

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/ownership"
)

// NewAdmissionGate treats fixture workspace names as already resolved identities.
func NewAdmissionGate() *ownership.Gate {
	gate, err := ownership.NewGate(ownership.GateConfig{ResolveTree: func(path string) (string, error) { return path, nil }, RetirementPending: NoResourceRetirements})
	if err != nil {
		panic(err)
	}
	return gate
}

func NoResourceRetirements(context.Context, string) (bool, error) { return false, nil }
