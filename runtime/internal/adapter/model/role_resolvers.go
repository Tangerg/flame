package model

import (
	"context"

	"github.com/Tangerg/scope/core/chatclient"

	agentmemoryapp "github.com/Tangerg/flame/runtime/internal/application/agentmemory"
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
	ResolveChat(context.Context, modelref.Selection) (*chatclient.Client, error)
}

// LiveUtilityClient resolves the optional specialized role on every use. When
// that role is unavailable it resolves the main selection through the same live
// boundary; no process-start client or retired credential generation is kept.
func LiveUtilityClient(
	resolver ChatClientResolver,
	mainSelection modelref.Selection,
	roles RoleSource,
) func(context.Context) *chatclient.Client {
	return func(ctx context.Context) *chatclient.Client {
		selection := mainSelection
		if roles != nil {
			if role := roles.Role(); role.Configured() {
				selection = role
			}
		}
		client, err := resolver.ResolveChat(ctx, selection)
		if err == nil && client != nil {
			return client
		}
		if selection == mainSelection {
			return nil
		}
		client, _ = resolver.ResolveChat(ctx, mainSelection)
		return client
	}
}

// RoleEmbedder resolves the live embedding role through an embedding resolver.
type RoleEmbedder struct {
	resolver *EmbeddingResolver
	roles    RoleSource
}

// NewRoleEmbedder builds a live embedding-role resolver.
func NewRoleEmbedder(resolver *EmbeddingResolver, roles RoleSource) *RoleEmbedder {
	return &RoleEmbedder{resolver: resolver, roles: roles}
}

// ResolveMemory returns the optional embedder configured for agent-memory
// ranking. An absent role is a normal keyword-only configuration.
func (r *RoleEmbedder) ResolveMemory(ctx context.Context) (agentmemoryapp.Embedder, error) {
	if r == nil || r.resolver == nil || r.roles == nil {
		return nil, nil
	}
	role := r.roles.Role()
	if !role.Configured() {
		return nil, nil
	}
	return r.resolver.Resolve(ctx, role)
}
