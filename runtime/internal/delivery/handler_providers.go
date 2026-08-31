package delivery

import (
	"context"
	"fmt"

	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ListProviders projects the application-owned supported-provider set onto the
// protocol page. The application combines static support and runtime state.
func (s *Handler) ListProviders(ctx context.Context) (*protocol.Page[protocol.Provider], error) {
	providers, err := s.models.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Provider, 0, len(providers))
	for _, provider := range providers {
		wire, err := presentProvider(provider)
		if err != nil {
			return nil, err
		}
		out = append(out, wire)
	}
	return protocol.NewPage(out), nil
}

// UpdateProvider validates and persists one provider through the application
// use case, then projects its redacted result onto the wire.
func (s *Handler) UpdateProvider(ctx context.Context, in protocol.UpdateProviderRequest) (*protocol.Provider, error) {
	apiKey, err := providerConfigChange(in.APIKey, provider.NewAPIKey)
	if err != nil {
		return nil, err
	}
	baseURL, err := providerConfigChange(in.BaseURL, provider.NewBaseURL)
	if err != nil {
		return nil, err
	}
	configured, err := s.models.UpdateProvider(ctx, modelapp.UpdateProviderCommand{
		ID:      in.Provider,
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
	if err != nil {
		return nil, mapModelError(err)
	}
	out, err := presentProvider(configured)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func providerConfigChange[T any](change *protocol.ProviderConfigChange, parse func(string) (T, error)) (provider.Change[T], error) {
	if change == nil {
		return provider.Preserve[T](), nil
	}
	switch change.Type {
	case protocol.ProviderConfigSet:
		if change.Value == nil {
			return provider.Change[T]{}, protocol.ErrInvalidParams
		}
		value, err := parse(*change.Value)
		if err != nil {
			return provider.Change[T]{}, protocol.ErrInvalidParams
		}
		return provider.Set(value), nil
	case protocol.ProviderConfigClear:
		if change.Value != nil {
			return provider.Change[T]{}, protocol.ErrInvalidParams
		}
		return provider.Clear[T](), nil
	default:
		return provider.Change[T]{}, protocol.ErrInvalidParams
	}
}

// TestProvider returns an inline verdict for a supported, configured provider.
// The application owns eligibility and probing; Delivery selects the protocol
// failure envelope.
func (s *Handler) TestProvider(ctx context.Context, providerID string) (*protocol.ProviderTestResult, error) {
	outcome, err := s.models.TestProvider(ctx, providerID)
	if err != nil {
		return nil, mapModelError(err)
	}
	switch outcome {
	case modelapp.ProviderTestSucceeded:
		return &protocol.ProviderTestResult{OK: true}, nil
	case modelapp.ProviderTestNotConfigured:
		return &protocol.ProviderTestResult{OK: false, Error: &protocol.ProblemData{
			Type: protocol.ProblemProviderNotConfigured,
		}}, nil
	case modelapp.ProviderTestFailed:
		return &protocol.ProviderTestResult{OK: false, Error: &protocol.ProblemData{
			Type: protocol.ProblemProviderTestFailed,
		}}, nil
	default:
		return nil, fmt.Errorf("delivery: unknown provider test outcome %q", outcome)
	}
}
