package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	MCPServersList                 Name = "mcp.servers.list"
	MCPServersCreate               Name = "mcp.servers.create"
	MCPServersUpdate               Name = "mcp.servers.update"
	MCPServersDelete               Name = "mcp.servers.delete"
	MCPServersTest                 Name = "mcp.servers.test"
	MCPToolsList                   Name = "mcp.tools.list"
	MCPServersReconnect            Name = "mcp.servers.reconnect"
	MCPAuthorizationAttemptsCreate Name = "mcp.authorizationAttempts.create"
	MCPAuthorizationAttemptsGet    Name = "mcp.authorizationAttempts.get"
)

func registerMCP(registry *Registry) {
	registry.query(MethodMeta{
		Name:            MCPServersList,
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service *Handler, ctx context.Context, _ struct{}) (*protocol.Page[protocol.MCPServer], error) {
		return service.ListMCPServers(ctx)
	})

	registry.command(MethodMeta{
		Name:            MCPServersCreate,
		Errors:          []string{protocol.ErrMCPServerAlreadyExists.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, (*Handler).CreateMCPServer)

	registry.command(MethodMeta{
		Name:            MCPServersUpdate,
		Errors:          []string{protocol.ErrMCPServerNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, (*Handler).UpdateMCPServer)

	registry.commandAck(MethodMeta{
		Name:            MCPServersDelete,
		Errors:          []string{protocol.ErrMCPServerNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service *Handler, ctx context.Context, request protocol.MCPServerRequest) error {
		return service.DeleteMCPServer(ctx, request.Server)
	})

	// A connection probe persists nothing, so a retry is not a replay concern.
	registry.query(MethodMeta{
		Name:            MCPServersTest,
		CapabilityRules: requires(protocol.FeatureMCP),
	}, (*Handler).TestMCPServer)

	registry.query(MethodMeta{
		Name:            MCPToolsList,
		CapabilityRules: requires(protocol.FeatureMCP),
	}, (*Handler).ListMCPTools)

	registry.commandAck(MethodMeta{
		Name:            MCPServersReconnect,
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPServerDisabled.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service *Handler, ctx context.Context, request protocol.MCPServerRequest) error {
		return service.ReconnectMCPServer(ctx, request.Server)
	})

	registry.command(MethodMeta{
		Name:            MCPAuthorizationAttemptsCreate,
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPServerDisabled.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service *Handler, ctx context.Context, request protocol.CreateMCPAuthorizationAttemptRequest) (*protocol.MCPAuthorizationAttempt, error) {
		return service.CreateMCPAuthorizationAttempt(ctx, request.Server)
	})

	registry.query(MethodMeta{
		Name:            MCPAuthorizationAttemptsGet,
		Errors:          []string{protocol.ErrMCPAuthorizationAttemptNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service *Handler, ctx context.Context, request protocol.MCPAuthorizationAttemptRequest) (*protocol.MCPAuthorizationAttempt, error) {
		return service.GetMCPAuthorizationAttempt(ctx, request.AttemptID)
	})
}
