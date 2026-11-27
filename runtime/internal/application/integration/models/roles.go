package models

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

type roleStore interface {
	Load(context.Context) (modelref.Role, bool, error)
	Save(context.Context, modelref.Role) error
}

func (c *Coordinator) UtilityRole(ctx context.Context) (modelref.Role, error) {
	role, _, err := c.utilityRole.Load(ctx)
	return role, err
}

func (c *Coordinator) SetUtilityRole(ctx context.Context, role modelref.Role, complete operation.Reply[modelref.Role]) error {
	return c.setRole(ctx, c.utilityRole, role, c.validateUtilityRole, complete)
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

func (c *Coordinator) EmbeddingRole(ctx context.Context) (modelref.Role, error) {
	role, _, err := c.embeddingRole.Load(ctx)
	return role, err
}

func (c *Coordinator) SetEmbeddingRole(ctx context.Context, role modelref.Role, complete operation.Reply[modelref.Role]) error {
	return c.setRole(ctx, c.embeddingRole, role, c.validateEmbeddingRole, complete)
}

func (c *Coordinator) setRole(
	ctx context.Context,
	store roleStore,
	role modelref.Role,
	validate func(context.Context, modelref.Role) error,
	complete operation.Reply[modelref.Role],
) error {
	if role.Configured() {
		if err := validate(ctx, role); err != nil {
			return err
		}
	}
	if err := complete(ctx, func(tx context.Context) (modelref.Role, error) {
		return role, store.Save(tx, role)
	}); err != nil {
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
