package delivery

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
)

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

func (s *Handler) UpdateProvider(ctx context.Context, in protocol.UpdateProviderRequest) (*protocol.Provider, error) {
	var configured modelapp.ProviderSummary
	if err := s.updateProvider(ctx, in, operation.DirectReply(&configured)); err != nil {
		return nil, err
	}
	out, err := presentProvider(configured)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Handler) commitProviderUpdate(ctx context.Context, in protocol.UpdateProviderRequest, outcome atomicOutcome) error {
	reply := atomicReply(outcome.complete, func(configured modelapp.ProviderSummary) ([]byte, error) {
		out, err := presentProvider(configured)
		if err != nil {
			return nil, err
		}
		return encodeAtomicResult(outcome.method, &out)
	})
	return outcome.settleRefusal(ctx, s.updateProvider(ctx, in, reply))
}

func (s *Handler) updateProvider(ctx context.Context, in protocol.UpdateProviderRequest, complete operation.Reply[modelapp.ProviderSummary]) error {
	apiKey, err := providerConfigChange(in.APIKey, provider.NewAPIKey)
	if err != nil {
		return err
	}
	baseURL, err := providerConfigChange(in.BaseURL, provider.NewBaseURL)
	if err != nil {
		return err
	}
	return mapModelError(s.models.UpdateProvider(ctx, in.Provider, provider.Patch{APIKey: apiKey, BaseURL: baseURL}, complete))
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
			return provider.Change[T]{}, protocol.InvalidParameters(err)
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

func (s *Handler) TestProvider(ctx context.Context, providerID string) (*protocol.ProviderTestResult, error) {
	outcome, err := s.models.TestProvider(ctx, providerID)
	if err != nil {
		return nil, mapModelError(err)
	}
	switch outcome {
	case modelapp.ProviderTestSucceeded:
		return &protocol.ProviderTestResult{Outcome: protocol.ProviderTestReachable}, nil
	case modelapp.ProviderTestNotConfigured:
		return &protocol.ProviderTestResult{Outcome: protocol.ProviderTestNotConfigured}, nil
	case modelapp.ProviderTestInvalidCredentials:
		return &protocol.ProviderTestResult{Outcome: protocol.ProviderTestInvalidCredentials}, nil
	case modelapp.ProviderTestTimedOut:
		return &protocol.ProviderTestResult{Outcome: protocol.ProviderTestTimedOut}, nil
	case modelapp.ProviderTestFailed:
		return &protocol.ProviderTestResult{Outcome: protocol.ProviderTestFailed}, nil
	default:
		return nil, fmt.Errorf("delivery: unknown provider test outcome %q", outcome)
	}
}
