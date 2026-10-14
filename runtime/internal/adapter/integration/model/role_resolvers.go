package model

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	agentmemoryapp "github.com/Tangerg/flame/runtime/internal/plugins/memory/application"
)

// RoleSource is the read view a live specialized-model resolver needs. The
// source's owner decides how role changes are synchronized.
type RoleSource interface {
	Role() modelref.Role
}

// ChatModelResolver resolves an exact selection from the current provider
// configuration snapshot. The utility role adapter depends only on that
// behavior, not on ChatResolver's registry implementation.
type ChatModelResolver interface {
	ResolveChat(context.Context, modelref.Selection) (ResolvedChat, error)
}

// LiveUtilityModel resolves the optional specialized role on every use. An
// absent role selects the main model; a configured role is exact and never
// silently falls back to another provider/model when resolution fails.
func LiveUtilityModel(
	resolver ChatModelResolver,
	mainSelection modelref.Selection,
	roles RoleSource,
	selector modelapp.Selector,
) (AuxiliaryResolver, error) {
	if dependency.Missing(resolver) {
		return nil, errors.New("model: utility chat resolver is required")
	}
	if err := mainSelection.ValidateExact(); err != nil {
		return nil, fmt.Errorf("model: utility main selection: %w", err)
	}
	if dependency.Missing(roles) {
		return nil, errors.New("model: utility role source is required")
	}
	if selector != nil && dependency.Missing(selector) {
		return nil, errors.New("model: utility selector is typed nil")
	}
	return func(ctx context.Context, budget AuxiliaryBudget) (AuxiliaryModel, func(), error) {
		if err := budget.Validate(); err != nil {
			return AuxiliaryModel{}, nil, err
		}
		selection := mainSelection
		if role := roles.Role(); role.Configured() {
			selection = role.Selection()
		}
		if selector != nil {
			selected, err := selector.SelectModel(ctx, selection, nil)
			if err != nil {
				return AuxiliaryModel{}, nil, fmt.Errorf("auxiliary model: select route: %w", err)
			}
			if err := selected.ValidateExact(); err != nil {
				return AuxiliaryModel{}, nil, err
			}
			selection = selected
		}
		trace.SpanFromContext(ctx).SetAttributes(
			attribute.String("flame.provider.id", selection.Provider()),
			attribute.String("gen_ai.request.model", selection.Model()),
		)
		resolved, err := resolver.ResolveChat(ctx, selection)
		if err != nil {
			return AuxiliaryModel{}, nil, fmt.Errorf(
				"auxiliary model: resolve %s/%s: %w",
				selection.Provider(),
				selection.Model(),
				err,
			)
		}
		capture, err := NewAuxiliaryModel(selection, resolved)
		if err != nil {
			return AuxiliaryModel{}, nil, err
		}
		return capture, func() {}, nil
	}, nil
}

// RoleEmbedder resolves the live embedding role through an embedding resolver.
type RoleEmbedder struct {
	resolver *EmbeddingResolver
	roles    RoleSource
}

// NewRoleEmbedder builds a complete live embedding-role resolver.
func NewRoleEmbedder(resolver *EmbeddingResolver, roles RoleSource) (*RoleEmbedder, error) {
	if resolver == nil {
		return nil, errors.New("model: embedding resolver is required")
	}
	if dependency.Missing(roles) {
		return nil, errors.New("model: embedding role source is required")
	}
	return &RoleEmbedder{resolver: resolver, roles: roles}, nil
}

// ResolveMemory returns the optional embedder configured for agent-memory
// ranking. An absent role is a normal keyword-only configuration.
func (r *RoleEmbedder) ResolveMemory(ctx context.Context) (agentmemoryapp.Embedder, error) {
	if r == nil || r.resolver == nil || dependency.Missing(r.roles) {
		return nil, errors.New("model: embedding role resolver is not configured")
	}
	role := r.roles.Role()
	if !role.Configured() {
		return nil, nil
	}
	return r.resolver.Resolve(ctx, role.Selection())
}
