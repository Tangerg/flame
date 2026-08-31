package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/llm"
	"github.com/Tangerg/scope/core/chatclient"
)

// CredentialLookup is the model-client construction view of the provider
// registry: resolving a chat or embedding client needs only one provider's
// credentials, not list/configure capabilities.
type CredentialLookup interface {
	Get(ctx context.Context, id string) (provider.Provider, bool, error)
}

// ChatResolver resolves a per-Run [chatclient.Client] for an explicit model
// selection. The provider is taken as given by the selection and is never
// inferred from the model id; the resolver pulls the provider's current
// configuration from the registry and builds an immutable client. It does not
// retain prior credential generations in a process-lifetime cache.
type ChatResolver struct {
	providers CredentialLookup
}

// NewChatResolver returns a chat resolver over the provider configuration lookup.
func NewChatResolver(providers CredentialLookup) *ChatResolver {
	return &ChatResolver{providers: providers}
}

// ResolveChat returns the chat client for selection, building it from the
// provider's registry configuration. Required authentication fails as invalid
// credentials; optional-key providers may resolve without a registry row.
func (c *ChatResolver) ResolveChat(ctx context.Context, selection modelref.Selection) (*chatclient.Client, error) {
	if !selection.Configured() {
		return nil, errors.New("model: explicit model selection is required")
	}
	providerID, model := selection.Provider(), selection.Model()
	entry, ok, err := c.providers.Get(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		entry, err = provider.New(providerID)
		if err != nil {
			return nil, err
		}
	}

	inputs, err := resolveProviderClientInputs(providerID, entry)
	if err != nil {
		return nil, err
	}
	spec, err := inputs.clientSpec(model)
	if err != nil {
		if errors.Is(err, ErrCredentialUnavailable) {
			return nil, &run.FailureError{
				Kind: run.FailureInvalidCredentials,
				Err:  fmt.Errorf("model: provider %q requires an API key", providerID),
			}
		}
		return nil, err
	}
	client, err := llm.BuildClient(spec)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// ValidateChatModel implements the application model-role validation port
// without leaking the concrete chat client into the use-case layer.
func (c *ChatResolver) ValidateChatModel(ctx context.Context, providerID, model string) error {
	selection, err := modelref.New(providerID, model)
	if err != nil {
		return err
	}
	_, err = c.ResolveChat(ctx, selection)
	return err
}
