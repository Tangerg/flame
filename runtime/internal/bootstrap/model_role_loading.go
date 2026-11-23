package bootstrap

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

type roleLoader interface {
	Load(ctx context.Context) (modelref.Role, bool, error)
}

func loadRole(ctx context.Context, loader roleLoader, name string) (modelref.Role, error) {
	role, _, err := loader.Load(ctx)
	if err != nil {
		return modelref.Role{}, fmt.Errorf("bootstrap: load %s role: %w", name, err)
	}
	return role, nil
}
