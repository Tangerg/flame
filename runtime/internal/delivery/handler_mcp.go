package delivery

import (
	"context"
	"errors"
	"fmt"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/protocol"
)

// mcp.* is runtime-global, so these methods take no workspace reference.

// ListMCPServers returns the single authoritative MCP resource collection in
// name order: durable configuration enriched with current live state.
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

// CreateMCPServer creates and returns one unified MCP server resource.
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

// UpdateMCPServer applies an explicit partial update and returns the resulting
// unified resource.
func (s *Handler) UpdateMCPServer(ctx context.Context, in protocol.UpdateMCPServerRequest) (*protocol.MCPServer, error) {
	name, err := parseMCPServerName(in.Server)
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

// DeleteMCPServer deletes one configured server and its live projection.
func (s *Handler) DeleteMCPServer(ctx context.Context, server string) error {
	name, err := parseMCPServerName(server)
	if err != nil {
		return err
	}
	return wireMCPError(s.mcp.DeleteServer(ctx, name))
}

// TestMCPServer probes a complete candidate without persisting it.
func (s *Handler) TestMCPServer(ctx context.Context, in protocol.MCPServerCandidate) (*protocol.MCPTestResult, error) {
	input, err := mcpServerInputFromCandidate(in)
	if err != nil {
		return nil, err
	}
	result, err := s.mcp.TestServer(ctx, input)
	if err != nil {
		return nil, wireMCPError(err)
	}
	var kind string
	switch result {
	case mcpapp.TestSucceeded:
		return &protocol.MCPTestResult{OK: true}, nil
	case mcpapp.TestAuthorizationRequired:
		kind = protocol.ProblemMCPAuthorizationRequired
	case mcpapp.TestTimedOut:
		kind = protocol.ProblemTimeout
	case mcpapp.TestFailed:
		kind = protocol.ProblemMCPDialFailed
	default:
		return nil, fmt.Errorf("delivery: unknown MCP test outcome %q", result)
	}
	return &protocol.MCPTestResult{Error: &protocol.ProblemData{Type: kind}}, nil
}

// ListMCPTools lists tools advertised by connected MCP servers in server/name
// order, optionally narrowed to one server.
func (s *Handler) ListMCPTools(ctx context.Context, in protocol.MCPListToolsRequest) (*protocol.Page[protocol.MCPTool], error) {
	var name *mcpserver.ServerName
	if in.Server != "" {
		parsed, err := parseMCPServerName(in.Server)
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

// ReconnectMCPServer starts a new live dial. Its state transitions invalidate
// the server resource through runtime.event.
func (s *Handler) ReconnectMCPServer(ctx context.Context, server string) error {
	name, err := parseMCPServerName(server)
	if err != nil {
		return err
	}
	return wireMCPError(s.mcp.ReconnectServer(ctx, name))
}

// CreateMCPAuthorizationAttempt starts interactive OAuth and returns its
// observable asynchronous resource immediately.
func (s *Handler) CreateMCPAuthorizationAttempt(ctx context.Context, server string) (*protocol.MCPAuthorizationAttempt, error) {
	name, err := parseMCPServerName(server)
	if err != nil {
		return nil, err
	}
	attempt, err := s.mcp.CreateAuthorizationAttempt(ctx, name)
	if err != nil {
		return nil, wireMCPError(err)
	}
	out := presentMCPAuthorizationAttempt(attempt)
	return &out, nil
}

// GetMCPAuthorizationAttempt returns a pending or retained terminal OAuth flow.
func (s *Handler) GetMCPAuthorizationAttempt(ctx context.Context, attemptID string) (*protocol.MCPAuthorizationAttempt, error) {
	attempt, err := s.mcp.AuthorizationAttempt(ctx, attemptID)
	if err != nil {
		return nil, wireMCPError(err)
	}
	out := presentMCPAuthorizationAttempt(attempt)
	return &out, nil
}

func wireMCPError(err error) error {
	switch {
	case errors.Is(err, mcpapp.ErrUnknownServer):
		return NewFailure(errors.Join(protocol.ErrMCPServerNotFound, err), err.Error())
	case errors.Is(err, mcpapp.ErrServerAlreadyExists):
		return NewFailure(errors.Join(protocol.ErrMCPServerAlreadyExists, err), err.Error())
	case errors.Is(err, mcpapp.ErrServerDisabled):
		return NewFailure(errors.Join(protocol.ErrMCPServerDisabled, err), err.Error())
	case errors.Is(err, mcpapp.ErrAuthorizationAttemptNotFound):
		return NewFailure(errors.Join(protocol.ErrMCPAuthorizationAttemptNotFound, err), err.Error())
	case errors.Is(err, mcpapp.ErrAuthorizationUnsupported):
		return NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	case errors.Is(err, mcpapp.ErrInvalidServerConfiguration):
		return NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	}
	return err
}
