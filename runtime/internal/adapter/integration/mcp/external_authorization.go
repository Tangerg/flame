package mcp

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	framecall "github.com/Tangerg/frame/call"
)

func (c *ExternalConnector) AdmitProbe(ctx context.Context) (func() error, error) {
	u, err := c.acquire(ctx)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, plugin.ErrUnavailable
	}
	return func() error { return u.session.release(context.WithoutCancel(ctx)) }, nil
}

func (c *ExternalConnector) StartAuthorization(ctx context.Context, server mcpserver.Server, id mcpapp.AuthorizationAttemptID, receipt operation.ReplayClaim) (settlement *mcpapp.AuthorizationStartSettlement, returned error) {
	u, err := c.acquire(ctx)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, plugin.ErrUnavailable
	}
	defer func() {
		if settlement == nil {
			returned = errors.Join(returned, u.session.release(context.WithoutCancel(ctx)))
		}
	}()
	if err := c.recoverAuthorizationEpoch(ctx, u); err != nil {
		return nil, err
	}
	intent, err := c.prepareAuthorizationStart(ctx, u, server, id)
	if err != nil {
		return nil, err
	}
	pending := mcpapp.PendingAuthorizationStart{Intent: intent, Receipt: receipt}
	if err := c.vault.PrepareAuthorizationStart(ctx, pending); err != nil {
		return nil, err
	}
	return c.settleAuthorizationStart(ctx, u, pending, false)
}

func (c *ExternalConnector) prepareAuthorizationStart(ctx context.Context, u *connectorUse, server mcpserver.Server, id mcpapp.AuthorizationAttemptID) (mcpapp.AuthorizationIntent, error) {
	e, found, err := c.find(ctx, u, server.Name)
	if err != nil {
		return mcpapp.AuthorizationIntent{}, err
	}
	if !found {
		return mcpapp.AuthorizationIntent{}, mcpapp.ErrUnknownServer
	}
	current, err := c.materialize(ctx, u, e)
	if err != nil {
		return mcpapp.AuthorizationIntent{}, err
	}
	if !current.SameConnection(server) {
		return mcpapp.AuthorizationIntent{}, plugin.ErrStale
	}
	target, eligible := current.OAuthTarget()
	if !eligible {
		return mcpapp.AuthorizationIntent{}, mcpapp.ErrAuthorizationUnsupported
	}
	command := operation.Bind(ctx, c.methods.Start.Name(), operation.Identity(ctx))
	invocation, err := operation.Capture(command, framecall.Target{Scope: extension.ScopeInstallation, Reference: u.source.InstallationID.String()})
	if err != nil {
		return mcpapp.AuthorizationIntent{}, err
	}
	return mcpapp.AuthorizationIntent{
		Source: u.source.ContributionSource(), Invocation: invocation,
		Start:     wire.Start{Name: e.Name, Grant: e.LaunchGrant, Epoch: u.epoch, AttemptID: id.String()},
		Recipient: target.Fingerprint(),
	}, nil
}

func (c *ExternalConnector) ReadAuthorization(ctx context.Context, id mcpapp.AuthorizationAttemptID) (attempt mcpapp.AuthorizationAttempt, err error) {
	route, found, err := c.vault.AuthorizationRoute(ctx, id)
	if err != nil {
		return mcpapp.AuthorizationAttempt{}, err
	}
	if !found {
		return mcpapp.AuthorizationAttempt{}, mcpapp.ErrAuthorizationAttemptNotFound
	}
	u, err := c.acquireCaptured(ctx, route.Source)
	if errors.Is(err, plugin.ErrUnavailable) {
		return mcpapp.AuthorizationAttempt{}, errors.Join(mcpapp.ErrAuthorizationOwnerUnavailable, err)
	}
	if err != nil {
		return mcpapp.AuthorizationAttempt{}, err
	}
	defer func() { err = errors.Join(err, u.session.release(context.WithoutCancel(ctx))) }()
	result, err := connectorCall(ctx, u, c.methods.Get, wire.Request{ID: id.String(), Epoch: route.Epoch})
	if err != nil {
		return mcpapp.AuthorizationAttempt{}, err
	}
	attempt, err = readExternalAttempt(result, id)
	if err == nil && attempt.Server.Name() != route.Name {
		return mcpapp.AuthorizationAttempt{}, mcpapp.ErrInvalidServerConfiguration
	}
	return attempt, err
}

func readExternalAttempt(r wire.AttemptResult, id mcpapp.AuthorizationAttemptID) (mcpapp.AuthorizationAttempt, error) {
	if r.Attempt == nil {
		return mcpapp.AuthorizationAttempt{}, mcpapp.ErrAuthorizationAttemptNotFound
	}
	a := r.Attempt
	status := mcpapp.AuthorizationAttemptStatus(a.Status)
	if a.ID != id.String() || !status.Valid() {
		return mcpapp.AuthorizationAttempt{}, mcpapp.ErrInvalidServerConfiguration
	}
	name, err := mcpserver.ParseServerName(a.Name)
	if err != nil {
		return mcpapp.AuthorizationAttempt{}, err
	}
	server, err := mcpserver.NewID(mcpserver.UserOrigin(), name)
	if err != nil {
		return mcpapp.AuthorizationAttempt{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, a.StartedAt)
	if err != nil {
		return mcpapp.AuthorizationAttempt{}, err
	}
	if created.IsZero() {
		return mcpapp.AuthorizationAttempt{}, mcpapp.ErrInvalidServerConfiguration
	}
	if err := instant.Validate(created); err != nil {
		return mcpapp.AuthorizationAttempt{}, err
	}
	value := mcpapp.AuthorizationAttempt{ID: id, Server: server, Status: status, CreatedAt: created.UTC()}
	if a.FinishedAt != "" {
		finished, err := time.Parse(time.RFC3339Nano, a.FinishedAt)
		if err != nil {
			return mcpapp.AuthorizationAttempt{}, err
		}
		if err := instant.Validate(finished); err != nil {
			return mcpapp.AuthorizationAttempt{}, err
		}
		if finished.Before(created) {
			return mcpapp.AuthorizationAttempt{}, mcpapp.ErrInvalidServerConfiguration
		}
		finished = finished.UTC()
		value.FinishedAt = &finished
	}
	if (status == mcpapp.AuthorizationAttemptPending) != (value.FinishedAt == nil) {
		return mcpapp.AuthorizationAttempt{}, mcpapp.ErrInvalidServerConfiguration
	}
	return value, nil
}
