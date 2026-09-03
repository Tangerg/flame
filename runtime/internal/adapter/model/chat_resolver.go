package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/llm"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

// InputTokenCounter is the narrow optional provider capability consumed by
// model-context budgeting and discovered alongside ordinary chat resolution.
type InputTokenCounter interface {
	CountInputTokens(context.Context, *corechat.Request) (int64, error)
}

// ResolvedChat is one immutable provider-model construction projected through
// the capabilities Runtime consumes. Its client and optional token counter
// always share the same underlying model and credential snapshot.
type ResolvedChat struct {
	client            *chatclient.Client
	inputTokenCounter InputTokenCounter
}

// NewResolvedChat validates and freezes one resolved chat construction.
func NewResolvedChat(client *chatclient.Client, counter InputTokenCounter) (ResolvedChat, error) {
	if client == nil {
		return ResolvedChat{}, errors.New("model: resolved chat client is nil")
	}
	if counter != nil && missingDependency(counter) {
		return ResolvedChat{}, errors.New("model: resolved chat input token counter is nil")
	}
	return ResolvedChat{client: client, inputTokenCounter: counter}, nil
}

// Client returns the ordinary chat projection.
func (r ResolvedChat) Client() *chatclient.Client { return r.client }

// InputTokenCounter returns the optional complete-request counting projection.
func (r ResolvedChat) InputTokenCounter() (InputTokenCounter, bool) {
	return r.inputTokenCounter, r.inputTokenCounter != nil
}

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

// NewChatResolver returns a complete resolver over the provider configuration lookup.
func NewChatResolver(providers CredentialLookup) (*ChatResolver, error) {
	if missingDependency(providers) {
		return nil, errors.New("model: chat provider credential lookup is required")
	}
	return &ChatResolver{providers: providers}, nil
}

// ResolveChat builds one provider model from the registry configuration and
// returns all capabilities Runtime consumes from that exact instance. Required
// authentication fails as invalid credentials; optional-key providers may
// resolve without a registry row.
func (c *ChatResolver) ResolveChat(ctx context.Context, selection modelref.Selection) (ResolvedChat, error) {
	if c == nil || missingDependency(c.providers) {
		return ResolvedChat{}, errors.New("model: chat resolver is not configured")
	}
	spec, err := c.resolveClientSpec(ctx, selection)
	if err != nil {
		return ResolvedChat{}, err
	}
	client, counter, err := llm.BuildChat(spec)
	if err != nil {
		return ResolvedChat{}, err
	}
	return NewResolvedChat(client, counter)
}

func (c *ChatResolver) resolveClientSpec(ctx context.Context, selection modelref.Selection) (llm.ClientSpec, error) {
	if err := selection.ValidateExact(); err != nil {
		return llm.ClientSpec{}, fmt.Errorf("model: chat selection: %w", err)
	}
	providerID, model := selection.Provider(), selection.Model()
	entry, ok, err := c.providers.Get(ctx, providerID)
	if err != nil {
		return llm.ClientSpec{}, err
	}
	if !ok {
		entry, err = provider.New(providerID)
		if err != nil {
			return llm.ClientSpec{}, err
		}
	}

	inputs, err := resolveProviderClientInputs(providerID, entry)
	if err != nil {
		return llm.ClientSpec{}, err
	}
	spec, err := inputs.clientSpec(model)
	if err != nil {
		if errors.Is(err, ErrCredentialUnavailable) {
			return llm.ClientSpec{}, &run.FailureError{
				Kind: run.FailureInvalidCredentials,
				Err:  fmt.Errorf("model: provider %q requires an API key", providerID),
			}
		}
		return llm.ClientSpec{}, err
	}
	return spec, nil
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
