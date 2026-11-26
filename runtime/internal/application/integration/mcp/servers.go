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

var errClosed = errors.New("mcp: closed")

var ErrInvalidServerConfiguration = errors.New("mcp: invalid MCP server configuration")

var ErrUnknownServer = errors.New("mcp: unknown MCP server")

var ErrServerAlreadyExists = errors.New("mcp: MCP server already exists")

var ErrServerDisabled = errors.New("mcp: MCP server is disabled")

var ErrOwnedByInstallation = errors.New("mcp: server is owned by installation")

var ErrAuthorizationAttemptNotFound = errors.New("mcp: MCP authorization attempt not found")

var ErrAuthorizationUnsupported = errors.New("mcp: MCP authorization requires streamable HTTP")

func (c *Coordinator) TestServer(ctx context.Context, input ServerInput) (result TestResult, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	release, err := c.external.AdmitProbe(ctx)
	if err != nil {
		return "", err
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			result, err = "", errors.Join(err, releaseErr)
		}
	}()
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
		Enabled:          input.Enabled,
		Description:      input.Description,
		Connection:       connection,
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
		updated.Connection = connection
	}
	if patch.HandshakeTimeout != nil {
		updated.HandshakeTimeout = *patch.HandshakeTimeout
	}
	if err := updated.Validate(); err != nil {
		return mcpserver.Server{}, fmt.Errorf("%w: %w", ErrInvalidServerConfiguration, err)
	}
	return updated, nil
}

func resolveConnection(input ConnectionInput, current *mcpserver.Server) (mcpserver.Connection, error) {
	switch input.Transport {
	case mcpserver.TransportStreamableHTTP:
		return resolveHTTPConnection(input, current)
	case mcpserver.TransportStdio:
		return resolveStdioConnection(input, current)
	default:
		return nil, fmt.Errorf("%w: unknown transport %q", ErrInvalidServerConfiguration, input.Transport)
	}
}

// A secret is inherited only by the same target: a different HTTP origin or
// process must state its credentials again.
func resolveHTTPConnection(input ConnectionInput, current *mcpserver.Server) (mcpserver.Connection, error) {
	if input.Environment != nil {
		return nil, fmt.Errorf("%w: environment applies to stdio transport only", ErrInvalidServerConfiguration)
	}
	var previous mcpserver.HTTPConnection
	wasHTTP := false
	if current != nil {
		previous, wasHTTP = current.Connection.(mcpserver.HTTPConnection)
	}
	sameOrigin := wasHTTP && httporigin.Same(previous.URL, input.URL)
	authorization, err := resolveSecret("authorization", "HTTP origin", input.Authorization, previous.Authorization, current != nil, sameOrigin, func(value string) bool { return value == "" })
	if err != nil {
		return nil, err
	}
	headers, err := resolveSecret("headers", "HTTP origin", input.Headers, previous.Headers, current != nil, sameOrigin, emptyValues)
	if err != nil {
		return nil, err
	}
	return mcpserver.HTTPConnection{URL: input.URL, Authorization: authorization, Headers: maps.Clone(headers)}, nil
}

func resolveStdioConnection(input ConnectionInput, current *mcpserver.Server) (mcpserver.Connection, error) {
	if input.Authorization != nil || input.Headers != nil {
		return nil, fmt.Errorf("%w: authorization and headers apply to HTTP transport only", ErrInvalidServerConfiguration)
	}
	var previous mcpserver.StdioConnection
	wasStdio := false
	if current != nil {
		previous, wasStdio = current.Connection.(mcpserver.StdioConnection)
	}
	sameTarget := wasStdio && previous.Command == input.Command && slices.Equal(previous.Args, input.Args) && previous.Dir == input.Dir
	environment, err := resolveSecret("environment variables", "stdio process target", input.Environment, previous.Env, current != nil, sameTarget, emptyValues)
	if err != nil {
		return nil, err
	}
	return mcpserver.StdioConnection{Command: input.Command, Args: slices.Clone(input.Args), Env: maps.Clone(environment), Dir: input.Dir}, nil
}

type ToolView struct {
	mcpserver.AdvertisedTool
	ModelName string
	Conflicts []tool.Ref
}

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
		views = append(views, ToolView{AdvertisedTool: advertised, ModelName: ref.NameForCatalog(conflicts), Conflicts: conflicts[ref]})
	}
	return views, nil
}

func emptyValues(values map[string]string) bool { return len(values) == 0 }

func resolveSecret[T any](name, target string, change *SecretChange[T], previous T, exists, mayInherit bool, empty func(T) bool) (T, error) {
	var zero T
	switch {
	case change == nil && mayInherit:
		return previous, nil
	case change == nil && exists && !empty(previous):
		return zero, fmt.Errorf("%w: changing the %s requires %s to be explicitly set or cleared", ErrInvalidServerConfiguration, target, name)
	case change == nil:
		return zero, nil
	case change.Kind == SecretSet && empty(change.Value):
		return zero, fmt.Errorf("%w: %s set value is empty", ErrInvalidServerConfiguration, name)
	case change.Kind == SecretSet:
		return change.Value, nil
	case change.Kind == SecretClear && !exists:
		return zero, fmt.Errorf("%w: %s clear requires an existing server", ErrInvalidServerConfiguration, name)
	case change.Kind == SecretClear:
		return zero, nil
	default:
		return zero, fmt.Errorf("%w: unknown %s change", ErrInvalidServerConfiguration, name)
	}
}
