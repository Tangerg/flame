package model

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

type pointerCredentialLookup struct{}

func (*pointerCredentialLookup) Get(context.Context, string) (provider.Provider, bool, error) {
	return provider.Provider{}, false, nil
}

type pointerChatResolver struct{}

func (*pointerChatResolver) ResolveChat(context.Context, modelref.Selection) (ResolvedChat, error) {
	return ResolvedChat{}, nil
}

type pointerRoleSource struct{}

func (*pointerRoleSource) Role() modelref.Selection { return modelref.Selection{} }

func TestModelResolversRejectMissingCredentialLookup(t *testing.T) {
	var lookup *pointerCredentialLookup
	if resolver, err := NewChatResolver(lookup); err == nil || resolver != nil {
		t.Fatalf("NewChatResolver typed-nil lookup = (%v, %v)", resolver, err)
	}
	if resolver, err := NewEmbeddingResolver(lookup); err == nil || resolver != nil {
		t.Fatalf("NewEmbeddingResolver typed-nil lookup = (%v, %v)", resolver, err)
	}
}

func TestLiveRoleResolversRejectIncompleteConstruction(t *testing.T) {
	selection := mustRoleSelection(t, "deepseek", "deepseek-chat")
	role := &pointerRoleSource{}
	resolver := &pointerChatResolver{}

	var nilResolver *pointerChatResolver
	if _, err := LiveUtilityClient(nilResolver, selection, role); err == nil || !strings.Contains(err.Error(), "chat resolver") {
		t.Fatalf("LiveUtilityClient typed-nil resolver error = %v", err)
	}
	if _, err := LiveUtilityClient(resolver, modelref.Selection{}, role); err == nil || !strings.Contains(err.Error(), "model selection") {
		t.Fatalf("LiveUtilityClient empty main selection error = %v", err)
	}
	var nilRole *pointerRoleSource
	if _, err := LiveUtilityClient(resolver, selection, nilRole); err == nil || !strings.Contains(err.Error(), "role source") {
		t.Fatalf("LiveUtilityClient typed-nil role error = %v", err)
	}
	if _, err := NewRoleEmbedder(nil, role); err == nil || !strings.Contains(err.Error(), "embedding resolver") {
		t.Fatalf("NewRoleEmbedder nil resolver error = %v", err)
	}

	embeddingResolver, err := NewEmbeddingResolver(&pointerCredentialLookup{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRoleEmbedder(embeddingResolver, nilRole); err == nil || !strings.Contains(err.Error(), "role source") {
		t.Fatalf("NewRoleEmbedder typed-nil role error = %v", err)
	}
}

func TestUnconfiguredEmbeddingRoleIsTheOnlyKeywordOnlyState(t *testing.T) {
	resolver, err := NewEmbeddingResolver(&pointerCredentialLookup{})
	if err != nil {
		t.Fatal(err)
	}
	role, err := NewRoleEmbedder(resolver, &pointerRoleSource{})
	if err != nil {
		t.Fatal(err)
	}
	embedder, err := role.ResolveMemory(t.Context())
	if err != nil || embedder != nil {
		t.Fatalf("ResolveMemory unconfigured role = (%v, %v), want keyword-only", embedder, err)
	}
}
