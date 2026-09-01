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
	secret := ValueChange{Kind: SetValue, Value: "secret"}
	update := UpdateProvider{Provider: "deepseek", APIKey: &secret}
	if err := update.Validate(); err != nil {
		t.Fatal(err)
	}
	secret.Value = ""
	if err := update.Validate(); err == nil {
		t.Fatal("empty key update was accepted")
	}
}

func TestProviderConfiguredStateIsNotCredentialPresence(t *testing.T) {
	ollama, err := NewProvider(ProviderSpec{
		ID: "ollama", Configured: true, CredentialRequirement: protocol.ProviderAPIKeyOptional,
		EmbeddingCapable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ollama.Configured() || ollama.RequiresAPIKey() {
		t.Fatal("optional API-key provider was not restored as configured")
	}
	if _, present := ollama.Credential(); present {
		t.Fatal("optional provider invented a credential")
	}

	if _, err := NewProvider(ProviderSpec{
		ID: "openai", Configured: true, CredentialRequirement: protocol.ProviderAPIKeyRequired,
	}); err == nil {
		t.Fatal("required API-key provider was configured without a credential")
	}
	if _, err := NewProvider(ProviderSpec{
		ID: "compatible", Configured: true, CredentialRequirement: protocol.ProviderAPIKeyOptional, RequiresBaseURL: true,
	}); err == nil {
		t.Fatal("provider was configured without its required endpoint")
	}

	credential, err := NewCredential("sk****ed", protocol.ProviderKeySourceStored)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewProvider(ProviderSpec{
		ID: "openai", Credential: &credential, Configured: false, CredentialRequirement: protocol.ProviderAPIKeyRequired,
	}); err == nil {
		t.Fatal("ready provider was accepted as not configured")
	}
	if _, err := NewProvider(ProviderSpec{
		ID: "ollama", Configured: false, CredentialRequirement: protocol.ProviderAPIKeyOptional,
	}); err == nil {
		t.Fatal("keyless provider with a built-in endpoint was accepted as not configured")
	}

	endpoint := "https://gateway.example/v1"
	partial, err := NewProvider(ProviderSpec{
		ID: "openai-compatible", BaseURL: &endpoint, Configured: false,
		CredentialRequirement: protocol.ProviderAPIKeyRequired, RequiresBaseURL: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if partial.Configured() {
		t.Fatal("provider without its required credential became configured")
	}
}
