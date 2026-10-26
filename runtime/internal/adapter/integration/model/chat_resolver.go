package model

import (
	"context"
	"errors"
	"fmt"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

// InputTokenCounter is the narrow optional provider capability consumed by
// model-context budgeting and discovered alongside ordinary chat resolution.
type InputTokenCounter interface {
	CountInputTokens(context.Context, *corechat.Request) (int64, error)
}

// ResolvedChat is one immutable provider-model construction projected through
// the capabilities Runtime consumes. Its model and optional token counter
// always share the same underlying model and credential snapshot.
type ResolvedChat struct {
	model             corechat.Model
	inputTokenCounter InputTokenCounter
}

// NewResolvedChat validates and freezes one resolved chat construction.
func NewResolvedChat(model corechat.Model, counter InputTokenCounter) (ResolvedChat, error) {
	if lo.IsNil(model) {
		return ResolvedChat{}, errors.New("model: resolved chat model is nil")
	}
	if counter != nil && lo.IsNil(counter) {
		return ResolvedChat{}, errors.New("model: resolved chat input token counter is nil")
	}
	return ResolvedChat{model: model, inputTokenCounter: counter}, nil
}

// Model returns the exact provider instance every other projection is taken
// from. Streaming and token counting are optional capabilities of that one
// instance, so a caller that needs them asks it rather than a second build.
func (r ResolvedChat) Model() corechat.Model { return r.model }

// Streamer returns the streaming projection when this provider has one.
func (r ResolvedChat) Streamer() (corechat.Streamer, bool) {
	streamer, ok := r.model.(corechat.Streamer)
	return streamer, ok
}

// InputTokenCounter returns the optional complete-request counting projection.
func (r ResolvedChat) InputTokenCounter() (InputTokenCounter, bool) {
	return r.inputTokenCounter, r.inputTokenCounter != nil
}

// CredentialLookup is the model-client construction view of the provider
// registry: resolving a chat or embedding client needs only one provider's
// credentials, not list/configure capabilities.
type credentialLookup interface {
	Get(ctx context.Context, id string) (provider.Provider, bool, error)
}

// ChatResolver resolves a per-Run provider model for an explicit model
// selection. The provider is taken as given by the selection and is never
// inferred from the model id; the resolver pulls the provider's current
// configuration from the registry and builds an immutable model. It does not
// retain prior credential generations in a process-lifetime cache.
type ChatResolver struct {
	providers credentialLookup
}

// NewChatResolver returns a complete resolver over the provider configuration lookup.
func NewChatResolver(providers credentialLookup) (*ChatResolver, error) {
	if lo.IsNil(providers) {
		return nil, errors.New("model: chat provider credential lookup is required")
	}
	return &ChatResolver{providers: providers}, nil
}

// ResolveChat builds one provider model from the registry configuration and
// returns all capabilities Runtime consumes from that exact instance. Required
// authentication fails as invalid credentials; optional-key providers may
// resolve without a registry row.
func (c *ChatResolver) ResolveChat(ctx context.Context, selection modelchoice.Selection) (ResolvedChat, error) {
	if c == nil || lo.IsNil(c.providers) {
		return ResolvedChat{}, errors.New("model: chat resolver is not configured")
	}
	spec, err := c.resolveClientSpec(ctx, selection)
	if err != nil {
		return ResolvedChat{}, err
	}
	client, counter, err := buildChat(ctx, spec, selection.Model())
	if err != nil {
		return ResolvedChat{}, err
	}
	return NewResolvedChat(client, counter)
}

func (c *ChatResolver) resolveClientSpec(ctx context.Context, selection modelchoice.Selection) (clientSpec, error) {
	if err := selection.ValidateExact(); err != nil {
		return clientSpec{}, fmt.Errorf("model: chat selection: %w", err)
	}
	providerID := selection.Provider()
	entry, ok, err := c.providers.Get(ctx, providerID)
	if err != nil {
		return clientSpec{}, err
	}
	if !ok {
		entry, err = provider.New(providerID)
		if err != nil {
			return clientSpec{}, err
		}
	}

	inputs, err := resolveProviderClientInputs(providerID, entry)
	if err != nil {
		return clientSpec{}, err
	}
	spec, err := inputs.clientSpec()
	if err != nil {
		if errors.Is(err, errCredentialUnavailable) {
			return clientSpec{}, &run.FailureError{
				Kind: run.FailureInvalidCredentials,
				Err:  fmt.Errorf("model: provider %q requires an API key", providerID),
			}
		}
		return clientSpec{}, err
	}
	return spec, nil
}

// ValidateChatModel implements the application model-role validation port
// without leaking the concrete chat client into the use-case layer.
func (c *ChatResolver) ValidateChatModel(ctx context.Context, providerID, model string) error {
	selection, err := modelchoice.New(providerID, model)
	if err != nil {
		return err
	}
	_, err = c.ResolveChat(ctx, selection)
	return err
}
