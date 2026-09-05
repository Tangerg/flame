package bootstrap

import (
	"context"

	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
)

// TerminalResource is a process-owned adapter whose Close call is one-shot:
// once Close returns, the resource has reached its final state even when it
// reports a diagnostic. Instance bounds and joins the call itself, so adapters do
// not need a second timeout or retry layer.
type TerminalResource interface {
	Close() error
}

// HookResolver is the runtime's consumer view of lifecycle-hook resolution.
type HookResolver interface {
	For(ctx context.Context, cwd string) (*apphooks.Bound, error)
	Inspect(ctx context.Context, cwd string) (apphooks.Inspection, error)
}
