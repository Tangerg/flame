package pluginservices

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/Tangerg/frame"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type installedCall struct {
	calls   *CommandCalls
	request plugins.CallRequest
	method  *frame.Method
}

func (c *CommandCalls) Command(ctx context.Context, id string) (*plugin.Command, error) {
	return c.routing.Get(ctx, id)
}

func (c *CommandCalls) RetireCommand(ctx context.Context, id string) error {
	return c.routing.Retire(ctx, id)
}

func (c *CommandCalls) Prepare(_ context.Context, request plugins.CallRequest) (plugins.Call, error) {
	method, err := admitCallRequest(request, unaryCall)
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

func (c *installedCall) Kind() frame.MethodKind { return c.method.Kind() }

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
		return errors.Join(framecall.ErrConflict, plugin.ErrCommandConflict)
	}
	return nil
}

func (c *installedCall) target(plugins.ContributionBinding) framecall.Target { return c.request.Target }

func (c *installedCall) Invoke(ctx context.Context) (framevalue.JSON, error) {
	if c.Kind() == frame.Command {
		reply, err := invokeRawCommand(ctx, c.calls, c.request.Method, c.request.Input, framevalue.JSON.Validate, c.accepts, c.resolve, func(ctx context.Context, binding plugins.ContributionBinding, session *sdk.ClientSession) (sdk.ParamsBase, func() error, error) {
			metadata, release, err := c.calls.pool.OpenRPC(ctx, binding.ServerID, session)
			if err != nil {
				return metadata, nil, err
			}
			return metadata, func() error { release(); return nil }, nil
		}, c.target)
		return reply.value, err
	}
	ctx, query, err := c.openQuery(ctx)
	if err != nil {
		return framevalue.JSON{}, err
	}
	defer query.release()
	return query.method.InvokeWithMetadata(ctx, "", c.request.Input, query.metadata)
}

// openedQuery is a query bound to a borrowed session of its resolved
// contribution under a captured invocation.
type openedQuery struct {
	location plugins.ContributionBinding
	session  *sdk.ClientSession
	method   *pluginmcp.Binding
	metadata sdk.ParamsBase
	release  func()
}

func (c *installedCall) openQuery(ctx context.Context) (context.Context, openedQuery, error) {
	location, err := c.resolve(ctx)
	if err != nil {
		return ctx, openedQuery{}, err
	}
	borrowed, err := c.calls.pool.BorrowSession(ctx, location.ServerID)
	if err != nil {
		return ctx, openedQuery{}, err
	}
	fail := func(err error) (context.Context, openedQuery, error) {
		_ = borrowed.Close()
		return ctx, openedQuery{}, err
	}
	ctx = borrowed.Context()
	session := borrowed.Session()
	exported, err := pluginmcp.ResolveCapability(ctx, session, location.Export, c.request.Contract)
	if err != nil {
		return fail(err)
	}
	method, err := exported.Method(c.request.Method)
	if err != nil {
		return fail(err)
	}
	if err := validateInput(ctx, method, c.request.Input); err != nil {
		return fail(err)
	}
	invocation, err := operation.Capture(ctx, c.request.Target)
	if err != nil {
		return fail(err)
	}
	if ctx, err = pluginmcp.WithInvocation(ctx, invocation); err != nil {
		return fail(err)
	}
	metadata, release, err := c.calls.pool.OpenRPC(ctx, location.ServerID, session)
	if err != nil {
		return fail(err)
	}
	return ctx, openedQuery{
		location: location, session: session, method: method, metadata: metadata,
		release: func() { release(); _ = borrowed.Close() },
	}, nil
}

func validateInput(ctx context.Context, binding *pluginmcp.Binding, input framevalue.JSON) error {
	err := binding.ValidateInput(ctx, input)
	if errors.Is(err, pluginmcp.ErrInvalidInput) {
		return errors.Join(plugin.ErrInvalid, err)
	}
	return err
}

// callDelivery is how the caller consumes a call's outcome; it must match
// whether the declared method streams events.
type callDelivery uint8

const (
	unaryCall callDelivery = iota + 1
	streamedCall
)

func (d callDelivery) admits(method frame.MethodDescriptor) bool {
	return (len(method.Stream) != 0) == (d == streamedCall)
}

// refuseHostOwnedContract keeps a client from calling a contract whose
// authority only the host can grant.
func refuseHostOwnedContract(reference string) error {
	switch reference {
	case mcpconnector.Reference:
		return fmt.Errorf("%w: external MCP commands require a host-admitted launch grant", plugin.ErrInvalid)
	case skilllearning.Reference:
		return fmt.Errorf("%w: Skill governance requires a host-admitted library and review authority", plugin.ErrInvalid)
	case hookprovider.Reference:
		return fmt.Errorf("%w: lifecycle hook evaluation requires host-admitted project trust", plugin.ErrInvalid)
	default:
		return nil
	}
}

func admitCallRequest(request plugins.CallRequest, delivery callDelivery) (*frame.Method, error) {
	if request.Contract == nil {
		return nil, plugin.ErrInvalid
	}
	if err := refuseHostOwnedContract(request.Contract.Descriptor().Reference); err != nil {
		return nil, err
	}
	if err := errors.Join(request.Installation.Validate(), request.Digest.Validate(), request.Contribution.Validate()); err != nil {
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
	if method.Kind() == frame.Query {
		if err := method.ValidateInput(request.Input); err != nil {
			return nil, errors.Join(plugin.ErrInvalid, err)
		}
	}
	if !delivery.admits(method.Descriptor()) {
		return nil, plugin.ErrInvalid
	}
	return method, nil
}
