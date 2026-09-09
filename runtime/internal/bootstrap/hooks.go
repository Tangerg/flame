package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	adapterhooks "github.com/Tangerg/flame/runtime/internal/adapter/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/dependency"
)

// HookTrust reports whether a project root may run user lifecycle hooks.
type HookTrust interface {
	IsTrusted(ctx context.Context, projectRoot string) (bool, error)
}

// NewHookResolver builds the runtime hook resolver from the composition root's
// user-home snapshot and the durable project trust policy.
func NewHookResolver(userHome string, trust HookTrust) (*adapterhooks.Resolver, error) {
	if dependency.Missing(trust) {
		return nil, fmt.Errorf("hooks: trust store is required")
	}
	return adapterhooks.NewResolver(userHome,
		func(ctx context.Context, projectRoot string) (bool, error) {
			ok, err := trust.IsTrusted(ctx, projectRoot)
			if err != nil {
				return false, fmt.Errorf("hooks: read trust for project %q: %w", projectRoot, err)
			}
			return ok, nil
		},
		func(ctx context.Context, source string, err error) {
			slog.ErrorContext(ctx, "hooks: command failed", "source", source, "error", err)
		},
	), nil
}
