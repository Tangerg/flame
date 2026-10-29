package model

import (
	"context"
	"errors"
	"fmt"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/catalog"
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
	metadata          *catalog.Model
	capture           *ProviderCapture
	pricing           accounting.Pricing
	close             func(context.Context) error
	fits              func(context.Context, *corechat.Request) (bool, error)
}

// NewResolvedChat validates and freezes one resolved chat construction.
func NewResolvedChat(model corechat.Model, counter InputTokenCounter, metadata *catalog.Model) (ResolvedChat, error) {
	if lo.IsNil(model) {
		return ResolvedChat{}, errors.New("model: resolved chat model is nil")
	}
	if counter != nil && lo.IsNil(counter) {
		return ResolvedChat{}, errors.New("model: resolved chat input token counter is nil")
	}
	if metadata != nil {
		if err := metadata.Validate(); err != nil {
			return ResolvedChat{}, err
		}
		copy := metadata.Clone()
		metadata = &copy
	}
	return ResolvedChat{model: model, inputTokenCounter: counter, metadata: metadata}, nil
}

func (r ResolvedChat) Capture() (ProviderCapture, bool) {
	if r.capture == nil {
		return ProviderCapture{}, false
	}
	return r.capture.Clone(), true
}

func (r ResolvedChat) Close(ctx context.Context) error {
	if r.close != nil {
		return r.close(ctx)
	}
	return nil
}

func (r ResolvedChat) WithModel(model corechat.Model) (ResolvedChat, error) {
	if lo.IsNil(model) {
		return ResolvedChat{}, errors.New("model: replacement model is required")
	}
	r.model = model
	return r, nil
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
	clients   chatFactory
}

type chatFactory interface {
	OpenChat(context.Context, modelchoice.Selection, modelprovider.Connection) (ResolvedChat, error)
	RestoreChat(context.Context, modelchoice.Selection, *ProviderCapture) (ResolvedChat, error)
}

// NewChatResolver returns a complete resolver over the provider configuration lookup.
func NewChatResolver(providers credentialLookup, clients chatFactory) (*ChatResolver, error) {
	if lo.IsNil(providers) || lo.IsNil(clients) {
		return nil, errors.New("model: chat provider credentials and installed adapter are required")
	}
	return &ChatResolver{providers: providers, clients: clients}, nil
}

// ResolveChat builds one provider model from the registry configuration and
// returns all capabilities Runtime consumes from that exact instance. Required
// authentication fails as invalid credentials; optional-key providers may
// resolve without a registry row.
func (c *ChatResolver) ResolveChat(ctx context.Context, selection modelchoice.Selection) (ResolvedChat, error) {
	if c == nil || lo.IsNil(c.providers) {
		return ResolvedChat{}, errors.New("model: chat resolver is not configured")
	}
	connection, err := resolveConnection(ctx, c.providers, selection)
	if err != nil {
		return ResolvedChat{}, err
	}
	return c.clients.OpenChat(ctx, selection, connection)
}

func (c *ChatResolver) RestoreChat(ctx context.Context, selection modelchoice.Selection, capture *ProviderCapture) (ResolvedChat, error) {
	if err := selection.ValidateExact(); err != nil {
		return ResolvedChat{}, err
	}
	return c.clients.RestoreChat(ctx, selection, capture)
}

func resolveConnection(ctx context.Context, providers credentialLookup, selection modelchoice.Selection) (modelprovider.Connection, error) {
	if err := selection.ValidateExact(); err != nil {
		return modelprovider.Connection{}, fmt.Errorf("model: selection: %w", err)
	}
	providerID := selection.Provider()
	entry, ok, err := providers.Get(ctx, providerID)
	if err != nil {
		return modelprovider.Connection{}, err
	}
	if !ok {
		entry, err = provider.New(providerID)
		if err != nil {
			return modelprovider.Connection{}, err
		}
	}

	return connectionFromProvider(providerID, entry)
}

func connectionFromProvider(providerID string, entry provider.Provider) (modelprovider.Connection, error) {
	if entry.ID() != providerID {
		return modelprovider.Connection{}, errors.New("model: provider registry identity mismatch")
	}
	connection := modelprovider.Connection{Provider: providerID}
	if key, present := entry.APIKey(); present {
		connection.APIKey = key.Reveal()
	}
	if endpoint, present := entry.BaseURL(); present {
		connection.BaseURL = endpoint.String()
	}
	return connection, connection.Validate()
}

// ValidateChatModel implements the application model-role validation port
// without leaking the concrete chat client into the use-case layer.
func (c *ChatResolver) ValidateChatModel(ctx context.Context, providerID, model string) error {
	selection, err := modelchoice.New(providerID, model)
	if err != nil {
		return err
	}
	resolved, err := c.ResolveChat(ctx, selection)
	if err != nil {
		return err
	}
	return resolved.Close(ctx)
}
