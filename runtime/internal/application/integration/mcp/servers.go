package mcp

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// errClosed reports that a post-commit reconcile / background task could not be
// launched because the component is shutting down.
var errClosed = errors.New("mcp: closed")

// ErrInvalidServerConfiguration marks a malformed MCP configuration command.
// Callers map it to their validation error without re-running domain
// validation or inspecting persistence state.
var ErrInvalidServerConfiguration = errors.New("mcp: invalid MCP server configuration")

// ErrUnknownServer is the application boundary's stable unknown-server
// result. The underlying domain sentinel remains internal to this package.
var ErrUnknownServer = errors.New("mcp: unknown MCP server")

// ErrServerAlreadyExists reports a create whose stable name is already in
// use. Create and update remain distinct operations; storage never guesses from
// an upsert whether the caller meant to replace an existing resource.
var ErrServerAlreadyExists = errors.New("mcp: MCP server already exists")

// ErrServerDisabled reports a connection command against a configured
// server whose durable enablement gate is closed.
var ErrServerDisabled = errors.New("mcp: MCP server is disabled")

var ErrOwnedByInstallation = errors.New("mcp: server is owned by installation")

// ErrAuthorizationAttemptNotFound reports an unknown or expired interactive
// authorization attempt.
var ErrAuthorizationAttemptNotFound = errors.New("mcp: MCP authorization attempt not found")

// ErrAuthorizationUnsupported reports interactive authorization requested
// for a transport that cannot perform OAuth.
var ErrAuthorizationUnsupported = errors.New("mcp: MCP authorization requires streamable HTTP")

// CreateServer creates one durable resource and projects it into the live MCP
// pool. A duplicate name is a conflict, never an implicit update.
func (c *Coordinator) CreateServer(ctx context.Context, input ServerInput) (Server, error) {
	id, err := userServer(input.Name)
	if err != nil {
		return Server{}, err
	}
	write, err := c.beginMutation(ctx)
	if err != nil {
		return Server{}, err
	}
	defer write.close()
	if _, found, getErr := c.registry.Definition(write.requestCtx, id); getErr != nil {
		return Server{}, getErr
	} else if found {
		return Server{}, ErrServerAlreadyExists
	}
	srv, err := serverCandidate(input, nil)
	if err != nil {
		return Server{}, err
	}
	return c.commitServer(write, srv, true)
}

// UpdateServer applies an explicit partial update to an existing resource.
// The mutation lock keeps the read/patch/save sequence atomic inside the runtime.
func (c *Coordinator) UpdateServer(ctx context.Context, name mcpserver.ID, patch ServerPatch) (Server, error) {
	if name.Origin().Kind() != mcpserver.OriginUser {
		return Server{}, ErrOwnedByInstallation
	}
	if patch.Empty() {
		return Server{}, fmt.Errorf("%w: update contains no changes", ErrInvalidServerConfiguration)
	}
	write, err := c.beginMutation(ctx)
	if err != nil {
		return Server{}, err
	}
	defer write.close()
	current, found, err := c.registry.Definition(write.requestCtx, name)
	if err != nil {
		return Server{}, err
	}
	if !found {
		return Server{}, ErrUnknownServer
	}
	updated, err := applyServerPatch(current, patch)
	if err != nil {
		return Server{}, err
	}
	return c.commitServer(write, updated, !current.Enabled || !current.SameConnection(updated))
}

func (c *Coordinator) commitServer(write *mutationScope, srv mcpserver.Server, reconnect bool) (Server, error) {
	if err := c.store.Save(write.requestCtx, srv); err != nil {
		return Server{}, err
	}
	reconcileErr := c.applyRegistryChange(srv)
	var redialErr error
	start := make(chan struct{})
	defer close(start)
	if srv.Enabled && reconnect && reconcileErr == nil {
		redialErr = c.dispatchConnection(write.ownerCtx, srv.ID(), c.connections.BeginConnect, start, nil, nil)
	}
	event := c.statusQueue.prepare(srv.ID())
	write.unlock()
	c.statusQueue.publish(event)
	if reconcileErr != nil {
		return Server{}, reconcileErr
	}
	if redialErr != nil {
		return Server{}, redialErr
	}
	status, ok := c.statusesByName()[srv.ID()]
	if ok {
		return serverView(srv, &status)
	}
	return serverView(srv, nil)
}

// DeleteServer deletes a server from the registry and drops it from the live
// connections.
func (c *Coordinator) DeleteServer(ctx context.Context, name mcpserver.ID) error {
	if name.Origin().Kind() != mcpserver.OriginUser {
		return ErrOwnedByInstallation
	}
	write, err := c.beginMutation(ctx)
	if err != nil {
		return err
	}
	defer write.close()
	if _, found, err := c.registry.Definition(write.requestCtx, name); err != nil {
		return err
	} else if !found {
		return ErrUnknownServer
	}
	if err := c.store.Remove(write.requestCtx, name.Name()); err != nil {
		return err
	}
	// Detachment retires live executables; exposure still closes if it fails.
	projectionErr := c.connections.Detach(name)
	c.exposure.removeServer(name)
	event := c.statusQueue.prepare(name)
	write.unlock()
	c.statusQueue.publish(event)
	return projectionErr
}

// mutationScope owns one registry mutation's request lifetime, detached repair
// lifetime, and serialization lock.
type mutationScope struct {
	coordinator *Coordinator
	requestCtx  context.Context
	ownerCtx    context.Context
	finish      func()
	locked      bool
}

// beginMutation owns both task scopes and the durable mutation lock. Callers
// release the lock before invoking status sinks or dispatching live dials, then
// defer close to release the task scopes on every exit.
func (c *Coordinator) beginMutation(ctx context.Context) (*mutationScope, error) {
	if ctx == nil {
		return nil, errors.New("mcp: mutation context is required")
	}
	ownerCtx, releaseOwner, ok := c.tasks.Attach(ctx)
	if !ok {
		return nil, errClosed
	}
	requestCtx, releaseRequest, ok := c.tasks.AttachLinked(ctx)
	if !ok {
		releaseOwner()
		return nil, errClosed
	}
	c.mutationMu.Lock()
	if err := requestCtx.Err(); err != nil {
		c.mutationMu.Unlock()
		releaseRequest()
		releaseOwner()
		return nil, err
	}
	return &mutationScope{
		coordinator: c,
		requestCtx:  requestCtx,
		ownerCtx:    ownerCtx,
		finish: func() {
			releaseRequest()
			releaseOwner()
		},
		locked: true,
	}, nil
}

func (m *mutationScope) unlock() {
	if m != nil && m.locked {
		m.locked = false
		m.coordinator.mutationMu.Unlock()
	}
}

func (m *mutationScope) close() {
	if m == nil || m.finish == nil {
		return
	}
	m.unlock()
	m.finish()
	m.finish = nil
}

// Publish the committed enablement even when the live adapter fails to detach.
// The exposure gate must not depend on a second registry read after the write.
func (c *Coordinator) applyRegistryChange(srv mcpserver.Server) error {
	var err error
	if !srv.Enabled {
		err = c.connections.Detach(srv.ID())
	}
	c.exposure.setServer(srv)
	return err
}

// TestServer dials srv with a throwaway client and proves its tools list — a
// connection test that touches neither the registry nor the live set, EXCEPT it
// reuses an active OAuth sign-in for the same-named server (so an authorized
// OAuth server tests as connected, not "unauthorized"). Returns the dial /
// tools-list failure as a sanitized outcome; invalid candidates and registry failures
// are returned as errors.
func (c *Coordinator) TestServer(ctx context.Context, input ServerInput) (TestResult, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	srv, err := c.validatedServer(ctx, input)
	if err != nil {
		return "", err
	}
	probeErr := c.connections.Probe(ctx, srv)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	switch {
	case probeErr == nil:
		return TestSucceeded, nil
	case errors.Is(probeErr, mcpserver.ErrAuthorizationRequired):
		return TestAuthorizationRequired, nil
	case errors.Is(probeErr, context.DeadlineExceeded):
		return TestTimedOut, nil
	default:
		return TestFailed, nil
	}
}

func (c *Coordinator) validatedServer(ctx context.Context, input ServerInput) (mcpserver.Server, error) {
	var current *mcpserver.Server
	if id, err := userServer(input.Name); err == nil {
		stored, found, err := c.registry.Definition(ctx, id)
		if err != nil {
			return mcpserver.Server{}, err
		}
		if found {
			current = &stored
		}
	}
	return serverCandidate(input, current)
}

// Create and test address the user's registry; an installation's records
// arrive only through its own admission.
func userServer(name mcpserver.ServerName) (mcpserver.ID, error) {
	id, err := mcpserver.NewID(mcpserver.UserOrigin(), name)
	if err != nil {
		return mcpserver.ID{}, fmt.Errorf("%w: %w", ErrInvalidServerConfiguration, err)
	}
	return id, nil
}

func serverCandidate(input ServerInput, current *mcpserver.Server) (mcpserver.Server, error) {
	connection, err := resolveConnection(input.Connection, current)
	if err != nil {
		return mcpserver.Server{}, err
	}
	srv := mcpserver.Server{
		Source:           mcpserver.UserSource(),
		Name:             input.Name,
		Transport:        connection.Transport,
		Enabled:          input.Enabled,
		Description:      input.Description,
		URL:              connection.URL,
		Authorization:    connection.Authorization,
		Headers:          connection.Headers,
		Command:          connection.Command,
		Args:             connection.Args,
		Env:              connection.Env,
		Dir:              connection.Dir,
		HandshakeTimeout: input.HandshakeTimeout,
	}
	if err := srv.Validate(); err != nil {
		return mcpserver.Server{}, fmt.Errorf("%w: %w", ErrInvalidServerConfiguration, err)
	}
	return srv, nil
}

func applyServerPatch(current mcpserver.Server, patch ServerPatch) (mcpserver.Server, error) {
	updated := current
	if patch.Enabled != nil {
		updated.Enabled = *patch.Enabled
	}
	if patch.Description != nil {
		updated.Description = *patch.Description
	}
	if patch.Connection != nil {
		connection, err := resolveConnection(*patch.Connection, &current)
		if err != nil {
			return mcpserver.Server{}, err
		}
		updated.Transport = connection.Transport
		updated.URL = connection.URL
		updated.Authorization = connection.Authorization
		updated.Headers = connection.Headers
		updated.Command = connection.Command
		updated.Args = connection.Args
		updated.Env = connection.Env
		updated.Dir = connection.Dir
	}
	if patch.HandshakeTimeout != nil {
		updated.HandshakeTimeout = *patch.HandshakeTimeout
	}
	if err := updated.Validate(); err != nil {
		return mcpserver.Server{}, fmt.Errorf("%w: %w", ErrInvalidServerConfiguration, err)
	}
	return updated, nil
}

func resolveConnection(input ConnectionInput, current *mcpserver.Server) (mcpserver.Server, error) {
	connection := mcpserver.Server{
		Transport: input.Transport,
		URL:       input.URL,
		Command:   input.Command,
		Args:      slices.Clone(input.Args),
		Dir:       input.Dir,
	}
	switch input.Transport {
	case mcpserver.TransportStreamableHTTP:
		return resolveHTTPConnection(input, current, connection)
	case mcpserver.TransportStdio:
		return resolveStdioConnection(input, current, connection)
	default:
		return mcpserver.Server{}, fmt.Errorf("%w: unknown transport %q", ErrInvalidServerConfiguration, input.Transport)
	}
}

func resolveHTTPConnection(
	input ConnectionInput,
	current *mcpserver.Server,
	connection mcpserver.Server,
) (mcpserver.Server, error) {
	if input.Environment != nil {
		return mcpserver.Server{}, fmt.Errorf(
			"%w: environment applies to stdio transport only",
			ErrInvalidServerConfiguration,
		)
	}
	sameOrigin := current != nil &&
		current.Transport == mcpserver.TransportStreamableHTTP &&
		httporigin.Same(current.URL, input.URL)
	authorization, err := resolveAuthorization(input.Authorization, current, sameOrigin)
	if err != nil {
		return mcpserver.Server{}, err
	}
	headers, err := resolveHeaders(input.Headers, current, sameOrigin)
	if err != nil {
		return mcpserver.Server{}, err
	}
	connection.Authorization = authorization
	connection.Headers = headers
	return connection, nil
}

func resolveAuthorization(
	change *AuthorizationChange,
	current *mcpserver.Server,
	mayInherit bool,
) (string, error) {
	switch {
	case change == nil && mayInherit:
		return current.Authorization, nil
	case change == nil && current != nil && current.Authorization != "":
		return "", fmt.Errorf(
			"%w: changing the HTTP origin requires authorization to be explicitly set or cleared",
			ErrInvalidServerConfiguration,
		)
	case change == nil:
		return "", nil
	case change.Kind == SecretSet && change.Value == "":
		return "", fmt.Errorf("%w: authorization set value is empty", ErrInvalidServerConfiguration)
	case change.Kind == SecretSet:
		return change.Value, nil
	case change.Kind == SecretClear && current == nil:
		return "", fmt.Errorf(
			"%w: authorization clear requires an existing server",
			ErrInvalidServerConfiguration,
		)
	case change.Kind == SecretClear:
		return "", nil
	default:
		return "", fmt.Errorf("%w: unknown authorization change", ErrInvalidServerConfiguration)
	}
}

func resolveHeaders(
	change *HeadersChange,
	current *mcpserver.Server,
	mayInherit bool,
) (map[string]string, error) {
	switch {
	case change == nil && mayInherit:
		return maps.Clone(current.Headers), nil
	case change == nil && current != nil && len(current.Headers) > 0:
		return nil, fmt.Errorf(
			"%w: changing the HTTP origin requires headers to be explicitly set or cleared",
			ErrInvalidServerConfiguration,
		)
	case change == nil:
		return nil, nil
	case change.Kind == SecretSet && len(change.Value) == 0:
		return nil, fmt.Errorf("%w: headers set value is empty", ErrInvalidServerConfiguration)
	case change.Kind == SecretSet:
		return maps.Clone(change.Value), nil
	case change.Kind == SecretClear && current == nil:
		return nil, fmt.Errorf(
			"%w: headers clear requires an existing server",
			ErrInvalidServerConfiguration,
		)
	case change.Kind == SecretClear:
		return nil, nil
	default:
		return nil, fmt.Errorf("%w: unknown headers change", ErrInvalidServerConfiguration)
	}
}

func resolveStdioConnection(
	input ConnectionInput,
	current *mcpserver.Server,
	connection mcpserver.Server,
) (mcpserver.Server, error) {
	if input.Authorization != nil || input.Headers != nil {
		return mcpserver.Server{}, fmt.Errorf(
			"%w: authorization and headers apply to HTTP transport only",
			ErrInvalidServerConfiguration,
		)
	}
	sameTarget := current != nil &&
		current.Transport == mcpserver.TransportStdio &&
		current.Command == input.Command &&
		slices.Equal(current.Args, input.Args) &&
		current.Dir == input.Dir
	environment, err := resolveEnvironment(input.Environment, current, sameTarget)
	if err != nil {
		return mcpserver.Server{}, err
	}
	connection.Env = environment
	return connection, nil
}

func resolveEnvironment(
	change *EnvironmentChange,
	current *mcpserver.Server,
	mayInherit bool,
) (map[string]string, error) {
	switch {
	case change == nil && mayInherit:
		return maps.Clone(current.Env), nil
	case change == nil && current != nil && len(current.Env) > 0:
		return nil, fmt.Errorf(
			"%w: changing the stdio process target requires environment variables to be explicitly set or cleared",
			ErrInvalidServerConfiguration,
		)
	case change == nil:
		return nil, nil
	case change.Kind == SecretSet && len(change.Value) == 0:
		return nil, fmt.Errorf("%w: environment set value is empty", ErrInvalidServerConfiguration)
	case change.Kind == SecretSet:
		return maps.Clone(change.Value), nil
	case change.Kind == SecretClear && current == nil:
		return nil, fmt.Errorf(
			"%w: environment clear requires an existing server",
			ErrInvalidServerConfiguration,
		)
	case change.Kind == SecretClear:
		return nil, nil
	default:
		return nil, fmt.Errorf("%w: unknown environment change", ErrInvalidServerConfiguration)
	}
}

type ToolView struct {
	mcpserver.AdvertisedTool
	ModelName string
	Conflicts []tool.Ref
}

// Tools lists tools advertised by the connected MCP servers (scoped to server
// when non-empty) for tool discovery, ordered by server then tool name.
func (c *Coordinator) Tools(_ context.Context, server *mcpserver.ID) ([]ToolView, error) {
	if server != nil {
		if err := server.Validate(); err != nil {
			return nil, fmt.Errorf("mcp: tool catalog server: %w", err)
		}
	}
	tools, conflicts, err := c.toolCatalog.MCPTools(server)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(tools, func(first, second mcpserver.AdvertisedTool) int {
		return cmp.Or(
			first.Server.Compare(second.Server),
			cmp.Compare(first.Name.String(), second.Name.String()),
		)
	})
	views := make([]ToolView, 0, len(tools))
	for _, advertised := range tools {
		ref, err := tool.MCP(advertised.Server, advertised.Name)
		if err != nil {
			return nil, err
		}
		views = append(views, ToolView{AdvertisedTool: advertised, ModelName: ref.ModelName(), Conflicts: conflicts[ref]})
	}
	return views, nil
}
