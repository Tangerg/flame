package models

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

func (c *Coordinator) UtilityRole() modelref.Role { return c.utilityRole.Role() }

func (c *Coordinator) SetUtilityRole(ctx context.Context, role modelref.Role, complete operation.Reply[modelref.Role]) error {
	if err := c.utilityRole.set(ctx, role, c.validateUtilityRole, complete); err != nil {
		return err
	}
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.Models})
	return nil
}

func (c *Coordinator) validateUtilityRole(ctx context.Context, role modelref.Role) error {
	if _, _, err := c.configuredProvider(ctx, role.Provider()); err != nil {
		return err
	}
	if err := c.utilityValidator.ValidateChatModel(ctx, role.Provider(), role.Model()); err != nil {
		return fmt.Errorf("models: utility model %q on %q: %w", role.Model(), role.Provider(), err)
	}
	return nil
}

func (c *Coordinator) EmbeddingRole() modelref.Role { return c.embeddingRole.Role() }

func (c *Coordinator) SetEmbeddingRole(ctx context.Context, role modelref.Role, complete operation.Reply[modelref.Role]) error {
	if err := c.embeddingRole.set(ctx, role, c.validateEmbeddingRole, complete); err != nil {
		return err
	}
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.Models})
	return nil
}

func (c *Coordinator) validateEmbeddingRole(ctx context.Context, role modelref.Role) error {
	meta, _, err := c.configuredProvider(ctx, role.Provider())
	if err != nil {
		return err
	}
	if !meta.Embedding().Supported() {
		return fmt.Errorf("%w: provider %q", ErrEmbeddingUnsupported, role.Provider())
	}
	if err := c.embeddingValidator.ValidateEmbeddingModel(ctx, role.Provider(), role.Model()); err != nil {
		return fmt.Errorf("models: build embedding model %q on %q: %w", role.Model(), role.Provider(), err)
	}
	return nil
}
