package mcp

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
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

func (c *ExternalConnector) StartAuthorization(ctx context.Context, server mcpserver.Server, id mcpapp.AuthorizationAttemptID) (mcpapp.AuthorizationAttempt, func(context.Context, mcpapp.AuthorizationAttemptStatus) error, error) {
	u, err := c.acquire(ctx)
	if err != nil {
		return mcpapp.AuthorizationAttempt{}, nil, err
	}
	if u == nil {
		return mcpapp.AuthorizationAttempt{}, nil, plugin.ErrUnavailable
	}
	attempt, err := c.startAttempt(ctx, u, server, id)
	if err != nil {
		return mcpapp.AuthorizationAttempt{}, nil, errors.Join(err, u.session.release(context.WithoutCancel(ctx)))
	}
	completion := externalAttemptCompletion{
		connector: c, server: server.ID(), id: id, source: u.source, epoch: u.epoch,
		operationID: operation.Identity(operation.Start(ctx, extension.Automatic, c.methods.Finish.Name(), id.String())),
		parentID:    operation.Identity(ctx),
	}
	if err := u.session.release(context.WithoutCancel(ctx)); err != nil {
		return mcpapp.AuthorizationAttempt{}, nil, err
	}
	return attempt, completion.finish, nil
}

func (c *ExternalConnector) startAttempt(ctx context.Context, u *connectorUse, server mcpserver.Server, id mcpapp.AuthorizationAttemptID) (mcpapp.AuthorizationAttempt, error) {
	e, found, err := c.find(ctx, u, server.Name)
	if err != nil || !found {
		return mcpapp.AuthorizationAttempt{}, errors.Join(err, mcpapp.ErrUnknownServer)
	}
	current, err := c.materialize(ctx, u, e)
	if err != nil || !current.SameConnection(server) {
		return mcpapp.AuthorizationAttempt{}, errors.Join(err, mcpapp.ErrUnknownServer)
	}
	input := wire.Start{Name: e.Name, Grant: e.LaunchGrant, Epoch: u.epoch, AttemptID: id.String()}
	result, err := connectorCall(ctx, u, c.methods.Start, input)
	if err != nil {
		return mcpapp.AuthorizationAttempt{}, err
	}
	return readStartedExternalAttempt(result, id, server.ID())
}

type externalAttemptCompletion struct {
	connector   *ExternalConnector
	server      mcpserver.ID
	id          mcpapp.AuthorizationAttemptID
	source      plugins.ContributionBinding
	epoch       string
	operationID string
	parentID    string
}

func (a externalAttemptCompletion) finish(ctx context.Context, status mcpapp.AuthorizationAttemptStatus) error {
	selected, err := a.connector.use(ctx, func(current *connectorUse) error { return a.settle(ctx, current, status) })
	if err == nil && !selected {
		return plugin.ErrUnavailable
	}
	return err
}

func (a externalAttemptCompletion) settle(ctx context.Context, current *connectorUse, status mcpapp.AuthorizationAttemptStatus) error {
	c := a.connector
	if !current.source.SameContribution(a.source) || current.epoch != a.epoch {
		return plugin.ErrStale
	}
	ctx = operation.Adopt(ctx, a.operationID, extension.Automatic, a.parentID)
	input := wire.Completion{ID: a.id.String(), Epoch: a.epoch, Status: wire.Outcome(status)}
	decision, err := connectorCall(ctx, current, c.methods.Finish, input)
	if err == nil && decision == wire.Applied {
		return nil
	}
	request := wire.Request{ID: a.id.String(), Epoch: a.epoch}
	result, readErr := connectorCall(operation.Start(ctx, extension.Automatic, c.methods.Get.Name(), a.id.String()), current, c.methods.Get, request)
	if readErr == nil {
		observed, decodeErr := readExternalAttempt(result, a.id)
		if decodeErr == nil && observed.Server == a.server && observed.Status != mcpapp.AuthorizationAttemptPending {
			return nil
		}
		readErr = decodeErr
	}
	return errors.Join(plugin.ErrUnavailable, err, readErr)
}

func (c *ExternalConnector) ReadAuthorization(ctx context.Context, id mcpapp.AuthorizationAttemptID) (mcpapp.AuthorizationAttempt, error) {
	var attempt mcpapp.AuthorizationAttempt
	selected, err := c.use(ctx, func(u *connectorUse) error {
		result, err := connectorCall(ctx, u, c.methods.Get, wire.Request{ID: id.String(), Epoch: u.epoch})
		if err != nil {
			return err
		}
		attempt, err = readExternalAttempt(result, id)
		return err
	})
	if err != nil {
		return mcpapp.AuthorizationAttempt{}, err
	}
	if !selected {
		return mcpapp.AuthorizationAttempt{}, mcpapp.ErrAuthorizationAttemptNotFound
	}
	return attempt, nil
}

func readStartedExternalAttempt(r wire.AttemptResult, id mcpapp.AuthorizationAttemptID, server mcpserver.ID) (mcpapp.AuthorizationAttempt, error) {
	attempt, err := readExternalAttempt(r, id)
	if err != nil {
		return mcpapp.AuthorizationAttempt{}, err
	}
	if attempt.Server != server || attempt.Status != mcpapp.AuthorizationAttemptPending {
		return mcpapp.AuthorizationAttempt{}, mcpapp.ErrInvalidServerConfiguration
	}
	return attempt, nil
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
