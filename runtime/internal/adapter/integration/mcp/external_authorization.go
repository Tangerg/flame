package mcp

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	app "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func (c *ExternalConnector) AdmitProbe(ctx context.Context) (func(), error) {
	u, err := c.acquire(ctx)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, plugin.ErrUnavailable
	}
	return func() { _ = u.session.release(context.Background()) }, nil
}
func (c *ExternalConnector) StartAuthorization(ctx context.Context, server mcpserver.Server, id app.AuthorizationAttemptID) (app.AuthorizationAttempt, func(context.Context, app.AuthorizationAttemptStatus) error, error) {
	u, err := c.acquire(ctx)
	if err != nil {
		return app.AuthorizationAttempt{}, nil, err
	}
	if u == nil {
		return app.AuthorizationAttempt{}, nil, plugin.ErrUnavailable
	}
	release := func() { _ = u.session.release(context.Background()) }
	e, found, err := c.find(ctx, u, server.Name)
	if err != nil || !found {
		release()
		return app.AuthorizationAttempt{}, nil, errors.Join(err, app.ErrUnknownServer)
	}
	current, err := c.materialize(ctx, u, e)
	if err != nil || !current.SameConnection(server) {
		release()
		return app.AuthorizationAttempt{}, nil, errors.Join(err, app.ErrUnknownServer)
	}
	input := wire.Start{Name: e.Name, Grant: e.LaunchGrant, Epoch: u.epoch, AttemptID: id.String()}
	result, err := connectorCall(ctx, u, c.methods.Start, input)
	if err != nil {
		release()
		return app.AuthorizationAttempt{}, nil, err
	}
	attempt, err := readStartedExternalAttempt(result, id, server.ID())
	if err != nil {
		release()
		return app.AuthorizationAttempt{}, nil, err
	}
	source, epoch := u.source, u.epoch
	completionID := operation.Identity(operation.Start(ctx, extension.Automatic, c.methods.Finish.Name(), id.String()))
	parentID := operation.Identity(ctx)
	if err := u.session.release(context.Background()); err != nil {
		return app.AuthorizationAttempt{}, nil, err
	}
	// A browser wait retains no Frame Use. Only the same directory incarnation can settle its original receipt.
	finish := func(ctx context.Context, status app.AuthorizationAttemptStatus) error {
		current, err := c.acquire(ctx)
		if err != nil {
			return err
		}
		if current == nil {
			return plugin.ErrUnavailable
		}
		defer current.session.release(context.Background())
		if !current.source.SameContribution(source) || current.epoch != epoch {
			return plugin.ErrStale
		}
		ctx = operation.Adopt(ctx, completionID, extension.Automatic, parentID)
		input := wire.Completion{ID: id.String(), Epoch: epoch, Status: wire.Outcome(status)}
		decision, err := connectorCall(ctx, current, c.methods.Finish, input)
		if err == nil && decision == wire.Applied {
			return nil
		}
		// A lost acknowledgement or superseding sign-in may already have settled
		// this exact attempt. Only its owner can confirm a terminal status.
		result, readErr := connectorCall(operation.Start(ctx, extension.Automatic, c.methods.Get.Name(), id.String()), current, c.methods.Get, wire.Request{ID: id.String(), Epoch: epoch})
		if readErr == nil {
			observed, decodeErr := readExternalAttempt(result, id)
			if decodeErr == nil && observed.Server == server.ID() && observed.Status != app.AuthorizationAttemptPending {
				return nil
			}
			readErr = decodeErr
		}
		return errors.Join(plugin.ErrUnavailable, err, readErr)
	}
	return attempt, finish, nil
}
func (c *ExternalConnector) ReadAuthorization(ctx context.Context, id app.AuthorizationAttemptID) (app.AuthorizationAttempt, error) {
	u, err := c.acquire(ctx)
	if err != nil {
		return app.AuthorizationAttempt{}, err
	}
	if u == nil {
		return app.AuthorizationAttempt{}, app.ErrAuthorizationAttemptNotFound
	}
	defer u.session.release(context.Background())
	result, err := connectorCall(ctx, u, c.methods.Get, wire.Request{ID: id.String(), Epoch: u.epoch})
	if err != nil {
		return app.AuthorizationAttempt{}, err
	}
	return readExternalAttempt(result, id)
}

func readStartedExternalAttempt(r wire.AttemptResult, id app.AuthorizationAttemptID, server mcpserver.ID) (app.AuthorizationAttempt, error) {
	attempt, err := readExternalAttempt(r, id)
	if err != nil {
		return app.AuthorizationAttempt{}, err
	}
	if attempt.Server != server || attempt.Status != app.AuthorizationAttemptPending {
		return app.AuthorizationAttempt{}, app.ErrInvalidServerConfiguration
	}
	return attempt, nil
}

func readExternalAttempt(r wire.AttemptResult, id app.AuthorizationAttemptID) (app.AuthorizationAttempt, error) {
	if r.Attempt == nil {
		return app.AuthorizationAttempt{}, app.ErrAuthorizationAttemptNotFound
	}
	a := r.Attempt
	status := app.AuthorizationAttemptStatus(a.Status)
	if a.ID != id.String() || !status.Valid() {
		return app.AuthorizationAttempt{}, app.ErrInvalidServerConfiguration
	}
	name, err := mcpserver.ParseServerName(a.Name)
	if err != nil {
		return app.AuthorizationAttempt{}, err
	}
	server, err := mcpserver.NewID(mcpserver.UserOrigin(), name)
	if err != nil {
		return app.AuthorizationAttempt{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, a.StartedAt)
	if err != nil {
		return app.AuthorizationAttempt{}, err
	}
	if created.IsZero() {
		return app.AuthorizationAttempt{}, app.ErrInvalidServerConfiguration
	}
	if err := instant.Validate(created); err != nil {
		return app.AuthorizationAttempt{}, err
	}
	value := app.AuthorizationAttempt{ID: id, Server: server, Status: status, CreatedAt: created.UTC()}
	if a.FinishedAt != "" {
		finished, err := time.Parse(time.RFC3339Nano, a.FinishedAt)
		if err != nil {
			return app.AuthorizationAttempt{}, err
		}
		if err := instant.Validate(finished); err != nil {
			return app.AuthorizationAttempt{}, err
		}
		if finished.Before(created) {
			return app.AuthorizationAttempt{}, app.ErrInvalidServerConfiguration
		}
		finished = finished.UTC()
		value.FinishedAt = &finished
	}
	if (status == app.AuthorizationAttemptPending) != (value.FinishedAt == nil) {
		return app.AuthorizationAttempt{}, app.ErrInvalidServerConfiguration
	}
	return value, nil
}
