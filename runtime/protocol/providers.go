package protocol

// TestProviderRequest identifies the configured provider to probe.
type TestProviderRequest struct {
	Provider string `json:"provider"`
}

// Provider is one configured LLM provider. Provider list results are ordered by
// ID ascending. The key is returned masked, never reconstructable.
type Provider struct {
	ID                    string                        `json:"id"`
	BaseURL               *string                       `json:"baseUrl,omitempty"`
	Credential            *ProviderCredential           `json:"credential,omitempty"`
	Configured            bool                          `json:"configured"`
	CredentialRequirement ProviderCredentialRequirement `json:"credentialRequirement"`
	// RequiresBaseURL marks providers with no built-in endpoint — the generic
	// "openai-compatible" / "anthropic-compatible" passthroughs and Azure
	// (per-resource URL). The client must collect a base URL when configuring
	// them, and (since they carry no catalog) a free-form model id.
	RequiresBaseURL bool `json:"requiresBaseUrl,omitempty"`
	// EmbeddingCapable marks providers with an embeddings adapter — the set the
	// agent-memory embedding-role picker offers (models.setEmbeddingRole).
	// DefaultEmbeddingModel is a sensible default model id to prefill. It is
	// absent when the id is user-supplied, e.g. an Azure deployment.
	EmbeddingCapable      bool    `json:"embeddingCapable,omitempty"`
	DefaultEmbeddingModel *string `json:"defaultEmbeddingModel,omitempty"`
}

// ProviderCredentialRequirement distinguishes API-key vendors from endpoints
// such as a local Ollama daemon that are usable without authentication. An
// optional provider may still carry a stored or environment credential.
type ProviderCredentialRequirement string

const (
	ProviderAPIKeyRequired ProviderCredentialRequirement = "apiKeyRequired"
	ProviderAPIKeyOptional ProviderCredentialRequirement = "apiKeyOptional"
)

// ProviderCredential is the redacted projection of actual secret material.
// Its absence does not imply the provider is unconfigured: an optional-key
// endpoint may be ready without carrying a credential.
type ProviderCredential struct {
	Masked string            `json:"masked"`
	Source ProviderKeySource `json:"source"`
}

// ProviderKeySource records where the visible API key originates.
type ProviderKeySource string

const (
	ProviderKeySourceStored ProviderKeySource = "stored"
	ProviderKeySourceEnv    ProviderKeySource = "env"
)

// ProviderConfigChangeType is the operation applied to one persisted provider
// setting. Omitting the enclosing request field preserves the stored value.
type ProviderConfigChangeType string

const (
	ProviderConfigSet   ProviderConfigChangeType = "set"
	ProviderConfigClear ProviderConfigChangeType = "clear"
)

// ProviderConfigChange is an explicit provider-setting mutation. Set requires
// a non-empty value; clear carries no value. Its tagged shape avoids overloading
// an empty string or JSON null with hidden update semantics.
type ProviderConfigChange struct {
	Type  ProviderConfigChangeType `json:"type"`
	Value *string                  `json:"value,omitempty"`
}

// UpdateProviderRequest — providers.update body. Provider is the
// provider id (Provider.id), e.g. "deepseek" — a meaningful slug, named to
// match the `provider` reference field elsewhere (Model.provider,
// runs.start), not "providerId".
// Omitted configuration fields are preserved; each present field explicitly
// sets or clears its stored value.
type UpdateProviderRequest struct {
	Provider string                `json:"provider"`
	BaseURL  *ProviderConfigChange `json:"baseUrl,omitempty"`
	APIKey   *ProviderConfigChange `json:"apiKey,omitempty"`
}

// ProviderTestResult — providers.test result.
type ProviderTestResult struct {
	OK    bool         `json:"ok"`
	Error *ProblemData `json:"error,omitempty"`
}
