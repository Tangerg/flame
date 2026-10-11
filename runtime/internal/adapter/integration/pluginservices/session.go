package pluginservices

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

var errInactive = errors.New("managed plugin: no enabled owner")

func serviceSession(ctx context.Context, store plugins.Store, pool *mcpconnection.Pool, kind plugin.ServiceKind) (plugins.ServiceBinding, *sdk.ClientSession, error) {
	binding, found, err := plugins.ServiceOwner(ctx, store, kind)
	if err != nil {
		return binding, nil, err
	}
	if !found {
		return binding, nil, fmt.Errorf("%w: %w: %s", plugin.ErrUnavailable, errInactive, kind)
	}
	session, err := pool.ServiceSession(ctx, binding.ServerID)
	return binding, session, err
}
