package pluginservices

import (
	"context"
	"errors"
	hookmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider/mcp"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	adapterhooks "github.com/Tangerg/flame/runtime/internal/adapter/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/google/uuid"
)

func (c *CommandCalls) AcquireHooks(ctx context.Context) (adapterhooks.Use, error) {
	contract, err := hookmcp.Contract()
	if err != nil {
		return nil, err
	}
	binding, found, err := plugins.SelectedContribution(ctx, c.framework, c.installations, c.selections, contract)
	if err != nil || !found {
		return nil, err
	}
	activation, err := c.pool.AwaitInstallationActivation(ctx, binding.ServerID)
	if err != nil {
		if errors.Is(err, mcp.ErrUnavailable) {
			err = errors.Join(plugin.ErrUnavailable, err)
		}
		return nil, err
	}
	activation, err = plugins.OriginalActivation([]plugin.Activation{activation}, binding.InstallationID, binding.Digest, binding.ServerName)
	if err != nil {
		return nil, err
	}
	binding.Activation = &activation
	borrowed, err := borrowContribution(ctx, c.pool, binding)
	if err != nil {
		return nil, err
	}
	return &hookUse{calls: c, binding: binding, session: borrowed, captureID: uuid.NewString()}, nil
}

type hookUse struct {
	calls     *CommandCalls
	binding   plugins.ContributionBinding
	session   *mcp.BorrowedSession
	captureID string
}

func (u *hookUse) Discover(ctx context.Context, input hookprovider.Discovery) ([]hook.Hook, error) {
	method, err := hookmcp.DiscoverMethod()
	if err != nil {
		return nil, err
	}
	calling, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(u.session.Context(), func() { cancel(context.Cause(u.session.Context())) })
	defer func() { stop(); cancel(nil) }()
	return query[hookprovider.Discovery, []hook.Hook](calling, u.binding, u.session.Session(), method.Name(), input, sdk.ParamsBase{})
}

func (u *hookUse) Evaluate(ctx context.Context, request hookprovider.Evaluation) (hookprovider.Advice, error) {
	method, err := hookmcp.EvaluateMethod()
	if err != nil {
		return hookprovider.Advice{}, err
	}
	calling, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(u.session.Context(), func() { cancel(context.Cause(u.session.Context())) })
	defer func() { stop(); cancel(nil) }()
	if operation.Identity(calling) == "" {
		if caller, err := qimumcp.InvocationFromContext(calling); err == nil {
			calling = operation.Adopt(calling, caller.ID, caller.Origin, caller.Parent)
		} else {
			calling = operation.Start(calling, extension.Automatic, "hooks.capture", u.captureID)
		}
	}
	calling = operation.Bind(calling, "hooks.evaluate", operation.Identity(calling)+":"+string(request.Input.Event))
	reply, err := invokeResolvedCommand[hookprovider.Evaluation, hookprovider.Advice](calling, u.calls, method.Name(), request, hookprovider.Advice.Validate, func(context.Context) (plugins.ContributionBinding, error) { return u.binding, nil }, nil)
	return reply.value, err
}

func (u *hookUse) Close() error { return u.session.Close() }
