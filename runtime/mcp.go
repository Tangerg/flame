package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ListMCPServers returns configured MCP servers ordered by name ascending.
func (r *binding) ListMCPServers(ctx context.Context, options CallOptions) (*protocol.Page[protocol.MCPServer], error) {
	return r.invoke[struct{}, *protocol.Page[protocol.MCPServer]](ctx, delivery.MCPServersList, struct{}{}, callOptions(options))
}

// CreateMCPServer creates an MCP server configuration.
func (r *binding) CreateMCPServer(ctx context.Context, request protocol.MCPServerCandidate, options CommandOptions) (*protocol.MCPServer, error) {
	return r.invoke[protocol.MCPServerCandidate, *protocol.MCPServer](ctx, delivery.MCPServersCreate, request, commandOptions(options))
}

// UpdateMCPServer updates an MCP server configuration.
func (r *binding) UpdateMCPServer(ctx context.Context, request protocol.UpdateMCPServerRequest, options CommandOptions) (*protocol.MCPServer, error) {
	return r.invoke[protocol.UpdateMCPServerRequest, *protocol.MCPServer](ctx, delivery.MCPServersUpdate, request, commandOptions(options))
}

// DeleteMCPServer deletes an MCP server configuration.
func (r *binding) DeleteMCPServer(ctx context.Context, request protocol.MCPServerRequest, options CommandOptions) error {
	return r.invokeAck(ctx, delivery.MCPServersDelete, request, commandOptions(options))
}

// TestMCPServer probes an MCP server candidate without persisting it.
func (r *binding) TestMCPServer(ctx context.Context, request protocol.MCPServerCandidate, options CallOptions) (*protocol.MCPTestResult, error) {
	return r.invoke[protocol.MCPServerCandidate, *protocol.MCPTestResult](ctx, delivery.MCPServersTest, request, callOptions(options))
}

// ListMCPTools returns advertised tools ordered by server then tool name.
func (r *binding) ListMCPTools(ctx context.Context, request protocol.MCPListToolsRequest, options CallOptions) (*protocol.Page[protocol.MCPTool], error) {
	return r.invoke[protocol.MCPListToolsRequest, *protocol.Page[protocol.MCPTool]](ctx, delivery.MCPToolsList, request, callOptions(options))
}

// ReconnectMCPServer reconnects one enabled MCP server.
func (r *binding) ReconnectMCPServer(ctx context.Context, request protocol.MCPServerRequest, options CommandOptions) error {
	return r.invokeAck(ctx, delivery.MCPServersReconnect, request, commandOptions(options))
}

// CreateMCPAuthorizationAttempt starts browser authorization for an MCP server.
func (r *binding) CreateMCPAuthorizationAttempt(ctx context.Context, request protocol.CreateMCPAuthorizationAttemptRequest, options CommandOptions) (*protocol.MCPAuthorizationAttempt, error) {
	return r.invoke[protocol.CreateMCPAuthorizationAttemptRequest, *protocol.MCPAuthorizationAttempt](ctx, delivery.MCPAuthorizationAttemptsCreate, request, commandOptions(options))
}

// GetMCPAuthorizationAttempt returns one MCP authorization attempt.
func (r *binding) GetMCPAuthorizationAttempt(ctx context.Context, request protocol.MCPAuthorizationAttemptRequest, options CallOptions) (*protocol.MCPAuthorizationAttempt, error) {
	return r.invoke[protocol.MCPAuthorizationAttemptRequest, *protocol.MCPAuthorizationAttempt](ctx, delivery.MCPAuthorizationAttemptsGet, request, callOptions(options))
}
