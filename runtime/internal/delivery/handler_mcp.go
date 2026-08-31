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

// ListMCPServers returns the single authoritative MCP resource collection:
// durable configuration enriched with current live state.
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
	var problem *protocol.ProblemData
	if !result.OK {
		problem = mcpProbeProblem()
	}
	return &protocol.MCPTestResult{OK: result.OK, Error: problem}, nil
}

// ListMCPTools lists tools advertised by connected MCP servers, optionally
// narrowed to one server.
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
		out = append(out, presentMCPTool(tool))
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
		return fmt.Errorf("%w: %w", protocol.ErrMCPServerNotFound, err)
	case errors.Is(err, mcpapp.ErrServerAlreadyExists):
		return fmt.Errorf("%w: %w", protocol.ErrMCPServerAlreadyExists, err)
	case errors.Is(err, mcpapp.ErrServerDisabled):
		return fmt.Errorf("%w: %w", protocol.ErrMCPServerDisabled, err)
	case errors.Is(err, mcpapp.ErrAuthorizationAttemptNotFound):
		return fmt.Errorf("%w: %w", protocol.ErrMCPAuthorizationAttemptNotFound, err)
	case errors.Is(err, mcpapp.ErrAuthorizationUnsupported):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	case errors.Is(err, mcpapp.ErrInvalidServerConfiguration):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	return err
}
