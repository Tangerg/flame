package delivery

import (
	"fmt"
	"time"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/protocol"
)

func presentMCPServer(server mcpapp.Server) (protocol.MCPServer, error) {
	connection, err := presentMCPConnection(server.Connection)
	if err != nil {
		return protocol.MCPServer{}, err
	}
	status, err := presentMCPServerState(server.State)
	if err != nil {
		return protocol.MCPServer{}, err
	}
	return protocol.MCPServer{
		ID:               presentMCPServerID(server.ID),
		Description:      server.Description,
		Connection:       connection,
		HandshakeTimeout: presentMCPHandshakeTimeout(server.HandshakeTimeout),
		Status:           status,
	}, nil
}

func presentMCPServerID(id mcpserver.ID) protocol.MCPServerID {
	origin := protocol.MCPOrigin{Type: protocol.MCPOriginUser}
	if installation, found := id.Origin().Installation(); found {
		origin = protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installation.String()}
	}
	return protocol.MCPServerID{Origin: origin, Name: id.Name().String()}
}

func presentMCPServerIDs(ids []mcpserver.ID) []protocol.MCPServerID {
	if len(ids) == 0 {
		return nil
	}
	result := make([]protocol.MCPServerID, 0, len(ids))
	for _, id := range ids {
		result = append(result, presentMCPServerID(id))
	}
	return result
}

func presentMCPHandshakeTimeout(timeout mcpserver.HandshakeTimeout) protocol.MCPHandshakeTimeout {
	duration, bounded := timeout.Duration()
	if !bounded {
		return protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded}
	}
	seconds := int(duration / time.Second)
	return protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeBounded, Seconds: &seconds}
}

func presentMCPConnection(connection mcpapp.Connection) (protocol.MCPConnection, error) {
	transport, ok := presentMCPTransport(connection.Transport)
	if !ok {
		return protocol.MCPConnection{}, fmt.Errorf("mcp: unsupported transport %q", connection.Transport)
	}
	return protocol.MCPConnection{
		Type:                transport,
		URL:                 connection.URL,
		AuthorizationMasked: connection.AuthorizationMasked,
		HeadersMasked:       connection.HeadersMasked,
		Command:             connection.Command,
		Args:                connection.Args,
		EnvMasked:           connection.EnvironmentMasked,
		Dir:                 connection.Dir,
	}, nil
}

func presentMCPServerState(state mcpapp.ServerState) (protocol.MCPServerState, error) {
	out := protocol.MCPServerState{ToolCount: state.ToolCount}
	switch state.Type {
	case mcpapp.ServerDisabled:
		out.Type = protocol.MCPServerDisabled
	case mcpapp.ServerDisconnected:
		out.Type = protocol.MCPServerDisconnected
	case mcpapp.ServerConnecting:
		out.Type = protocol.MCPServerConnecting
	case mcpapp.ServerConnected:
		out.Type = protocol.MCPServerConnected
	case mcpapp.ServerFailed:
		problem, err := mcpFailureProblem(state.Failure)
		if err != nil {
			return protocol.MCPServerState{}, err
		}
		out.Type = protocol.MCPServerFailed
		out.Error = &protocol.MCPStatusProblem{Type: problem}
	case mcpapp.ServerNeedsAuth:
		out.Type = protocol.MCPServerNeedsAuth
		out.Error = &protocol.MCPStatusProblem{Type: protocol.MCPStatusAuthorizationRequired}
	default:
		return protocol.MCPServerState{}, fmt.Errorf("mcp: project server state %q", state.Type)
	}
	return out, nil
}

func mcpFailureProblem(failure mcpserver.ConnectionFailure) (protocol.MCPStatusProblemType, error) {
	switch failure {
	case mcpserver.FailureUnavailableRelease:
		return protocol.MCPStatusReleaseUnavailable, nil
	case mcpserver.FailureUnavailableBackend:
		return protocol.MCPStatusBackendUnavailable, nil
	case mcpserver.FailureConfiguration:
		return protocol.MCPStatusConfigurationFailed, nil
	case mcpserver.FailureConnection:
		return protocol.MCPStatusDialFailed, nil
	case mcpserver.FailureToolDiscovery:
		return protocol.MCPStatusToolDiscoveryFailed, nil
	case mcpserver.FailureAuthorization:
		return protocol.MCPStatusAuthorizationFailed, nil
	default:
		return "", fmt.Errorf("mcp: project connection failure %q", failure)
	}
}

func presentMCPAuthorizationAttempt(attempt mcpapp.AuthorizationAttempt) (protocol.MCPAuthorizationAttempt, error) {
	status := protocol.MCPAuthorizationAttemptStatus{}
	switch attempt.Status {
	case mcpapp.AuthorizationAttemptPending:
		status.Type = protocol.MCPAuthorizationAttemptPending
	case mcpapp.AuthorizationAttemptSucceeded:
		status.Type = protocol.MCPAuthorizationAttemptSucceeded
	case mcpapp.AuthorizationAttemptFailed:
		status.Type = protocol.MCPAuthorizationAttemptFailed
		status.Error = &protocol.MCPStatusProblem{Type: protocol.MCPStatusAuthorizationFailed}
	case mcpapp.AuthorizationAttemptCanceled:
		status.Type = protocol.MCPAuthorizationAttemptCanceled
	default:
		return protocol.MCPAuthorizationAttempt{}, fmt.Errorf("mcp: project authorization attempt status %q", attempt.Status)
	}
	return protocol.MCPAuthorizationAttempt{
		ID: attempt.ID.String(), Server: presentMCPServerID(attempt.Server), Status: status,
		CreatedAt: attempt.CreatedAt, FinishedAt: attempt.FinishedAt,
	}, nil
}

func presentMCPTool(tool mcpapp.ToolView) (protocol.MCPTool, error) {
	schema, err := presentToolSchema(tool.Definition)
	if err != nil {
		return protocol.MCPTool{}, err
	}
	conflicts := make([]protocol.ToolRef, 0, len(tool.Conflicts))
	for _, ref := range tool.Conflicts {
		wire, err := presentToolRef(ref)
		if err != nil {
			return protocol.MCPTool{}, err
		}
		conflicts = append(conflicts, wire)
	}
	return protocol.MCPTool{
		ModelName:     tool.ModelName,
		NameConflicts: conflicts,
		Server:        presentMCPServerID(tool.Server),
		Name:          tool.Name.String(),
		Description:   tool.Definition.Description,
		InputSchema:   schema,
	}, nil
}

func presentMCPTransport(transport mcpserver.Transport) (protocol.MCPTransport, bool) {
	switch transport {
	case mcpserver.TransportStdio:
		return protocol.MCPTransportStdio, true
	case mcpserver.TransportStreamableHTTP:
		return protocol.MCPTransportStreamableHTTP, true
	default:
		return "", false
	}
}
