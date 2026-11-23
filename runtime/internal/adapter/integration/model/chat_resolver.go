package model

import (
	"context"
	"errors"
	"fmt"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/catalog"
	"github.com/samber/lo"
)

type InputTokenCounter interface {
	CountInputTokens(context.Context, *corechat.Request) (int64, error)
}

type ResolvedChat struct {
	model             corechat.Model
	inputTokenCounter InputTokenCounter
	metadata          *catalog.Model
	capture           *ProviderCapture
	pricing           accounting.Pricing
	close             func(context.Context) error
	fits              func(context.Context, *corechat.Request) (bool, error)
}

// The provider owns pricing, release and the request-fit check; a token
// counter and catalog metadata are capabilities a provider may lack.
func NewResolvedChat(
	model corechat.Model,
	counter InputTokenCounter,
	metadata *catalog.Model,
	pricing accounting.Pricing,
	release func(context.Context) error,
	fits func(context.Context, *corechat.Request) (bool, error),
) (ResolvedChat, error) {
	if lo.IsNil(model) || pricing == nil || release == nil || fits == nil {
		return ResolvedChat{}, errors.New("model: resolved chat requires a model, pricing, release and request-fit check")
	}
	if counter != nil && lo.IsNil(counter) {
		return ResolvedChat{}, errors.New("model: resolved chat input token counter is nil")
	}
	if metadata != nil {
		if err := metadata.Validate(); err != nil {
			return ResolvedChat{}, err
		}
		cloned := metadata.Clone()
		metadata = &cloned
	}
	return ResolvedChat{model: model, inputTokenCounter: counter, metadata: metadata, pricing: pricing, close: release, fits: fits}, nil
}

func (r ResolvedChat) Capture() (ProviderCapture, bool) {
	if r.capture == nil {
		return ProviderCapture{}, false
	}
	return r.capture.Clone(), true
}

func (r ResolvedChat) Close(ctx context.Context) error { return r.close(ctx) }

func (r ResolvedChat) WithModel(model corechat.Model) (ResolvedChat, error) {
	if lo.IsNil(model) {
		return ResolvedChat{}, errors.New("model: replacement model is required")
	}
	r.model = model
	return r, nil
}

func (r ResolvedChat) Model() corechat.Model { return r.model }

func (r ResolvedChat) Streamer() (corechat.Streamer, bool) {
	streamer, ok := r.model.(corechat.Streamer)
	return streamer, ok
}

func (r ResolvedChat) InputTokenCounter() (InputTokenCounter, bool) {
	return r.inputTokenCounter, r.inputTokenCounter != nil
}

type credentialLookup interface {
	Get(ctx context.Context, id string) (provider.Provider, bool, error)
}

type ChatResolver struct {
	providers credentialLookup
	clients   chatFactory
}

type chatFactory interface {
	OpenChat(context.Context, flamemodel.Selection, modelprovider.Connection) (ResolvedChat, error)
	RestoreChat(context.Context, flamemodel.Selection, *ProviderCapture) (ResolvedChat, error)
}

func NewChatResolver(providers credentialLookup, clients chatFactory) (*ChatResolver, error) {
	if lo.IsNil(providers) || lo.IsNil(clients) {
		return nil, errors.New("model: chat provider credentials and installed adapter are required")
	}
	return &ChatResolver{providers: providers, clients: clients}, nil
}

func (c *ChatResolver) ResolveChat(ctx context.Context, selection flamemodel.Selection) (ResolvedChat, error) {
	connection, err := resolveConnection(ctx, c.providers, selection)
	if err != nil {
		return ResolvedChat{}, err
	}
	return c.clients.OpenChat(ctx, selection, connection)
}

func (c *ChatResolver) RestoreChat(ctx context.Context, selection flamemodel.Selection, capture *ProviderCapture) (ResolvedChat, error) {
	if err := selection.ValidateExact(); err != nil {
		return ResolvedChat{}, err
	}
	return c.clients.RestoreChat(ctx, selection, capture)
}

func resolveConnection(ctx context.Context, providers credentialLookup, selection flamemodel.Selection) (modelprovider.Connection, error) {
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

func (c *ChatResolver) ValidateChatModel(ctx context.Context, providerID, model string) error {
	return validateByOpening(ctx, providerID, model, c.ResolveChat)
}

// A model is valid when its provider can open it; the handle is released at once.
func validateByOpening[M interface{ Close(context.Context) error }](
	ctx context.Context,
	providerID, model string,
	open func(context.Context, flamemodel.Selection) (M, error),
) error {
	selection, err := flamemodel.New(providerID, model)
	if err != nil {
		return err
	}
	resolved, err := open(ctx, selection)
	if err != nil {
		return err
	}
	return resolved.Close(ctx)
}
