// Package modelclient resolves per-(provider, model) chat and embedding clients
// from the runtime-mutable provider registry credentials, caching by the
// credential tuple so a credential mutation (new key or base URL) is picked up
// rather than serving a stale client. It is the driven adapter the runtime's
// per-run model selection, utility-model role, and Agent Memory embedding role all
// resolve through.
package modelclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/infra/llm"
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
		return nil, errors.New("modelclient: explicit model selection is required")
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
		if errors.Is(err, ErrCredentialUnavailable) {
			return nil, &run.FailureError{
				Kind: run.FailureInvalidCredentials,
				Err:  fmt.Errorf("modelclient: provider %q requires an API key", providerID),
			}
		}
		return nil, err
	}
	spec, err := inputs.clientSpec(providerID, model)
	if err != nil {
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
