package delivery

import (
	"context"
	"reflect"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	MCPServersList                 Name = "mcp.servers.list"
	MCPServersCreate               Name = "mcp.servers.create"
	MCPServersUpdate               Name = "mcp.servers.update"
	MCPServersDelete               Name = "mcp.servers.delete"
	mcpServersTest                 Name = "mcp.servers.test"
	mcpToolsExposure               Name = "mcp.tools.exposure"
	mcpToolsSetExposure            Name = "mcp.tools.setExposure"
	MCPToolsList                   Name = "mcp.tools.list"
	MCPServersReconnect            Name = "mcp.servers.reconnect"
	MCPAuthorizationAttemptsCreate Name = "mcp.authorizationAttempts.create"
	mcpAuthorizationAttemptsGet    Name = "mcp.authorizationAttempts.get"
)

func registerMCP(registry *Registry) {
	registry.query(MethodMeta{Name: mcpToolsExposure, GoName: "GetMCPToolExposure", CapabilityRules: requires(protocol.FeatureMCP)}, func(service interface {
		GetMCPToolExposure(context.Context, protocol.MCPServerRequest) (*protocol.MCPToolExposure, error)
	}, ctx context.Context, request protocol.MCPServerRequest) (*protocol.MCPToolExposure, error) {
		return service.GetMCPToolExposure(ctx, request)
	})
	registry.commandAck(MethodMeta{Name: mcpToolsSetExposure, GoName: "SetMCPToolExposure", CapabilityRules: requires(protocol.FeatureMCP)}, func(service interface {
		SetMCPToolExposure(context.Context, protocol.SetMCPToolExposureRequest) error
	}, ctx context.Context, request protocol.SetMCPToolExposureRequest) error {
		return service.SetMCPToolExposure(ctx, request)
	}).withOwnedReplay(atomicReplay((interface {
		commitMCPToolExposure(context.Context, protocol.SetMCPToolExposureRequest, atomicOutcome) error
	}).commitMCPToolExposure))

	registry.query(MethodMeta{
		Name: MCPServersList, GoName: "ListMCPServers",
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		ListMCPServers(context.Context) (*protocol.Page[protocol.MCPServer], error)
	}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.MCPServer], error) {
		return service.ListMCPServers(ctx)
	})

	registry.requiredReplayCommand(MethodMeta{
		Name: MCPServersCreate, GoName: "CreateMCPServer",
		Errors:          []string{protocol.ErrMCPServerAlreadyExists.Error(), protocol.ErrPluginUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, directoryReplay((*Handler).createMCPServer))

	registry.requiredReplayCommand(MethodMeta{
		Name: MCPServersUpdate, GoName: "UpdateMCPServer",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPOwnedByInstallation.Error(), protocol.ErrRevisionConflict.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, directoryReplay((*Handler).updateMCPServer))

	registry.requiredReplayCommand(MethodMeta{
		Name: MCPServersDelete, GoName: "DeleteMCPServer",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPOwnedByInstallation.Error(), protocol.ErrRevisionConflict.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, directoryReplay((*Handler).deleteMCPServer))

	registry.query(MethodMeta{
		Name: mcpServersTest, GoName: "TestMCPServer",
		Errors:          []string{protocol.ErrPluginUnavailable.Error()},
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
	}).withOwnedReplay(atomicReplay((interface {
		commitMCPReconnect(context.Context, protocol.MCPServerRequest, atomicOutcome) error
	}).commitMCPReconnect))

	registry.requiredReplayCommand(MethodMeta{
		Name: MCPAuthorizationAttemptsCreate, GoName: "CreateMCPAuthorizationAttempt",
		Result: reflect.TypeFor[*protocol.MCPAuthorizationAttemptRef](),
		Errors: []string{
			protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPServerDisabled.Error(),
			protocol.ErrPluginUnavailable.Error(), protocol.ErrPluginCommandRetired.Error(),
		},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, (*Handler).replayMCPAuthorization)

	registry.query(MethodMeta{
		Name: mcpAuthorizationAttemptsGet, GoName: "GetMCPAuthorizationAttempt",
		Errors:          []string{protocol.ErrMCPAuthorizationAttemptNotFound.Error(), protocol.ErrPluginUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		GetMCPAuthorizationAttempt(context.Context, string) (*protocol.MCPAuthorizationAttempt, error)
	}, ctx context.Context, request protocol.MCPAuthorizationAttemptRef) (*protocol.MCPAuthorizationAttempt, error) {
		return service.GetMCPAuthorizationAttempt(ctx, request.AttemptID)
	})
}
