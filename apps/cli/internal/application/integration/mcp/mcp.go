package mcp

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func UserServer(name string) protocol.MCPServerID {
	return protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginUser}, Name: name}
}

func ParseServerReference(text string) (protocol.MCPServerID, error) {
	text = strings.TrimSpace(text)
	id := UserServer(text)
	if installation, name, found := strings.Cut(text, "/"); found {
		id = protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installation}, Name: name}
	}
	if err := protocol.ValidateWireTree(id); err != nil {
		return protocol.MCPServerID{}, fmt.Errorf("MCP server %q: %w", text, err)
	}
	return id, nil
}

func ServerLabel(id protocol.MCPServerID) string {
	if id.Origin.Type == protocol.MCPOriginInstallation {
		return id.Origin.InstallationID + "/" + id.Name
	}
	return id.Name
}

func ToolLabel(ref protocol.ToolRef) (string, error) {
	switch ref.Type {
	case protocol.ToolRefBuiltIn:
		return "builtIn/" + ref.Name, nil
	case protocol.ToolRefMCP:
		if ref.Server == nil {
			return "", fmt.Errorf("runtime contract violation: MCP tool %q has no server", ref.Name)
		}
		return "mcp/" + ServerLabel(*ref.Server) + "/" + ref.Name, nil
	default:
		return "", fmt.Errorf("runtime contract violation: unknown tool source %q", ref.Type)
	}
}

// Server is retained although the query is keyed by ID so adapters can reject a
// response that crosses authorization ownership.
type AuthorizationReference struct {
	ID     string
	Server protocol.MCPServerID
}

func (a AuthorizationReference) Validate() error {
	var problems []error
	if err := protocol.ValidateWireTree(protocol.MCPAuthorizationAttemptRef{AttemptID: a.ID}); err != nil {
		problems = append(problems, err)
	}
	if err := protocol.ValidateWireTree(protocol.MCPServerRequest{Server: a.Server}); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("MCP authorization reference: %w", err)
	}
	return nil
}

func ValidateAuthorizationAttempt(attempt protocol.MCPAuthorizationAttempt) error {
	if attempt.FinishedAt != nil && attempt.FinishedAt.Before(attempt.CreatedAt) {
		return errors.New("MCP authorization finished before it started")
	}
	return nil
}

func AuthorizationReferenceFrom(attempt protocol.MCPAuthorizationAttempt) AuthorizationReference {
	return AuthorizationReference{ID: attempt.ID, Server: attempt.Server}
}
