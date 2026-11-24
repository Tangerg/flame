package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	adapterhooks "github.com/Tangerg/flame/runtime/internal/adapter/integration/hooks"
	"github.com/samber/lo"

	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

type hookTrust interface {
	IsTrusted(ctx context.Context, repositoryRoot string) (bool, error)
}

func newHookResolver(userHome string, trust hookTrust, source adapterhooks.Source) (*adapterhooks.Resolver, error) {
	if lo.IsNil(trust) {
		return nil, errors.New("hooks: trust store is required")
	}
	return adapterhooks.NewResolver(userHome,
		func(ctx context.Context, repositoryRoot string) (bool, error) {
			ok, err := trust.IsTrusted(ctx, repositoryRoot)
			if err != nil {
				return false, fmt.Errorf("hooks: read trust for repository %q: %w", repositoryRoot, err)
			}
			return ok, nil
		},
		source,
		func(ctx context.Context, source string, err error) {
			slog.ErrorContext(ctx, "hooks: command failed", slog.Any("source", source), telemetry.Error(err))
		},
	)
}
