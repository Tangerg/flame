package modelconfig

import "testing"

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
		ID: "ollama", Configured: true, CredentialRequirement: APIKeyOptional,
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
		ID: "openai", Configured: true, CredentialRequirement: APIKeyRequired,
	}); err == nil {
		t.Fatal("required API-key provider was configured without a credential")
	}
	if _, err := NewProvider(ProviderSpec{
		ID: "compatible", Configured: true, CredentialRequirement: APIKeyOptional, RequiresBaseURL: true,
	}); err == nil {
		t.Fatal("provider was configured without its required endpoint")
	}
}
