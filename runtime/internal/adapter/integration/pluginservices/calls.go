package pluginservices

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/idempotency"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

type installedCall struct {
	calls   *CommandCalls
	request plugins.CallRequest
	method  *capability.Method
}

func (c *CommandCalls) Prepare(_ context.Context, request plugins.CallRequest) (plugins.Call, error) {
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
	return &installedCall{calls: c, request: request, method: method}, nil
}

func (c *CommandCalls) Resolve(ctx context.Context, installation resourceid.InstallationID, digest fingerprint.Digest, contribution plugin.ContributionID) ([]plugins.Collaborator, error) {
	return plugins.ResolveRequirements(ctx, c.installations, c.selections, installation, digest, contribution)
}

func (c *installedCall) Kind() capability.MethodKind { return c.method.Kind() }

func (c *installedCall) resolve(ctx context.Context) (plugins.ContributionBinding, error) {
	r := c.request
	snapshot, found, err := c.calls.installations.Get(ctx, r.Installation)
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
	if _, err := c.calls.Resolve(ctx, r.Installation, r.Digest, r.Contribution); err != nil {
		return plugins.ContributionBinding{}, err
	}
	if actor, managed := operation.ActorFrom(ctx); managed {
		consumer, found, err := c.calls.installations.Get(ctx, actor.InstallationID())
		if err != nil {
			return plugins.ContributionBinding{}, err
		}
		if !found || consumer.Selected.Digest() != actor.Release() {
			return plugins.ContributionBinding{}, plugin.ErrUnavailable
		}
		declared := false
		for _, entry := range consumer.Selected.Declaration().Contributions {
			if entry.Server != actor.Server() {
				continue
			}
			collaborators, err := c.calls.Resolve(ctx, actor.InstallationID(), actor.Release(), entry.ID)
			if err != nil {
				return plugins.ContributionBinding{}, err
			}
			for _, collaborator := range collaborators {
				for _, provider := range collaborator.Bindings {
					if provider.InstallationID == r.Installation && provider.Digest == r.Digest && provider.ContributionID == r.Contribution && provider.Contract == r.Contract.Identity() {
						declared = true
					}
				}
			}
		}
		if !declared {
			return plugins.ContributionBinding{}, fmt.Errorf("%w: plugin call is outside the activation's declared requirements", plugin.ErrInvalid)
		}
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
		reply, err := invokeRawCommand(ctx, c.calls, c.request.Method, c.request.Input, nil, c.accepts, c.resolve, nil, func(plugins.ContributionBinding) capability.Target { return c.request.Target })
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
	invocation, err := operation.Capture(ctx, c.request.Target)
	if err != nil {
		return jsoninput.Input{}, err
	}
	ctx, err = pluginsdk.WithInvocation(ctx, invocation)
	if err != nil {
		return jsoninput.Input{}, err
	}
	return bound.Invoke(ctx, "", c.request.Input)
}
