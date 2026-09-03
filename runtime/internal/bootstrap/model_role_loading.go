package bootstrap

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// utilityRoleLoader is the boot-time load view of the utility-role store.
// Persistence after startup belongs to the model-role application use case.
type utilityRoleLoader interface {
	LoadUtilityRole(ctx context.Context) (modelref.Role, bool, error)
}

// loadUtilityRole reads the persisted startup assignment. Runtime mutation and
// client resolution are owned by their respective application and adapter types.
func loadUtilityRole(ctx context.Context, loader utilityRoleLoader) (modelref.Role, error) {
	var role modelref.Role
	if loader != nil {
		loaded, _, err := loader.LoadUtilityRole(ctx)
		if err != nil {
			return modelref.Role{}, fmt.Errorf("bootstrap: load utility role: %w", err)
		}
		role = loaded
	}
	return role, nil
}

// embeddingRoleLoader is the boot-time load view of the embedding-role store.
// Persistence after startup belongs to the model-role application use case.
type embeddingRoleLoader interface {
	LoadEmbeddingRole(ctx context.Context) (modelref.Role, error)
}

// loadEmbeddingRole reads the persisted startup assignment. Runtime mutation
// and embedding resolution remain with their application and adapter owners.
func loadEmbeddingRole(ctx context.Context, loader embeddingRoleLoader) (modelref.Role, error) {
	var role modelref.Role
	if loader != nil {
		loaded, err := loader.LoadEmbeddingRole(ctx)
		if err != nil {
			return modelref.Role{}, fmt.Errorf("bootstrap: load embedding role: %w", err)
		}
		role = loaded
	}
	return role, nil
}
