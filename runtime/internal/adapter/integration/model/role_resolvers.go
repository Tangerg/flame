package model

import (
	"context"
	"errors"
	"fmt"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// The source's owner decides how role changes are synchronized.
type roleSource interface {
	Role() modelref.Role
}

// ChatModelResolver resolves an exact selection from the current provider
// configuration snapshot. The utility role adapter depends only on that
// behavior, not on ChatResolver's registry implementation.
type ChatModelResolver interface {
	ResolveChat(context.Context, modelchoice.Selection) (ResolvedChat, error)
	RestoreChat(context.Context, modelchoice.Selection, *ProviderCapture) (ResolvedChat, error)
}

// LiveUtilityModel resolves the optional specialized role on every use. An
// absent role proposes the Runtime default; a configured role proposes its
// exact selection. An installed selector may replace that proposal before
// resolution, whose failure never triggers another route.
func LiveUtilityModel(
	resolver ChatModelResolver,
	defaultSelection modelchoice.Selection,
	roles roleSource,
	selector modelapp.Selector,
) (AuxiliaryResolver, error) {
	if lo.IsNil(resolver) {
		return nil, errors.New("model: utility chat resolver is required")
	}
	if err := defaultSelection.ValidateExact(); err != nil {
		return nil, fmt.Errorf("model: utility default selection: %w", err)
	}
	if lo.IsNil(roles) {
		return nil, errors.New("model: utility role source is required")
	}
	if selector != nil && lo.IsNil(selector) {
		return nil, errors.New("model: utility selector is typed nil")
	}
	return func(ctx context.Context, budget AuxiliaryBudget) (SelectedChat, func() error, error) {
		if err := budget.Validate(); err != nil {
			return SelectedChat{}, nil, err
		}
		selection := defaultSelection
		if role := roles.Role(); role.Configured() {
			selection = role.Selection()
		}
		if selector != nil {
			selected, err := selector.SelectModel(ctx, modelapp.SelectionInput{
				Context: modelchoice.SelectionContext{Use: modelchoice.Utility, Limits: &modelchoice.SelectionLimits{
					InputKind: modelchoice.TextBytes, MaxInputBytes: int64(budget.MaxInputBytes), MaxOutputTokens: budget.MaxOutputTokens,
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
			attribute.String("flame.provider.id", selection.Provider()),
			attribute.String("gen_ai.request.model", selection.Model()),
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
			return SelectedChat{}, nil, errors.Join(err, resolved.Close(context.WithoutCancel(ctx)))
		}
		return capture, func() error { return resolved.Close(context.WithoutCancel(ctx)) }, nil
	}, nil
}

// RoleEmbedder resolves the live embedding role through an embedding resolver.
type RoleEmbedder struct {
	resolver *EmbeddingResolver
	roles    roleSource
}

// NewRoleEmbedder builds a complete live embedding-role resolver.
func NewRoleEmbedder(resolver *EmbeddingResolver, roles roleSource) (*RoleEmbedder, error) {
	if resolver == nil {
		return nil, errors.New("model: embedding resolver is required")
	}
	if lo.IsNil(roles) {
		return nil, errors.New("model: embedding role source is required")
	}
	return &RoleEmbedder{resolver: resolver, roles: roles}, nil
}

// Resolve returns the optional configured embedding role. An absent role supplies no
// embedding client; each consumer retains its own behavior for that absence.
func (r *RoleEmbedder) Resolve(ctx context.Context) (TextEmbedder, error) {
	if r == nil || r.resolver == nil || lo.IsNil(r.roles) {
		return nil, errors.New("model: embedding role resolver is not configured")
	}
	role := r.roles.Role()
	if !role.Configured() {
		return nil, nil
	}
	return r.resolver.Resolve(ctx, role.Selection())
}
