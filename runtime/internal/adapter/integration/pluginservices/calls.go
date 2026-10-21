package pluginservices

import (
	"context"
	"errors"
	"fmt"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/fingerprint"
	"github.com/Tangerg/flame/runtime/idempotency"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/point"
	"github.com/Tangerg/flame/runtime/resourceid"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type installedCall struct {
	calls   *CommandCalls
	request plugins.CallRequest
	method  *capability.Method
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

func (c *CommandCalls) Resolve(ctx context.Context, installation resourceid.InstallationID, digest fingerprint.Digest, contribution plugin.ContributionID) ([]plugins.Collaborator, error) {
	return plugins.ResolveRequirements(ctx, c.installations, c.selections, c.compositions, installation, digest, contribution)
}

func (c *CommandCalls) ResolvePoint(ctx context.Context, contract *point.Contract, target point.Context, order []plugins.PointReference) ([]plugins.PointBinding, error) {
	return plugins.ResolvePoint(ctx, c.installations, contract, target, order)
}

func (c *installedCall) Kind() capability.MethodKind { return c.method.Kind() }

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
		return errors.Join(idempotency.ErrKeyConflict, plugin.ErrCommandConflict)
	}
	return nil
}

func (c *installedCall) Invoke(ctx context.Context) (jsoninput.Input, error) {
	if c.Kind() == capability.Mutation {
		reply, err := invokeRawCommand(ctx, c.calls, c.request.Method, c.request.Input, nil, c.accepts, c.resolve, func(ctx context.Context, binding plugins.ContributionBinding, session *sdk.ClientSession) (sdk.ParamsBase, func(), error) {
			return c.calls.pool.OpenRPC(ctx, binding.ServerID, session)
		}, func(plugins.ContributionBinding) capability.Target { return c.request.Target })
		return reply.value, err
	}
	location, err := c.resolve(ctx)
	if err != nil {
		return jsoninput.Input{}, err
	}
	session, err := c.calls.pool.ServiceSession(ctx, location.ServerID)
	if err != nil {
		return jsoninput.Input{}, err
	}
	exported, err := pluginsdk.ResolveCapability(ctx, session, location.Export, c.request.Contract)
	if err != nil {
		return jsoninput.Input{}, err
	}
	bound, err := exported.Method(c.request.Method)
	if err != nil {
		return jsoninput.Input{}, err
	}
	if err := validateInput(ctx, bound, c.request.Input); err != nil {
		return jsoninput.Input{}, err
	}
	invocation, err := operation.Capture(ctx, c.request.Target)
	if err != nil {
		return jsoninput.Input{}, err
	}
	ctx, err = pluginsdk.WithInvocation(ctx, invocation)
	if err != nil {
		return jsoninput.Input{}, err
	}
	metadata, release, err := c.calls.pool.OpenRPC(ctx, location.ServerID, session)
	if err != nil {
		return jsoninput.Input{}, err
	}
	defer release()
	return bound.InvokeWithMetadata(ctx, "", c.request.Input, metadata)
}

func validateInput(ctx context.Context, binding *pluginsdk.Binding, input jsoninput.Input) error {
	err := binding.ValidateInput(ctx, input)
	if errors.Is(err, pluginsdk.ErrInvalidInput) {
		return errors.Join(plugin.ErrInvalid, err)
	}
	return err
}

func admitCallRequest(request plugins.CallRequest, streaming bool) (*capability.Method, error) {
	if request.Contract == nil {
		return nil, plugin.ErrInvalid
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
	if request.Target.Scope != method.Descriptor().Target || request.Target.Scope == "installation" && request.Target.Reference != request.Installation.String() {
		return nil, plugin.ErrInvalid
	}
	if err := request.Input.Validate(); err != nil {
		return nil, errors.Join(plugin.ErrInvalid, err)
	}
	if method.Kind() == capability.Query {
		if err := method.ValidateInput(request.Input); err != nil {
			return nil, errors.Join(plugin.ErrInvalid, err)
		}
	}
	if (len(method.Descriptor().Stream) != 0) != streaming {
		return nil, plugin.ErrInvalid
	}
	return method, nil
}
