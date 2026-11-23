package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func (s *Handler) ListMCPServers(ctx context.Context) (*protocol.Page[protocol.MCPServer], error) {
	servers, err := s.mcp.Servers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.MCPServer, 0, len(servers))
	for _, server := range servers {
		wire, err := presentMCPServer(server)
		if err != nil {
			return nil, err
		}
		out = append(out, wire)
	}
	return protocol.NewPage(out), nil
}

func (s *Handler) CreateMCPServer(ctx context.Context, in protocol.MCPServerCandidate) (*protocol.MCPServer, error) {
	input, err := mcpServerInputFromCandidate(in)
	if err != nil {
		return nil, err
	}
	server, err := s.mcp.CreateServer(ctx, input)
	if err != nil {
		return nil, wireMCPError(err)
	}
	out, err := presentMCPServer(server)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Handler) UpdateMCPServer(ctx context.Context, in protocol.UpdateMCPServerRequest) (*protocol.MCPServer, error) {
	name, err := mcpServerIDFromWire(in.Server)
	if err != nil {
		return nil, err
	}
	patch, err := mcpServerPatchFromRequest(in)
	if err != nil {
		return nil, err
	}
	server, err := s.mcp.UpdateServer(ctx, name, patch)
	if err != nil {
		return nil, wireMCPError(err)
	}
	out, err := presentMCPServer(server)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Handler) DeleteMCPServer(ctx context.Context, server protocol.MCPServerID) error {
	name, err := mcpServerIDFromWire(server)
	if err != nil {
		return err
	}
	return wireMCPError(s.mcp.DeleteServer(ctx, name))
}

func (s *Handler) TestMCPServer(ctx context.Context, in protocol.MCPServerCandidate) (*protocol.MCPTestResult, error) {
	input, err := mcpServerInputFromCandidate(in)
	if err != nil {
		return nil, err
	}
	result, err := s.mcp.TestServer(ctx, input)
	if err != nil {
		return nil, wireMCPError(err)
	}
	switch result {
	case mcpapp.TestSucceeded:
		return &protocol.MCPTestResult{Outcome: protocol.MCPTestReachable}, nil
	case mcpapp.TestAuthorizationRequired:
		return &protocol.MCPTestResult{Outcome: protocol.MCPTestAuthorizationRequired}, nil
	case mcpapp.TestTimedOut:
		return &protocol.MCPTestResult{Outcome: protocol.MCPTestTimedOut}, nil
	case mcpapp.TestFailed:
		return &protocol.MCPTestResult{Outcome: protocol.MCPTestFailed}, nil
	default:
		return nil, fmt.Errorf("delivery: unknown MCP test outcome %q", result)
	}
}

func (s *Handler) ListMCPTools(ctx context.Context, in protocol.MCPListToolsRequest) (*protocol.Page[protocol.MCPTool], error) {
	var name *mcpserver.ID
	if in.Server != nil {
		parsed, err := mcpServerIDFromWire(*in.Server)
		if err != nil {
			return nil, err
		}
		name = &parsed
	}
	found, err := s.mcp.Tools(ctx, name)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.MCPTool, 0, len(found))
	for _, tool := range found {
		projected, err := presentMCPTool(tool)
		if err != nil {
			return nil, err
		}
		out = append(out, projected)
	}
	return protocol.NewPage(out), nil
}

func (s *Handler) ReconnectMCPServer(ctx context.Context, server protocol.MCPServerID) error {
	name, err := mcpServerIDFromWire(server)
	if err != nil {
		return err
	}
	return wireMCPError(s.mcp.ReconnectServer(ctx, name))
}

func (s *Handler) CreateMCPAuthorizationAttempt(ctx context.Context, server protocol.MCPServerID) (*protocol.MCPAuthorizationAttempt, error) {
	name, err := mcpServerIDFromWire(server)
	if err != nil {
		return nil, err
	}
	attempt, err := s.mcp.CreateAuthorizationAttempt(ctx, name)
	if err != nil {
		return nil, wireMCPError(err)
	}
	out, err := presentMCPAuthorizationAttempt(attempt)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Handler) GetMCPAuthorizationAttempt(ctx context.Context, attemptID string) (*protocol.MCPAuthorizationAttempt, error) {
	attempt, err := s.mcp.AuthorizationAttempt(ctx, attemptID)
	if err != nil {
		return nil, wireMCPError(err)
	}
	out, err := presentMCPAuthorizationAttempt(attempt)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func wireMCPError(err error) error {
	switch {
	case errors.Is(err, plugin.ErrUnavailable):
		return protocol.NewFailure(protocol.ErrPluginUnavailable, "select an enabled external MCP connector", err)
	case errors.Is(err, mcpapp.ErrOwnedByInstallation):
		return protocol.NewFailure(protocol.ErrMCPOwnedByInstallation, err.Error(), err)
	case errors.Is(err, mcpapp.ErrUnknownServer):
		return protocol.NewFailure(protocol.ErrMCPServerNotFound, err.Error(), err)
	case errors.Is(err, mcpapp.ErrServerAlreadyExists):
		return protocol.NewFailure(protocol.ErrMCPServerAlreadyExists, err.Error(), err)
	case errors.Is(err, mcpapp.ErrServerDisabled):
		return protocol.NewFailure(protocol.ErrMCPServerDisabled, err.Error(), err)
	case errors.Is(err, mcpapp.ErrAuthorizationAttemptNotFound):
		return protocol.NewFailure(protocol.ErrMCPAuthorizationAttemptNotFound, err.Error(), err)
	case errors.Is(err, mcpapp.ErrAuthorizationUnsupported):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	case errors.Is(err, mcpapp.ErrInvalidServerConfiguration):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	}
	return err
}

func (s *Handler) GetMCPToolExposure(ctx context.Context, in protocol.MCPServerRequest) (*protocol.MCPToolExposure, error) {
	server, err := mcpServerIDFromWire(in.Server)
	if err != nil {
		return nil, err
	}
	refs, err := s.mcp.ToolExposure(ctx, server)
	if err != nil {
		return nil, wireMCPError(err)
	}
	out := &protocol.MCPToolExposure{Server: presentMCPServerID(server), DisabledTools: make([]string, 0, len(refs))}
	for _, ref := range refs {
		_, remote, _ := ref.MCP()
		out.DisabledTools = append(out.DisabledTools, remote.String())
	}
	return out, nil
}

func (s *Handler) SetMCPToolExposure(ctx context.Context, in protocol.SetMCPToolExposureRequest) error {
	ref, err := toolRefFromWire(protocol.ToolRef{Type: protocol.ToolRefMCP, Server: &in.Server, Name: in.Name})
	if err != nil {
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	}
	return wireMCPError(s.mcp.SetToolExposure(ctx, ref, in.Disabled))
}
