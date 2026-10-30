package pluginservices

import (
	"context"
	"errors"
	"strconv"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	adapterhooks "github.com/Tangerg/flame/runtime/internal/adapter/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/google/uuid"
)

func (c *CommandCalls) AcquireHooks(ctx context.Context) (adapterhooks.Use, error) {
	contract, err := hookprovider.Contract()
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
	return &hookUse{calls: c, binding: binding, session: borrowed, identity: uuid.NewString()}, nil
}

type hookUse struct {
	calls    *CommandCalls
	binding  plugins.ContributionBinding
	session  *mcp.BorrowedSession
	identity string
}

func (u *hookUse) Discover(ctx context.Context, input hookprovider.Discovery) ([]hook.Hook, error) {
	method, err := hookprovider.DiscoverMethod()
	if err != nil {
		return nil, err
	}
	calling, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(u.session.Context(), func() { cancel(context.Cause(u.session.Context())) })
	defer func() { stop(); cancel(nil) }()
	return query[hookprovider.Discovery, []hook.Hook](calling, u.binding, u.session.Session(), method.Name(), input, sdk.ParamsBase{})
}

func (u *hookUse) RunHookCommand(ctx context.Context, request apphooks.CommandRequest) apphooks.CommandResult {
	method, err := hookprovider.ExecuteMethod()
	if err != nil {
		return apphooks.CommandResult{ExitCode: -1, Err: err}
	}
	calling, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(u.session.Context(), func() { cancel(context.Cause(u.session.Context())) })
	defer func() { stop(); cancel(nil) }()
	if operation.Identity(calling) == "" {
		if caller, err := qimumcp.InvocationFromContext(calling); err == nil {
			calling = operation.Adopt(calling, caller.ID, caller.Origin, caller.Parent)
		} else {
			calling = operation.Start(calling, extension.Automatic, "hooks.capture", u.identity)
		}
	}
	// Re-observing an event under this capture reads its original command receipt.
	key := u.identity + ":" + string(request.Input.Event) + ":" + strconv.Itoa(request.Ordinal)
	calling = operation.Bind(calling, "hooks.execute", key)
	input := hookprovider.Command{Command: request.Command, CWD: request.CWD, Input: request.Input, TimeoutMillis: request.Timeout.Milliseconds()}
	reply, err := invokeResolvedCommand[hookprovider.Command, hookprovider.Result](calling, u.calls, method.Name(), input, hookprovider.Result.Validate, func(context.Context) (plugins.ContributionBinding, error) { return u.binding, nil }, nil)
	if err != nil {
		return apphooks.CommandResult{ExitCode: -1, Err: err}
	}
	result := apphooks.CommandResult{ExitCode: reply.value.ExitCode, Stderr: reply.value.Stderr}
	if reply.value.Decision != nil {
		result.Decision = *reply.value.Decision
	}
	if failure := reply.value.Failure; failure != nil {
		result.Err = errors.New(failure.Message)
		switch failure.Kind {
		case hookprovider.TimedOut:
			result.TimedOut = true
			result.Err = errors.Join(result.Err, context.DeadlineExceeded)
		case hookprovider.Canceled:
			result.Err = errors.Join(result.Err, context.Canceled)
		}
	}
	return result
}

func (u *hookUse) Close() error { return u.session.Close() }
