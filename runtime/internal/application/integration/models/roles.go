package models

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

func (c *Coordinator) UtilityRole() modelref.Role { return c.utilityRoleState.Role() }

func (c *Coordinator) SetUtilityRole(ctx context.Context, provider, model string) (modelref.Role, error) {
	c.utilityMu.Lock()
	defer c.utilityMu.Unlock()
	role, err := modelref.NewRole(provider, model)
	if err != nil {
		return modelref.Role{}, err
	}
	if role.Configured() {
		if _, _, err := c.configuredProvider(ctx, role.Provider()); err != nil {
			return modelref.Role{}, err
		}
		if err := c.utilityValidator.ValidateChatModel(ctx, role.Provider(), role.Model()); err != nil {
			return modelref.Role{}, fmt.Errorf("models: utility model %q on %q: %w", role.Model(), role.Provider(), err)
		}
	}
	if err := c.utilityStore.Save(ctx, role); err != nil {
		return modelref.Role{}, err
	}

	c.utilityRoleState.store(role)
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.Models})
	return role, nil
}

func (c *Coordinator) EmbeddingRole() modelref.Role { return c.embeddingRoleState.Role() }

func (c *Coordinator) SetEmbeddingRole(ctx context.Context, providerID, model string) (modelref.Role, error) {
	c.embeddingMu.Lock()
	defer c.embeddingMu.Unlock()
	role, err := modelref.NewRole(providerID, model)
	if err != nil {
		return modelref.Role{}, err
	}
	if role.Configured() {
		meta, _, err := c.configuredProvider(ctx, role.Provider())
		if err != nil {
			return modelref.Role{}, err
		}
		if !meta.Embedding().Supported() {
			return modelref.Role{}, fmt.Errorf("%w: provider %q", ErrEmbeddingUnsupported, role.Provider())
		}
		if err := c.embeddingValidator.ValidateEmbeddingModel(ctx, role.Provider(), role.Model()); err != nil {
			return modelref.Role{}, fmt.Errorf("models: build embedding model %q on %q: %w", role.Model(), role.Provider(), err)
		}
	}
	if err := c.embeddingStore.Save(ctx, role); err != nil {
		return modelref.Role{}, fmt.Errorf("models: persist embedding role: %w", err)
	}

	c.embeddingRoleState.store(role)
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.Models})
	return role, nil
}
