package model

import (
	"context"
	"strings"
	"testing"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
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

type pointerChatResolver struct{}

func (*pointerChatResolver) ResolveChat(context.Context, modelchoice.Selection) (ResolvedChat, error) {
	return ResolvedChat{}, nil
}

type pointerRoleSource struct{}

func (*pointerRoleSource) Role() modelref.Role { return modelref.Role{} }

func TestModelResolversRejectMissingCredentialLookup(t *testing.T) {
	var lookup *pointerCredentialLookup
	if resolver, err := NewChatResolver(lookup, &fixtureModelFactory{}); err == nil || resolver != nil {
		t.Fatalf("NewChatResolver typed-nil lookup = (%v, %v)", resolver, err)
	}
	if resolver, err := NewEmbeddingResolver(lookup, &fixtureModelFactory{}, fakeEndpointJournal{}, nil); err == nil || resolver != nil {
		t.Fatalf("NewEmbeddingResolver typed-nil lookup = (%v, %v)", resolver, err)
	}
}

func TestLiveRoleResolversRejectIncompleteConstruction(t *testing.T) {
	selection := testsupport.MustModelSelection("deepseek", "deepseek-chat")
	role := &pointerRoleSource{}
	resolver := &pointerChatResolver{}

	var nilResolver *pointerChatResolver
	if _, err := LiveUtilityModel(nilResolver, selection, role, nil); err == nil || !strings.Contains(err.Error(), "chat resolver") {
		t.Fatalf("LiveUtilityModel typed-nil resolver error = %v", err)
	}
	if _, err := LiveUtilityModel(resolver, modelchoice.Selection{}, role, nil); err == nil || !strings.Contains(err.Error(), "model selection") {
		t.Fatalf("LiveUtilityModel empty main selection error = %v", err)
	}
	var nilRole *pointerRoleSource
	if _, err := LiveUtilityModel(resolver, selection, nilRole, nil); err == nil || !strings.Contains(err.Error(), "role source") {
		t.Fatalf("LiveUtilityModel typed-nil role error = %v", err)
	}
	if _, err := NewRoleEmbedder(nil, role); err == nil || !strings.Contains(err.Error(), "embedding resolver") {
		t.Fatalf("NewRoleEmbedder nil resolver error = %v", err)
	}

	embeddingResolver, err := NewEmbeddingResolver(&pointerCredentialLookup{}, &fixtureModelFactory{}, fakeEndpointJournal{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRoleEmbedder(embeddingResolver, nilRole); err == nil || !strings.Contains(err.Error(), "role source") {
		t.Fatalf("NewRoleEmbedder typed-nil role error = %v", err)
	}
}

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

func (r *pointerChatResolver) RestoreChat(ctx context.Context, selection modelchoice.Selection, _ *ProviderCapture) (ResolvedChat, error) {
	return r.ResolveChat(ctx, selection)
}
