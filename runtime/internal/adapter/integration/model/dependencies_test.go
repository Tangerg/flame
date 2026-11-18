package model

import (
	"context"

	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

type pointerCredentialLookup struct{}

func (*pointerCredentialLookup) Get(context.Context, string) (provider.Provider, bool, error) {
	return provider.Provider{}, false, nil
}

type fakeEndpointJournal struct{}

func (fakeEndpointJournal) StartEndpointInvocation(context.Context, modelref.EndpointInvocation, string) error {
	return nil
}

func (fakeEndpointJournal) SettleEndpointInvocation(context.Context, modelref.EndpointInvocation, modelref.EndpointOutcome) error {
	return nil
}

type pointerRoleSource struct{}

func (*pointerRoleSource) Role() modelref.Role { return modelref.Role{} }

func TestUnconfiguredEmbeddingRoleIsTheOnlyKeywordOnlyState(t *testing.T) {
	resolver, err := NewEmbeddingResolver(&pointerCredentialLookup{}, &fixtureModelFactory{}, fakeEndpointJournal{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	role, err := NewRoleEmbedder(resolver, &pointerRoleSource{})
	if err != nil {
		t.Fatal(err)
	}
	embedder, err := role.Resolve(t.Context())
	if err != nil || embedder != nil {
		t.Fatalf("Resolve unconfigured role = (%v, %v), want keyword-only", embedder, err)
	}
}
