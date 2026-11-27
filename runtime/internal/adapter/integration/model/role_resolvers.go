package model

import (
	"context"
	"errors"
	"fmt"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel/trace"
)

type roleSource interface {
	Load(context.Context) (modelref.Role, bool, error)
}

type ChatModelResolver interface {
	ResolveChat(context.Context, flamemodel.Selection) (ResolvedChat, error)
	RestoreChat(context.Context, flamemodel.Selection, *ProviderCapture) (ResolvedChat, error)
}

func LiveUtilityModel(
	resolver ChatModelResolver,
	defaultSelection flamemodel.Selection,
	utilityRole roleSource,
	selector modelapp.Selector,
) (AuxiliaryResolver, error) {
	if lo.IsNil(resolver) {
		return nil, errors.New("model: utility chat resolver is required")
	}
	if err := defaultSelection.ValidateExact(); err != nil {
		return nil, fmt.Errorf("model: utility default selection: %w", err)
	}
	if lo.IsNil(utilityRole) {
		return nil, errors.New("model: utility role source is required")
	}
	if selector != nil && lo.IsNil(selector) {
		return nil, errors.New("model: utility selector is typed nil")
	}
	return func(ctx context.Context, budget AuxiliaryBudget) (SelectedChat, func() error, error) {
		if err := budget.Validate(); err != nil {
			return SelectedChat{}, nil, err
		}
		role, _, err := utilityRole.Load(ctx)
		if err != nil {
			return SelectedChat{}, nil, fmt.Errorf("auxiliary model: read utility role: %w", err)
		}
		selection := defaultSelection
		if role.Configured() {
			selection = role.Selection()
		}
		if selector != nil {
			selected, err := selector.SelectModel(ctx, modelapp.SelectionInput{
				Context: flamemodel.SelectionContext{Use: flamemodel.Utility, Limits: &flamemodel.SelectionLimits{
					InputKind: flamemodel.TextBytes, MaxInputBytes: int64(budget.MaxInputBytes), MaxOutputTokens: budget.MaxOutputTokens,
				}},
				Proposed: selection,
			})
			if err != nil {
				return SelectedChat{}, nil, fmt.Errorf("auxiliary model: select route: %w", err)
			}
			if err := selected.ValidateExact(); err != nil {
				return SelectedChat{}, nil, err
			}
			selection = selected
		}
		trace.SpanFromContext(ctx).SetAttributes(
			telemetry.ProviderIDKey.String(selection.Provider()),
			telemetry.ModelKey.String(selection.Model()),
		)
		resolved, err := resolver.ResolveChat(ctx, selection)
		if err != nil {
			return SelectedChat{}, nil, fmt.Errorf(
				"auxiliary model: resolve %s/%s: %w",
				selection.Provider(),
				selection.Model(),
				err,
			)
		}
		capture, err := NewSelectedChat(selection, resolved)
		if err != nil {
			return SelectedChat{}, nil, errors.Join(err, resolved.Close(ctx))
		}
		return capture, func() error { return resolved.Close(ctx) }, nil
	}, nil
}

type RoleEmbedder struct {
	resolver *EmbeddingResolver
	role     roleSource
}

func NewRoleEmbedder(resolver *EmbeddingResolver, role roleSource) (*RoleEmbedder, error) {
	if resolver == nil {
		return nil, errors.New("model: embedding resolver is required")
	}
	if lo.IsNil(role) {
		return nil, errors.New("model: embedding role source is required")
	}
	return &RoleEmbedder{resolver: resolver, role: role}, nil
}

func (r *RoleEmbedder) Resolve(ctx context.Context) (*Embedder, bool, error) {
	role, _, err := r.role.Load(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("model: read embedding role: %w", err)
	}
	if !role.Configured() {
		return nil, false, nil
	}
	embedder, err := r.resolver.Resolve(ctx, role.Selection())
	return embedder, err == nil, err
}
