package sessions

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// RunReader returns one Session's complete valid Run aggregates in admission
// order. Every boundary this package computes reads that same list.
type RunReader interface {
	ListRuns(ctx context.Context, sessionID string) ([]run.Run, error)
}
