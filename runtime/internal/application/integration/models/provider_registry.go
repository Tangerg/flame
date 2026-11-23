package models

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
)

type ProviderRegistry interface {
	List(ctx context.Context) ([]provider.Provider, error)
	Get(ctx context.Context, id string) (provider.Provider, bool, error)
	Update(ctx context.Context, id string, patch provider.Patch) (provider.Provider, error)
}
