package pluginservices

import (
	"context"
	"errors"
	"fmt"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
)

type installedCall struct {
	calls   *CommandCalls
	request plugins.CallRequest
	method  *portable.Method
}

func (c *CommandCalls) Command(ctx context.Context, id string) (*plugin.Command, error) {
	return c.routing.Get(ctx, id)
}

func (c *CommandCalls) RetireCommand(ctx context.Context, id string) error {
	return c.routing.Retire(ctx, id)
}

func (c *CommandCalls) Prepare(_ context.Context, request plugins.CallRequest) (plugins.Call, error) {
	method, err := admitCallRequest(request, false)
	if err != nil {
		return nil, err
	}
	return &installedCall{calls: c, request: request, method: method}, nil
}

func (c *CommandCalls) Resolve(ctx context.Context, installation identity.InstallationID, digest framevalue.Digest, contribution plugin.ContributionID) ([]plugins.Collaborator, error) {
	return plugins.ResolveRequirements(ctx, c.framework, c.installations, c.selections, c.compositions, installation, digest, contribution)
}

func (c *CommandCalls) ResolvePoint(ctx context.Context, contract *plugincontract.Point, target plugincontract.PointContext, order []plugins.PointReference) ([]plugins.PointBinding, error) {
	return plugins.ResolvePoint(ctx, c.framework, c.installations, contract, target, order)
}

func (c *installedCall) Kind() portable.MethodKind { return c.method.Kind() }

func (c *installedCall) resolve(ctx context.Context) (plugins.ContributionBinding, error) {
	r := c.request
	if actor, managed := operation.ActorFrom(ctx); managed {
		if !actor.Allows(plugin.Dependency{InstallationID: r.Installation, Digest: r.Digest}, r.Contribution, r.Contract.Identity()) {
			return plugins.ContributionBinding{}, fmt.Errorf("%w: plugin call is outside the activation's captured requirements", plugin.ErrInvalid)
		}
	}
	return c.calls.resolveContribution(ctx, r)
}

// Runtime-selected model policies belong to the host composition, not to the
// invoking plugin's cross-plugin requirements. Both still validate current bytes.
func (c *CommandCalls) resolveContribution(ctx context.Context, r plugins.CallRequest) (plugins.ContributionBinding, error) {
	snapshot, found, err := c.installations.Get(ctx, r.Installation)
	if err != nil {
		return plugins.ContributionBinding{}, err
	}
	if !found {
		return plugins.ContributionBinding{}, plugin.ErrNotFound
	}
	if snapshot.Selected.Digest() != r.Digest {
		return plugins.ContributionBinding{}, plugin.ErrStale
	}
	contribution, found := snapshot.Selected.Contribution(r.Contribution)
	if !found || contribution.Contract != r.Contract.Identity() {
		return plugins.ContributionBinding{}, plugin.ErrUnavailable
	}
	if !snapshot.Installation.ServerEnabled(contribution.Server) {
		return plugins.ContributionBinding{}, plugin.ErrDisabled
	}
	id, err := snapshot.Installation.ServerID(contribution.Server)
	if err != nil {
		return plugins.ContributionBinding{}, err
	}
	return plugins.ContributionBinding{InstallationID: r.Installation, Digest: r.Digest, ContributionID: contribution.ID, ServerID: id, ServerName: contribution.Server, Export: contribution.Export, Contract: contribution.Contract}, nil
}

func (c *installedCall) accepts(record plugin.CommandBinding) error {
	r := c.request
	if record.Provider.InstallationID != r.Installation || record.Provider.Digest != r.Digest || record.Contribution != r.Contribution || record.Contract != r.Contract.Identity() {
		return errors.Join(framemcp.ErrInvocationConflict, plugin.ErrCommandConflict)
	}
	return nil
}

func (c *installedCall) Invoke(ctx context.Context) (framevalue.JSON, error) {
	if c.Kind() == portable.Command {
		reply, err := invokeRawCommand(ctx, c.calls, c.request.Method, c.request.Input, nil, c.accepts, c.resolve, func(ctx context.Context, binding plugins.ContributionBinding, session *sdk.ClientSession) (sdk.ParamsBase, func() error, error) {
			metadata, release, err := c.calls.pool.OpenRPC(ctx, binding.ServerID, session)
			if err != nil {
				return metadata, nil, err
			}
			return metadata, func() error { release(); return nil }, nil
		}, func(plugins.ContributionBinding) portable.Target { return c.request.Target })
		return reply.value, err
	}
	location, err := c.resolve(ctx)
	if err != nil {
		return framevalue.JSON{}, err
	}
	borrowed, err := c.calls.pool.BorrowSession(ctx, location.ServerID)
	if err != nil {
		return framevalue.JSON{}, err
	}
	defer borrowed.Close()
	ctx = borrowed.Context()
	session := borrowed.Session()
	exported, err := pluginmcp.ResolveCapability(ctx, session, location.Export, c.request.Contract)
	if err != nil {
		return framevalue.JSON{}, err
	}
	bound, err := exported.Method(c.request.Method)
	if err != nil {
		return framevalue.JSON{}, err
	}
	if err := validateInput(ctx, bound, c.request.Input); err != nil {
		return framevalue.JSON{}, err
	}
	invocation, err := operation.Capture(ctx, c.request.Target)
	if err != nil {
		return framevalue.JSON{}, err
	}
	ctx, err = pluginmcp.WithInvocation(ctx, invocation)
	if err != nil {
		return framevalue.JSON{}, err
	}
	metadata, release, err := c.calls.pool.OpenRPC(ctx, location.ServerID, session)
	if err != nil {
		return framevalue.JSON{}, err
	}
	defer release()
	return bound.InvokeWithMetadata(ctx, "", c.request.Input, metadata)
}

func validateInput(ctx context.Context, binding *pluginmcp.Binding, input framevalue.JSON) error {
	err := binding.ValidateInput(ctx, input)
	if errors.Is(err, pluginmcp.ErrInvalidInput) {
		return errors.Join(plugin.ErrInvalid, err)
	}
	return err
}

func admitCallRequest(request plugins.CallRequest, streaming bool) (*portable.Method, error) {
	if request.Contract == nil {
		return nil, plugin.ErrInvalid
	}
	if request.Contract.Descriptor().Reference == mcpconnector.Reference {
		return nil, fmt.Errorf("%w: external MCP commands require a host-admitted launch grant", plugin.ErrInvalid)
	}
	if request.Contract.Descriptor().Reference == skilllearning.Reference {
		return nil, fmt.Errorf("%w: Skill governance requires a host-admitted library and review authority", plugin.ErrInvalid)
	}
	if request.Contract.Descriptor().Reference == hookprovider.Reference {
		return nil, fmt.Errorf("%w: lifecycle hook evaluation requires host-admitted project trust", plugin.ErrInvalid)
	}
	if err := request.Installation.Validate(); err != nil {
		return nil, err
	}
	if err := request.Digest.Validate(); err != nil {
		return nil, err
	}
	if err := request.Contribution.Validate(); err != nil {
		return nil, err
	}
	method, found := request.Contract.Method(request.Method)
	if !found {
		return nil, plugin.ErrInvalid
	}
	if err := request.Target.Validate(); err != nil {
		return nil, errors.Join(plugin.ErrInvalid, err)
	}
	if request.Target.Scope != method.Descriptor().Target || request.Target.Scope == extension.ScopeInstallation && request.Target.Reference != request.Installation.String() {
		return nil, plugin.ErrInvalid
	}
	if err := request.Input.Validate(); err != nil {
		return nil, errors.Join(plugin.ErrInvalid, err)
	}
	if method.Kind() == portable.Query {
		if err := method.ValidateInput(request.Input); err != nil {
			return nil, errors.Join(plugin.ErrInvalid, err)
		}
	}
	if (len(method.Descriptor().Stream) != 0) != streaming {
		return nil, plugin.ErrInvalid
	}
	return method, nil
}
