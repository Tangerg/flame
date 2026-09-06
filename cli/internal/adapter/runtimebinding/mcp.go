package runtimebinding

import (
	"context"
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

func (r *Connection) Servers(ctx context.Context) ([]protocol.MCPServer, error) {
	page, err := r.mcp.ListMCPServers(ctx, r.callOptions())
	if err != nil {
		return nil, classifyMCPError(err)
	}
	values, err := requireCompletePage("list MCP servers", page)
	if err != nil {
		return nil, err
	}
	for index, server := range values {
		if err := mcp.ValidateServer(server); err != nil {
			return nil, runtimeContractViolation("list MCP servers item %d is invalid: %v", index+1, err)
		}
		if index == 0 {
			continue
		}
		previous := values[index-1]
		if server.Name == previous.Name {
			return nil, runtimeContractViolation("list MCP servers repeats %q", server.Name)
		}
		if server.Name < previous.Name {
			return nil, runtimeContractViolation(
				"list MCP servers returned server %q out of catalog order after %q",
				server.Name, previous.Name,
			)
		}
	}
	return values, nil
}

func (r *Connection) CreateServer(ctx context.Context, candidate mcp.Candidate) (protocol.MCPServer, error) {
	if err := candidate.Validate(); err != nil {
		return protocol.MCPServer{}, err
	}
	request, err := projectMCPCandidate(candidate)
	if err != nil {
		return protocol.MCPServer{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return protocol.MCPServer{}, err
	}
	result, err := r.mcp.CreateMCPServer(ctx, request, options)
	if err != nil {
		return protocol.MCPServer{}, classifyMCPError(err)
	}
	if result == nil {
		return protocol.MCPServer{}, runtimeContractViolation("create MCP server returned nil")
	}
	if err := candidate.ValidateResult(*result); err != nil {
		return protocol.MCPServer{}, runtimeContractViolation("create MCP server returned an invalid acknowledgement: %v", err)
	}
	return *result, nil
}

func (r *Connection) UpdateServer(ctx context.Context, update mcp.ServerUpdate) (protocol.MCPServer, error) {
	if err := update.Validate(); err != nil {
		return protocol.MCPServer{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return protocol.MCPServer{}, err
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
		return protocol.MCPServer{}, fmt.Errorf("MCP update %s violates runtime wire contract: %w", update.Server, err)
	}
	result, err := r.mcp.UpdateMCPServer(ctx, request, options)
	if err != nil {
		return protocol.MCPServer{}, classifyMCPError(err)
	}
	if result == nil {
		return protocol.MCPServer{}, runtimeContractViolation("update MCP server returned nil")
	}
	if err := update.ValidateResult(*result); err != nil {
		return protocol.MCPServer{}, runtimeContractViolation("update MCP server returned an invalid acknowledgement: %v", err)
	}
	return *result, nil
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
	request := protocol.MCPServerRequest{Server: strings.TrimSpace(server)}
	if err := request.ValidateWire(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyMCPError(mutate(ctx, request, options))
}

func (r *Connection) TestServer(ctx context.Context, candidate mcp.Candidate) (protocol.MCPTestResult, error) {
	if err := candidate.Validate(); err != nil {
		return protocol.MCPTestResult{}, err
	}
	request, err := projectMCPCandidate(candidate)
	if err != nil {
		return protocol.MCPTestResult{}, err
	}
	result, err := r.mcp.TestMCPServer(ctx, request, r.callOptions())
	if err != nil {
		return protocol.MCPTestResult{}, classifyMCPError(err)
	}
	if result == nil {
		return protocol.MCPTestResult{}, runtimeContractViolation("test MCP server returned nil")
	}
	if err := protocol.ValidateWireTree(*result); err != nil {
		return protocol.MCPTestResult{}, runtimeContractViolation("test MCP server returned an invalid result: %v", err)
	}
	if result.OK == (result.Error != nil) {
		return protocol.MCPTestResult{}, runtimeContractViolation("test MCP server returned contradictory success and error states")
	}
	return *result, nil
}

func (r *Connection) Tools(ctx context.Context, server string) ([]protocol.MCPTool, error) {
	request := protocol.MCPListToolsRequest{Server: strings.TrimSpace(server)}
	if err := request.ValidateWire(); err != nil {
		return nil, fmt.Errorf("list MCP tools: %w", err)
	}
	page, err := r.mcp.ListMCPTools(ctx, request, r.callOptions())
	if err != nil {
		return nil, classifyMCPError(err)
	}
	values, err := requireCompletePage("list MCP tools", page)
	if err != nil {
		return nil, err
	}
	for index, tool := range values {
		if err := protocol.ValidateWireTree(tool); err != nil {
			return nil, runtimeContractViolation("list MCP tools item %d is invalid: %v", index+1, err)
		}
		if request.Server != "" && tool.Server != request.Server {
			return nil, runtimeContractViolation("list MCP tools for %q returned a tool from %q", request.Server, tool.Server)
		}
		if index == 0 {
			continue
		}
		previous, current := values[index-1], tool
		if current.Server == previous.Server && current.Name == previous.Name {
			return nil, runtimeContractViolation("list MCP tools repeats %s/%s", tool.Server, tool.Name)
		}
		if current.Server < previous.Server || current.Server == previous.Server && current.Name < previous.Name {
			return nil, runtimeContractViolation(
				"list MCP tools returned tool %s/%s out of catalog order after %s/%s",
				current.Server,
				current.Name,
				previous.Server,
				previous.Name,
			)
		}
	}
	return values, nil
}

func (r *Connection) StartAuthorization(ctx context.Context, server string) (protocol.MCPAuthorizationAttempt, error) {
	request := protocol.CreateMCPAuthorizationAttemptRequest{Server: strings.TrimSpace(server)}
	if err := request.ValidateWire(); err != nil {
		return protocol.MCPAuthorizationAttempt{}, fmt.Errorf("start MCP authorization: %w", err)
	}
	options, err := r.commandOptions()
	if err != nil {
		return protocol.MCPAuthorizationAttempt{}, err
	}
	result, err := r.mcp.CreateMCPAuthorizationAttempt(ctx, request, options)
	return projectMCPAuthorizationResult("start MCP authorization", mcpAuthorizationIdentity{server: request.Server}, result, err)
}

func (r *Connection) GetAuthorization(ctx context.Context, reference mcp.AuthorizationReference) (protocol.MCPAuthorizationAttempt, error) {
	if err := reference.Validate(); err != nil {
		return protocol.MCPAuthorizationAttempt{}, fmt.Errorf("get MCP authorization: %w", err)
	}
	request := protocol.MCPAuthorizationAttemptRequest{AttemptID: reference.ID}
	result, err := r.mcp.GetMCPAuthorizationAttempt(ctx, request, r.callOptions())
	return projectMCPAuthorizationResult(
		"get MCP authorization",
		mcpAuthorizationIdentity{attemptID: reference.ID, server: reference.Server},
		result,
		err,
	)
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
) (protocol.MCPAuthorizationAttempt, error) {
	if err != nil {
		return protocol.MCPAuthorizationAttempt{}, classifyMCPError(err)
	}
	if result == nil {
		return protocol.MCPAuthorizationAttempt{}, runtimeContractViolation("%s returned nil", operation)
	}
	attempt := *result
	attempt.Status.Error = failure.Clone(result.Status.Error)
	attempt.FinishedAt = clonePointer(result.FinishedAt)
	if err := mcp.ValidateAuthorizationAttempt(attempt); err != nil {
		return protocol.MCPAuthorizationAttempt{}, runtimeContractViolation(
			"%s returned an invalid authorization attempt: %v",
			operation,
			err,
		)
	}
	if expected.attemptID != "" && attempt.ID != expected.attemptID {
		return protocol.MCPAuthorizationAttempt{}, runtimeContractViolation(
			"%s returned attempt %q for %q",
			operation,
			attempt.ID,
			expected.attemptID,
		)
	}
	if attempt.Server != expected.server {
		return protocol.MCPAuthorizationAttempt{}, runtimeContractViolation(
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
