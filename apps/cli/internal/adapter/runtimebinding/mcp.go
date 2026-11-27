package runtimebinding

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/apps/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type mcpBinding interface {
	GetMCPToolExposure(context.Context, protocol.MCPServerRequest, protocol.CallOptions) (*protocol.MCPToolExposure, error)
	SetMCPToolExposure(context.Context, protocol.SetMCPToolExposureRequest, protocol.CommandOptions) error

	ListMCPServers(context.Context, protocol.CallOptions) (*protocol.Page[protocol.MCPServer], error)
	CreateMCPServer(context.Context, protocol.MCPServerCandidate, protocol.CommandOptions) error
	UpdateMCPServer(context.Context, protocol.UpdateMCPServerRequest, protocol.CommandOptions) error
	DeleteMCPServer(context.Context, protocol.MCPDirectoryTarget, protocol.CommandOptions) error
	TestMCPServer(context.Context, protocol.MCPServerCandidate, protocol.CallOptions) (*protocol.MCPTestResult, error)
	ListMCPTools(context.Context, protocol.MCPListToolsRequest, protocol.CallOptions) (*protocol.Page[protocol.MCPTool], error)
	ReconnectMCPServer(context.Context, protocol.MCPServerRequest, protocol.CommandOptions) error
	CreateMCPAuthorizationAttempt(context.Context, protocol.CreateMCPAuthorizationAttemptRequest, protocol.CommandOptions) (*protocol.MCPAuthorizationAttemptRef, error)
	GetMCPAuthorizationAttempt(context.Context, protocol.MCPAuthorizationAttemptRef, protocol.CallOptions) (*protocol.MCPAuthorizationAttempt, error)
}

func (r *Connection) Servers(ctx context.Context) ([]protocol.MCPServer, error) {
	page, err := r.mcp.ListMCPServers(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list MCP servers", page)
	if err != nil {
		return nil, err
	}
	return values, nil
}

func (r *Connection) CreateServer(ctx context.Context, commandID authoring.CommandID, candidate protocol.MCPServerCandidate) error {
	options, err := r.originalCommandOptions(commandID)
	if err != nil {
		return err
	}
	return classifyError(r.mcp.CreateMCPServer(ctx, candidate, options))
}

func (r *Connection) UpdateServer(ctx context.Context, commandID authoring.CommandID, update protocol.UpdateMCPServerRequest) error {
	options, err := r.originalCommandOptions(commandID)
	if err != nil {
		return err
	}
	return classifyError(r.mcp.UpdateMCPServer(ctx, update, options))
}

func (r *Connection) DeleteServer(ctx context.Context, target protocol.MCPDirectoryTarget) error {
	if err := protocol.ValidateWireTree(target); err != nil {
		return fmt.Errorf("delete MCP server: %w", err)
	}
	return classifyError(r.mcp.DeleteMCPServer(ctx, target, r.commandOptions()))
}

func (r *Connection) ReconnectServer(ctx context.Context, server protocol.MCPServerID) error {
	request := protocol.MCPServerRequest{Server: server}
	if err := protocol.ValidateWireTree(request); err != nil {
		return fmt.Errorf("reconnect MCP server: %w", err)
	}
	return classifyError(r.mcp.ReconnectMCPServer(ctx, request, r.commandOptions()))
}

func (r *Connection) TestServer(ctx context.Context, candidate protocol.MCPServerCandidate) (protocol.MCPTestOutcome, error) {
	result, err := r.mcp.TestMCPServer(ctx, candidate, r.callOptions())
	if err != nil {
		return "", classifyError(err)
	}
	if result == nil {
		return "", runtimeContractViolation("test MCP server returned nil")
	}
	if err := protocol.ValidateWireTree(*result); err != nil {
		return "", runtimeContractViolation("test MCP server returned an invalid result: %v", err)
	}
	return result.Outcome, nil
}

func (r *Connection) Tools(ctx context.Context, server *protocol.MCPServerID) ([]protocol.MCPTool, error) {
	request := protocol.MCPListToolsRequest{Server: server}
	if err := protocol.ValidateWireTree(request); err != nil {
		return nil, fmt.Errorf("list MCP tools: %w", err)
	}
	page, err := r.mcp.ListMCPTools(ctx, request, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list MCP tools", page)
	if err != nil {
		return nil, err
	}
	for _, tool := range values {
		if server != nil && tool.Server != *server {
			return nil, runtimeContractViolation("list MCP tools for %q returned a tool from %q", mcp.ServerLabel(*server), mcp.ServerLabel(tool.Server))
		}
	}
	return values, nil
}

func (r *Connection) StartAuthorization(ctx context.Context, commandID authoring.CommandID, server protocol.MCPServerID) (mcp.AuthorizationReference, error) {
	request := protocol.CreateMCPAuthorizationAttemptRequest{Server: server}
	if err := protocol.ValidateWireTree(request); err != nil {
		return mcp.AuthorizationReference{}, fmt.Errorf("start MCP authorization: %w", err)
	}
	options, err := r.originalCommandOptions(commandID)
	if err != nil {
		return mcp.AuthorizationReference{}, err
	}
	result, err := r.mcp.CreateMCPAuthorizationAttempt(ctx, request, options)
	if err != nil {
		return mcp.AuthorizationReference{}, classifyError(err)
	}
	if result == nil {
		return mcp.AuthorizationReference{}, runtimeContractViolation("start MCP authorization returned nil")
	}
	reference := mcp.AuthorizationReference{ID: result.AttemptID, Server: server}
	if err := reference.Validate(); err != nil {
		return mcp.AuthorizationReference{}, runtimeContractViolation("start MCP authorization returned an invalid reference: %v", err)
	}
	return reference, nil
}

func (r *Connection) GetAuthorization(ctx context.Context, reference mcp.AuthorizationReference) (protocol.MCPAuthorizationAttempt, error) {
	if err := reference.Validate(); err != nil {
		return protocol.MCPAuthorizationAttempt{}, fmt.Errorf("get MCP authorization: %w", err)
	}
	request := protocol.MCPAuthorizationAttemptRef{AttemptID: reference.ID}
	result, err := r.mcp.GetMCPAuthorizationAttempt(ctx, request, r.callOptions())
	if err != nil {
		return protocol.MCPAuthorizationAttempt{}, classifyError(err)
	}
	if result == nil {
		return protocol.MCPAuthorizationAttempt{}, runtimeContractViolation("get MCP authorization returned nil")
	}
	if result.FinishedAt != nil && result.FinishedAt.Before(result.CreatedAt) {
		return protocol.MCPAuthorizationAttempt{}, runtimeContractViolation("get MCP authorization returned an attempt that finished before it started")
	}
	if err := requireIdentity("get MCP authorization", result.ID, reference.ID); err != nil {
		return protocol.MCPAuthorizationAttempt{}, err
	}
	if result.Server != reference.Server {
		return protocol.MCPAuthorizationAttempt{}, runtimeContractViolation("get MCP authorization returned server %q for %q", mcp.ServerLabel(result.Server), mcp.ServerLabel(reference.Server))
	}
	return *result, nil
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return new(*value)
}

func (r *Connection) ToolExposure(ctx context.Context, server protocol.MCPServerID) (protocol.MCPToolExposure, error) {
	request := protocol.MCPServerRequest{Server: server}
	if err := protocol.ValidateWireTree(request); err != nil {
		return protocol.MCPToolExposure{}, err
	}
	result, err := r.mcp.GetMCPToolExposure(ctx, request, r.callOptions())
	if err != nil {
		return protocol.MCPToolExposure{}, classifyError(err)
	}
	if result == nil || result.Server != server {
		return protocol.MCPToolExposure{}, runtimeContractViolation("MCP tool exposure did not identify the requested server")
	}
	return *result, nil
}

func (r *Connection) SetToolExposure(ctx context.Context, request protocol.SetMCPToolExposureRequest) error {
	if err := protocol.ValidateWireTree(request); err != nil {
		return err
	}
	return classifyError(r.mcp.SetMCPToolExposure(ctx, request, r.commandOptions()))
}
