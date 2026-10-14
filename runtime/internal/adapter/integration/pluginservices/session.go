package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

var errInactive = errors.New("managed plugin: no enabled owner")

func contributionSession(ctx context.Context, store plugins.Store, selections plugins.SelectionStore, pool *mcpconnection.Pool, required *capability.Contract) (plugins.ContributionBinding, *sdk.ClientSession, error) {
	binding, found, err := plugins.SelectedContribution(ctx, store, selections, required)
	if err != nil {
		return binding, nil, err
	}
	if !found {
		return binding, nil, fmt.Errorf("%w: %w: %s", plugin.ErrUnavailable, errInactive, required.Descriptor().Reference)
	}
	session, err := pool.ServiceSession(ctx, binding.ServerID)
	if errors.Is(err, mcp.ErrUnavailable) {
		err = fmt.Errorf("%w: %w", plugin.ErrUnavailable, err)
	}
	return binding, session, err
}

func query[P, R any](ctx context.Context, location plugins.ContributionBinding, session *sdk.ClientSession, name string, input P, metadata sdk.ParamsBase) (R, error) {
	var zero R
	ctx, err := queryContext(ctx, location)
	if err != nil {
		return zero, err
	}
	contract, err := capability.Parse(location.Contract.JSON())
	if err != nil {
		return zero, err
	}
	exported, err := pluginsdk.ResolveCapability(ctx, session, location.Export, contract)
	if err != nil {
		return zero, err
	}
	bound, err := exported.Method(name)
	if err != nil {
		return zero, err
	}
	admitted, err := jsoninput.Encode(input)
	if err != nil {
		return zero, err
	}
	output, err := bound.InvokeWithMetadata(ctx, "", admitted, metadata)
	if err != nil {
		return zero, err
	}
	var value R
	if err := json.Unmarshal(output.JSON(), &value, json.RejectUnknownMembers(true)); err != nil {
		return zero, err
	}
	return value, nil
}

// Collaborators must inherit the same admitted query as its backend invocation.
// Capturing after a grant opens would detach model origin and parent evidence.
func queryContext(ctx context.Context, location plugins.ContributionBinding) (context.Context, error) {
	if operation.Identity(ctx) == "" {
		ctx = operation.Bind(ctx, "plugins.query", "")
	}
	invocation, err := operation.Capture(ctx, capability.Target{Scope: "installation", Reference: location.InstallationID.String()})
	if err != nil {
		return nil, err
	}
	return pluginsdk.WithInvocation(ctx, invocation)
}
