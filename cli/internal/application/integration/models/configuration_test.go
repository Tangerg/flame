package models

import (
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestRoleAndProviderChangesHaveExplicitSemantics(t *testing.T) {
	if err := (Role{}).Validate(); err == nil {
		t.Fatal("zero role was accepted")
	}
	if err := InheritedUtilityRole().Validate(); err != nil {
		t.Fatal(err)
	}
	if err := DisabledEmbeddingRole().Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewConfiguredRole(EmbeddingRole, "deepseek", ""); err == nil {
		t.Fatal("half-configured role was constructed")
	}
	if _, err := NewConfiguredRole(UtilityRole, " deepseek", "chat"); err == nil {
		t.Fatal("non-canonical role was constructed")
	}
	// A mode the constructors do set, paired with a kind they never name: the
	// zero value is refused by either check, so this is what proves the kind is
	// its own question.
	if _, err := NewConfiguredRole(RoleKind("neither"), "deepseek", "chat"); err == nil {
		t.Fatal("role of an unknown kind was constructed")
	}
	secret := ValueChange{Kind: protocol.ProviderConfigSet, Value: "secret"}
	update := UpdateProvider{Provider: "deepseek", APIKey: &secret}
	if err := update.Validate(); err != nil {
		t.Fatal(err)
	}
	secret.Value = ""
	if err := update.Validate(); err == nil {
		t.Fatal("empty key update was accepted")
	}
}

func TestProviderConfiguredStateIsRuntimeVerdict(t *testing.T) {
	optionalProvider, err := NewProvider(ProviderSpec{ID: "test-endpoint", Configured: true, EmbeddingCapable: true})
	if err != nil {
		t.Fatal(err)
	}
	if !optionalProvider.Configured() {
		t.Fatal("optional API-key provider was not restored as configured")
	}
	if _, present := optionalProvider.Credential(); present {
		t.Fatal("optional provider invented a credential")
	}

	// Runtime may know readiness inputs the redacted projection omits, so a
	// verdict that disagrees with the visible credential and endpoint stands.
	endpointless, err := NewProvider(ProviderSpec{ID: "compatible", Configured: true, RequiresBaseURL: true})
	if err != nil {
		t.Fatalf("Runtime's configured verdict was rejected: %v", err)
	}
	if !endpointless.Configured() {
		t.Fatal("Runtime's configured verdict was replaced by a client derivation")
	}
	credential, err := NewCredential("sk****ed", protocol.ProviderKeySourceStored)
	if err != nil {
		t.Fatal(err)
	}
	unready, err := NewProvider(ProviderSpec{ID: "openai", Credential: &credential, Configured: false})
	if err != nil {
		t.Fatalf("Runtime's not-configured verdict was rejected: %v", err)
	}
	if unready.Configured() {
		t.Fatal("credential presence overrode Runtime's not-configured verdict")
	}
}
