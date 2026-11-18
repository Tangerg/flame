package bootstrap

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// roleLoader is the boot-time load view of a model-role store. Persistence
// after startup belongs to the model-role application use case.
type roleLoader interface {
	Load(ctx context.Context) (modelref.Role, bool, error)
}

// loadRole reads the persisted startup assignment; absence is the zero role.
func loadRole(ctx context.Context, loader roleLoader, name string) (modelref.Role, error) {
	role, _, err := loader.Load(ctx)
	if err != nil {
		return modelref.Role{}, fmt.Errorf("bootstrap: load %s role: %w", name, err)
	}
	return role, nil
}
