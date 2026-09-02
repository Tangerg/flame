package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/core/chatclient"

	agentmemoryapp "github.com/Tangerg/flame/runtime/internal/application/workspace/agentmemory"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// RoleSource is the read view a live specialized-model resolver needs. The
// source's owner decides how role changes are synchronized.
type RoleSource interface {
	Role() modelref.Selection
}

// ChatClientResolver resolves an exact selection from the current provider
// configuration snapshot. The utility role adapter depends only on that
// behavior, not on ChatResolver's registry implementation.
type ChatClientResolver interface {
	ResolveChat(context.Context, modelref.Selection) (ResolvedChat, error)
}

// LiveUtilityClient resolves the optional specialized role on every use. An
// absent role selects the main model; a configured role is exact and never
// silently falls back to another provider/model when resolution fails.
func LiveUtilityClient(
	resolver ChatClientResolver,
	mainSelection modelref.Selection,
	roles RoleSource,
) (AuxiliaryResolver, error) {
	if missingDependency(resolver) {
		return nil, errors.New("model: utility chat resolver is required")
	}
	if !mainSelection.Configured() {
		return nil, errors.New("model: utility main model selection is required")
	}
	if missingDependency(roles) {
		return nil, errors.New("model: utility role source is required")
	}
	return func(ctx context.Context) (*chatclient.Client, error) {
		selection := mainSelection
		if role := roles.Role(); role.Configured() {
			selection = role
		}
		resolved, err := resolver.ResolveChat(ctx, selection)
		if err != nil {
			return nil, fmt.Errorf(
				"auxiliary model: resolve %s/%s: %w",
				selection.Provider(),
				selection.Model(),
				err,
			)
		}
		if resolved.Client() == nil {
			return nil, errors.New("auxiliary model: resolved client is nil")
		}
		return resolved.Client(), nil
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
	if missingDependency(roles) {
		return nil, errors.New("model: embedding role source is required")
	}
	return &RoleEmbedder{resolver: resolver, roles: roles}, nil
}

// ResolveMemory returns the optional embedder configured for agent-memory
// ranking. An absent role is a normal keyword-only configuration.
func (r *RoleEmbedder) ResolveMemory(ctx context.Context) (agentmemoryapp.Embedder, error) {
	if r == nil || r.resolver == nil || missingDependency(r.roles) {
		return nil, errors.New("model: embedding role resolver is not configured")
	}
	role := r.roles.Role()
	if !role.Configured() {
		return nil, nil
	}
	return r.resolver.Resolve(ctx, role)
}
