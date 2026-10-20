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
	MCPToolsExposure               Name = "mcp.tools.exposure"
	MCPToolsSetExposure            Name = "mcp.tools.setExposure"
	MCPToolsList                   Name = "mcp.tools.list"
	MCPServersReconnect            Name = "mcp.servers.reconnect"
	MCPAuthorizationAttemptsCreate Name = "mcp.authorizationAttempts.create"
	MCPAuthorizationAttemptsGet    Name = "mcp.authorizationAttempts.get"
)

func registerMCP(registry *Registry) {
	registry.query(MethodMeta{Name: MCPToolsExposure, GoName: "GetMCPToolExposure", CapabilityRules: requires(protocol.FeatureMCP)}, func(service interface {
		GetMCPToolExposure(context.Context, protocol.MCPServerRequest) (*protocol.MCPToolExposure, error)
	}, ctx context.Context, request protocol.MCPServerRequest) (*protocol.MCPToolExposure, error) {
		return service.GetMCPToolExposure(ctx, request)
	})
	registry.commandAck(MethodMeta{Name: MCPToolsSetExposure, GoName: "SetMCPToolExposure", CapabilityRules: requires(protocol.FeatureMCP)}, func(service interface {
		SetMCPToolExposure(context.Context, protocol.SetMCPToolExposureRequest) error
	}, ctx context.Context, request protocol.SetMCPToolExposureRequest) error {
		return service.SetMCPToolExposure(ctx, request)
	})

	registry.query(MethodMeta{
		Name: MCPServersList, GoName: "ListMCPServers",
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		ListMCPServers(context.Context) (*protocol.Page[protocol.MCPServer], error)
	}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.MCPServer], error) {
		return service.ListMCPServers(ctx)
	})

	registry.command(MethodMeta{
		Name: MCPServersCreate, GoName: "CreateMCPServer",
		Errors:          []string{protocol.ErrMCPServerAlreadyExists.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		CreateMCPServer(context.Context, protocol.MCPServerCandidate) (*protocol.MCPServer, error)
	}, ctx context.Context, request protocol.MCPServerCandidate) (*protocol.MCPServer, error) {
		return service.CreateMCPServer(ctx, request)
	})

	registry.command(MethodMeta{
		Name: MCPServersUpdate, GoName: "UpdateMCPServer",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		UpdateMCPServer(context.Context, protocol.UpdateMCPServerRequest) (*protocol.MCPServer, error)
	}, ctx context.Context, request protocol.UpdateMCPServerRequest) (*protocol.MCPServer, error) {
		return service.UpdateMCPServer(ctx, request)
	})

	registry.commandAck(MethodMeta{
		Name: MCPServersDelete, GoName: "DeleteMCPServer",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		DeleteMCPServer(context.Context, protocol.MCPServerID) error
	}, ctx context.Context, request protocol.MCPServerRequest) error {
		return service.DeleteMCPServer(ctx, request.Server)
	})

	// A connection probe persists nothing, so a retry is not a replay concern.
	registry.query(MethodMeta{
		Name: MCPServersTest, GoName: "TestMCPServer",
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		TestMCPServer(context.Context, protocol.MCPServerCandidate) (*protocol.MCPTestResult, error)
	}, ctx context.Context, request protocol.MCPServerCandidate) (*protocol.MCPTestResult, error) {
		return service.TestMCPServer(ctx, request)
	})

	registry.query(MethodMeta{
		Name: MCPToolsList, GoName: "ListMCPTools",
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		ListMCPTools(context.Context, protocol.MCPListToolsRequest) (*protocol.Page[protocol.MCPTool], error)
	}, ctx context.Context, request protocol.MCPListToolsRequest) (*protocol.Page[protocol.MCPTool], error) {
		return service.ListMCPTools(ctx, request)
	})

	registry.commandAck(MethodMeta{
		Name: MCPServersReconnect, GoName: "ReconnectMCPServer",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPServerDisabled.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		ReconnectMCPServer(context.Context, protocol.MCPServerID) error
	}, ctx context.Context, request protocol.MCPServerRequest) error {
		return service.ReconnectMCPServer(ctx, request.Server)
	})

	registry.command(MethodMeta{
		Name: MCPAuthorizationAttemptsCreate, GoName: "CreateMCPAuthorizationAttempt",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPServerDisabled.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		CreateMCPAuthorizationAttempt(context.Context, protocol.MCPServerID) (*protocol.MCPAuthorizationAttempt, error)
	}, ctx context.Context, request protocol.CreateMCPAuthorizationAttemptRequest) (*protocol.MCPAuthorizationAttempt, error) {
		return service.CreateMCPAuthorizationAttempt(ctx, request.Server)
	})

	registry.query(MethodMeta{
		Name: MCPAuthorizationAttemptsGet, GoName: "GetMCPAuthorizationAttempt",
		Errors:          []string{protocol.ErrMCPAuthorizationAttemptNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		GetMCPAuthorizationAttempt(context.Context, string) (*protocol.MCPAuthorizationAttempt, error)
	}, ctx context.Context, request protocol.MCPAuthorizationAttemptRequest) (*protocol.MCPAuthorizationAttempt, error) {
		return service.GetMCPAuthorizationAttempt(ctx, request.AttemptID)
	})
}
