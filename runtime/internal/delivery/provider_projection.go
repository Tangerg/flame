package delivery

import (
	"fmt"

	modelapp "github.com/Tangerg/flame/runtime/internal/application/models"
	"github.com/Tangerg/flame/runtime/internal/domain/provider"
	"github.com/Tangerg/flame/runtime/protocol"
)

func presentProvider(info modelapp.ProviderSummary) (protocol.Provider, error) {
	var credential *protocol.ProviderCredential
	if info.Credential != nil {
		keySource, err := presentProviderKeySource(info.Credential.Source)
		if err != nil {
			return protocol.Provider{}, err
		}
		credential = &protocol.ProviderCredential{
			Masked: info.Credential.Masked,
			Source: keySource,
		}
	}
	credentialRequirement := protocol.ProviderAPIKeyOptional
	if info.RequiresAPIKey {
		credentialRequirement = protocol.ProviderAPIKeyRequired
	}
	return protocol.Provider{
		ID: info.ID, BaseURL: info.BaseURL, Credential: credential,
		Configured: info.Configured, CredentialRequirement: credentialRequirement,
		RequiresBaseURL: info.RequiresBaseURL, EmbeddingCapable: info.EmbeddingCapable,
		DefaultEmbeddingModel: info.DefaultEmbeddingModel,
	}, nil
}

func presentProviderKeySource(source provider.KeySource) (protocol.ProviderKeySource, error) {
	switch source {
	case provider.KeyStored:
		return protocol.ProviderKeySourceStored, nil
	case provider.KeyEnvironment:
		return protocol.ProviderKeySourceEnv, nil
	default:
		return "", fmt.Errorf("providers: unsupported key source %q", source)
	}
}
