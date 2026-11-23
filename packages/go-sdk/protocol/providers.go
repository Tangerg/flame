package protocol

import flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"

type TestProviderRequest struct {
	Provider string `json:"provider"`
}

type Provider struct {
	ID                    string                        `json:"id"`
	BaseURL               *string                       `json:"baseUrl,omitzero"`
	Credential            *ProviderCredential           `json:"credential,omitzero"`
	Configured            bool                          `json:"configured"`
	CredentialRequirement ProviderCredentialRequirement `json:"credentialRequirement"`
	Endpoint              flamemodel.EndpointPolicy     `json:"endpoint,omitzero"`
	RequiresBaseURL       bool                          `json:"requiresBaseUrl,omitzero"`
	EmbeddingCapable      bool                          `json:"embeddingCapable,omitzero"`
	DefaultEmbeddingModel *string                       `json:"defaultEmbeddingModel,omitzero"`
}

//flame:enum
type ProviderCredentialRequirement string

const (
	ProviderAPIKeyRequired ProviderCredentialRequirement = "apiKeyRequired"
	ProviderAPIKeyOptional ProviderCredentialRequirement = "apiKeyOptional"
)

type ProviderCredential struct {
	Masked string            `json:"masked"`
	Source ProviderKeySource `json:"source"`
}

//flame:enum
type ProviderKeySource string

const (
	ProviderKeySourceStored ProviderKeySource = "stored"
	ProviderKeySourceEnv    ProviderKeySource = "env"
)

//flame:enum
type ProviderConfigChangeType string

const (
	ProviderConfigSet   ProviderConfigChangeType = "set"
	ProviderConfigClear ProviderConfigChangeType = "clear"
)

type ProviderConfigChange struct {
	Type  ProviderConfigChangeType `json:"type"`
	Value *string                  `json:"value,omitzero"`
}

type UpdateProviderRequest struct {
	Provider string                `json:"provider"`
	BaseURL  *ProviderConfigChange `json:"baseUrl,omitzero"`
	APIKey   *ProviderConfigChange `json:"apiKey,omitzero"`
}

//flame:enum
type ProviderTestOutcome string

const (
	ProviderTestReachable          ProviderTestOutcome = "reachable"
	ProviderTestNotConfigured      ProviderTestOutcome = "notConfigured"
	ProviderTestInvalidCredentials ProviderTestOutcome = "invalidCredentials"
	ProviderTestTimedOut           ProviderTestOutcome = "timedOut"
	ProviderTestFailed             ProviderTestOutcome = "failed"
)

type ProviderTestResult struct {
	Outcome ProviderTestOutcome `json:"outcome"`
}
