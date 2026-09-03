package runtimebinding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/cli/internal/domain/failure"
)

type mcpBinding interface {
	ListMCPServers(context.Context, flameruntime.CallOptions) (*protocol.Page[protocol.MCPServer], error)
	CreateMCPServer(context.Context, protocol.MCPServerCandidate, flameruntime.CommandOptions) (*protocol.MCPServer, error)
	UpdateMCPServer(context.Context, protocol.UpdateMCPServerRequest, flameruntime.CommandOptions) (*protocol.MCPServer, error)
	DeleteMCPServer(context.Context, protocol.MCPServerRequest, flameruntime.CommandOptions) error
	TestMCPServer(context.Context, protocol.MCPServerCandidate, flameruntime.CallOptions) (*protocol.MCPTestResult, error)
	ListMCPTools(context.Context, protocol.MCPListToolsRequest, flameruntime.CallOptions) (*protocol.Page[protocol.MCPTool], error)
	ReconnectMCPServer(context.Context, protocol.MCPServerRequest, flameruntime.CommandOptions) error
	CreateMCPAuthorizationAttempt(context.Context, protocol.CreateMCPAuthorizationAttemptRequest, flameruntime.CommandOptions) (*protocol.MCPAuthorizationAttempt, error)
	GetMCPAuthorizationAttempt(context.Context, protocol.MCPAuthorizationAttemptRequest, flameruntime.CallOptions) (*protocol.MCPAuthorizationAttempt, error)
}

func (r *Connection) Servers(ctx context.Context) ([]mcp.Server, error) {
	page, err := r.mcp.ListMCPServers(ctx, r.callOptions())
	if err != nil {
		return nil, classifyMCPError(err)
	}
	values, err := requireCompletePage("list MCP servers", page)
	if err != nil {
		return nil, err
	}
	return projectUniqueValuesFallible("list MCP servers", values, projectMCPServer, func(server mcp.Server) string {
		return server.Name
	})
}

func (r *Connection) CreateServer(ctx context.Context, candidate mcp.Candidate) (mcp.Server, error) {
	if err := candidate.Validate(); err != nil {
		return mcp.Server{}, err
	}
	request, err := projectMCPCandidate(candidate)
	if err != nil {
		return mcp.Server{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return mcp.Server{}, err
	}
	result, err := r.mcp.CreateMCPServer(ctx, request, options)
	projected, err := projectMCPServerResult("create MCP server", candidate.Name, result, err)
	if err != nil {
		return mcp.Server{}, err
	}
	if err := candidate.ValidateResult(projected); err != nil {
		return mcp.Server{}, runtimeContractViolation("create MCP server returned an invalid acknowledgement: %v", err)
	}
	return projected, nil
}

func (r *Connection) UpdateServer(ctx context.Context, update mcp.ServerUpdate) (mcp.Server, error) {
	if err := update.Validate(); err != nil {
		return mcp.Server{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return mcp.Server{}, err
	}
	request := protocol.UpdateMCPServerRequest{
		Server: update.Server, Enabled: clonePointer(update.Enabled), Description: clonePointer(update.Description),
	}
	if update.HandshakeTimeout != nil {
		timeout := projectMCPHandshakeTimeout(*update.HandshakeTimeout)
		request.HandshakeTimeout = &timeout
	}
	if update.Connection != nil {
		connection := projectMCPConnectionInput(*update.Connection)
		request.Connection = &connection
	}
	if update.DisabledTools != nil {
		values := slices.Clone(*update.DisabledTools)
		request.DisabledTools = &values
	}
	if update.AutoApproveTools != nil {
		values := slices.Clone(*update.AutoApproveTools)
		request.AutoApproveTools = &values
	}
	if err := protocol.ValidateWireTree(request); err != nil {
		return mcp.Server{}, fmt.Errorf("MCP update %s violates runtime wire contract: %w", update.Server, err)
	}
	result, err := r.mcp.UpdateMCPServer(ctx, request, options)
	projected, err := projectMCPServerResult("update MCP server", update.Server, result, err)
	if err != nil {
		return mcp.Server{}, err
	}
	if err := update.ValidateResult(projected); err != nil {
		return mcp.Server{}, runtimeContractViolation("update MCP server returned an invalid acknowledgement: %v", err)
	}
	return projected, nil
}

func (r *Connection) DeleteServer(ctx context.Context, server string) error {
	return r.mutateMCPServer(ctx, "delete MCP server", server, r.mcp.DeleteMCPServer)
}

func (r *Connection) ReconnectServer(ctx context.Context, server string) error {
	return r.mutateMCPServer(ctx, "reconnect MCP server", server, r.mcp.ReconnectMCPServer)
}

func (r *Connection) mutateMCPServer(
	ctx context.Context,
	operation, server string,
	mutate func(context.Context, protocol.MCPServerRequest, flameruntime.CommandOptions) error,
) error {
	server = strings.TrimSpace(server)
	if server == "" {
		return fmt.Errorf("%s: server name is empty", operation)
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyMCPError(mutate(ctx, protocol.MCPServerRequest{Server: server}, options))
}

func (r *Connection) TestServer(ctx context.Context, candidate mcp.Candidate) (mcp.TestResult, error) {
	if err := candidate.Validate(); err != nil {
		return mcp.TestResult{}, err
	}
	request, err := projectMCPCandidate(candidate)
	if err != nil {
		return mcp.TestResult{}, err
	}
	result, err := r.mcp.TestMCPServer(ctx, request, r.callOptions())
	if err != nil {
		return mcp.TestResult{}, classifyMCPError(err)
	}
	if result == nil {
		return mcp.TestResult{}, runtimeContractViolation("test MCP server returned nil")
	}
	projected := mcp.TestResult{OK: result.OK, Problem: failure.Clone(result.Error)}
	if err := projected.Validate(); err != nil {
		return mcp.TestResult{}, runtimeContractViolation("test MCP server returned an invalid result: %v", err)
	}
	return projected, nil
}

func (r *Connection) Tools(ctx context.Context, server string) ([]mcp.Tool, error) {
	server = strings.TrimSpace(server)
	page, err := r.mcp.ListMCPTools(ctx, protocol.MCPListToolsRequest{Server: server}, r.callOptions())
	if err != nil {
		return nil, classifyMCPError(err)
	}
	values, err := requireCompletePage("list MCP tools", page)
	if err != nil {
		return nil, err
	}
	tools := make([]mcp.Tool, 0, len(values))
	seen := make(map[[2]string]struct{}, len(values))
	for index, value := range values {
		tool := mcp.Tool{Server: value.Server, Name: value.Name, Description: value.Description}
		if value.InputSchema != nil {
			schema, marshalErr := json.Marshal(value.InputSchema)
			if marshalErr != nil {
				return nil, runtimeContractViolation("list MCP tools item %d has an invalid schema: %v", index+1, marshalErr)
			}
			tool.InputSchema = schema
		}
		if err := tool.Validate(); err != nil {
			return nil, runtimeContractViolation("list MCP tools item %d is invalid: %v", index+1, err)
		}
		if server != "" && tool.Server != server {
			return nil, runtimeContractViolation("list MCP tools for %q returned a tool from %q", server, tool.Server)
		}
		identity := [2]string{tool.Server, tool.Name}
		if _, duplicate := seen[identity]; duplicate {
			return nil, runtimeContractViolation("list MCP tools repeats %s/%s", tool.Server, tool.Name)
		}
		seen[identity] = struct{}{}
		tools = append(tools, tool)
	}
	return tools, nil
}

func (r *Connection) StartAuthorization(ctx context.Context, server string) (mcp.AuthorizationAttempt, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return mcp.AuthorizationAttempt{}, errors.New("start MCP authorization: server name is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return mcp.AuthorizationAttempt{}, err
	}
	result, err := r.mcp.CreateMCPAuthorizationAttempt(ctx, protocol.CreateMCPAuthorizationAttemptRequest{Server: server}, options)
	return projectMCPAuthorizationResult("start MCP authorization", mcpAuthorizationIdentity{server: server}, result, err)
}

func (r *Connection) GetAuthorization(ctx context.Context, reference mcp.AuthorizationReference) (mcp.AuthorizationAttempt, error) {
	if err := reference.Validate(); err != nil {
		return mcp.AuthorizationAttempt{}, fmt.Errorf("get MCP authorization: %w", err)
	}
	result, err := r.mcp.GetMCPAuthorizationAttempt(ctx, protocol.MCPAuthorizationAttemptRequest{AttemptID: reference.ID}, r.callOptions())
	return projectMCPAuthorizationResult(
		"get MCP authorization",
		mcpAuthorizationIdentity{attemptID: reference.ID, server: reference.Server},
		result,
		err,
	)
}

func projectMCPServerResult(operation, expectedName string, result *protocol.MCPServer, err error) (mcp.Server, error) {
	if err != nil {
		return mcp.Server{}, classifyMCPError(err)
	}
	if result == nil {
		return mcp.Server{}, runtimeContractViolation("%s returned nil", operation)
	}
	server, projectionErr := projectMCPServer(*result)
	if projectionErr != nil {
		return mcp.Server{}, runtimeContractViolation("%s returned an invalid handshake timeout: %v", operation, projectionErr)
	}
	if err := server.Validate(); err != nil {
		return mcp.Server{}, runtimeContractViolation("%s returned an invalid server: %v", operation, err)
	}
	if server.Name != expectedName {
		return mcp.Server{}, runtimeContractViolation(
			"%s returned server %q for %q",
			operation,
			server.Name,
			expectedName,
		)
	}
	return server, nil
}

func projectMCPServer(value protocol.MCPServer) (mcp.Server, error) {
	timeout, err := mcpHandshakeTimeoutFromWire(value.HandshakeTimeout)
	if err != nil {
		return mcp.Server{}, err
	}
	return mcp.Server{
		Name: value.Name, Description: value.Description,
		Connection: mcp.Connection{
			Transport: value.Connection.Type, URL: value.Connection.URL,
			AuthorizationMasked: value.Connection.AuthorizationMasked,
			HeadersMasked:       maps.Clone(value.Connection.HeadersMasked),
			Command:             value.Connection.Command, Args: slices.Clone(value.Connection.Args),
			EnvironmentMasked: maps.Clone(value.Connection.EnvMasked), Directory: value.Connection.Dir,
		},
		HandshakeTimeout: timeout, DisabledTools: slices.Clone(value.DisabledTools),
		AutoApproveTools: slices.Clone(value.AutoApproveTools),
		State: mcp.State{
			Type: value.Status.Type, ToolCount: clonePointer(value.Status.ToolCount),
			Problem: failure.Clone(value.Status.Error),
		},
	}, nil
}

func projectMCPCandidate(candidate mcp.Candidate) (protocol.MCPServerCandidate, error) {
	projected := protocol.MCPServerCandidate{
		Name: candidate.Name, Enabled: candidate.Enabled, Description: candidate.Description,
		Connection: projectMCPConnectionInput(candidate.Connection), HandshakeTimeout: projectMCPHandshakeTimeout(candidate.HandshakeTimeout),
		DisabledTools: slices.Clone(candidate.DisabledTools), AutoApproveTools: slices.Clone(candidate.AutoApproveTools),
	}
	if err := protocol.ValidateWireTree(projected); err != nil {
		return protocol.MCPServerCandidate{}, fmt.Errorf("MCP candidate %s violates runtime wire contract: %w", candidate.Name, err)
	}
	return projected, nil
}

func projectMCPHandshakeTimeout(timeout mcp.HandshakeTimeout) protocol.MCPHandshakeTimeout {
	seconds, bounded := timeout.Seconds()
	if !bounded {
		return protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded}
	}
	return protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeBounded, Seconds: &seconds}
}

func mcpHandshakeTimeoutFromWire(timeout protocol.MCPHandshakeTimeout) (mcp.HandshakeTimeout, error) {
	switch timeout.Type {
	case protocol.MCPHandshakeUnbounded:
		if timeout.Seconds != nil {
			return mcp.HandshakeTimeout{}, errors.New("unbounded policy carries seconds")
		}
		return mcp.HandshakeTimeout{}, nil
	case protocol.MCPHandshakeBounded:
		if timeout.Seconds == nil {
			return mcp.HandshakeTimeout{}, errors.New("bounded policy omits seconds")
		}
		return mcp.NewHandshakeTimeout(*timeout.Seconds)
	default:
		return mcp.HandshakeTimeout{}, fmt.Errorf("unknown policy %q", timeout.Type)
	}
}

func projectMCPConnectionInput(connection mcp.ConnectionInput) protocol.MCPConnectionInput {
	projected := protocol.MCPConnectionInput{
		Type: connection.Transport, URL: connection.URL,
		Command: connection.Command, Args: slices.Clone(connection.Args), Dir: connection.Directory,
	}
	if connection.Authorization != nil {
		projected.Authorization = &protocol.MCPAuthorizationChange{
			Type: connection.Authorization.Kind, Value: connection.Authorization.Value,
		}
	}
	if connection.Headers != nil {
		projected.Headers = &protocol.MCPHeadersChange{
			Type: connection.Headers.Kind, Value: maps.Clone(connection.Headers.Value),
		}
	}
	if connection.Environment != nil {
		projected.Env = &protocol.MCPEnvironmentChange{
			Type: connection.Environment.Kind, Value: maps.Clone(connection.Environment.Value),
		}
	}
	return projected
}

type mcpAuthorizationIdentity struct {
	attemptID string
	server    string
}

func projectMCPAuthorizationResult(
	operation string,
	expected mcpAuthorizationIdentity,
	result *protocol.MCPAuthorizationAttempt,
	err error,
) (mcp.AuthorizationAttempt, error) {
	if err != nil {
		return mcp.AuthorizationAttempt{}, classifyMCPError(err)
	}
	if result == nil {
		return mcp.AuthorizationAttempt{}, runtimeContractViolation("%s returned nil", operation)
	}
	attempt := mcp.AuthorizationAttempt{
		ID: result.ID, Server: result.Server, Status: result.Status.Type,
		Problem: failure.Clone(result.Status.Error), CreatedAt: result.CreatedAt,
		FinishedAt: clonePointer(result.FinishedAt),
	}
	if err := attempt.Validate(); err != nil {
		return mcp.AuthorizationAttempt{}, runtimeContractViolation(
			"%s returned an invalid authorization attempt: %v",
			operation,
			err,
		)
	}
	if expected.attemptID != "" && attempt.ID != expected.attemptID {
		return mcp.AuthorizationAttempt{}, runtimeContractViolation(
			"%s returned attempt %q for %q",
			operation,
			attempt.ID,
			expected.attemptID,
		)
	}
	if attempt.Server != expected.server {
		return mcp.AuthorizationAttempt{}, runtimeContractViolation(
			"%s returned server %q for %q",
			operation,
			attempt.Server,
			expected.server,
		)
	}
	return attempt, nil
}

func classifyMCPError(err error) error {
	classified := classifyError(err)
	for _, mapping := range []struct {
		source error
		target error
	}{
		{protocol.ErrMCPServerNotFound, mcp.ErrServerNotFound},
		{protocol.ErrMCPServerAlreadyExists, mcp.ErrServerAlreadyExists},
		{protocol.ErrMCPServerDisabled, mcp.ErrServerDisabled},
		{protocol.ErrMCPAuthorizationAttemptNotFound, mcp.ErrAuthorizationAttemptNotFound},
	} {
		if errors.Is(classified, mapping.source) {
			return fmt.Errorf("%w: %w", mapping.target, classified)
		}
	}
	return classified
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return new(*value)
}
